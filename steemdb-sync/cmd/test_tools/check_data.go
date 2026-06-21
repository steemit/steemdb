package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
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
	protocol "github.com/steemit/steemutil/protocol"
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
	OrphanCheckSampled bool // true when orphan check used sampling (no full $lookup)
	OrphanSampleSize   int  // number of distinct block_nums checked in sample
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
	BlockNum             uint32
	Type                 string // "block_id", "timestamp", "ops_count", etc.
	Message              string
	MongoOpsCount        *int64                      // set for ops_count mismatch: MongoDB count
	RPCOpsCount          *int64                      // set for ops_count mismatch: RPC count
	MongoOps             []bson.M                    // set for ops_count mismatch: raw MongoDB docs for printing
	RPCOps               []*protocol.OperationObject // set for ops_count mismatch: RPC operations
	RPCUnmarshalErrorRaw string                      // set when RPC get_ops_in_block response failed to unmarshal (raw JSON for steemutil debugging)
	MongoLoadError       string                      // set when MongoDB operations could not be loaded for printing (e.g. Find/Decode error)
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

	// Get RPC validation settings from .env (override command line defaults if set)
	if envRPC := getEnv(env, "RPC_API", ""); envRPC != "" {
		*apiEndpoint = envRPC
	}
	if envValidate := getEnv(env, "VALIDATE_RPC", ""); envValidate != "" {
		if envValidate == "true" || envValidate == "1" {
			*validateRPC = true
		}
	}
	if envSample := getEnv(env, "RPC_SAMPLE_RATE", ""); envSample != "" {
		if sample, err := strconv.Atoi(envSample); err == nil && sample > 0 {
			*sampleRate = sample
		}
	}

	mongoURI := fmt.Sprintf("mongodb://%s:%s@%s:%s/%s?authSource=admin",
		mongoUsername, mongoPassword, mongoHost, mongoPort, mongoDatabase)

	stopReplayAtBlockStr := getEnv(env, "STOP_REPLAY_AT_BLOCK", "0")
	stopReplayAtBlock, _ := strconv.ParseUint(stopReplayAtBlockStr, 10, 32)

	fmt.Printf("%s=== Checking Cold Ingest Mongo Data (blocks + operations) ===%s\n\n", colorGreen, colorReset)

	// Connect to MongoDB with longer timeout for large datasets
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
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
		if result.OrphanCheckSampled {
			fmt.Printf("%s✗ Orphan block_nums detected in sample (ops reference missing blocks)%s\n", colorRed, colorReset)
		} else {
			fmt.Printf("%s✗ Orphan operations detected (ops referencing missing blocks)%s\n", colorRed, colorReset)
		}
		issues = 1
	} else {
		if result.OrphanCheckSampled {
			fmt.Printf("%s✓ No orphan operations in sample (%d block_nums checked)%s\n", colorGreen, result.OrphanSampleSize, colorReset)
		} else {
			fmt.Printf("%s✓ No orphan operations%s\n", colorGreen, colorReset)
		}
	}

	// RPC validation (optional)
	if *validateRPC {
		fmt.Printf("\n%sStarting RPC API validation...%s\n", colorBlue, colorReset)
		fmt.Printf("API endpoint: %s\n", *apiEndpoint)
		fmt.Printf("Sample rate: 1/%d blocks\n\n", *sampleRate)

		rpcClient := rpc.NewClient(*apiEndpoint, 3, 30*time.Second)
		validationErrors, stats := validateWithRPC(ctx, rpcClient, *apiEndpoint, blocksColl, operationsColl, expectedMaxBlock, *sampleRate)
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
				// Print raw RPC response when present (unmarshal/unparseable errors)
				if err.RPCUnmarshalErrorRaw != "" {
					fmt.Printf("    RPC raw response (unparseable by steemutil):\n")
					var pretty bytes.Buffer
					if json.Indent(&pretty, []byte(err.RPCUnmarshalErrorRaw), "      ", "  ") == nil {
						fmt.Printf("      %s\n", pretty.String())
					} else {
						fmt.Printf("      %s\n", err.RPCUnmarshalErrorRaw)
					}
				}
				if err.Type == "ops_count" && err.MongoOpsCount != nil && err.RPCOpsCount != nil {
					fmt.Printf("    MongoDB operations: %d, RPC operations: %d\n", *err.MongoOpsCount, *err.RPCOpsCount)
					// Print MongoDB operations (raw docs so structure always matches DB)
					if len(err.MongoOps) > 0 {
						fmt.Printf("    MongoDB operations details:\n")
						for j, doc := range err.MongoOps {
							if j >= 20 { // Limit to first 20 for readability
								fmt.Printf("      ... and %d more MongoDB operations\n", len(err.MongoOps)-20)
								break
							}
							opJSON, _ := json.MarshalIndent(doc, "      ", "  ")
							fmt.Printf("      [%d] %s\n", j+1, string(opJSON))
						}
					} else if *err.MongoOpsCount > 0 {
						fmt.Printf("    MongoDB operations details: (failed to load %d documents for printing)\n", *err.MongoOpsCount)
						if err.MongoLoadError != "" {
							fmt.Printf("    MongoDB load error: %s\n", err.MongoLoadError)
						}
					}
					// Print RPC operations
					if len(err.RPCOps) > 0 {
						fmt.Printf("    RPC operations details:\n")
						for j, op := range err.RPCOps {
							if j >= 20 { // Limit to first 20 for readability
								fmt.Printf("      ... and %d more RPC operations\n", len(err.RPCOps)-20)
								break
							}
							opJSON, _ := json.MarshalIndent(op, "      ", "  ")
							fmt.Printf("      [%d] %s\n", j+1, string(opJSON))
						}
					}
				}
			}
			if len(validationErrors) > 10 {
				fmt.Printf("  ... and %d more errors\n", len(validationErrors)-10)
			}
			criticalRPCErrors := 0
			for _, err := range validationErrors {
				if err.Type != "ops_count" {
					criticalRPCErrors++
				}
			}
			if criticalRPCErrors > 0 {
				issues = 1
			}
			if stats.OpsCountErrors > 0 && criticalRPCErrors == 0 {
				fmt.Printf("%s  (ops_count mismatches are warnings only; RPC API may differ from local ingest)%s\n", colorYellow, colorReset)
			}
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
	if result.OrphanCheckSampled {
		fmt.Printf("orphan_check_sampled=true\n")
		fmt.Printf("orphan_sample_size=%d\n", result.OrphanSampleSize)
	}
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
	if result.OrphanCheckSampled {
		fmt.Printf("  orphan_check:        sample of %d block_nums (not full scan)\n", result.OrphanSampleSize)
	}
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

	// Orphan check: sample-based (avoids full $lookup which times out on large collections).
	// Sample random operations, get distinct block_num, then verify each exists in blocks.
	fmt.Printf("  Checking for orphan operations (sample)...")
	os.Stdout.Sync()
	const orphanSampleSize = 20000
	samplePipeline := []bson.M{
		{"$sample": bson.M{"size": orphanSampleSize}},
		{"$group": bson.M{"_id": "$block_num"}},
	}
	orphanCtx, orphanCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer orphanCancel()
	cursor, err = operationsColl.Aggregate(orphanCtx, samplePipeline)
	if err != nil {
		return nil, errors.Wrap(err, "failed to sample operations for orphan check")
	}
	defer cursor.Close(orphanCtx)

	var sampledBlockNums []uint32
	for cursor.Next(orphanCtx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		var bn uint32
		if id, ok := doc["_id"].(int32); ok {
			bn = uint32(id)
		} else if id, ok := doc["_id"].(int64); ok {
			bn = uint32(id)
		} else if id, ok := doc["_id"].(float64); ok {
			bn = uint32(id)
		} else {
			continue
		}
		sampledBlockNums = append(sampledBlockNums, bn)
	}
	if err := cursor.Err(); err != nil {
		return nil, errors.Wrap(err, "cursor error during orphan sample")
	}

	result.OrphanCheckSampled = true
	result.OrphanSampleSize = len(sampledBlockNums)

	// Batch-check which block_nums exist in blocks (avoids N single-doc lookups).
	// Use int64 for _id in query: BSON may store block _id as int64 (Go uint32 often encodes as long).
	const batchSize = 500
	for i := 0; i < len(sampledBlockNums); i += batchSize {
		end := i + batchSize
		if end > len(sampledBlockNums) {
			end = len(sampledBlockNums)
		}
		batch := sampledBlockNums[i:end]
		ids := make([]interface{}, len(batch))
		for j, bn := range batch {
			ids[j] = int64(bn)
		}
		cur, err := blocksColl.Find(orphanCtx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"_id": 1}))
		if err != nil {
			return nil, errors.Wrap(err, "failed to check blocks for orphan sample")
		}
		existingIDs := make(map[int64]bool)
		for cur.Next(orphanCtx) {
			var doc bson.M
			if err := cur.Decode(&doc); err != nil {
				continue
			}
			var id int64
			switch v := doc["_id"].(type) {
			case int32:
				id = int64(v)
			case int64:
				id = v
			case float64:
				id = int64(v)
			default:
				continue
			}
			existingIDs[id] = true
		}
		cur.Close(orphanCtx)
		for _, bn := range batch {
			if !existingIDs[int64(bn)] {
				result.OrphanOpsCount++
				if len(result.OrphanOpsSample) < 10 {
					result.OrphanOpsSample = append(result.OrphanOpsSample, fmt.Sprintf("block:%d", bn))
				}
			}
		}
	}

	if result.OrphanOpsCount > 0 {
		fmt.Printf(" done (%d orphan block_nums in sample of %d)\n", result.OrphanOpsCount, result.OrphanSampleSize)
	} else {
		fmt.Printf(" done (no orphans in sample of %d block_nums)\n", result.OrphanSampleSize)
	}

	// Count distinct blocks with operations (use dedicated timeout for large collections)
	// Use aggregation instead of distinct to avoid 16MB limit
	fmt.Printf("  Counting distinct blocks with operations...")
	os.Stdout.Sync()
	distinctPipeline := []bson.M{
		{"$match": bson.M{"block_num": bson.M{"$gte": 1, "$lte": expectedMaxBlock}}},
		{"$group": bson.M{"_id": "$block_num"}},
		{"$group": bson.M{"_id": nil, "count": bson.M{"$sum": 1}}},
	}
	distinctCtx, distinctCancel := context.WithTimeout(ctx, 45*time.Minute)
	defer distinctCancel()
	distinctCursor, err := operationsColl.Aggregate(distinctCtx, distinctPipeline)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count distinct blocks with operations")
	}
	defer distinctCursor.Close(distinctCtx)

	result.BlocksWithOpsCount = 0
	if distinctCursor.Next(distinctCtx) {
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

// fetchRPCGetOpsInBlockRaw performs a raw HTTP POST to condenser_api.get_ops_in_block and returns the response body (for debugging unmarshal failures).
func fetchRPCGetOpsInBlockRaw(ctx context.Context, apiEndpoint string, blockNum uint32) (string, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "condenser_api.get_ops_in_block",
		"params":  []interface{}{blockNum, false},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validateWithRPC(ctx context.Context, rpcClient *rpc.Client, apiEndpoint string, blocksColl, operationsColl *mongo.Collection, expectedMaxBlock uint32, sampleRate int) ([]validationError, *validationStats) {
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
				// Get operations from RPC with retry for unmarshal errors
				// Retry up to 3 times for unmarshal errors (which indicate RPC API limitations)
				var rpcOps []*protocol.OperationObject
				var rpcErr error
				retryCount := 0
				maxRetries := 3

				for retryCount < maxRetries {
					rpcOps, rpcErr = rpcClient.GetOpsInBlock(ctx, bn, false)
					if rpcErr == nil {
						break
					}

					// Check if this is an unmarshal error (RPC API limitation)
					errStr := rpcErr.Error()
					if strings.Contains(errStr, "failed to unmarshal") && strings.Contains(errStr, "value out of range") {
						retryCount++
						if retryCount < maxRetries {
							// Retry after a short delay (unmarshal errors might be transient)
							time.Sleep(100 * time.Millisecond)
							continue
						}
						// After max retries, skip this block's ops_count validation
						// This is a known RPC API limitation for historical blocks
						// Fetch raw RPC response so user can inspect unparseable data (e.g. for steemutil fixes)
						var rawResp string
						if raw, err := fetchRPCGetOpsInBlockRaw(ctx, apiEndpoint, bn); err == nil {
							rawResp = raw
						}
						errorsMu.Lock()
						errors = append(errors, validationError{
							BlockNum:             bn,
							Type:                 "ops_count",
							Message:              fmt.Sprintf("Skipped operations count validation due to RPC API unmarshal limitation: %v", rpcErr),
							RPCUnmarshalErrorRaw: rawResp,
						})
						errorsMu.Unlock()
						atomic.AddInt64(&stats.TotalErrors, 1)
						atomic.AddInt64(&stats.OpsCountErrors, 1)
						break
					}

					// For non-unmarshal errors, record the error immediately; also fetch raw response when possible (may be unparseable)
					var rawResp string
					if raw, err := fetchRPCGetOpsInBlockRaw(ctx, apiEndpoint, bn); err == nil {
						rawResp = raw
					}
					errorsMu.Lock()
					errors = append(errors, validationError{
						BlockNum:             bn,
						Type:                 "ops_count",
						Message:              fmt.Sprintf("Failed to get operations from RPC: %v", rpcErr),
						RPCUnmarshalErrorRaw: rawResp,
					})
					errorsMu.Unlock()
					atomic.AddInt64(&stats.TotalErrors, 1)
					atomic.AddInt64(&stats.OpsCountErrors, 1)
					break
				}

				// Only compare counts if we successfully got RPC data
				if rpcErr == nil {
					rpcOpsCount := int64(len(rpcOps))
					if mongoOpsCount != rpcOpsCount {
						// Fetch MongoDB operations for detailed comparison (raw bson.M so decode never fails)
						// Query with $in so we match whether block_num is stored as int32 or int64 in BSON
						var mongoOps []bson.M
						var mongoLoadErr string
						blockNumFilter := bson.M{"block_num": bson.M{"$in": []interface{}{int32(bn), int64(bn)}}}
						mongoCursor, mongoErr := operationsColl.Find(ctx, blockNumFilter, options.Find().SetSort(bson.M{"trx_index": 1, "op_index": 1}))
						if mongoErr != nil {
							mongoLoadErr = mongoErr.Error()
						} else {
							for mongoCursor.Next(ctx) {
								var doc bson.M
								if err := mongoCursor.Decode(&doc); err != nil {
									if mongoLoadErr == "" {
										mongoLoadErr = fmt.Sprintf("Decode error: %v", err)
									}
									continue
								}
								docCopy := make(bson.M, len(doc))
								for k, v := range doc {
									docCopy[k] = v
								}
								mongoOps = append(mongoOps, docCopy)
							}
							mongoCursor.Close(ctx)
							if len(mongoOps) == 0 && mongoLoadErr == "" && mongoOpsCount > 0 {
								mongoLoadErr = "Find returned no documents (query may not match document types)"
							}
						}
						// Deep copy RPC ops to avoid race conditions
						rpcOpsCopy := make([]*protocol.OperationObject, len(rpcOps))
						for i, op := range rpcOps {
							if op != nil {
								// Create a copy (OperationObject is a struct, so we can copy by value)
								opCopy := *op
								rpcOpsCopy[i] = &opCopy
							}
						}
						mongoCnt, rpcCnt := mongoOpsCount, rpcOpsCount
						errorsMu.Lock()
						errors = append(errors, validationError{
							BlockNum:       bn,
							Type:           "ops_count",
							Message:        fmt.Sprintf("Operations count mismatch: MongoDB=%d, RPC=%d", mongoOpsCount, rpcOpsCount),
							MongoOpsCount:  &mongoCnt,
							RPCOpsCount:    &rpcCnt,
							MongoOps:       mongoOps,
							RPCOps:         rpcOpsCopy,
							MongoLoadError: mongoLoadErr,
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
