package refresher

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/steemit/steemdb-sync/internal/rpc"
)

// fundsRefresher appends reward-fund snapshots to funds_history (legacy
// history.py update_fund_history; interval tightened from 24h to 1h — the RPC
// is cheap and the dashboard reads the newest snapshot).
type fundsRefresher struct {
	rpcClient *rpc.Client
	fundsCol  *mongo.Collection
}

func newFundsRefresher(rpcClient *rpc.Client, db *mongo.Database) *fundsRefresher {
	return &fundsRefresher{
		rpcClient: rpcClient,
		fundsCol:  db.Collection("funds_history"),
	}
}

func (f *fundsRefresher) tick(ctx context.Context) {
	raw, err := f.rpcClient.GetRewardFund(ctx, "post")
	if err != nil {
		log.Printf("[Refresher:Funds] RPC failed: %v", err)
		return
	}

	doc := convertRewardFund(raw)
	if _, err := f.fundsCol.InsertOne(ctx, doc); err != nil {
		log.Printf("[Refresher:Funds] Failed to insert funds_history: %v", err)
		return
	}

	log.Printf("[Refresher:Funds] Snapshot inserted (reward_balance=%v)", doc["reward_balance"])
}

// convertRewardFund applies the legacy conversions: recent_claims and
// content_constant (string numbers) become float64, reward_balance drops the
// asset suffix, last_update becomes time.Time.
func convertRewardFund(raw map[string]interface{}) bson.M {
	doc := bson.M{}
	for k, v := range raw {
		doc[k] = v
	}

	for _, f := range []string{"recent_claims", "content_constant"} {
		if s, ok := doc[f].(string); ok {
			if f64, err := strconv.ParseFloat(s, 64); err == nil {
				doc[f] = f64
			}
		}
	}

	if s, ok := doc["reward_balance"].(string); ok {
		if idx := strings.IndexByte(s, ' '); idx > 0 {
			if f64, err := strconv.ParseFloat(s[:idx], 64); err == nil {
				doc["reward_balance"] = f64
			}
		}
	}

	if s, ok := doc["last_update"].(string); ok && s != "" {
		if t, err := time.Parse(steemDateFormat, s); err == nil {
			doc["last_update"] = t
		}
	}

	return doc
}
