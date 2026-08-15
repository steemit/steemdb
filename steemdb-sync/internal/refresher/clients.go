package refresher

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// clientsWindowDays is the comment window scanned for client distribution.
const clientsWindowDays = 90

// clientsRefresher aggregates client usage from comment.json_metadata.app
// (legacy history.py update_clients) into status.clients-snapshot (consumed by
// the web labs/clients endpoint) and clients_history (daily record).
type clientsRefresher struct {
	commentCol *mongo.Collection
	statusCol  *mongo.Collection
	historyCol *mongo.Collection
}

func newClientsRefresher(db *mongo.Database) *clientsRefresher {
	return &clientsRefresher{
		commentCol: db.Collection("comment"),
		statusCol:  db.Collection("status"),
		historyCol: db.Collection("clients_history"),
	}
}

func (c *clientsRefresher) tick(ctx context.Context) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -clientsWindowDays)

	// Same pipeline as legacy history.py: match 90d comments whose
	// json_metadata.app looks like "name/version", group by client × day,
	// then re-group per day with per-client count/reward arrays.
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"created": bson.M{"$gte": start, "$lte": now},
				"json_metadata.app": bson.M{
					"$type":  "string",
					"$regex": primitive.Regex{Pattern: `[\w-]+/[\w.]+`, Options: "i"},
				},
			},
		},
		{
			"$project": bson.M{
				"created": "$created",
				"parts":   bson.M{"$split": []interface{}{"$json_metadata.app", "/"}},
				"reward": bson.M{"$add": []interface{}{
					"$total_payout_value", "$pending_payout_value", "$total_pending_payout_value",
				}},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"client": bson.M{"$arrayElemAt": []interface{}{"$parts", 0}},
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
			"$sort": bson.M{"_id.year": 1, "_id.doy": 1, "value": -1},
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
			"$sort": bson.M{"_id.year": -1, "_id.doy": -1},
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

	if _, err := c.statusCol.UpdateOne(ctx,
		bson.M{"_id": "clients-snapshot"},
		bson.M{"$set": bson.M{"data": data}},
		options.Update().SetUpsert(true),
	); err != nil {
		log.Printf("[Refresher:Clients] Failed to write clients-snapshot: %v", err)
		return
	}

	if _, err := c.historyCol.UpdateOne(ctx,
		bson.M{"date": today},
		bson.M{"$set": bson.M{"data": data}},
		options.Update().SetUpsert(true),
	); err != nil {
		log.Printf("[Refresher:Clients] Failed to write clients_history: %v", err)
	}

	log.Printf("[Refresher:Clients] Snapshot updated (%d days)", len(data))
}
