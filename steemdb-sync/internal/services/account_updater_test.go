package services

import (
	"testing"

	"github.com/steemit/steemdb/sync/internal/utils"
)

// TestAccountUpdater_ConvertSteemAccountToDBAccount tests the conversion function
func TestAccountUpdater_ConvertSteemAccountToDBAccount(t *testing.T) {
	config := &utils.Config{
		Sync: utils.SyncConfig{
			AccountBatchSize: 100,
		},
	}

	logger := &TestLogger{}

	updater := NewAccountUpdater(config, nil, nil, logger)

	// Create a test steem account
	steemAcc := &utils.Account{
		Name:          "testuser",
		Balance:       "100.000 STEEM",
		SBDBalance:    "50.000 SBD",
		VestingShares: "1000.000000 VESTS",
		Reputation:    "1000000",
		PostCount:     10,
		CommentCount:  20,
	}

	// Convert
	dbAccount := updater.convertSteemAccountToDBAccount(steemAcc)

	// Verify conversion
	if dbAccount.Name != "testuser" {
		t.Errorf("Expected name 'testuser', got '%s'", dbAccount.Name)
	}
	if dbAccount.Balance != "100.000 STEEM" {
		t.Errorf("Expected balance '100.000 STEEM', got '%s'", dbAccount.Balance)
	}
	if dbAccount.SBDBalance != "50.000 SBD" {
		t.Errorf("Expected SBD balance '50.000 SBD', got '%s'", dbAccount.SBDBalance)
	}
	if dbAccount.PostCount != 10 {
		t.Errorf("Expected PostCount 10, got %d", dbAccount.PostCount)
	}
	if dbAccount.CommentCount != 20 {
		t.Errorf("Expected CommentCount 20, got %d", dbAccount.CommentCount)
	}
	if dbAccount.NeedsUpdate {
		t.Error("Expected NeedsUpdate to be false after conversion")
	}
	if dbAccount.NameLower != "testuser" {
		t.Errorf("Expected NameLower 'testuser', got '%s'", dbAccount.NameLower)
	}
}
