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

type MockCollection struct{}

func (m *MockCollection) InsertOne(ctx context.Context, document interface{}) (*mongo.InsertOneResult, error) {
	return &mongo.InsertOneResult{InsertedID: nil}, nil
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

	// Create processor
	processor := &OperationProcessor{
		db:       mockDB,
		logger:   mockLogger,
		handlers: make(map[string]OperationHandler),
	}

	// Register a simple handler for testing
	processor.handlers["test_op"] = func(ctx context.Context, op *Operation) error {
		return nil
	}

	// Create test operation
	testOp := &Operation{
		Block: &steem.Block{
			Number:    12345,
			Timestamp: time.Now(),
		},
		Operation: &steem.Operation{
			Op: []interface{}{
				"test_op",
				map[string]interface{}{
					"test_field": "test_value",
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
		},
		Operation: &steem.Operation{
			Op: []interface{}{
				"unknown_op",
				map[string]interface{}{},
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
		},
		Operation: &steem.Operation{
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
