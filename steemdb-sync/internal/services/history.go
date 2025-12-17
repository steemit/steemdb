package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/steemdb/sync/internal/database"
	"github.com/steemdb/sync/internal/utils"
)

// HistoryService handles account history data collection
type HistoryService struct {
	config *utils.Config
	db     *database.MongoDB
	steem  *utils.SteemClient
	logger utils.Logger

	scheduler *cron.Cron
}

// NewHistoryService creates a new history service
func NewHistoryService(
	config *utils.Config,
	db *database.MongoDB,
	steemClient *utils.SteemClient,
	logger utils.Logger,
) *HistoryService {
	return &HistoryService{
		config:    config,
		db:        db,
		steem:     steemClient,
		logger:    logger,
		scheduler: cron.New(),
	}
}

// Start starts the history service
func (h *HistoryService) Start(ctx context.Context) error {
	h.logger.Info("Starting history service")

	// Schedule history update job
	_, err := h.scheduler.AddFunc("@every 6h", func() {
		if err := h.updateHistory(ctx); err != nil {
			h.logger.Error("History update failed", utils.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to schedule history job: %w", err)
	}

	// Schedule fund history update job
	_, err = h.scheduler.AddFunc("@every 1h", func() {
		if err := h.updateFundHistory(ctx); err != nil {
			h.logger.Error("Fund history update failed", utils.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to schedule fund history job: %w", err)
	}

	// Start scheduler
	h.scheduler.Start()

	// Run initial update
	go func() {
		if err := h.updateHistory(ctx); err != nil {
			h.logger.Error("Initial history update failed", utils.Error(err))
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Stop scheduler
	h.scheduler.Stop()
	return nil
}

// updateHistory updates account history data
func (h *HistoryService) updateHistory(ctx context.Context) error {
	h.logger.Info("Starting account history update")
	startTime := time.Now()

	// Update fund history first
	if err := h.updateFundHistory(ctx); err != nil {
		h.logger.Error("Failed to update fund history", utils.Error(err))
	}

	// Get all account names
	accounts, err := h.getAllAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all accounts: %w", err)
	}

	h.logger.Info("Processing accounts", utils.Int("count", len(accounts)))

	// Process accounts in batches
	if err := h.processAccountsBatch(ctx, accounts); err != nil {
		return fmt.Errorf("failed to process accounts: %w", err)
	}

	duration := time.Since(startTime)
	h.logger.Info("Account history update completed",
		utils.Int("accounts", len(accounts)),
		utils.Duration("duration", duration),
	)

	return nil
}

// getAllAccounts gets all account names from the blockchain
func (h *HistoryService) getAllAccounts(ctx context.Context) ([]string, error) {
	var accounts []string
	lastAccount := ""

	for {
		batch, err := h.steem.LookupAccounts(ctx, lastAccount, h.config.History.AccountScanLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup accounts: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		accounts = append(accounts, batch...)

		if len(batch) < h.config.History.AccountScanLimit {
			break
		}

		lastAccount = batch[len(batch)-1]
	}

	return accounts, nil
}

// processAccountsBatch processes accounts in batches
func (h *HistoryService) processAccountsBatch(ctx context.Context, accounts []string) error {
	var wg sync.WaitGroup
	accountChan := make(chan []string, h.config.History.Workers)

	// Start workers
	for i := 0; i < h.config.History.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			h.accountWorker(ctx, workerID, accountChan)
		}(i)
	}

	// Send batches to workers
	go func() {
		defer close(accountChan)
		for i := 0; i < len(accounts); i += h.config.History.BatchSize {
			end := i + h.config.History.BatchSize
			if end > len(accounts) {
				end = len(accounts)
			}

			batch := accounts[i:end]
			select {
			case accountChan <- batch:
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()
	return nil
}

// accountWorker processes account batches
func (h *HistoryService) accountWorker(ctx context.Context, workerID int, accountChan <-chan []string) {
	for {
		select {
		case <-ctx.Done():
			return
		case batch, ok := <-accountChan:
			if !ok {
				return
			}

			if err := h.processBatch(ctx, batch); err != nil {
				h.logger.Error("Failed to process batch",
					utils.Int("worker", workerID),
					utils.Error(err),
				)
			}
		}
	}
}

// processBatch processes a batch of accounts
func (h *HistoryService) processBatch(ctx context.Context, accountNames []string) error {
	// Get account details from blockchain
	accounts, err := h.steem.GetAccounts(ctx, accountNames)
	if err != nil {
		return fmt.Errorf("failed to get accounts: %w", err)
	}

	// Prepare bulk operations
	var accountOps []mongo.WriteModel
	var historyOps []mongo.WriteModel

	for _, account := range accounts {
		// Process account data
		processedAccount := h.processAccountData(&account)

		// Account update operation
		accountOps = append(accountOps, mongo.NewUpdateOneModel().
			SetFilter(map[string]interface{}{"_id": account.Name}).
			SetUpdate(map[string]interface{}{"$set": processedAccount}).
			SetUpsert(true))

		// Account history snapshot
		snapshot := h.createAccountSnapshot(processedAccount)
		historyOps = append(historyOps, mongo.NewUpdateOneModel().
			SetFilter(map[string]interface{}{
				"account": account.Name,
				"date":    time.Now().Truncate(24 * time.Hour),
			}).
			SetUpdate(map[string]interface{}{"$set": snapshot}).
			SetUpsert(true))
	}

	// Execute bulk operations
	if len(accountOps) > 0 {
		if err := h.db.BulkWrite(ctx, "account", accountOps); err != nil {
			return fmt.Errorf("failed to bulk write accounts: %w", err)
		}
	}

	if len(historyOps) > 0 {
		if err := h.db.BulkWrite(ctx, "account_history", historyOps); err != nil {
			return fmt.Errorf("failed to bulk write account history: %w", err)
		}
	}

	return nil
}

// processAccountData processes raw account data from blockchain
// Note: This function is deprecated. Use AccountUpdater instead which uses condenser_api.get_accounts
func (h *HistoryService) processAccountData(account *utils.Account) *database.Account {
	// Convert Authority to bson.M
	owner := map[string]interface{}{
		"weight_threshold": account.Owner.WeightThreshold,
		"account_auths":    account.Owner.AccountAuths,
		"key_auths":        account.Owner.KeyAuths,
	}
	active := map[string]interface{}{
		"weight_threshold": account.Active.WeightThreshold,
		"account_auths":    account.Active.AccountAuths,
		"key_auths":        account.Active.KeyAuths,
	}
	posting := map[string]interface{}{
		"weight_threshold": account.Posting.WeightThreshold,
		"account_auths":    account.Posting.AccountAuths,
		"key_auths":        account.Posting.KeyAuths,
	}

	return &database.Account{
		ID:                            account.Name,
		Name:                          account.Name,
		Owner:                         owner,
		Active:                        active,
		Posting:                       posting,
		MemoKey:                       account.MemoKey,
		JsonMetadata:                  account.JsonMetadata,
		Proxy:                         account.Proxy,
		RecoveryAccount:               account.RecoveryAccount,
		ResetAccount:                  account.ResetAccount,
		Balance:                       account.Balance, // Already string format
		SavingsBalance:                account.SavingsBalance,
		SBDBalance:                    account.SBDBalance,
		SavingsSBDBalance:             account.SavingsSBDBalance,
		RewardSBDBalance:              account.RewardSBDBalance,
		RewardSteemBalance:            account.RewardSteemBalance,
		RewardVestingBalance:          account.RewardVestingBalance,
		RewardVestingSteem:            account.RewardVestingSteem,
		VestingShares:                 account.VestingShares, // Already string format
		DelegatedVestingShares:        account.DelegatedVestingShares,
		ReceivedVestingShares:         account.ReceivedVestingShares,
		VestingWithdrawRate:           account.VestingWithdrawRate, // Already string format
		NextVestingWithdrawal:         utils.ToTime(account.NextVestingWithdrawal),
		Withdrawn:                     account.Withdrawn,
		ToWithdraw:                    account.ToWithdraw,
		SBDSeconds:                    account.SBDSeconds,
		SBDSecondsLastUpdate:          utils.ToTime(account.SBDSecondsLastUpdate),
		SBDLastInterestPayment:        utils.ToTime(account.SBDLastInterestPayment),
		SavingsSBDSeconds:             "", // Not available in utils.Account
		SavingsSBDSecondsLastUpdate:   utils.ToTime(account.SavingsSBDSecondsLastUpdate),
		SavingsSBDLastInterestPayment: utils.ToTime(account.SavingsSBDLastInterestPayment),
		SavingsWithdrawRequests:       account.SavingsWithdrawRequests,
		VotingPower:                   account.VotingPower,
		LastVoteTime:                  utils.ToTime(account.LastVoteTime),
		CanVote:                       account.CanVote,
		CurationRewards:               account.CurationRewards,
		PostingRewards:                account.PostingRewards,
		ProxiedVSFVotes:               account.ProxiedVSFVotes,
		WitnessesVotedFor:             account.WitnessesVotedFor,
		WitnessVotes:                  account.WitnessVotes,
		WithdrawRoutes:                account.WithdrawRoutes,
		PostCount:                     account.PostCount,
		CommentCount:                  account.CommentCount,
		LifetimeVoteCount:             account.LifetimeVoteCount,
		Created:                       utils.ToTime(account.Created),
		LastOwnerUpdate:               utils.ToTime(account.LastOwnerUpdate),
		LastAccountUpdate:             utils.ToTime(account.LastAccountUpdate),
		LastAccountRecovery:           utils.ToTime(account.LastAccountRecovery),
		LastPost:                      utils.ToTime(account.LastPost),
		LastRootPost:                  utils.ToTime(account.LastRootPost),
		Mined:                         account.Mined,
		Reputation:                    account.Reputation, // Already string format
		VestingBalance:                "",                 // Not in steem.Account
		NameLower:                     strings.ToLower(account.Name),
		NeedsUpdate:                   false,
		LastUpdated:                   time.Now(),
	}
}

// createAccountSnapshot creates a historical snapshot of account data
func (h *HistoryService) createAccountSnapshot(account *database.Account) *database.AccountHistory {
	// Parse string values to numeric for history
	reputation := int64(0)
	if account.Reputation != "" {
		if rep, err := utils.ParseFloat64(account.Reputation); err == nil {
			reputation = int64(rep)
		}
	}
	vestingShares := utils.ParseAmountValue(account.VestingShares)
	balance := utils.ParseAmountValue(account.Balance)
	sbdBalance := utils.ParseAmountValue(account.SBDBalance)

	return &database.AccountHistory{
		ID:            fmt.Sprintf("%s|%s", account.Name, time.Now().Format("20060102")),
		Account:       account.Name,
		Date:          time.Now().Truncate(24 * time.Hour),
		Reputation:    reputation,
		VestingShares: vestingShares,
		Balance:       balance,
		SBDBalance:    sbdBalance,
		PostCount:     account.PostCount,
		CommentCount:  account.CommentCount,
		VotingPower:   account.VotingPower,
	}
}

// updateFundHistory updates reward fund history
func (h *HistoryService) updateFundHistory(ctx context.Context) error {
	h.logger.Info("Updating fund history")

	fund, err := h.steem.GetRewardFund(ctx, "post")
	if err != nil {
		return fmt.Errorf("failed to get reward fund: %w", err)
	}

	fundHistory := &database.FundsHistory{
		ID:                  fmt.Sprintf("post|%d", time.Now().Unix()),
		Name:                fund.Name,
		RewardBalance:       utils.ParseAmountValue(fund.RewardBalance),
		RecentClaims:        utils.ParseFloat64Value(fund.RecentClaims),
		ContentConstant:     utils.ParseFloat64Value(fund.ContentConstant),
		PercentCuration:     fund.PercentCurationRewards,
		PercentContent:      fund.PercentContentRewards,
		LastUpdate:          utils.ToTime(fund.LastUpdate),
		AuthorRewardCurve:   fund.AuthorRewardCurve,
		CurationRewardCurve: fund.CurationRewardCurve,
	}

	collection := h.db.Collection("funds_history")
	_, err = collection.InsertOne(ctx, fundHistory)
	if err != nil {
		return fmt.Errorf("failed to save fund history: %w", err)
	}

	return nil
}
