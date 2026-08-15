package refresher

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Block counts used as time windows (3-second blocks).
const (
	blocksPerDay  = 28800
	blocksPerHour = 1200
)

// statsRefresher maintains the status-collection activity counters consumed by
// the web dashboard (GetNetworkPerformance). Legacy aggregated block_30d
// transactions; the Go sync has no block_30d and blocks.transaction_count is
// always 0, so counts are derived from the operations collection by block range.
type statsRefresher struct {
	blocksCol *mongo.Collection
	opsCol    *mongo.Collection
	statusCol *mongo.Collection
}

func newStatsRefresher(db *mongo.Database) *statsRefresher {
	return &statsRefresher{
		blocksCol: db.Collection("blocks"),
		opsCol:    db.Collection("operations"),
		statusCol: db.Collection("status"),
	}
}

func (s *statsRefresher) tick(ctx context.Context) {
	head, err := s.headBlock(ctx)
	if err != nil {
		log.Printf("[Refresher:Stats] Failed to read head block: %v", err)
		return
	}
	if head == 0 {
		return
	}

	counts := []struct {
		id     string
		window int64
	}{
		{"transactions-24h", head - blocksPerDay},
		{"transactions-1h", head - blocksPerHour},
		{"operations-24h", head - blocksPerDay},
		{"operations-1h", head - blocksPerHour},
	}

	for _, c := range counts {
		var value int64
		var err error
		if c.id[:1] == "t" {
			// Real transactions: distinct non-empty trx_id in the window.
			// Virtual operations carry trx_id "" and are excluded.
			var ids []interface{}
			ids, err = s.opsCol.Distinct(ctx, "trx_id", bson.M{
				"block_num": bson.M{"$gt": c.window},
				"trx_id":    bson.M{"$ne": ""},
			})
			value = int64(len(ids))
		} else {
			value, err = s.opsCol.CountDocuments(ctx, bson.M{
				"block_num": bson.M{"$gt": c.window},
			})
		}
		if err != nil {
			log.Printf("[Refresher:Stats] Failed to compute %s: %v", c.id, err)
			continue
		}

		if _, err := s.statusCol.UpdateOne(ctx,
			bson.M{"_id": c.id},
			bson.M{"$set": bson.M{"data": value}},
			options.Update().SetUpsert(true),
		); err != nil {
			log.Printf("[Refresher:Stats] Failed to write %s: %v", c.id, err)
		}
	}

	log.Printf("[Refresher:Stats] Updated activity counters (head=%d)", head)
}

// headBlock returns the highest block number in the blocks collection.
func (s *statsRefresher) headBlock(ctx context.Context) (int64, error) {
	var doc struct {
		ID int64 `bson:"_id"`
	}
	err := s.blocksCol.FindOne(ctx,
		bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}}),
	).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	return doc.ID, err
}
