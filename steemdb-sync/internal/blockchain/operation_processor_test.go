package blockchain

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"

	"github.com/steemdb/sync/internal/utils"
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
		Block: &utils.Block{
			Number:    12345,
			Timestamp: time.Now(),
			BlockID:   "test_block_id",
		},
		Operation: &utils.Operation{
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
		Block: &utils.Block{
			Number:    12346,
			Timestamp: time.Now(),
			BlockID:   "test_block_id_2",
		},
		Operation: &utils.Operation{
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
		Block: &utils.Block{
			Number:    12345,
			Timestamp: time.Now(),
			BlockID:   "test_block_id",
		},
		Operation: &utils.Operation{
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

// TestHandleVestingDeposit tests the handleVestingDeposit function
func TestHandleVestingDeposit(t *testing.T) {
	mockDB := &MockDatabase{}
	mockLogger := &MockLogger{}

	processor := &OperationProcessor{
		db:     mockDB,
		logger: mockLogger,
	}

	ctx := context.Background()
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("successful processing with map data", func(t *testing.T) {
		op := &Operation{
			Block: &utils.Block{
				Number:    330781,
				Timestamp: testTime,
				BlockID:   "test_block_id",
			},
			Operation: &utils.Operation{
				TrxID:      "test_trx_id_1",
				Block:      330781,
				TrxInBlock: 0,
				OpInTrx:    0,
				Timestamp:  testTime,
				Op: []interface{}{
					"transfer_to_vesting",
					map[string]interface{}{
						"from":   "alice",
						"to":     "bob",
						"amount": "100.000 STEEM",
					},
				},
			},
		}

		err := processor.handleVestingDeposit(ctx, op)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("successful processing with struct data (JSON conversion)", func(t *testing.T) {
		// Simulate a struct type that needs JSON conversion
		type VestingOpData struct {
			From   string `json:"from"`
			To     string `json:"to"`
			Amount string `json:"amount"`
		}

		structData := VestingOpData{
			From:   "charlie",
			To:     "david",
			Amount: "50.000 STEEM",
		}

		op := &Operation{
			Block: &utils.Block{
				Number:    330782,
				Timestamp: testTime,
				BlockID:   "test_block_id_2",
			},
			Operation: &utils.Operation{
				TrxID:      "test_trx_id_2",
				Block:      330782,
				TrxInBlock: 0,
				OpInTrx:    1,
				Timestamp:  testTime,
				Op: []interface{}{
					"transfer_to_vesting",
					structData, // Struct instead of map
				},
			},
		}

		err := processor.handleVestingDeposit(ctx, op)
		if err != nil {
			t.Errorf("Expected no error with struct data, got: %v", err)
		}
	})

	t.Run("error: Op array too short", func(t *testing.T) {
		op := &Operation{
			Block: &utils.Block{
				Number:    330783,
				Timestamp: testTime,
				BlockID:   "test_block_id_3",
			},
			Operation: &utils.Operation{
				TrxID:      "test_trx_id_3",
				Block:      330783,
				TrxInBlock: 0,
				OpInTrx:    0,
				Timestamp:  testTime,
				Op: []interface{}{
					"transfer_to_vesting",
					// Missing second element
				},
			},
		}

		err := processor.handleVestingDeposit(ctx, op)
		if err == nil {
			t.Error("Expected error for Op array too short, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "Op array too short") {
			t.Errorf("Expected error about Op array too short, got: %v", err)
		}
	})

	t.Run("error: invalid data type (cannot marshal)", func(t *testing.T) {
		// Create a type that cannot be marshaled to JSON
		type UnmarshalableType struct {
			Channel chan int // Channels cannot be marshaled
		}

		invalidData := UnmarshalableType{
			Channel: make(chan int),
		}

		op := &Operation{
			Block: &utils.Block{
				Number:    330784,
				Timestamp: testTime,
				BlockID:   "test_block_id_4",
			},
			Operation: &utils.Operation{
				TrxID:      "test_trx_id_4",
				Block:      330784,
				TrxInBlock: 0,
				OpInTrx:    0,
				Timestamp:  testTime,
				Op: []interface{}{
					"transfer_to_vesting",
					invalidData,
				},
			},
		}

		err := processor.handleVestingDeposit(ctx, op)
		if err == nil {
			t.Error("Expected error for invalid data type, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "failed to marshal") {
			t.Errorf("Expected error about marshal failure, got: %v", err)
		}
	})

	t.Run("successful processing with empty amount", func(t *testing.T) {
		op := &Operation{
			Block: &utils.Block{
				Number:    330785,
				Timestamp: testTime,
				BlockID:   "test_block_id_5",
			},
			Operation: &utils.Operation{
				TrxID:      "test_trx_id_5",
				Block:      330785,
				TrxInBlock: 0,
				OpInTrx:    0,
				Timestamp:  testTime,
				Op: []interface{}{
					"transfer_to_vesting",
					map[string]interface{}{
						"from":   "eve",
						"to":     "frank",
						"amount": "", // Empty amount should be handled gracefully
					},
				},
			},
		}

		err := processor.handleVestingDeposit(ctx, op)
		if err != nil {
			t.Errorf("Expected no error with empty amount, got: %v", err)
		}
	})

	t.Run("successful processing with missing optional fields", func(t *testing.T) {
		op := &Operation{
			Block: &utils.Block{
				Number:    330786,
				Timestamp: testTime,
				BlockID:   "test_block_id_6",
			},
			Operation: &utils.Operation{
				TrxID:      "test_trx_id_6",
				Block:      330786,
				TrxInBlock: 0,
				OpInTrx:    0,
				Timestamp:  testTime,
				Op: []interface{}{
					"transfer_to_vesting",
					map[string]interface{}{
						"from": "grace",
						"to":   "henry",
						// Missing amount field
					},
				},
			},
		}

		err := processor.handleVestingDeposit(ctx, op)
		if err != nil {
			t.Errorf("Expected no error with missing amount, got: %v", err)
		}
	})
}

// TestGetOperationData tests the getOperationData function
func TestGetOperationData(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("successful extraction with map data", func(t *testing.T) {
		op := &Operation{
			Block: &utils.Block{
				Number:    1000,
				Timestamp: testTime,
			},
			Operation: &utils.Operation{
				Op: []interface{}{
					"test_op",
					map[string]interface{}{
						"field1": "value1",
						"field2": 123,
					},
				},
			},
		}

		opData, err := getOperationData(op)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if opData == nil {
			t.Error("Expected non-nil opData")
		}
		if opData["field1"] != "value1" {
			t.Errorf("Expected field1='value1', got '%v'", opData["field1"])
		}
		if opData["field2"] != 123 {
			t.Errorf("Expected field2=123, got '%v'", opData["field2"])
		}
	})

	t.Run("successful extraction with struct data (JSON conversion)", func(t *testing.T) {
		type TestOpData struct {
			Field1 string `json:"field1"`
			Field2 int    `json:"field2"`
		}

		structData := TestOpData{
			Field1: "value1",
			Field2: 123,
		}

		op := &Operation{
			Block: &utils.Block{
				Number:    1001,
				Timestamp: testTime,
			},
			Operation: &utils.Operation{
				Op: []interface{}{
					"test_op",
					structData, // Struct instead of map
				},
			},
		}

		opData, err := getOperationData(op)
		if err != nil {
			t.Errorf("Expected no error with struct data, got: %v", err)
		}
		if opData == nil {
			t.Error("Expected non-nil opData")
		}
		if opData["field1"] != "value1" {
			t.Errorf("Expected field1='value1', got '%v'", opData["field1"])
		}
		// JSON unmarshal converts numbers to float64
		if opData["field2"] != float64(123) {
			t.Errorf("Expected field2=123, got '%v'", opData["field2"])
		}
	})

	t.Run("error: Op array too short", func(t *testing.T) {
		op := &Operation{
			Block: &utils.Block{
				Number:    1002,
				Timestamp: testTime,
			},
			Operation: &utils.Operation{
				Op: []interface{}{
					"test_op",
					// Missing second element
				},
			},
		}

		opData, err := getOperationData(op)
		if err == nil {
			t.Error("Expected error for Op array too short, got nil")
		}
		if opData != nil {
			t.Error("Expected nil opData on error")
		}
		if !strings.Contains(err.Error(), "Op array too short") {
			t.Errorf("Expected error about Op array too short, got: %v", err)
		}
	})

	t.Run("error: cannot marshal data", func(t *testing.T) {
		type UnmarshalableType struct {
			Channel chan int // Channels cannot be marshaled
		}

		invalidData := UnmarshalableType{
			Channel: make(chan int),
		}

		op := &Operation{
			Block: &utils.Block{
				Number:    1003,
				Timestamp: testTime,
			},
			Operation: &utils.Operation{
				Op: []interface{}{
					"test_op",
					invalidData,
				},
			},
		}

		opData, err := getOperationData(op)
		if err == nil {
			t.Error("Expected error for unmarshalable data, got nil")
		}
		if opData != nil {
			t.Error("Expected nil opData on error")
		}
		if !strings.Contains(err.Error(), "failed to marshal") {
			t.Errorf("Expected error about marshal failure, got: %v", err)
		}
	})
}

// TestSaveOperation tests the saveOperation function
func TestSaveOperation(t *testing.T) {
	mockDB := &MockDatabase{}
	mockLogger := &MockLogger{}

	processor := &OperationProcessor{
		db:     mockDB,
		logger: mockLogger,
	}

	ctx := context.Background()
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("successful save with map data", func(t *testing.T) {
		op := &Operation{
			Block: &utils.Block{
				Number:    2000,
				Timestamp: testTime,
			},
			Operation: &utils.Operation{
				TrxID:      "test_trx_1",
				Block:      2000,
				TrxInBlock: 0,
				OpInTrx:    0,
				Timestamp:  testTime,
				Op: []interface{}{
					"transfer",
					map[string]interface{}{
						"from":   "alice",
						"to":     "bob",
						"amount": "10.000 STEEM",
					},
				},
			},
		}

		opID, err := processor.saveOperation(ctx, op, "transfer")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if opID == nil {
			t.Error("Expected non-nil opID")
		}
	})

	t.Run("successful save with struct data", func(t *testing.T) {
		type TransferOpData struct {
			From   string `json:"from"`
			To     string `json:"to"`
			Amount string `json:"amount"`
		}

		structData := TransferOpData{
			From:   "charlie",
			To:     "david",
			Amount: "20.000 STEEM",
		}

		op := &Operation{
			Block: &utils.Block{
				Number:    2001,
				Timestamp: testTime,
			},
			Operation: &utils.Operation{
				TrxID:      "test_trx_2",
				Block:      2001,
				TrxInBlock: 0,
				OpInTrx:    0,
				Timestamp:  testTime,
				Op: []interface{}{
					"transfer",
					structData, // Struct instead of map
				},
			},
		}

		opID, err := processor.saveOperation(ctx, op, "transfer")
		if err != nil {
			t.Errorf("Expected no error with struct data, got: %v", err)
		}
		if opID == nil {
			t.Error("Expected non-nil opID")
		}
	})

	t.Run("error: invalid operation data", func(t *testing.T) {
		op := &Operation{
			Block: &utils.Block{
				Number:    2002,
				Timestamp: testTime,
			},
			Operation: &utils.Operation{
				TrxID:      "test_trx_3",
				Block:      2002,
				TrxInBlock: 0,
				OpInTrx:    0,
				Timestamp:  testTime,
				Op: []interface{}{
					"transfer",
					// Missing second element
				},
			},
		}

		opID, err := processor.saveOperation(ctx, op, "transfer")
		if err == nil {
			t.Error("Expected error for invalid operation data, got nil")
		}
		if opID != nil {
			t.Error("Expected nil opID on error")
		}
		if !strings.Contains(err.Error(), "invalid operation data") {
			t.Errorf("Expected error about invalid operation data, got: %v", err)
		}
	})
}
