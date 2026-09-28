package checker

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/processor/handlers"
)

// Finding is the outcome of one data health check.
type Finding struct {
	Name       string   // check identifier
	Healthy    bool     // true = no action needed
	RepairMode string   // `repair -mode` value that fixes the issue; "" = no repair mode exists yet
	Summary    string   // one-line human-readable result
	Details    []string // capped samples / hints
}

// HealthCheck is a read-only diagnostic paired with a repair mode. Every
// repair mode must ship with a check (docs/rules/repair-health-checks.md):
// the check tells operators whether the repair is needed at all, and
// re-running it after the repair verifies the fix. Checks must never write.
type HealthCheck interface {
	Name() string
	RepairMode() string
	Run(ctx context.Context) (*Finding, error)
}

// HealthStore is the read-only data surface health checks query. It is an
// interface so checks can be unit tested without MongoDB.
type HealthStore interface {
	// BlockStats returns the blocks document count and the min/max block
	// numbers present.
	BlockStats(ctx context.Context) (count int64, minBlock, maxBlock uint32, err error)
	// CountOperationsMissingAccounts counts operations in [fromBlock, toBlock]
	// that lack the accounts field (pre-backfill data).
	CountOperationsMissingAccounts(ctx context.Context, fromBlock, toBlock uint32) (int64, error)
	// InvalidAccountIDs returns account _ids that are not valid Steem account
	// names.
	InvalidAccountIDs(ctx context.Context) ([]string, error)
	// CountAccountStubs counts account documents that are dirty-mark stubs
	// (no name field) — phantom accounts queued from user-controlled payloads
	// that do not exist on chain.
	CountAccountStubs(ctx context.Context) (int64, error)
}

// DefaultChecks returns every registered health check. A new repair mode is
// incomplete until its check is registered here (see
// docs/rules/repair-health-checks.md).
func DefaultChecks(store HealthStore) []HealthCheck {
	return []HealthCheck{
		&blockContinuityCheck{store: store},
		&operationsAccountsCheck{store: store},
		&invalidAccountIDsCheck{store: store},
		&accountStubsCheck{store: store},
	}
}

// RunHealthChecks runs all checks in order, logging one line per check.
// Returns the findings and whether every check passed.
func RunHealthChecks(ctx context.Context, checks []HealthCheck, logger *log.Logger) ([]*Finding, bool) {
	findings := make([]*Finding, 0, len(checks))
	allHealthy := true
	for _, c := range checks {
		f, err := c.Run(ctx)
		if err != nil {
			logger.Printf("[ERROR] %-32s check failed: %v", c.Name(), err)
			allHealthy = false
			continue
		}
		findings = append(findings, f)
		status := "OK  "
		if !f.Healthy {
			status = "FAIL"
			allHealthy = false
		}
		repair := ""
		if !f.Healthy {
			if f.RepairMode != "" {
				repair = fmt.Sprintf(" → run: repair -mode=%s", f.RepairMode)
			} else {
				repair = " → no repair mode yet"
			}
		}
		logger.Printf("[%s] %-32s %s%s", status, f.Name, f.Summary, repair)
		for _, d := range f.Details {
			logger.Printf("       %s", d)
		}
	}
	return findings, allHealthy
}

// --- block-continuity (repair -mode=blocks) ---

// blockContinuityCheck is the quick smoke test for the blocks collection:
// document count must equal the min..max span. It cannot detect
// header-without-ops blocks — that needs the deep per-block scanner
// (repair -mode=blocks -dry-run), noted in the details.
type blockContinuityCheck struct{ store HealthStore }

func (c *blockContinuityCheck) Name() string       { return "block-continuity" }
func (c *blockContinuityCheck) RepairMode() string { return "blocks" }

func (c *blockContinuityCheck) Run(ctx context.Context) (*Finding, error) {
	count, minBlock, maxBlock, err := c.store.BlockStats(ctx)
	if err != nil {
		return nil, err
	}
	f := &Finding{Name: c.Name(), RepairMode: c.RepairMode()}
	if count == 0 {
		f.Healthy = false
		f.Summary = "blocks collection is empty"
		return f, nil
	}
	span := int64(maxBlock) - int64(minBlock) + 1
	gap := span - count
	f.Healthy = gap == 0
	f.Summary = fmt.Sprintf("blocks: %d docs, span %d..%d (%d), gap %d", count, minBlock, maxBlock, span, gap)
	if !f.Healthy {
		f.Details = append(f.Details,
			"count-vs-span is a smoke test; repair -mode=blocks -dry-run runs the deep per-block scan (also finds header-without-ops blocks)")
	}
	return f, nil
}

// --- operations-accounts-backfill (repair -mode=backfill-accounts) ---

// operationsAccountsCheck samples the oldest and newest 1000 blocks for
// operations missing the accounts field. Backfill is an all-or-nearly-all
// migration, so a sample detects an incomplete backfill; the recent range
// catches writers that stopped populating the field.
type operationsAccountsCheck struct{ store HealthStore }

func (c *operationsAccountsCheck) Name() string       { return "operations-accounts-backfill" }
func (c *operationsAccountsCheck) RepairMode() string { return "backfill-accounts" }

func (c *operationsAccountsCheck) Run(ctx context.Context) (*Finding, error) {
	_, _, maxBlock, err := c.store.BlockStats(ctx)
	if err != nil {
		return nil, err
	}
	f := &Finding{Name: c.Name(), RepairMode: c.RepairMode()}

	oldEnd := uint32(1000)
	if maxBlock < oldEnd {
		oldEnd = maxBlock
	}
	missingOld, err := c.store.CountOperationsMissingAccounts(ctx, 1, oldEnd)
	if err != nil {
		return nil, err
	}
	recentStart := uint32(1)
	if maxBlock > 1000 {
		recentStart = maxBlock - 999
	}
	missingRecent, err := c.store.CountOperationsMissingAccounts(ctx, recentStart, maxBlock)
	if err != nil {
		return nil, err
	}

	f.Healthy = missingOld+missingRecent == 0
	f.Summary = fmt.Sprintf("ops missing accounts field: %d in blocks 1..%d, %d in blocks %d..%d",
		missingOld, oldEnd, missingRecent, recentStart, maxBlock)
	if !f.Healthy {
		f.Details = append(f.Details, "sampled ranges; a full backfill re-run is idempotent (only updates docs lacking the field)")
	}
	return f, nil
}

// --- invalid-account-ids (repair -mode=cleanup-accounts) ---

type invalidAccountIDsCheck struct{ store HealthStore }

func (c *invalidAccountIDsCheck) Name() string       { return "invalid-account-ids" }
func (c *invalidAccountIDsCheck) RepairMode() string { return "cleanup-accounts" }

func (c *invalidAccountIDsCheck) Run(ctx context.Context) (*Finding, error) {
	ids, err := c.store.InvalidAccountIDs(ctx)
	if err != nil {
		return nil, err
	}
	f := &Finding{
		Name:       c.Name(),
		RepairMode: c.RepairMode(),
		Healthy:    len(ids) == 0,
		Summary:    fmt.Sprintf("account _ids failing name validation: %d", len(ids)),
	}
	const sampleCap = 20
	for i, id := range ids {
		if i >= sampleCap {
			f.Details = append(f.Details, fmt.Sprintf("... and %d more", len(ids)-sampleCap))
			break
		}
		f.Details = append(f.Details, fmt.Sprintf("invalid _id: %q", id))
	}
	return f, nil
}

// --- phantom-account-stubs (no repair mode yet) ---

// accountStubsCheck counts account documents that are dirty-mark stubs of
// accounts that do not exist on chain (queued from user-controlled
// custom_json payloads). They poison the AccountRefresher queue: the
// refresher fetches dirty ids in natural order, RPC never returns phantom
// accounts, and the same phantoms block the head of the queue forever.
type accountStubsCheck struct{ store HealthStore }

func (c *accountStubsCheck) Name() string       { return "phantom-account-stubs" }
func (c *accountStubsCheck) RepairMode() string { return "" }

func (c *accountStubsCheck) Run(ctx context.Context) (*Finding, error) {
	n, err := c.store.CountAccountStubs(ctx)
	if err != nil {
		return nil, err
	}
	f := &Finding{
		Name:    c.Name(),
		Healthy: n == 0,
		Summary: fmt.Sprintf("account stub docs (no name field, not on chain): %d", n),
	}
	if !f.Healthy {
		f.Details = append(f.Details,
			"stubs starve the AccountRefresher queue (head-of-line blocking); deleting them is safe — they carry no chain data")
	}
	return f, nil
}

// --- MongoHealthStore: HealthStore backed by the real MongoDB client ---

// MongoHealthStore implements HealthStore against the production MongoDB.
type MongoHealthStore struct {
	client *mongo.Client
}

// NewMongoHealthStore creates a MongoHealthStore.
func NewMongoHealthStore(client *mongo.Client) *MongoHealthStore {
	return &MongoHealthStore{client: client}
}

// BlockStats implements HealthStore.
func (s *MongoHealthStore) BlockStats(ctx context.Context) (int64, uint32, uint32, error) {
	blocks := s.client.Database().Collection("blocks")

	count, err := blocks.EstimatedDocumentCount(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("count blocks: %w", err)
	}
	if count == 0 {
		return 0, 0, 0, nil
	}

	var minDoc, maxDoc struct {
		ID uint32 `bson:"_id"`
	}
	if err := blocks.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"_id": 1})).Decode(&minDoc); err != nil {
		return 0, 0, 0, fmt.Errorf("min block: %w", err)
	}
	if err := blocks.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.M{"_id": -1})).Decode(&maxDoc); err != nil {
		return 0, 0, 0, fmt.Errorf("max block: %w", err)
	}
	return count, minDoc.ID, maxDoc.ID, nil
}

// CountOperationsMissingAccounts implements HealthStore.
func (s *MongoHealthStore) CountOperationsMissingAccounts(ctx context.Context, fromBlock, toBlock uint32) (int64, error) {
	return s.client.Database().Collection("operations").CountDocuments(ctx, bson.M{
		"block_num": bson.M{"$gte": fromBlock, "$lte": toBlock},
		"accounts":  bson.M{"$exists": false},
	})
}

// InvalidAccountIDs implements HealthStore.
func (s *MongoHealthStore) InvalidAccountIDs(ctx context.Context) ([]string, error) {
	raw, err := s.client.ListInvalidAccountIDs(ctx, handlers.IsValidAccountName)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(raw))
	for _, v := range raw {
		if str, ok := v.(string); ok {
			ids = append(ids, str)
		}
	}
	return ids, nil
}

// CountAccountStubs implements HealthStore.
func (s *MongoHealthStore) CountAccountStubs(ctx context.Context) (int64, error) {
	return s.client.Database().Collection("account").CountDocuments(ctx, bson.M{
		"name": bson.M{"$exists": false},
	})
}
