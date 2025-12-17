package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/steemit/steemdb/sync/internal/database"
	"github.com/steemit/steemdb/sync/internal/utils"
)

// AccountUpdater handles account updates from condenser_api.get_accounts
type AccountUpdater struct {
	config    *utils.Config
	db        *database.MongoDB
	steem     *utils.SteemClient
	logger    utils.Logger
	batchSize int
}

// NewAccountUpdater creates a new account updater service
func NewAccountUpdater(
	config *utils.Config,
	db *database.MongoDB,
	steemClient *utils.SteemClient,
	logger utils.Logger,
) *AccountUpdater {
	batchSize := 100 // Default batch size for get_accounts
	if config.Sync.AccountBatchSize > 0 {
		batchSize = config.Sync.AccountBatchSize
	}

	return &AccountUpdater{
		config:    config,
		db:        db,
		steem:     steemClient,
		logger:    logger,
		batchSize: batchSize,
	}
}

// UpdateAccounts updates accounts that need to be updated
func (a *AccountUpdater) UpdateAccounts(ctx context.Context) error {
	// Query accounts that need update
	accounts, err := a.db.FindAccountsNeedingUpdate(ctx, a.batchSize)
	if err != nil {
		return fmt.Errorf("failed to find accounts needing update: %w", err)
	}

	if len(accounts) == 0 {
		return nil
	}

	a.logger.Info("Updating accounts",
		utils.Int("count", len(accounts)),
	)

	// Extract account names
	accountNames := make([]string, len(accounts))
	for i, acc := range accounts {
		accountNames[i] = acc.Name
	}

	// Call condenser_api.get_accounts
	steemAccounts, err := a.steem.GetAccounts(ctx, accountNames)
	if err != nil {
		return fmt.Errorf("failed to get accounts from API: %w", err)
	}

	// Convert steem.Account to database.Account and update
	updatedCount := 0
	for _, steemAcc := range steemAccounts {
		dbAccount := a.convertSteemAccountToDBAccount(&steemAcc)
		dbAccount.NeedsUpdate = false
		dbAccount.LastUpdated = time.Now()

		if err := a.db.UpsertAccount(ctx, dbAccount); err != nil {
			a.logger.Error("Failed to upsert account",
				utils.String("account", dbAccount.Name),
				utils.Error(err),
			)
			continue
		}

		updatedCount++
	}

	a.logger.Info("Updated accounts",
		utils.Int("updated", updatedCount),
		utils.Int("total", len(accounts)),
	)

	return nil
}

// convertSteemAccountToDBAccount converts steem.Account to database.Account
func (a *AccountUpdater) convertSteemAccountToDBAccount(steemAcc *utils.Account) *database.Account {
	// Convert Authority to bson.M
	owner := map[string]interface{}{
		"weight_threshold": steemAcc.Owner.WeightThreshold,
		"account_auths":    steemAcc.Owner.AccountAuths,
		"key_auths":        steemAcc.Owner.KeyAuths,
	}
	active := map[string]interface{}{
		"weight_threshold": steemAcc.Active.WeightThreshold,
		"account_auths":    steemAcc.Active.AccountAuths,
		"key_auths":        steemAcc.Active.KeyAuths,
	}
	posting := map[string]interface{}{
		"weight_threshold": steemAcc.Posting.WeightThreshold,
		"account_auths":    steemAcc.Posting.AccountAuths,
		"key_auths":        steemAcc.Posting.KeyAuths,
	}

	// Convert protocol.Time to time.Time
	created := utils.ToTime(steemAcc.Created)
	lastOwnerUpdate := utils.ToTime(steemAcc.LastOwnerUpdate)
	lastAccountUpdate := utils.ToTime(steemAcc.LastAccountUpdate)
	lastAccountRecovery := utils.ToTime(steemAcc.LastAccountRecovery)
	lastPost := utils.ToTime(steemAcc.LastPost)
	lastRootPost := utils.ToTime(steemAcc.LastRootPost)
	lastVoteTime := utils.ToTime(steemAcc.LastVoteTime)
	nextVestingWithdrawal := utils.ToTime(steemAcc.NextVestingWithdrawal)
	sbdSecondsLastUpdate := utils.ToTime(steemAcc.SBDSecondsLastUpdate)
	sbdLastInterestPayment := utils.ToTime(steemAcc.SBDLastInterestPayment)
	savingsSBDSecondsLastUpdate := utils.ToTime(steemAcc.SavingsSBDSecondsLastUpdate)
	savingsSBDLastInterestPayment := utils.ToTime(steemAcc.SavingsSBDLastInterestPayment)

	// Convert ProxiedVSFVotes from []string to []string (already correct)
	proxiedVSFVotes := steemAcc.ProxiedVSFVotes

	return &database.Account{
		ID:                            steemAcc.Name,
		Name:                          steemAcc.Name,
		Owner:                         owner,
		Active:                        active,
		Posting:                       posting,
		MemoKey:                       steemAcc.MemoKey,
		JsonMetadata:                  steemAcc.JsonMetadata,
		Proxy:                         steemAcc.Proxy,
		RecoveryAccount:               steemAcc.RecoveryAccount,
		ResetAccount:                  steemAcc.ResetAccount,
		Balance:                       steemAcc.Balance,
		SavingsBalance:                steemAcc.SavingsBalance,
		SBDBalance:                    steemAcc.SBDBalance,
		SavingsSBDBalance:             steemAcc.SavingsSBDBalance,
		RewardSBDBalance:              steemAcc.RewardSBDBalance,
		RewardSteemBalance:            steemAcc.RewardSteemBalance,
		RewardVestingBalance:          steemAcc.RewardVestingBalance,
		RewardVestingSteem:            steemAcc.RewardVestingSteem,
		VestingShares:                 steemAcc.VestingShares,
		DelegatedVestingShares:        steemAcc.DelegatedVestingShares,
		ReceivedVestingShares:         steemAcc.ReceivedVestingShares,
		VestingWithdrawRate:           steemAcc.VestingWithdrawRate,
		NextVestingWithdrawal:         nextVestingWithdrawal,
		Withdrawn:                     steemAcc.Withdrawn,
		ToWithdraw:                    steemAcc.ToWithdraw,
		SBDSeconds:                    steemAcc.SBDSeconds,
		SBDSecondsLastUpdate:          sbdSecondsLastUpdate,
		SBDLastInterestPayment:        sbdLastInterestPayment,
		SavingsSBDSeconds:             "", // Not in steem.Account, will be empty
		SavingsSBDSecondsLastUpdate:   savingsSBDSecondsLastUpdate,
		SavingsSBDLastInterestPayment: savingsSBDLastInterestPayment,
		SavingsWithdrawRequests:       steemAcc.SavingsWithdrawRequests,
		VotingPower:                   steemAcc.VotingPower,
		LastVoteTime:                  lastVoteTime,
		CanVote:                       steemAcc.CanVote,
		CurationRewards:               steemAcc.CurationRewards,
		PostingRewards:                steemAcc.PostingRewards,
		ProxiedVSFVotes:               proxiedVSFVotes,
		WitnessesVotedFor:             steemAcc.WitnessesVotedFor,
		WitnessVotes:                  steemAcc.WitnessVotes,
		WithdrawRoutes:                steemAcc.WithdrawRoutes,
		PostCount:                     steemAcc.PostCount,
		CommentCount:                  steemAcc.CommentCount,
		LifetimeVoteCount:             steemAcc.LifetimeVoteCount,
		Created:                       created,
		LastOwnerUpdate:               lastOwnerUpdate,
		LastAccountUpdate:             lastAccountUpdate,
		LastAccountRecovery:           lastAccountRecovery,
		LastPost:                      lastPost,
		LastRootPost:                  lastRootPost,
		Mined:                         steemAcc.Mined,
		Reputation:                    steemAcc.Reputation,
		VestingBalance:                "", // Not in steem.Account, will be calculated if needed
		NameLower:                     strings.ToLower(steemAcc.Name),
		NeedsUpdate:                   false,
		LastUpdated:                   time.Now(),
	}
}
