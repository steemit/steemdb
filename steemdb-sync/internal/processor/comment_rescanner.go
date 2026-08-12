package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/rpc"
	drivermongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CommentRescanner periodically refreshes comment documents by calling get_content RPC.
// It fills dynamic fields (depth, active_votes, payouts, cashout_time, etc.) that the
// comment handler (Batch 2) cannot write from op data alone.
//
// Two queues (mirrors legacy sync.py update_queue):
//  1. Recent posts rescan: comments created within window_days, scanned > stale_hours ago
//  2. Past-payout rescan: comments with pending payout past cashout_time
//
// Uses a bounded worker pool (default 5) for concurrent get_content calls,
// since get_content is a single-item RPC (unlike get_accounts which is batched).
type CommentRescanner struct {
	cfg         *config.Config
	rpcClient   *rpc.Client
	commentCol  *drivermongo.Collection
	statusCol   *drivermongo.Collection

	interval    time.Duration
	batchSize   int
	workers     int
	windowDays  int
	staleHours  int
}

// NewCommentRescanner creates a new CommentRescanner from the processor context.
func NewCommentRescanner(ctx *Context) (*CommentRescanner, error) {
	interval, err := ctx.Cfg.CommentRescannerInterval()
	if err != nil {
		return nil, fmt.Errorf("invalid comment_rescanner.interval: %w", err)
	}

	batchSize := ctx.Cfg.Processor.CommentRescanner.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	workers := ctx.Cfg.Processor.CommentRescanner.Workers
	if workers <= 0 {
		workers = 5
	}

	windowDays := ctx.Cfg.Processor.CommentRescanner.WindowDays
	if windowDays <= 0 {
		windowDays = 3
	}

	staleHours := ctx.Cfg.Processor.CommentRescanner.StaleHours
	if staleHours <= 0 {
		staleHours = 6
	}

	db := ctx.MongoClient.Database()

	return &CommentRescanner{
		cfg:        ctx.Cfg,
		rpcClient:  ctx.RPCClient,
		commentCol: db.Collection("comment"),
		statusCol:  db.Collection("status"),
		interval:   interval,
		batchSize:  batchSize,
		workers:    workers,
		windowDays: windowDays,
		staleHours: staleHours,
	}, nil
}

// Run starts the rescanner loop. Blocks until ctx is cancelled.
func (r *CommentRescanner) Run(ctx context.Context) {
	log.Printf("[CommentRescanner] Started (interval=%s, batch=%d, workers=%d, window=%dd, stale=%dh)",
		r.interval, r.batchSize, r.workers, r.windowDays, r.staleHours)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[CommentRescanner] Shutting down")
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

// tick performs one rescan cycle.
func (r *CommentRescanner) tick(ctx context.Context) {
	// Skip during cold-start
	if r.isCatchingUp(ctx) {
		return
	}

	now := time.Now()
	maxDate := now.AddDate(0, 0, -r.windowDays)
	scanIgnore := now.Add(time.Duration(-r.staleHours) * time.Hour)

	// Queue 1: Recent posts needing rescan
	queue1 := r.findComments(ctx, bson.M{
		"created": bson.M{"$gt": maxDate},
		"scanned": bson.M{"$lt": scanIgnore},
	})

	// Queue 2: Past-payout comments needing rescan
	// Note: depth and mode fields may not exist yet (pre-rescan). This query
	// will only match comments that already have these fields from a prior rescan.
	queue2 := r.findComments(ctx, bson.M{
		"cashout_time":           bson.M{"$lt": now},
		"mode":                   bson.M{"$in": []string{"first_payout", "second_payout"}},
		"depth":                  0,
		"pending_payout_value":   bson.M{"$gt": 0},
	})

	// Bootstrap queue: comments that have never been rescanned (no depth field).
	// This handles the initial population of dynamic fields for comments created
	// by the comment handler (Batch 2) which only has op data, not get_content data.
	// Once a comment has depth, this query won't match it anymore.
	queue3 := r.findComments(ctx, bson.M{
		"depth": bson.M{"$exists": false},
	})

	// Combine and deduplicate
	all := deduplicateComments(append(append(queue1, queue2...), queue3...))

	if len(all) == 0 {
		return
	}

	log.Printf("[CommentRescanner] Rescanning %d comments (recent=%d, payout=%d, bootstrap=%d)",
		len(all), len(queue1), len(queue2), len(queue3))

	// Process with bounded concurrency
	r.processBatch(ctx, all)
}

// findComments queries the comment collection for documents matching the filter.
// Returns a slice of {author, permlink} pairs.
func (r *CommentRescanner) findComments(ctx context.Context, filter bson.M) []commentRef {
	cursor, err := r.commentCol.Find(ctx, filter,
		options.Find().
			SetProjection(bson.M{"author": 1, "permlink": 1}).
			SetSort(bson.M{"scanned": 1}).
			SetLimit(int64(r.batchSize)),
	)
	if err != nil {
		log.Printf("[CommentRescanner] Error querying comments: %v", err)
		return nil
	}
	defer cursor.Close(ctx)

	var results []struct {
		Author   string `bson:"author"`
		Permlink string `bson:"permlink"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		log.Printf("[CommentRescanner] Error decoding comments: %v", err)
		return nil
	}

	refs := make([]commentRef, 0, len(results))
	for _, r := range results {
		refs = append(refs, commentRef{Author: r.Author, Permlink: r.Permlink})
	}
	return refs
}

// processBatch rescan comments with a bounded worker pool.
func (r *CommentRescanner) processBatch(ctx context.Context, refs []commentRef) {
	sem := make(chan struct{}, r.workers) // bounded concurrency
	var wg sync.WaitGroup
	success := 0
	failed := 0
	var mu sync.Mutex

	for _, ref := range refs {
		select {
		case <-ctx.Done():
			break
		default:
		}

		wg.Add(1)
		go func(ref commentRef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			err := r.rescanOne(ctx, ref)
			mu.Lock()
			if err != nil {
				failed++
			} else {
				success++
			}
			mu.Unlock()
		}(ref)
	}

	wg.Wait()

	if success > 0 || failed > 0 {
		log.Printf("[CommentRescanner] Rescanned %d comments (%d success, %d failed)",
			success+failed, success, failed)
	}
}

// rescanOne calls get_content for a single comment and updates the document.
func (r *CommentRescanner) rescanOne(ctx context.Context, ref commentRef) error {
	content, err := r.rpcClient.GetContent(ref.Author, ref.Permlink)
	if err != nil {
		log.Printf("[CommentRescanner] get_content failed for %s/%s: %v", ref.Author, ref.Permlink, err)
		return err
	}

	doc := processContent(content)

	id := ref.Author + "/" + ref.Permlink
	_, err = r.commentCol.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": doc},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		log.Printf("[CommentRescanner] Failed to update comment %s: %v", id, err)
		return err
	}
	return nil
}

// isCatchingUp checks if the processor is still behind the sync cursor.
// Same logic as AccountRefresher.isCatchingUp.
func (r *CommentRescanner) isCatchingUp(ctx context.Context) bool {
	var statusDoc struct {
		Value uint32 `bson:"value"`
	}
	err := r.statusCol.FindOne(ctx, bson.M{"_id": "processor_height"}).Decode(&statusDoc)
	if err != nil {
		return true
	}

	// Fallback: read latest block from blocks collection
	var blockDoc struct {
		ID uint32 `bson:"_id"`
	}
	err = r.statusCol.Database().Collection("blocks").FindOne(ctx,
		bson.M{},
		options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}}),
	).Decode(&blockDoc)
	if err != nil {
		return true
	}

	return blockDoc.ID-statusDoc.Value > coldStartThreshold
}

// --- Content field transformations (mirrors legacy sync.py update_comment:320-358) ---

// processContent transforms a raw get_content result into a MongoDB document.
func processContent(content map[string]interface{}) bson.M {
	// Transform active_votes
	if votes, ok := content["active_votes"].([]interface{}); ok {
		transformedVotes := make([]map[string]interface{}, 0, len(votes))
		for _, v := range votes {
			if vote, ok := v.(map[string]interface{}); ok {
				// rshares and weight → float (legacy casts to float)
				transformRawMsgToFloat(vote, "rshares")
				transformRawMsgToFloat(vote, "weight")
				// time → parsed date
				if t, ok := vote["time"].(string); ok && t != "" {
					if parsed, err := time.Parse(steemDateFormat, t); err == nil {
						vote["time"] = parsed
					}
				}
				transformedVotes = append(transformedVotes, vote)
			}
		}
		content["active_votes"] = transformedVotes
	}

	// Float-cast numeric string fields (legacy float_keys)
	for _, key := range []string{"author_reputation", "net_rshares", "children_abs_rshares", "abs_rshares", "vote_rshares"} {
		transformRawMsgToFloat(content, key)
	}

	// Parse asset strings → float (legacy split_float_keys)
	for _, key := range []string{
		"total_pending_payout_value", "pending_payout_value",
		"max_accepted_payout", "total_payout_value", "curator_payout_value",
	} {
		transformAssetValue(content, key)
	}

	// Parse date fields (legacy date_keys)
	for _, key := range []string{"active", "created", "cashout_time", "last_payout", "last_update", "max_cashout_time"} {
		transformDate(content, key)
	}

	// Parse json_metadata
	if jm, ok := content["json_metadata"].(string); ok && jm != "" {
		var parsed interface{}
		if err := json.Unmarshal([]byte(jm), &parsed); err == nil {
			content["json_metadata"] = parsed
		}
		// On error: keep raw string (matches legacy behavior)
	}

	// Derive index fields (web frontend queries depend on these)
	if author, ok := content["author"].(string); ok {
		content["author_lower"] = strings.ToLower(author)
	}
	if category, ok := content["category"].(string); ok {
		content["category_lower"] = strings.ToLower(category)
	}
	if created, ok := content["created"].(time.Time); ok {
		content["date_idx"] = created.Format("2006-01-02")
	}

	// Set scanned timestamp
	content["scanned"] = time.Now().UTC()

	return content
}

// transformRawMsgToFloat converts a numeric string or json.RawMessage to float64 in place.
func transformRawMsgToFloat(m map[string]interface{}, key string) {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case string:
			m[key] = parseFloatSafe(val)
		case float64:
			// already numeric
		case json.RawMessage:
			m[key] = parseFloatSafe(string(val))
		}
	}
}

// --- Helpers ---

// commentRef is a reference to a comment document by author/permlink.
type commentRef struct {
	Author   string
	Permlink string
}

// deduplicateComments removes duplicate author/permlink pairs.
func deduplicateComments(refs []commentRef) []commentRef {
	seen := make(map[string]bool, len(refs))
	result := make([]commentRef, 0, len(refs))
	for _, r := range refs {
		key := r.Author + "/" + r.Permlink
		if !seen[key] {
			seen[key] = true
			result = append(result, r)
		}
	}
	return result
}
