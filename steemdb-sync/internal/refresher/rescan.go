package refresher

import (
	"context"
	"log"
	"time"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/processor"
	"github.com/steemit/steemdb-sync/internal/rpc"
)

// lookupPage is the lookup_accounts page size used to enumerate all accounts.
const lookupPage = 1000

// accountRescan optionally refreshes every account on the chain (legacy
// history.py did this daily). Default off: the dirty-based AccountRefresher
// covers active accounts, and a full pass is expensive on public RPC nodes.
// Field transformation is reused from processor.AccountRefresher.
type accountRescan struct {
	cfg       *config.Config
	rpcClient *rpc.Client
	refresher *processor.AccountRefresher
}

func newAccountRescan(cfg *config.Config, mongoClient *mongo.Client, rpcClient *rpc.Client) *accountRescan {
	pctx := &processor.Context{
		Cfg:         cfg,
		MongoClient: mongoClient,
		RPCClient:   rpcClient,
	}
	ar, err := processor.NewAccountRefresher(pctx)
	if err != nil {
		log.Printf("[Refresher:AccountRescan] Failed to create account refresher: %v", err)
		return &accountRescan{}
	}
	return &accountRescan{cfg: cfg, rpcClient: rpcClient, refresher: ar}
}

func (a *accountRescan) tick(ctx context.Context) {
	if a.refresher == nil {
		return
	}

	start := time.Now()
	total, refreshed, failed := 0, 0, 0

	// Page through all account names via lookup_accounts.
	after := ""
	for {
		names, err := a.rpcClient.LookupAccounts(ctx, after, lookupPage)
		if err != nil {
			log.Printf("[Refresher:AccountRescan] lookup_accounts failed at %q: %v", after, err)
			break
		}
		if len(names) == 0 {
			break
		}

		// lookup_accounts returns names strictly after `after`; the last one
		// becomes the next cursor.
		after = names[len(names)-1]

		ok, errs := a.refresher.RefreshNames(ctx, names)
		refreshed += ok
		failed += errs
		total += len(names)

		if len(names) < lookupPage {
			break
		}
		// Pace paging to be gentle on the RPC node.
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("[Refresher:AccountRescan] Completed: %d accounts (%d refreshed, %d failed) in %s",
		total, refreshed, failed, time.Since(start).Round(time.Second))
}
