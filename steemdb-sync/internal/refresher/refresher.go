// Package refresher periodically snapshots chain state into MongoDB,
// replacing the legacy history.py and witnesses.py scripts
// (PROCESSOR_PLAN.md Batch 7/9).
//
// Tickers (each config-gated, run once at startup then on interval):
//   - witness (30s): witness / witness_history / witness_misses collections
//   - stats   (5m):  status transactions/operations counters
//   - clients (1h):  status.clients-snapshot + clients_history
//   - funds   (1h):  funds_history reward fund snapshots
//   - account rescan (24h, default off): full account refresh via lookup_accounts
package refresher

import (
	"context"
	"log"
	"time"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/rpc"
)

// Refresher owns and schedules all tickers.
type Refresher struct {
	cfg         *config.Config
	mongoClient *mongo.Client
	rpc         *rpc.Client
}

// New creates a Refresher from shared clients.
func New(cfg *config.Config, mongoClient *mongo.Client, rpcClient *rpc.Client) *Refresher {
	return &Refresher{cfg: cfg, mongoClient: mongoClient, rpc: rpcClient}
}

// Run starts all enabled tickers and blocks until ctx is cancelled.
func (r *Refresher) Run(ctx context.Context) {
	db := r.mongoClient.Database()

	if r.cfg.Refresher.Witness.Enabled {
		interval, err := r.cfg.RefresherWitnessInterval()
		if err != nil {
			log.Fatalf("[Refresher] Invalid witness interval: %v", err)
		}
		w := newWitnessRefresher(r.rpc, db)
		go runTicker(ctx, "Witness", interval, w.tick)
	}

	if r.cfg.Refresher.Stats.Enabled {
		interval, err := r.cfg.RefresherStatsInterval()
		if err != nil {
			log.Fatalf("[Refresher] Invalid stats interval: %v", err)
		}
		s := newStatsRefresher(db)
		go runTicker(ctx, "Stats", interval, s.tick)
	}

	if r.cfg.Refresher.Clients.Enabled {
		interval, err := r.cfg.RefresherClientsInterval()
		if err != nil {
			log.Fatalf("[Refresher] Invalid clients interval: %v", err)
		}
		c := newClientsRefresher(db)
		go runTicker(ctx, "Clients", interval, c.tick)
	}

	if r.cfg.Refresher.Funds.Enabled {
		interval, err := r.cfg.RefresherFundsInterval()
		if err != nil {
			log.Fatalf("[Refresher] Invalid funds interval: %v", err)
		}
		f := newFundsRefresher(r.rpc, db)
		go runTicker(ctx, "Funds", interval, f.tick)
	}

	if r.cfg.Refresher.AccountRescan.Enabled {
		interval, err := r.cfg.RefresherAccountRescanInterval()
		if err != nil {
			log.Fatalf("[Refresher] Invalid account rescan interval: %v", err)
		}
		rs := newAccountRescan(r.cfg, r.mongoClient, r.rpc)
		go runTicker(ctx, "AccountRescan", interval, rs.tick)
	}

	<-ctx.Done()
	log.Printf("[Refresher] Shutting down")
}

// runTicker executes fn immediately once, then on every interval tick.
// A panic or error inside fn is logged, never fatal — the next tick retries.
func runTicker(ctx context.Context, name string, interval time.Duration, fn func(ctx context.Context)) {
	log.Printf("[Refresher:%s] Started (interval=%s)", name, interval)

	fn(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Refresher:%s] Stopped", name)
			return
		case <-ticker.C:
			fn(ctx)
		}
	}
}
