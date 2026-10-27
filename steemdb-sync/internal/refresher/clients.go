package refresher

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// clientsWindowDays is the comment window scanned for client distribution.
const clientsWindowDays = 90

// clientsRefresher aggregates client usage from comment.json_metadata.app
// (legacy history.py update_clients) into status.clients-snapshot (consumed by
// the web labs/clients endpoint) and clients_history (daily record).
//
// Gated by the catch-up rule on the processor cursor (blockedForProcessor):
// the aggregation runs over the comment collection, which is processor-
// derived and only advances with status.processor_height — that cursor lags
// the ingest frontier during cold start, so the ingest-frontier gate would
// open while the comment data is still partial. The overwrite-based snapshot
// (clients-snapshot) would self-heal, but the per-day clients_history row is
// written only on its own day — an under-counted day computed during
// catch-up would stay wrong forever.
type clientsRefresher struct {
	commentCol *mongo.Collection
	statusCol  *mongo.Collection
	historyCol *mongo.Collection
	gate       *catchupGate
}

func newClientsRefresher(db *mongo.Database) *clientsRefresher {
	return &clientsRefresher{
		commentCol: db.Collection("comment"),
		statusCol:  db.Collection("status"),
		historyCol: db.Collection("clients_history"),
		gate:       newCatchupGate(db),
	}
}

func (c *clientsRefresher) tick(ctx context.Context) {
	if skip, reason := c.gate.blockedForProcessor(ctx); skip {
		log.Printf("[Refresher:Clients] Skipped: %s", reason)
		return
	}
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -clientsWindowDays)
	window := bson.M{"$gte": start, "$lte": now}

	// Coverage of the client attribution: how many comments the window holds at
	// all, and how many carry an `app` string. The gap is the share of posts
	// with no client signal — it used to be ~11% and it is invisible to a
	// reader who only sees the per-client totals.
	postsScanned, err := c.commentCol.CountDocuments(ctx, bson.M{"created": window})
	if err != nil {
		log.Printf("[Refresher:Clients] Failed to count window comments: %v", err)
		return
	}
	postsWithApp, err := c.commentCol.CountDocuments(ctx, bson.M{
		"created":           window,
		"json_metadata.app": bson.M{"$type": "string", "$ne": ""},
	})
	if err != nil {
		log.Printf("[Refresher:Clients] Failed to count comments with app: %v", err)
		return
	}

	// Same pipeline as legacy history.py: match 90d comments whose
	// json_metadata.app is a non-empty string, group by client × day, then
	// re-group per day with per-client count/reward arrays.
	//
	// The app match used to require a `name/version` shape
	// (`[\w-]+/[\w.]+`), which silently dropped 13% of the posts that DO name a
	// client (`dkscore-poststudio`, `SteemX`, `drugwars` — sampled 80 of 600 in
	// one 8h window): only the version suffix is optional in practice, so the
	// version is simply not required any more — `$split` on "/" already strips
	// it.
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"created":           window,
				"json_metadata.app": bson.M{"$type": "string", "$ne": ""},
			},
		},
		{
			"$project": bson.M{
				"created": 1,
				"client": bson.M{"$trim": bson.M{"input": bson.M{
					"$arrayElemAt": []interface{}{
						bson.M{"$split": []interface{}{"$json_metadata.app", "/"}},
						0,
					},
				}}},
				// `$add` returns null as soon as one operand is missing or
				// non-numeric, and a null summand poisons the whole group —
				// this is how 70 of 91 days ended up with reward 0 while the
				// post counts were right. Each leg is guarded and summed.
				"reward": bson.M{"$sum": []interface{}{
					bson.M{"$ifNull": []interface{}{"$total_payout_value", 0}},
					bson.M{"$ifNull": []interface{}{"$pending_payout_value", 0}},
					bson.M{"$ifNull": []interface{}{"$total_pending_payout_value", 0}},
				}},
			},
		},
		{
			// An app string that is only whitespace or starts with "/" leaves
			// an empty client name.
			"$match": bson.M{"client": bson.M{"$ne": ""}},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"client": "$client",
					"doy":    bson.M{"$dayOfYear": "$created"},
					"year":   bson.M{"$year": "$created"},
					"month":  bson.M{"$month": "$created"},
					"day":    bson.M{"$dayOfMonth": "$created"},
					"dow":    bson.M{"$dayOfWeek": "$created"},
				},
				"reward": bson.M{"$sum": "$reward"},
				"value":  bson.M{"$sum": 1},
			},
		},
		{
			// bson.D, not bson.M: MongoDB reads a $sort document's field
			// order as precedence, while bson.M is an unordered map. With a
			// map, `value` could become primary and each day's client list
			// would be ordered across days instead of within one.
			"$sort": bson.D{
				{Key: "_id.year", Value: 1},
				{Key: "_id.doy", Value: 1},
				{Key: "value", Value: -1},
				{Key: "_id.client", Value: 1},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"doy": "$_id.doy", "year": "$_id.year",
					"month": "$_id.month", "day": "$_id.day", "dow": "$_id.dow",
				},
				"clients": bson.M{"$push": bson.M{
					"client": "$_id.client", "count": "$value", "reward": "$reward",
				}},
				"reward": bson.M{"$sum": "$reward"},
				"total":  bson.M{"$sum": "$value"},
			},
		},
		{
			// Newest day first; bson.D so year stays ahead of day-of-year.
			"$sort": bson.D{
				{Key: "_id.year", Value: -1},
				{Key: "_id.doy", Value: -1},
			},
		},
	}

	cursor, err := c.commentCol.Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("[Refresher:Clients] Aggregation failed: %v", err)
		return
	}

	// Initialize to an empty slice so an empty window stores [] (not null)
	data := make([]bson.M, 0)
	if err := cursor.All(ctx, &data); err != nil {
		log.Printf("[Refresher:Clients] Failed to decode aggregation: %v", err)
		return
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// The metadata is what makes the snapshot auditable: without a refresh
	// timestamp a stale snapshot can only be inferred from "is the last day
	// today", and without the coverage counters the unattributed share of posts
	// is unknowable from the API.
	snapshot := bson.M{
		"data":           data,
		"updated_at":     now,
		"window_from":    start,
		"window_to":      now,
		"posts_scanned":  postsScanned,
		"posts_with_app": postsWithApp,
	}

	if _, err := c.statusCol.UpdateOne(ctx,
		bson.M{"_id": "clients-snapshot"},
		bson.M{"$set": snapshot},
		options.Update().SetUpsert(true),
	); err != nil {
		log.Printf("[Refresher:Clients] Failed to write clients-snapshot: %v", err)
		return
	}

	if _, err := c.historyCol.UpdateOne(ctx,
		bson.M{"date": today},
		bson.M{"$set": snapshot},
		options.Update().SetUpsert(true),
	); err != nil {
		log.Printf("[Refresher:Clients] Failed to write clients_history: %v", err)
	}

	log.Printf("[Refresher:Clients] Snapshot updated (%d days, %d/%d posts attributed)",
		len(data), postsWithApp, postsScanned)
}
