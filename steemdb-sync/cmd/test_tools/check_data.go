package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/model"
	"github.com/steemit/steemdb-sync/internal/rpc"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorBlue   = "\033[0;34m"
)

type checkDataResult struct {
	ExpectedMaxBlock   uint32
	BlocksCount        int64
	BlocksMin          *uint32
	BlocksMax          *uint32
	MissingCount       int
	MissingSample      []uint32
	MissingRanges      string
	OpsTotal           int64
	OrphanOpsCount     int64
	OrphanOpsSample    []string
	BlocksWithOpsCount int
	BlocksZeroOpsCount int
	TailHistogram      []blockHistogram
	ValidationErrors   []validationError
}

type blockHistogram struct {
	BlockNum uint32
	Ops      int64
}

type validationError struct {
	BlockNum uint32
	Type     string // "block_id", "timestamp", "ops_count", etc.
	Message  string
}

type validationStats struct {
	TotalChecked    int64
	TotalErrors     int64
	BlockIDErrors   int64
	TimestampErrors int64
	OpsCountErrors  int64
}

func runCheckData(args []string) {
	fs := flag.NewFlagSet("check_data", flag.ExitOnError)
	var (
		envFile     = fs.String("env", "", "Path to .env file (default: look for .env in current directory)")
		apiEndpoint = fs.String("api", "https://api.steemit.com", "Steem RPC API endpoint for validation")
		validateRPC = fs.Bool("validate-rpc", false, "Enable RPC API validation (slower but more thorough)")
		sampleRate  = fs.Int("sample-rate", 100, "Sample rate for RPC validation (1 = check all blocks, 100 = check 1% of blocks)")
		workDir     = fs.String("work-dir", "", "Working directory (default: current directory)")
	)
	fs.Parse(args)

	workDirPath := *workDir
	if workDirPath == "" {
		var err error
		workDirPath, err = os.Getwd()
		if err != nil {
			fail("Failed to get current directory: %v", err)
		}
	}

	// Load .env file
	envPath := *envFile
	if envPath == "" {
		envPath = filepath.Join(workDirPath, ".env")
	}

	env := loadEnvFile(envPath)

	// Get MongoDB connection settings from .env
	mongoUsername := getEnv(env, "MONGO_USERNAME", "admin")
	mongoPassword := getEnv(env, "MONGO_PASSWORD", "123456")
	mongoDatabase := getEnv(env, "MONGO_DATABASE", "steemdb_test")
	mongoPort := getEnv(env, "MONGO_PORT", "27017")
	mongoHost := getEnv(env, "MONGO_HOST", "localhost")

	mongoURI := fmt.Sprintf("mongodb://%s:%s@%s:%s/%s?authSource=admin",
		mongoUsername, mongoPassword, mongoHost, mongoPort, mongoDatabase)

	stopReplayAtBlockStr := getEnv(env, "STOP_REPLAY_AT_BLOCK", "0")
	stopReplayAtBlock, _ := strconv.ParseUint(stopReplayAtBlockStr, 10, 32)

	fmt.Printf("%s=== Checking Cold Ingest Mongo Data (blocks + operations) ===%s\n\n", colorGreen, colorReset)

	// Connect to MongoDB with longer timeout for large datasets
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fmt.Printf("%sConnecting to MongoDB...%s\n", colorBlue, colorReset)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		fail("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	// Ping MongoDB
	if err := client.Ping(ctx, nil); err != nil {
		fail("MongoDB is not responding: %v", err)
	}
	fmt.Printf("%s✓ MongoDB is healthy%s\n\n", colorGreen, colorReset)

	db := client.Database(mongoDatabase)
	blocksColl := db.Collection("blocks")
	operationsColl := db.Collection("operations")
	metaColl := db.Collection("meta")

	// Derive expected_max_block
	var expectedMaxBlock uint32
	if stopReplayAtBlock > 0 {
		expectedMaxBlock = uint32(stopReplayAtBlock)
		fmt.Printf("%s✓ expected_max_block=%d%s (from STOP_REPLAY_AT_BLOCK)\n\n", colorGreen, expectedMaxBlock, colorReset)
	} else {
		// Query meta collection
		var meta model.Meta
		err := metaColl.FindOne(ctx, bson.M{"_id": "sync_state"}).Decode(&meta)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				fail("Cannot infer expected_max_block: STOP_REPLAY_AT_BLOCK=0 and meta.sync_state.max_block not found")
			}
			fail("Failed to query meta collection: %v", err)
		}
		expectedMaxBlock = meta.MaxBlock
		fmt.Printf("%s✓ expected_max_block=%d%s (from meta.sync_state.max_block)\n\n", colorGreen, expectedMaxBlock, colorReset)
	}

	if expectedMaxBlock < 1 {
		fail("Invalid expected_max_block: %d", expectedMaxBlock)
	}

	// Query blocks/operations summary
	fmt.Printf("%sQuerying blocks/operations summary...%s\n", colorBlue, colorReset)
	if expectedMaxBlock > 100000 {
		fmt.Printf("%sNote: Large dataset detected (%d blocks). This may take several minutes...%s\n", colorYellow, expectedMaxBlock, colorReset)
	}
	result, err := queryDataSummary(ctx, blocksColl, operationsColl, expectedMaxBlock)
	if err != nil {
		fail("Failed to query data summary: %v", err)
	}
	result.ExpectedMaxBlock = expectedMaxBlock

	// Print summary
	printSummary(result)

	// Check for basic issues
	if result.BlocksCount == 0 {
		fail("No blocks found in range [1..%d]. Did replay run?", expectedMaxBlock)
	}

	issues := 0
	if result.MissingCount > 0 {
		fmt.Printf("%s✗ Missing blocks detected%s\n", colorRed, colorReset)
		issues = 1
	} else {
		fmt.Printf("%s✓ No missing blocks in expected range%s\n", colorGreen, colorReset)
	}

	if result.OrphanOpsCount > 0 {
		fmt.Printf("%s✗ Orphan operations detected (ops referencing missing blocks)%s\n", colorRed, colorReset)
		issues = 1
	} else {
		fmt.Printf("%s✓ No orphan operations%s\n", colorGreen, colorReset)
	}

	// RPC validation (optional)
	if *validateRPC {
		fmt.Printf("\n%sStarting RPC API validation...%s\n", colorBlue, colorReset)
		fmt.Printf("API endpoint: %s\n", *apiEndpoint)
		fmt.Printf("Sample rate: 1/%d blocks\n\n", *sampleRate)

		rpcClient := rpc.NewClient(*apiEndpoint, 3, 30*time.Second)
		validationErrors, stats := validateWithRPC(ctx, rpcClient, blocksColl, operationsColl, expectedMaxBlock, *sampleRate)
		result.ValidationErrors = validationErrors

		fmt.Printf("\n%sRPC Validation Results:%s\n", colorBlue, colorReset)
		fmt.Printf("  Blocks checked: %d\n", stats.TotalChecked)
		fmt.Printf("  Total errors:   %d\n", stats.TotalErrors)
		fmt.Printf("  Block ID errors: %d\n", stats.BlockIDErrors)
		fmt.Printf("  Timestamp errors: %d\n", stats.TimestampErrors)
		fmt.Printf("  Operations count errors: %d\n", stats.OpsCountErrors)

		if len(validationErrors) > 0 {
			fmt.Printf("\n%sValidation Errors (first 10):%s\n", colorYellow, colorReset)
			for i, err := range validationErrors {
				if i >= 10 {
					break
				}
				fmt.Printf("  Block %d [%s]: %s\n", err.BlockNum, err.Type, err.Message)
			}
			if len(validationErrors) > 10 {
				fmt.Printf("  ... and %d more errors\n", len(validationErrors)-10)
			}
			issues = 1
		} else {
			fmt.Printf("\n%s✓ All validated blocks match RPC API%s\n", colorGreen, colorReset)
		}
	}

	fmt.Println()
	if issues > 0 {
		fmt.Printf("%s=== CHECK FAILED ===%s\n", colorRed, colorReset)
		os.Exit(1)
	}

	fmt.Printf("%s=== CHECK PASSED ===%s\n", colorGreen, colorReset)
	os.Exit(0)
}

func printSummary(result *checkDataResult) {
	fmt.Printf("expected_max_block=%d\n", result.ExpectedMaxBlock)
	fmt.Printf("blocks_count_in_range=%d\n", result.BlocksCount)
	if result.BlocksMin != nil {
		fmt.Printf("blocks_min_in_range=%d\n", *result.BlocksMin)
	} else {
		fmt.Printf("blocks_min_in_range=\n")
	}
	if result.BlocksMax != nil {
		fmt.Printf("blocks_max_in_range=%d\n", *result.BlocksMax)
	} else {
		fmt.Printf("blocks_max_in_range=\n")
	}
	fmt.Printf("missing_count=%d\n", result.MissingCount)
	if len(result.MissingSample) > 0 {
		sampleStrs := make([]string, len(result.MissingSample))
		for i, v := range result.MissingSample {
			sampleStrs[i] = fmt.Sprintf("%d", v)
		}
		fmt.Printf("missing_sample=%s\n", strings.Join(sampleStrs, ","))
	} else {
		fmt.Printf("missing_sample=\n")
	}
	fmt.Printf("missing_ranges=%s\n", result.MissingRanges)
	fmt.Printf("ops_total=%d\n", result.OpsTotal)
	fmt.Printf("orphan_ops_count=%d\n", result.OrphanOpsCount)
	if len(result.OrphanOpsSample) > 0 {
		fmt.Printf("orphan_ops_sample=%s\n", strings.Join(result.OrphanOpsSample, ","))
	} else {
		fmt.Printf("orphan_ops_sample=\n")
	}
	fmt.Printf("blocks_with_ops_in_range=%d\n", result.BlocksWithOpsCount)
	fmt.Printf("blocks_zero_ops_in_range=%d\n", result.BlocksZeroOpsCount)
	fmt.Printf("tail_histogram=%s\n", formatHistogram(result.TailHistogram))
	fmt.Println()

	fmt.Printf("%sInterpretation:%s\n", colorBlue, colorReset)
	fmt.Printf("  expected_max_block: %d\n", result.ExpectedMaxBlock)
	if result.BlocksMax != nil {
		fmt.Printf("  blocks_max_in_range: %d\n", *result.BlocksMax)
	} else {
		fmt.Printf("  blocks_max_in_range: <none>\n")
	}
	fmt.Printf("  missing_count:       %d\n", result.MissingCount)
	fmt.Printf("  orphan_ops_count:    %d\n", result.OrphanOpsCount)
	fmt.Println()
}

func queryDataSummary(ctx context.Context, blocksColl, operationsColl *mongo.Collection, expectedMaxBlock uint32) (*checkDataResult, error) {
	result := &checkDataResult{}

	// Count blocks in range
	fmt.Printf("  Counting blocks...")
	os.Stdout.Sync()
	filter := bson.M{"_id": bson.M{"$gte": 1, "$lte": expectedMaxBlock}}
	count, err := blocksColl.CountDocuments(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count blocks")
	}
	result.BlocksCount = count
	fmt.Printf(" %d blocks found\n", count)

	// Find min/max blocks
	var minBlock, maxBlock bson.M
	err = blocksColl.FindOne(ctx, filter, options.FindOne().SetSort(bson.M{"_id": 1})).Decode(&minBlock)
	if err == nil {
		var blockNum uint32
		if id, ok := minBlock["_id"].(int32); ok {
			blockNum = uint32(id)
		} else if id, ok := minBlock["_id"].(int64); ok {
			blockNum = uint32(id)
		} else if id, ok := minBlock["_id"].(float64); ok {
			blockNum = uint32(id)
		}
		if blockNum > 0 {
			result.BlocksMin = &blockNum
		}
	}

	err = blocksColl.FindOne(ctx, filter, options.FindOne().SetSort(bson.M{"_id": -1})).Decode(&maxBlock)
	if err == nil {
		var blockNum uint32
		if id, ok := maxBlock["_id"].(int32); ok {
			blockNum = uint32(id)
		} else if id, ok := maxBlock["_id"].(int64); ok {
			blockNum = uint32(id)
		} else if id, ok := maxBlock["_id"].(float64); ok {
			blockNum = uint32(id)
		}
		if blockNum > 0 {
			result.BlocksMax = &blockNum
		}
	}

	// Find missing blocks
	fmt.Printf("  Scanning for missing blocks...")
	os.Stdout.Sync()
	cursor, err := blocksColl.Find(ctx, filter, options.Find().SetSort(bson.M{"_id": 1}).SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find blocks")
	}
	defer cursor.Close(ctx)

	var missing []uint32
	var prev uint32
	var scanned int64
	totalToScan := result.BlocksCount
	lastProgress := time.Now()

	for cursor.Next(ctx) {
		scanned++
		// Show progress every 10% or every 5 seconds for large datasets
		if totalToScan > 10000 {
			now := time.Now()
			if scanned%10000 == 0 || now.Sub(lastProgress) > 5*time.Second {
				var percent float64
				if totalToScan > 0 {
					percent = float64(scanned) * 100 / float64(totalToScan)
				}
				fmt.Printf("\r  Scanning for missing blocks... %d/%d (%.1f%%)", scanned, totalToScan, percent)
				os.Stdout.Sync()
				lastProgress = now
			}
		}
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		var blockNum uint32
		if id, ok := doc["_id"].(int32); ok {
			blockNum = uint32(id)
		} else if id, ok := doc["_id"].(int64); ok {
			blockNum = uint32(id)
		} else if id, ok := doc["_id"].(float64); ok {
			blockNum = uint32(id)
		} else {
			continue
		}
		if blockNum > prev+1 {
			for x := prev + 1; x < blockNum; x++ {
				missing = append(missing, x)
			}
		}
		prev = blockNum
	}
	if prev < expectedMaxBlock {
		for x := prev + 1; x <= expectedMaxBlock; x++ {
			missing = append(missing, x)
		}
	}
	result.MissingCount = len(missing)
	if len(missing) > 20 {
		result.MissingSample = missing[:20]
	} else {
		result.MissingSample = missing
	}
	result.MissingRanges = formatRanges(missing)
	fmt.Printf("\r  Scanning for missing blocks... done (%d missing)\n", len(missing))

	// Count total operations
	fmt.Printf("  Counting operations...")
	os.Stdout.Sync()
	totalOps, err := operationsColl.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to count operations")
	}
	result.OpsTotal = totalOps
	fmt.Printf(" %d operations found\n", totalOps)

	// Find orphan operations
	fmt.Printf("  Checking for orphan operations...")
	os.Stdout.Sync()
	pipeline := []bson.M{
		{"$lookup": bson.M{
			"from":         "blocks",
			"localField":   "block_num",
			"foreignField": "_id",
			"as":           "b",
		}},
		{"$match": bson.M{"b": bson.M{"$eq": []interface{}{}}}},
		{"$group": bson.M{
			"_id":    nil,
			"count":  bson.M{"$sum": 1},
			"sample": bson.M{"$push": "$_id"},
		}},
		{"$project": bson.M{
			"_id":    0,
			"count":  1,
			"sample": bson.M{"$slice": []interface{}{"$sample", 5}},
		}},
	}

	cursor, err = operationsColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "failed to aggregate orphan operations")
	}
	defer cursor.Close(ctx)

	if cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err == nil {
			if count, ok := doc["count"].(int32); ok {
				result.OrphanOpsCount = int64(count)
			} else if count, ok := doc["count"].(int64); ok {
				result.OrphanOpsCount = count
			} else if count, ok := doc["count"].(float64); ok {
				result.OrphanOpsCount = int64(count)
			}
			if sample, ok := doc["sample"].(bson.A); ok {
				for _, v := range sample {
					if str, ok := v.(string); ok {
						result.OrphanOpsSample = append(result.OrphanOpsSample, str)
					}
				}
			}
		}
	}
	fmt.Printf(" done (%d orphan operations)\n", result.OrphanOpsCount)

	// Count distinct blocks with operations
	// Use aggregation instead of distinct to avoid 16MB limit
	fmt.Printf("  Counting distinct blocks with operations...")
	os.Stdout.Sync()
	distinctPipeline := []bson.M{
		{"$match": bson.M{"block_num": bson.M{"$gte": 1, "$lte": expectedMaxBlock}}},
		{"$group": bson.M{"_id": "$block_num"}},
		{"$group": bson.M{"_id": nil, "count": bson.M{"$sum": 1}}},
	}
	distinctCursor, err := operationsColl.Aggregate(ctx, distinctPipeline)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count distinct blocks with operations")
	}
	defer distinctCursor.Close(ctx)

	result.BlocksWithOpsCount = 0
	if distinctCursor.Next(ctx) {
		var doc bson.M
		if err := distinctCursor.Decode(&doc); err == nil {
			if count, ok := doc["count"].(int32); ok {
				result.BlocksWithOpsCount = int(count)
			} else if count, ok := doc["count"].(int64); ok {
				result.BlocksWithOpsCount = int(count)
			} else if count, ok := doc["count"].(float64); ok {
				result.BlocksWithOpsCount = int(count)
			}
		}
	}
	result.BlocksZeroOpsCount = int(result.BlocksCount) - result.BlocksWithOpsCount
	if result.BlocksZeroOpsCount < 0 {
		result.BlocksZeroOpsCount = 0
	}
	fmt.Printf(" done (%d blocks with operations)\n", result.BlocksWithOpsCount)

	// Tail histogram (last 20 blocks)
	fmt.Printf("  Generating tail histogram...")
	os.Stdout.Sync()
	tailN := 20
	tailStart := expectedMaxBlock - uint32(tailN) + 1
	if tailStart < 1 {
		tailStart = 1
	}
	for bn := tailStart; bn <= expectedMaxBlock; bn++ {
		count, err := operationsColl.CountDocuments(ctx, bson.M{"block_num": bn})
		if err != nil {
			continue
		}
		result.TailHistogram = append(result.TailHistogram, blockHistogram{
			BlockNum: bn,
			Ops:      count,
		})
	}
	fmt.Printf(" done\n\n")

	return result, nil
}

func validateWithRPC(ctx context.Context, rpcClient *rpc.Client, blocksColl, operationsColl *mongo.Collection, expectedMaxBlock uint32, sampleRate int) ([]validationError, *validationStats) {
	var errors []validationError
	var errorsMu sync.Mutex
	stats := &validationStats{}

	// Get all block numbers in range
	cursor, err := blocksColl.Find(ctx, bson.M{"_id": bson.M{"$gte": 1, "$lte": expectedMaxBlock}}, options.Find().SetSort(bson.M{"_id": 1}).SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return []validationError{{BlockNum: 0, Type: "query", Message: fmt.Sprintf("Failed to query blocks: %v", err)}}, stats
	}
	defer cursor.Close(ctx)

	var blockNums []uint32
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		var blockNum uint32
		if id, ok := doc["_id"].(int32); ok {
			blockNum = uint32(id)
		} else if id, ok := doc["_id"].(int64); ok {
			blockNum = uint32(id)
		} else if id, ok := doc["_id"].(float64); ok {
			blockNum = uint32(id)
		} else {
			continue
		}
		// Sample blocks based on sample rate
		if sampleRate <= 1 || int(blockNum)%sampleRate == 0 {
			blockNums = append(blockNums, blockNum)
		}
	}

	totalBlocks := len(blockNums)
	if totalBlocks == 0 {
		return errors, stats
	}

	fmt.Printf("Validating %d blocks (sampled from %d total blocks)...\n", totalBlocks, int(expectedMaxBlock))
	fmt.Println()

	// Progress tracking
	var checked int64
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit concurrent RPC calls

	// Create a context for progress reporter
	progressCtx, progressCancel := context.WithCancel(context.Background())
	defer progressCancel()

	// Progress reporter - show initial progress immediately
	fmt.Printf("%sProgress: 0/%d (0.0%%) checked, 0 errors found%s", colorBlue, totalBlocks, colorReset)
	os.Stdout.Sync()

	// Start progress reporter goroutine
	progressTicker := time.NewTicker(500 * time.Millisecond) // Update every 500ms for smoother progress
	defer progressTicker.Stop()
	go func() {
		for {
			select {
			case <-progressTicker.C:
				checkedVal := atomic.LoadInt64(&checked)
				errorsVal := atomic.LoadInt64(&stats.TotalErrors)
				percent := float64(checkedVal) * 100 / float64(totalBlocks)
				fmt.Printf("\r%sProgress: %d/%d (%.1f%%) checked, %d errors found%s", colorBlue, checkedVal, totalBlocks, percent, errorsVal, colorReset)
				os.Stdout.Sync()
			case <-progressCtx.Done():
				return
			}
		}
	}()

	// Validate each block
	for _, blockNum := range blockNums {
		wg.Add(1)
		go func(bn uint32) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Get block from MongoDB
			var mongoBlock model.Block
			err := blocksColl.FindOne(ctx, bson.M{"_id": bn}).Decode(&mongoBlock)
			if err != nil {
				errorsMu.Lock()
				errors = append(errors, validationError{
					BlockNum: bn,
					Type:     "mongo",
					Message:  fmt.Sprintf("Failed to get block from MongoDB: %v", err),
				})
				errorsMu.Unlock()
				atomic.AddInt64(&stats.TotalErrors, 1)
				atomic.AddInt64(&checked, 1)
				return
			}

			// Get block from RPC
			rpcBlock, err := rpcClient.GetBlock(ctx, bn)
			if err != nil {
				errorsMu.Lock()
				errors = append(errors, validationError{
					BlockNum: bn,
					Type:     "rpc",
					Message:  fmt.Sprintf("Failed to get block from RPC: %v", err),
				})
				errorsMu.Unlock()
				atomic.AddInt64(&stats.TotalErrors, 1)
				atomic.AddInt64(&checked, 1)
				return
			}

			// Validate block ID
			if mongoBlock.BlockID != rpcBlock.BlockId {
				errorsMu.Lock()
				errors = append(errors, validationError{
					BlockNum: bn,
					Type:     "block_id",
					Message:  fmt.Sprintf("Block ID mismatch: MongoDB=%s, RPC=%s", mongoBlock.BlockID, rpcBlock.BlockId),
				})
				errorsMu.Unlock()
				atomic.AddInt64(&stats.TotalErrors, 1)
				atomic.AddInt64(&stats.BlockIDErrors, 1)
			}

			// Validate timestamp (allow 1 second difference for rounding)
			mongoTime := mongoBlock.Timestamp.Unix()
			var rpcTime int64
			if rpcBlock.Timestamp != nil && rpcBlock.Timestamp.Time != nil {
				rpcTime = rpcBlock.Timestamp.Time.Unix()
			} else {
				errorsMu.Lock()
				errors = append(errors, validationError{
					BlockNum: bn,
					Type:     "timestamp",
					Message:  "RPC block timestamp is nil",
				})
				errorsMu.Unlock()
				atomic.AddInt64(&stats.TotalErrors, 1)
				atomic.AddInt64(&stats.TimestampErrors, 1)
				atomic.AddInt64(&checked, 1)
				return
			}
			if mongoTime != rpcTime && mongoTime != rpcTime+1 && mongoTime != rpcTime-1 {
				errorsMu.Lock()
				errors = append(errors, validationError{
					BlockNum: bn,
					Type:     "timestamp",
					Message:  fmt.Sprintf("Timestamp mismatch: MongoDB=%d, RPC=%d", mongoTime, rpcTime),
				})
				errorsMu.Unlock()
				atomic.AddInt64(&stats.TotalErrors, 1)
				atomic.AddInt64(&stats.TimestampErrors, 1)
			}

			// Validate operations count
			// Get operations from MongoDB
			mongoOpsCount, err := operationsColl.CountDocuments(ctx, bson.M{"block_num": bn})
			if err != nil {
				errorsMu.Lock()
				errors = append(errors, validationError{
					BlockNum: bn,
					Type:     "ops_count",
					Message:  fmt.Sprintf("Failed to count operations in MongoDB: %v", err),
				})
				errorsMu.Unlock()
				atomic.AddInt64(&stats.TotalErrors, 1)
				atomic.AddInt64(&stats.OpsCountErrors, 1)
			} else {
				// Get operations from RPC
				rpcOps, err := rpcClient.GetOpsInBlock(ctx, bn, false)
				if err != nil {
					errorsMu.Lock()
					errors = append(errors, validationError{
						BlockNum: bn,
						Type:     "ops_count",
						Message:  fmt.Sprintf("Failed to get operations from RPC: %v", err),
					})
					errorsMu.Unlock()
					atomic.AddInt64(&stats.TotalErrors, 1)
					atomic.AddInt64(&stats.OpsCountErrors, 1)
				} else {
					rpcOpsCount := int64(len(rpcOps))
					if mongoOpsCount != rpcOpsCount {
						errorsMu.Lock()
						errors = append(errors, validationError{
							BlockNum: bn,
							Type:     "ops_count",
							Message:  fmt.Sprintf("Operations count mismatch: MongoDB=%d, RPC=%d", mongoOpsCount, rpcOpsCount),
						})
						errorsMu.Unlock()
						atomic.AddInt64(&stats.TotalErrors, 1)
						atomic.AddInt64(&stats.OpsCountErrors, 1)
					}
				}
			}

			atomic.AddInt64(&checked, 1)
		}(blockNum)
	}

	wg.Wait()

	// Stop progress ticker and show final status
	progressCancel() // Stop progress reporter
	progressTicker.Stop()
	finalChecked := atomic.LoadInt64(&checked)
	finalErrors := atomic.LoadInt64(&stats.TotalErrors)
	percent := float64(finalChecked) * 100 / float64(totalBlocks)
	fmt.Printf("\r%sProgress: %d/%d (%.1f%%) checked, %d errors found%s\n", colorBlue, finalChecked, totalBlocks, percent, finalErrors, colorReset)
	os.Stdout.Sync()

	stats.TotalChecked = finalChecked
	return errors, stats
}

func formatRanges(nums []uint32) string {
	if len(nums) == 0 {
		return ""
	}

	// Sort
	for i := 0; i < len(nums)-1; i++ {
		for j := i + 1; j < len(nums); j++ {
			if nums[i] > nums[j] {
				nums[i], nums[j] = nums[j], nums[i]
			}
		}
	}

	var out []string
	s := nums[0]
	e := nums[0]
	for i := 1; i < len(nums); i++ {
		n := nums[i]
		if n == e+1 {
			e = n
			continue
		}
		if s == e {
			out = append(out, fmt.Sprintf("%d", s))
		} else {
			out = append(out, fmt.Sprintf("%d-%d", s, e))
		}
		s = n
		e = n
	}
	if s == e {
		out = append(out, fmt.Sprintf("%d", s))
	} else {
		out = append(out, fmt.Sprintf("%d-%d", s, e))
	}
	return strings.Join(out, ",")
}

func formatHistogram(hist []blockHistogram) string {
	var parts []string
	for _, h := range hist {
		parts = append(parts, fmt.Sprintf(`{"block_num":%d,"ops":%d}`, h.BlockNum, h.Ops))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func loadEnvFile(path string) map[string]string {
	env := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return env
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remove quotes if present
			if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
				value = value[1 : len(value)-1]
			}
			env[key] = value
		}
	}
	return env
}

func getEnv(env map[string]string, key, defaultValue string) string {
	if val, ok := env[key]; ok {
		return val
	}
	return defaultValue
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%sERROR:%s %s\n", colorRed, colorReset, fmt.Sprintf(format, args...))
	os.Exit(1)
}
