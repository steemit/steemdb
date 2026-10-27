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

// clientAppStringExpr narrows json_metadata.app to a string, or "" for anything
// else.
//
// The `$match` stages already filter on `$type: "string"`, but that query
// operator is NOT a type guard in the aggregation sense: MongoDB matches an
// array field against `$type: "string"` when any element is a string, so a
// document written as `{"app": ["steemx/1.0"]}` passes the match and then hits
// `$split`, which requires a string — `$split requires an expression that
// evaluates to a string as a first argument, found: array` (error 40085,
// reproduced on mongo 4.4). Because the aggregation aborts, the tick returns
// before writing the snapshot: one such document anywhere in the 90-day window
// freezes the whole clients snapshot, and the only trace is a log line.
//
// Guarding the value that feeds `$split` (rather than wrapping the branches in
// `$cond`) keeps the expression total no matter which branch the server
// evaluates; the array then projects an empty client name, is dropped by the
// `client != ""` match, and shows up as the gap between posts_scanned and
// posts_with_app instead of inventing an attribution for a shape no writer
// produces.
func clientAppStringExpr() bson.M {
	return bson.M{"$cond": []interface{}{
		bson.M{"$eq": []interface{}{bson.M{"$type": "$json_metadata.app"}, "string"}},
		"$json_metadata.app",
		"",
	}}
}

// clientNameExpr is the client-attribution expression: the part of
// json_metadata.app before the first "/", trimmed. A version suffix is optional
// in practice (`dkscore-poststudio`, `SteemX`, `drugwars`), so it must not gate
// attribution; the trim handles app strings that start with whitespace.
//
// It is shared by the per-day aggregation and by the coverage count so that
// "attributed" cannot mean two different things in the same snapshot: the count
// used to be a plain `json_metadata.app != ""`, which kept counting app strings
// like "/1.2.3" as attributed after the aggregation's `client != ""` match had
// already dropped them.
func clientNameExpr() bson.M {
	return bson.M{"$trim": bson.M{"input": bson.M{
		"$arrayElemAt": []interface{}{
			bson.M{"$split": []interface{}{clientAppStringExpr(), "/"}},
			0,
		},
	}}}
}

// countAttributedComments counts the window's comments that the aggregation
// below actually attributes to a client. It re-applies the attribution stages
// instead of restating them as a CountDocuments filter, because the verdict
// depends on the projected client name, not on the raw `app` string.
func (c *clientsRefresher) countAttributedComments(ctx context.Context, window bson.M) (int, error) {
	cursor, err := c.commentCol.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"created":           window,
			"json_metadata.app": bson.M{"$type": "string", "$ne": ""},
		}}},
		{{Key: "$project", Value: bson.M{"client": clientNameExpr()}}},
		{{Key: "$match", Value: bson.M{"client": bson.M{"$ne": ""}}}},
		{{Key: "$count", Value: "n"}},
	})
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var counts []struct {
		N int `bson:"n"`
	}
	if err := cursor.All(ctx, &counts); err != nil {
		return 0, err
	}
	if len(counts) == 0 {
		// $count emits nothing for an empty input.
		return 0, nil
	}
	return counts[0].N, nil
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
	// all, and how many of them end up attributed to a client. The gap is the
	// share with no client signal — it used to be ~11% and it is invisible to a
	// reader who only sees the per-client totals.
	//
	// `posts_scanned` counts COMMENTS (posts + replies), which is what the
	// window holds: it is the denominator of the coverage share, not a post
	// count.
	postsScanned, err := c.commentCol.CountDocuments(ctx, bson.M{"created": window})
	if err != nil {
		log.Printf("[Refresher:Clients] Failed to count window comments: %v", err)
		return
	}
	// `posts_with_app` must use the SAME predicate the aggregation below applies
	// before attributing, otherwise the coverage claim covers documents the
	// client lists do not contain: an app string of "/1.2.3" (or whitespace)
	// leaves an empty client name, is dropped by the `client != ""` match, and
	// was still counted as attributed while the count was a plain
	// `json_metadata.app != ""` — inflating the attributable share by exactly
	// those documents.
	postsWithApp, err := c.countAttributedComments(ctx, window)
	if err != nil {
		log.Printf("[Refresher:Clients] Failed to count attributed comments: %v", err)
		return
	}

	// Same pipeline as legacy history.py: match 90d comments whose
	// json_metadata.app is a non-empty string, group by client × day, then
	// re-group per day with per-client count/reward arrays.
	//
	// The `$type: "string"` match is a cheap pre-filter, not a type guarantee:
	// it also passes array values (see clientAppStringExpr), which is why the
	// projection has to be total rather than trusting the match.
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
				"client":  clientNameExpr(),
				// The reward legs, all three of what SteemDB stores per
				// comment: total_payout_value (already paid), pending_payout_value
				// (still accruing in the payout window) and
				// total_pending_payout_value (the chain's running estimate for the
				// same window). They are summed as written — the legs overlap in
				// meaning, and dropping one would silently change every historical
				// figure — so the column is "the payout legs summed", not "SBD
				// paid out".
				//
				// `$sum` over an array ignores null/missing operands, so a comment
				// without payout fields contributes 0 instead of turning the whole
				// group null: the previous `$add` did the latter, which is how 70
				// of 91 days read reward 0 while the post counts were right. The
				// `$ifNull` guards below are therefore belt-and-braces — removing
				// them changes nothing (verified on mongo 4.4), and they cost one
				// stage per leg in an aggregation that runs over a 90-day window.
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

	log.Printf("[Refresher:Clients] Snapshot updated (%d days, %d/%d comments attributed)",
		len(data), postsWithApp, postsScanned)
}
