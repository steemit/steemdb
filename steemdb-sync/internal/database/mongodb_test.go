package database

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// TestGetOperationsCollectionName tests collection name generation
func TestGetOperationsCollectionName(t *testing.T) {
	tests := []struct {
		name     string
		blockNum int64
		expected string
	}{
		{
			name:     "First range start",
			blockNum: 0,
			expected: "operations_0_10000000",
		},
		{
			name:     "First range middle",
			blockNum: 5000000,
			expected: "operations_0_10000000",
		},
		{
			name:     "First range end",
			blockNum: 9999999,
			expected: "operations_0_10000000",
		},
		{
			name:     "Second range start",
			blockNum: 10000000,
			expected: "operations_10000000_20000000",
		},
		{
			name:     "Second range middle",
			blockNum: 15000000,
			expected: "operations_10000000_20000000",
		},
		{
			name:     "Third range start",
			blockNum: 20000000,
			expected: "operations_20000000_30000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetOperationsCollectionName(tt.blockNum)
			if result != tt.expected {
				t.Errorf("GetOperationsCollectionName(%d) = %s, want %s", tt.blockNum, result, tt.expected)
			}
		})
	}
}

// TestGetCollectionsInRange tests collection range calculation
func TestGetCollectionsInRange(t *testing.T) {
	tests := []struct {
		name       string
		startBlock int64
		endBlock   int64
		expected   []string
	}{
		{
			name:       "Single collection",
			startBlock: 1000,
			endBlock:   2000,
			expected:   []string{"operations_0_10000000"},
		},
		{
			name:       "Cross collection boundary",
			startBlock: 9999999,
			endBlock:   10000001,
			expected:   []string{"operations_0_10000000", "operations_10000000_20000000"},
		},
		{
			name:       "Multiple collections",
			startBlock: 5000000,
			endBlock:   25000000,
			expected:   []string{"operations_0_10000000", "operations_10000000_20000000", "operations_20000000_30000000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCollectionsInRange(tt.startBlock, tt.endBlock)
			if len(result) != len(tt.expected) {
				t.Errorf("GetCollectionsInRange(%d, %d) returned %d collections, want %d",
					tt.startBlock, tt.endBlock, len(result), len(tt.expected))
				return
			}

			// Convert to map for easier comparison
			resultMap := make(map[string]bool)
			for _, name := range result {
				resultMap[name] = true
			}

			for _, expected := range tt.expected {
				if !resultMap[expected] {
					t.Errorf("GetCollectionsInRange(%d, %d) missing collection %s",
						tt.startBlock, tt.endBlock, expected)
				}
			}
		})
	}
}

// TestRawOperation_Structure tests RawOperation structure
func TestRawOperation_Structure(t *testing.T) {
	op := &RawOperation{
		BlockNum:     1000,
		TrxID:        "test_trx_id",
		TrxInBlock:   0,
		OpInTrx:      0,
		OpType:       "comment",
		OpData:       bson.M{"author": "test"},
		IsVirtual:    false,
		VirtualOpNum: 0,
		Timestamp:    time.Now(),
		CreatedAt:    time.Now(),
	}

	// Verify all fields are set
	if op.BlockNum != 1000 {
		t.Errorf("BlockNum = %d, want 1000", op.BlockNum)
	}
	if op.TrxID != "test_trx_id" {
		t.Errorf("TrxID = %s, want test_trx_id", op.TrxID)
	}
	if op.OpType != "comment" {
		t.Errorf("OpType = %s, want comment", op.OpType)
	}
	if op.IsVirtual {
		t.Error("IsVirtual should be false for regular operation")
	}
}

// TestRawOperation_VirtualOperation tests virtual operation structure
func TestRawOperation_VirtualOperation(t *testing.T) {
	op := &RawOperation{
		BlockNum:     1000,
		TrxID:        "virtual_1000_1",
		IsVirtual:    true,
		VirtualOpNum: 1,
	}

	// Verify virtual operation fields
	if op.BlockNum != 1000 {
		t.Errorf("BlockNum = %d, want 1000", op.BlockNum)
	}
	if !op.IsVirtual {
		t.Error("IsVirtual should be true for virtual operation")
	}
	if op.VirtualOpNum != 1 {
		t.Errorf("VirtualOpNum = %d, want 1", op.VirtualOpNum)
	}
	if op.TrxID != "virtual_1000_1" {
		t.Errorf("TrxID = %s, want virtual_1000_1", op.TrxID)
	}
}

// TestSyncState_DefaultValues tests SyncState default values
func TestSyncState_DefaultValues(t *testing.T) {
	state := &SyncState{
		ID:                    "current",
		LastBlock:             0,
		LastIrreversibleBlock: 0,
		UpdatedAt:             time.Now(),
	}

	if state.ID != "current" {
		t.Errorf("ID = %s, want current", state.ID)
	}
	if state.LastBlock != 0 {
		t.Errorf("LastBlock = %d, want 0", state.LastBlock)
	}
	if state.LastIrreversibleBlock != 0 {
		t.Errorf("LastIrreversibleBlock = %d, want 0", state.LastIrreversibleBlock)
	}
	if state.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

// TestBusinessProcessingState_DefaultValues tests BusinessProcessingState default values
func TestBusinessProcessingState_DefaultValues(t *testing.T) {
	state := &BusinessProcessingState{
		ID:        "comments",
		LastBlock: 0,
		UpdatedAt: time.Now(),
	}

	if state.ID != "comments" {
		t.Errorf("ID = %s, want comments", state.ID)
	}
	if state.LastBlock != 0 {
		t.Errorf("LastBlock = %d, want 0", state.LastBlock)
	}
}
