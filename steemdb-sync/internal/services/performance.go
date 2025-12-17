package services

import (
	"context"
	"fmt"
	"time"

	"github.com/steemit/steemdb/sync/internal/database"
	"github.com/steemit/steemdb/sync/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PerformanceTest runs performance tests for queries and operations
type PerformanceTest struct {
	db     *database.MongoDB
	logger utils.Logger
}

// NewPerformanceTest creates a new performance test instance
func NewPerformanceTest(db *database.MongoDB, logger utils.Logger) *PerformanceTest {
	return &PerformanceTest{
		db:     db,
		logger: logger,
	}
}

// TestResult holds the result of a performance test
type TestResult struct {
	TestName    string
	Duration    time.Duration
	Target      time.Duration
	Passed      bool
	Message     string
	RecordCount int64
}

// RunAllTests runs all performance tests
func (pt *PerformanceTest) RunAllTests(ctx context.Context) []TestResult {
	var results []TestResult

	results = append(results, pt.TestBlockQueryByNumber(ctx))
	results = append(results, pt.TestBlockQueryLatest(ctx))
	results = append(results, pt.TestAccountQueryByName(ctx))
	results = append(results, pt.TestAccountOperationsQuery(ctx))
	results = append(results, pt.TestOperationsByType(ctx))
	results = append(results, pt.TestCommentsByAuthor(ctx))

	return results
}

// TestBlockQueryByNumber tests querying a block by number
func (pt *PerformanceTest) TestBlockQueryByNumber(ctx context.Context) TestResult {
	target := 10 * time.Millisecond
	testName := "Block Query (by number)"

	start := time.Now()
	collection := pt.db.Collection("blocks")

	var block database.Block
	err := collection.FindOne(ctx, bson.D{{Key: "number", Value: 1}}).Decode(&block)
	duration := time.Since(start)

	passed := duration < target
	message := fmt.Sprintf("%v (target: < %v)", duration, target)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			message = "No blocks found in database"
			passed = false
		} else {
			message = fmt.Sprintf("Error: %v", err)
			passed = false
		}
	}

	return TestResult{
		TestName: testName,
		Duration: duration,
		Target:   target,
		Passed:   passed,
		Message:  message,
	}
}

// TestBlockQueryLatest tests querying latest blocks
func (pt *PerformanceTest) TestBlockQueryLatest(ctx context.Context) TestResult {
	target := 50 * time.Millisecond
	testName := "Latest Blocks Query"

	start := time.Now()
	collection := pt.db.Collection("blocks")

	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}}).SetLimit(20)
	cursor, err := collection.Find(ctx, bson.D{}, opts)
	duration := time.Since(start)

	var count int64
	if err == nil {
		count, _ = collection.CountDocuments(ctx, bson.D{})
		cursor.Close(ctx)
	}

	passed := duration < target
	message := fmt.Sprintf("%v (target: < %v)", duration, target)
	if err != nil {
		message = fmt.Sprintf("Error: %v", err)
		passed = false
	} else if count == 0 {
		message = "No blocks found in database"
		passed = false
	}

	return TestResult{
		TestName:    testName,
		Duration:    duration,
		Target:      target,
		Passed:      passed,
		Message:     message,
		RecordCount: count,
	}
}

// TestAccountQueryByName tests querying an account by name
func (pt *PerformanceTest) TestAccountQueryByName(ctx context.Context) TestResult {
	target := 50 * time.Millisecond
	testName := "Account Query (by name)"

	// First, find a sample account
	collection := pt.db.Collection("accounts")
	var sampleAccount database.Account
	err := collection.FindOne(ctx, bson.D{}).Decode(&sampleAccount)
	if err != nil {
		return TestResult{
			TestName: testName,
			Target:   target,
			Passed:   false,
			Message:  "No accounts found in database",
		}
	}

	start := time.Now()
	var account database.Account
	err = collection.FindOne(ctx, bson.D{{Key: "name", Value: sampleAccount.Name}}).Decode(&account)
	duration := time.Since(start)

	passed := duration < target
	message := fmt.Sprintf("%v (target: < %v)", duration, target)
	if err != nil {
		message = fmt.Sprintf("Error: %v", err)
		passed = false
	}

	return TestResult{
		TestName: testName,
		Duration: duration,
		Target:   target,
		Passed:   passed,
		Message:  message,
	}
}

// TestAccountOperationsQuery tests querying account operations
func (pt *PerformanceTest) TestAccountOperationsQuery(ctx context.Context) TestResult {
	target := 100 * time.Millisecond
	testName := "Account Operations Query"

	// First, find a sample account
	collection := pt.db.Collection("account_operations")
	var sampleOp database.AccountOperation
	err := collection.FindOne(ctx, bson.D{}).Decode(&sampleOp)
	if err != nil {
		return TestResult{
			TestName: testName,
			Target:   target,
			Passed:   false,
			Message:  "No account operations found in database",
		}
	}

	start := time.Now()
	opts := options.Find().
		SetSort(bson.D{{Key: "block_time", Value: -1}}).
		SetLimit(100)

	cursor, err := collection.Find(ctx, bson.D{{Key: "account", Value: sampleOp.Account}}, opts)
	duration := time.Since(start)

	var count int64
	if err == nil {
		var ops []database.AccountOperation
		cursor.All(ctx, &ops)
		count = int64(len(ops))
		cursor.Close(ctx)
	}

	passed := duration < target
	message := fmt.Sprintf("%v (target: < %v)", duration, target)
	if err != nil {
		message = fmt.Sprintf("Error: %v", err)
		passed = false
	}

	return TestResult{
		TestName:    testName,
		Duration:    duration,
		Target:      target,
		Passed:      passed,
		Message:     message,
		RecordCount: count,
	}
}

// TestOperationsByType tests querying operations by type
func (pt *PerformanceTest) TestOperationsByType(ctx context.Context) TestResult {
	target := 100 * time.Millisecond
	testName := "Operations Query (by type)"

	start := time.Now()
	collection := pt.db.Collection("operations")

	opts := options.Find().
		SetSort(bson.D{{Key: "block_time", Value: -1}}).
		SetLimit(100)

	cursor, err := collection.Find(ctx, bson.D{{Key: "op_type", Value: "transfer"}}, opts)
	duration := time.Since(start)

	var count int64
	if err == nil {
		var ops []database.Operation
		cursor.All(ctx, &ops)
		count = int64(len(ops))
		cursor.Close(ctx)
	}

	passed := duration < target
	message := fmt.Sprintf("%v (target: < %v)", duration, target)
	if err != nil {
		message = fmt.Sprintf("Error: %v", err)
		passed = false
	} else if count == 0 {
		message = "No operations found in database"
		passed = false
	}

	return TestResult{
		TestName:    testName,
		Duration:    duration,
		Target:      target,
		Passed:      passed,
		Message:     message,
		RecordCount: count,
	}
}

// TestCommentsByAuthor tests querying comments by author
func (pt *PerformanceTest) TestCommentsByAuthor(ctx context.Context) TestResult {
	target := 100 * time.Millisecond
	testName := "Comments Query (by author)"

	// First, find a sample author
	collection := pt.db.Collection("comments")
	var sampleComment database.Comment
	err := collection.FindOne(ctx, bson.D{}).Decode(&sampleComment)
	if err != nil {
		return TestResult{
			TestName: testName,
			Target:   target,
			Passed:   false,
			Message:  "No comments found in database",
		}
	}

	start := time.Now()
	opts := options.Find().
		SetSort(bson.D{{Key: "created", Value: -1}}).
		SetLimit(100)

	cursor, err := collection.Find(ctx, bson.D{{Key: "author", Value: sampleComment.Author}}, opts)
	duration := time.Since(start)

	var count int64
	if err == nil {
		var comments []database.Comment
		cursor.All(ctx, &comments)
		count = int64(len(comments))
		cursor.Close(ctx)
	}

	passed := duration < target
	message := fmt.Sprintf("%v (target: < %v)", duration, target)
	if err != nil {
		message = fmt.Sprintf("Error: %v", err)
		passed = false
	}

	return TestResult{
		TestName:    testName,
		Duration:    duration,
		Target:      target,
		Passed:      passed,
		Message:     message,
		RecordCount: count,
	}
}

// PrintResults prints test results in a formatted way
func (pt *PerformanceTest) PrintResults(results []TestResult) {
	pt.logger.Info("Performance Test Results")
	pt.logger.Info("==========================================")

	passed := 0
	failed := 0
	warnings := 0

	for _, result := range results {
		if result.Passed {
			pt.logger.Info("✓ "+result.TestName,
				utils.String("duration", result.Duration.String()),
				utils.String("target", result.Target.String()),
				utils.String("status", "PASS"),
			)
			passed++
		} else {
			if result.RecordCount == 0 {
				pt.logger.Warn("⚠ "+result.TestName,
					utils.String("message", result.Message),
					utils.String("status", "WARN"),
				)
				warnings++
			} else {
				pt.logger.Error("✗ "+result.TestName,
					utils.String("duration", result.Duration.String()),
					utils.String("target", result.Target.String()),
					utils.String("status", "FAIL"),
				)
				failed++
			}
		}
	}

	pt.logger.Info("==========================================")
	pt.logger.Info("Summary",
		utils.Int("passed", passed),
		utils.Int("failed", failed),
		utils.Int("warnings", warnings),
	)
}
