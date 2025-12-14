package blockchain

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"

	"github.com/steemdb/sync/internal/utils"
	"github.com/steemdb/sync/pkg/steem"
)

// MockLogger for testing
type MockLogger struct{}

func (m *MockLogger) Debug(msg string, fields ...zap.Field) {}
func (m *MockLogger) Info(msg string, fields ...zap.Field)  {}
func (m *MockLogger) Warn(msg string, fields ...zap.Field)  {}
func (m *MockLogger) Error(msg string, fields ...zap.Field) {}
func (m *MockLogger) Fatal(msg string, fields ...zap.Field) {}
func (m *MockLogger) With(fields ...zap.Field) utils.Logger { return m }
func (m *MockLogger) Sync() error                           { return nil }

// MockDatabase for testing
type MockDatabase struct{}

func (m *MockDatabase) Collection(name string) Collection {
	return &MockCollection{}
}

func (m *MockDatabase) MarkAccountNeedsUpdate(ctx context.Context, accountName string) error {
	return nil
}

type MockCollection struct{}

func (m *MockCollection) InsertOne(ctx context.Context, document interface{}) (*mongo.InsertOneResult, error) {
	return &mongo.InsertOneResult{InsertedID: nil}, nil
}

func (m *MockCollection) InsertMany(ctx context.Context, documents []interface{}) (*mongo.InsertManyResult, error) {
	insertedIDs := make([]interface{}, len(documents))
	for i := range documents {
		insertedIDs[i] = i
	}
	return &mongo.InsertManyResult{InsertedIDs: insertedIDs}, nil
}

func (m *MockCollection) UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	return &mongo.UpdateResult{MatchedCount: 1, ModifiedCount: 1}, nil
}

func (m *MockCollection) FindOne(ctx context.Context, filter interface{}) *mongo.SingleResult {
	return mongo.NewSingleResultFromDocument(nil, nil, nil)
}

type MockSingleResult struct{}

func (m *MockSingleResult) Decode(v interface{}) error {
	return nil
}

func TestOperationProcessor_Process(t *testing.T) {
	// Create mock dependencies
	mockDB := &MockDatabase{}
	mockLogger := &MockLogger{}

	// Create processor using NewOperationProcessor to initialize handlers
	// Note: We can't use NewOperationProcessor directly as it requires *database.MongoDB
	// So we'll create it manually and register handlers
	processor := &OperationProcessor{
		db:       mockDB,
		logger:   mockLogger,
		handlers: make(map[string]OperationHandler),
	}

	// Register a simple handler for testing
	processor.handlers["test_op"] = func(ctx context.Context, op *Operation) error {
		return nil
	}

	// Create test operation with TrxID and OpInTrx
	testOp := &Operation{
		Block: &steem.Block{
			Number:    12345,
			Timestamp: time.Now(),
			BlockID:   "test_block_id",
		},
		Operation: &steem.Operation{
			TrxID:   "test_trx_id",
			Block:   12345,
			OpInTrx: 0,
			Op: []interface{}{
				"test_op",
				map[string]interface{}{
					"test_field": "test_value",
					"account":    "testaccount",
				},
			},
			Timestamp: time.Now(),
		},
	}

	// Test processing
	err := processor.Process(testOp)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Test unknown operation type
	unknownOp := &Operation{
		Block: &steem.Block{
			Number:    12346,
			Timestamp: time.Now(),
			BlockID:   "test_block_id_2",
		},
		Operation: &steem.Operation{
			TrxID:   "test_trx_id_2",
			Block:   12346,
			OpInTrx: 0,
			Op: []interface{}{
				"unknown_op",
				map[string]interface{}{
					"account": "testaccount",
				},
			},
			Timestamp: time.Now(),
		},
	}

	err = processor.Process(unknownOp)
	if err != nil {
		t.Errorf("Unknown operation should not cause error, got: %v", err)
	}
}

func TestGetString(t *testing.T) {
	data := map[string]interface{}{
		"string_field": "test_value",
		"int_field":    123,
		"nil_field":    nil,
	}

	// Test valid string
	result := getString(data, "string_field")
	if result != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", result)
	}

	// Test non-string field
	result = getString(data, "int_field")
	if result != "" {
		t.Errorf("Expected empty string for non-string field, got '%s'", result)
	}

	// Test missing field
	result = getString(data, "missing_field")
	if result != "" {
		t.Errorf("Expected empty string for missing field, got '%s'", result)
	}
}

func TestGetFloat64(t *testing.T) {
	data := map[string]interface{}{
		"float_field":    123.456,
		"int_field":      123,
		"string_field":   "456.789",
		"invalid_string": "not_a_number",
	}

	// Test float64
	result := getFloat64(data, "float_field")
	if result != 123.456 {
		t.Errorf("Expected 123.456, got %f", result)
	}

	// Test int
	result = getFloat64(data, "int_field")
	if result != 123.0 {
		t.Errorf("Expected 123.0, got %f", result)
	}

	// Test valid string
	result = getFloat64(data, "string_field")
	if result != 456.789 {
		t.Errorf("Expected 456.789, got %f", result)
	}

	// Test invalid string
	result = getFloat64(data, "invalid_string")
	if result != 0 {
		t.Errorf("Expected 0 for invalid string, got %f", result)
	}
}

func BenchmarkOperationProcessor_Process(b *testing.B) {
	// Create mock dependencies
	mockDB := &MockDatabase{}
	mockLogger := &MockLogger{}

	processor := &OperationProcessor{
		db:       mockDB,
		logger:   mockLogger,
		handlers: make(map[string]OperationHandler),
	}

	processor.handlers["vote"] = func(ctx context.Context, op *Operation) error {
		return nil
	}

	testOp := &Operation{
		Block: &steem.Block{
			Number:    12345,
			Timestamp: time.Now(),
			BlockID:   "test_block_id",
		},
		Operation: &steem.Operation{
			TrxID:   "test_trx_id",
			Block:   12345,
			OpInTrx: 0,
			Op: []interface{}{
				"vote",
				map[string]interface{}{
					"voter":    "testuser",
					"author":   "testauthor",
					"permlink": "testpost",
					"weight":   10000.0,
				},
			},
			Timestamp: time.Now(),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.Process(testOp)
	}
}

// TestExtractAccounts tests the extractAccounts function
func TestExtractAccounts(t *testing.T) {
	processor := &OperationProcessor{}

	// Test transfer operation
	transferData := map[string]interface{}{
		"from": "alice",
		"to":   "bob",
	}
	accounts := processor.extractAccounts("transfer", transferData)
	if len(accounts) != 2 || accounts[0] != "alice" || accounts[1] != "bob" {
		t.Errorf("Expected [alice, bob], got %v", accounts)
	}

	// Test vote operation
	voteData := map[string]interface{}{
		"voter":  "alice",
		"author": "bob",
	}
	accounts = processor.extractAccounts("vote", voteData)
	if len(accounts) != 2 || accounts[0] != "alice" || accounts[1] != "bob" {
		t.Errorf("Expected [alice, bob], got %v", accounts)
	}

	// Test comment operation
	commentData := map[string]interface{}{
		"author": "alice",
	}
	accounts = processor.extractAccounts("comment", commentData)
	if len(accounts) != 1 || accounts[0] != "alice" {
		t.Errorf("Expected [alice], got %v", accounts)
	}

	// Test unknown operation type
	unknownData := map[string]interface{}{}
	accounts = processor.extractAccounts("unknown", unknownData)
	if len(accounts) != 0 {
		t.Errorf("Expected empty accounts, got %v", accounts)
	}
}

// TestCreateOperationSummary tests the createOperationSummary function
func TestCreateOperationSummary(t *testing.T) {
	processor := &OperationProcessor{}

	// Test transfer summary
	transferData := map[string]interface{}{
		"from":   "alice",
		"to":     "bob",
		"amount": "10.000 STEEM",
	}
	summary := processor.createOperationSummary("transfer", transferData)
	if summary["from"] != "alice" || summary["to"] != "bob" || summary["amount"] != "10.000 STEEM" {
		t.Errorf("Invalid transfer summary: %v", summary)
	}

	// Test vote summary
	voteData := map[string]interface{}{
		"voter":    "alice",
		"author":   "bob",
		"permlink": "test-post",
		"weight":   10000.0,
	}
	summary = processor.createOperationSummary("vote", voteData)
	if summary["voter"] != "alice" || summary["author"] != "bob" {
		t.Errorf("Invalid vote summary: %v", summary)
	}
}
