package refresher

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb-sync/internal/rpc"
)

// witnessTopN is the number of witnesses tracked (RPC max for get_witnesses_by_vote).
const witnessTopN = 100

// steemDateFormat is the date format used by Steem RPC timestamps.
const steemDateFormat = "2006-01-02T15:04:05"

// witnessRefresher rebuilds the witness / witness_history / witness_misses
// collections from get_witnesses_by_vote (legacy witnesses.py).
type witnessRefresher struct {
	rpcClient  *rpc.Client
	witnessCol *mongo.Collection
	historyCol *mongo.Collection
	missesCol  *mongo.Collection

	// misses remembers the last observed total_missed per witness owner to
	// detect increases (legacy kept this in process memory too).
	misses   map[string]int64
	missesMu sync.Mutex
}

func newWitnessRefresher(rpcClient *rpc.Client, db *mongo.Database) *witnessRefresher {
	return &witnessRefresher{
		rpcClient:  rpcClient,
		witnessCol: db.Collection("witness"),
		historyCol: db.Collection("witness_history"),
		missesCol:  db.Collection("witness_misses"),
		misses:     make(map[string]int64),
	}
}

func (w *witnessRefresher) tick(ctx context.Context) {
	witnesses, err := w.rpcClient.GetWitnessesByVote(ctx, "", witnessTopN)
	if err != nil {
		log.Printf("[Refresher:Witness] RPC failed: %v", err)
		return
	}
	if len(witnesses) == 0 {
		return
	}

	now := time.Now().UTC()
	today := now.Format("20060102")

	owners := make([]string, 0, len(witnesses))
	for _, raw := range witnesses {
		doc := convertWitness(raw)
		owner, _ := doc["owner"].(string)
		if owner == "" {
			continue
		}
		owners = append(owners, owner)

		// witness: upsert by owner (idempotent; no clear-rebuild window)
		if _, err := w.witnessCol.UpdateOne(ctx,
			bson.M{"_id": owner},
			bson.M{"$set": doc},
			options.Update().SetUpsert(true),
		); err != nil {
			log.Printf("[Refresher:Witness] Failed to upsert witness %s: %v", owner, err)
		}

		// witness_history: one snapshot per owner per day (same _id overwritten)
		snapshot := bson.M{}
		for k, v := range doc {
			snapshot[k] = v
		}
		snapshot["_id"] = owner + "|" + today
		snapshot["created"] = now
		if _, err := w.historyCol.UpdateOne(ctx,
			bson.M{"_id": snapshot["_id"]},
			bson.M{"$set": snapshot},
			options.Update().SetUpsert(true),
		); err != nil {
			log.Printf("[Refresher:Witness] Failed to upsert witness_history %s: %v", owner, err)
		}

		// witness_misses: insert when total_missed increased since last observation
		if record := w.detectMiss(owner, readInt64(doc, "total_missed"), now); record != nil {
			if _, err := w.missesCol.InsertOne(ctx, record); err != nil {
				log.Printf("[Refresher:Witness] Failed to insert witness_miss for %s: %v", owner, err)
			} else {
				log.Printf("[Refresher:Witness] Miss recorded: %s +%d (total %d)",
					owner, record["increase"], record["total"])
			}
		}
	}

	// Keep the collection at exactly the current top-N (legacy removed all docs
	// first; deleting absent owners afterwards achieves the same state without
	// the empty-collection window).
	if _, err := w.witnessCol.DeleteMany(ctx,
		bson.M{"owner": bson.M{"$nin": owners}},
	); err != nil {
		log.Printf("[Refresher:Witness] Failed to remove stale witnesses: %v", err)
	}

	log.Printf("[Refresher:Witness] Refreshed %d witnesses", len(owners))
}

// detectMiss compares the observed total_missed against the in-memory baseline
// and returns an insert record when it increased (nil otherwise).
func (w *witnessRefresher) detectMiss(owner string, totalMissed int64, now time.Time) bson.M {
	if totalMissed < 0 {
		return nil
	}
	w.missesMu.Lock()
	defer w.missesMu.Unlock()

	last, seen := w.misses[owner]
	w.misses[owner] = totalMissed
	if !seen || totalMissed <= last {
		return nil
	}
	return bson.M{
		"date":     now,
		"witness":  owner,
		"increase": totalMissed - last,
		"total":    totalMissed,
	}
}

// convertWitness applies the legacy field conversions: votes and the three
// virtual_* counters (string big-ints) become float64 for mongo-sortability,
// last_sbd_exchange_update becomes time.Time. All other fields pass through.
func convertWitness(raw map[string]interface{}) bson.M {
	doc := bson.M{}
	for k, v := range raw {
		doc[k] = v
	}

	for _, f := range []string{"votes", "virtual_last_update", "virtual_position", "virtual_scheduled_time"} {
		if s, ok := doc[f].(string); ok {
			if f64, err := strconv.ParseFloat(s, 64); err == nil {
				doc[f] = f64
			}
		}
	}

	if s, ok := doc["last_sbd_exchange_update"].(string); ok && s != "" {
		if t, err := time.Parse(steemDateFormat, s); err == nil {
			doc["last_sbd_exchange_update"] = t
		}
	}

	return doc
}

// readInt64 extracts an integer from a raw map value (string or number).
func readInt64(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok {
		return -1
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case string:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
	}
	return -1
}
