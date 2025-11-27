package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemdb/sync/internal/database"
	"github.com/steemdb/sync/internal/utils"
	"github.com/steemdb/sync/pkg/steem"
)

// Operation represents a blockchain operation with context
type Operation struct {
	Block     *steem.Block
	Operation *steem.Operation
}

// OperationProcessor processes blockchain operations
type OperationProcessor struct {
	db       *database.MongoDB
	logger   utils.Logger
	handlers map[string]OperationHandler
}

// OperationHandler is a function that processes a specific operation type
type OperationHandler func(ctx context.Context, op *Operation) error

// NewOperationProcessor creates a new operation processor
func NewOperationProcessor(db *database.MongoDB, logger utils.Logger) *OperationProcessor {
	p := &OperationProcessor{
		db:       db,
		logger:   logger,
		handlers: make(map[string]OperationHandler),
	}

	// Register operation handlers
	p.registerHandlers()
	return p
}

// registerHandlers registers all operation handlers
func (p *OperationProcessor) registerHandlers() {
	p.handlers["comment"] = p.handleComment
	p.handlers["vote"] = p.handleVote
	p.handlers["transfer"] = p.handleTransfer
	p.handlers["curation_reward"] = p.handleCurationReward
	p.handlers["author_reward"] = p.handleAuthorReward
	p.handlers["transfer_to_vesting"] = p.handleVestingDeposit
	p.handlers["fill_vesting_withdraw"] = p.handleVestingWithdraw
	p.handlers["convert"] = p.handleConvert
	p.handlers["feed_publish"] = p.handleFeedPublish
	p.handlers["account_witness_vote"] = p.handleWitnessVote
	p.handlers["pow"] = p.handlePow
	p.handlers["pow2"] = p.handlePow
	p.handlers["custom_json"] = p.handleCustomJson
	p.handlers["comment_options"] = p.handleCommentOptions
	p.handlers["comment_benefactor_reward"] = p.handleBenefactorReward
}

// Process processes a blockchain operation
func (p *OperationProcessor) Process(op *Operation) error {
	if len(op.Operation.Op) < 2 {
		return fmt.Errorf("invalid operation format")
	}

	opType, ok := op.Operation.Op[0].(string)
	if !ok {
		return fmt.Errorf("invalid operation type")
	}

	handler, exists := p.handlers[opType]
	if !exists {
		p.logger.Debug("Unknown operation type", utils.String("type", opType))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return handler(ctx, op)
}

// handleComment processes comment operations
func (p *OperationProcessor) handleComment(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid comment operation data")
	}

	comment := &database.Comment{
		ID:             fmt.Sprintf("%s/%s", opData["author"], opData["permlink"]),
		Author:         getString(opData, "author"),
		Permlink:       getString(opData, "permlink"),
		Title:          getString(opData, "title"),
		Body:           getString(opData, "body"),
		ParentAuthor:   getString(opData, "parent_author"),
		ParentPermlink: getString(opData, "parent_permlink"),
		Created:        op.Operation.Timestamp,
		LastUpdate:     op.Operation.Timestamp,
		BlockNum:       op.Block.Number,
		Scanned:        time.Now(),
	}

	// Parse JSON metadata
	if jsonMeta := getString(opData, "json_metadata"); jsonMeta != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(jsonMeta), &metadata); err == nil {
			comment.JsonMetadata = metadata
		}
	}

	// Set category
	if comment.ParentPermlink != "" {
		comment.Category = comment.ParentPermlink
	}

	// Calculate depth
	if comment.ParentAuthor != "" {
		// This is a reply, find parent depth
		parentCollection := p.db.Collection("comment")
		var parent database.Comment
		err := parentCollection.FindOne(ctx, map[string]interface{}{
			"_id": fmt.Sprintf("%s/%s", comment.ParentAuthor, comment.ParentPermlink),
		}).Decode(&parent)
		if err == nil {
			comment.Depth = parent.Depth + 1
		} else {
			comment.Depth = 1
		}
	}

	// Save comment
	collection := p.db.Collection("comment")
	filter := map[string]interface{}{"_id": comment.ID}
	update := map[string]interface{}{"$set": comment}
	opts := options.Update().SetUpsert(true)

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save comment: %w", err)
	}

	return nil
}

// handleVote processes vote operations
func (p *OperationProcessor) handleVote(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid vote operation data")
	}

	vote := &database.Vote{
		ID:        fmt.Sprintf("%d/%s/%s/%s", op.Block.Number, getString(opData, "voter"), getString(opData, "author"), getString(opData, "permlink")),
		Voter:     getString(opData, "voter"),
		Author:    getString(opData, "author"),
		Permlink:  getString(opData, "permlink"),
		Weight:    int(getFloat64(opData, "weight")),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	collection := p.db.Collection("vote")
	_, err := collection.InsertOne(ctx, vote)
	if err != nil {
		return fmt.Errorf("failed to save vote: %w", err)
	}

	return nil
}

// handleTransfer processes transfer operations
func (p *OperationProcessor) handleTransfer(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid transfer operation data")
	}

	amountStr := getString(opData, "amount")
	amount, currency := parseAmount(amountStr)

	transfer := &database.Transfer{
		ID:        fmt.Sprintf("%d/%s/%s", op.Block.Number, getString(opData, "from"), getString(opData, "to")),
		From:      getString(opData, "from"),
		To:        getString(opData, "to"),
		Amount:    amount,
		Currency:  currency,
		Memo:      getString(opData, "memo"),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	collection := p.db.Collection("transfer")
	_, err := collection.InsertOne(ctx, transfer)
	if err != nil {
		return fmt.Errorf("failed to save transfer: %w", err)
	}

	return nil
}

// handleAuthorReward processes author reward operations
func (p *OperationProcessor) handleAuthorReward(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid author reward operation data")
	}

	reward := &database.AuthorReward{
		ID:            fmt.Sprintf("%d/%s/%s", op.Block.Number, getString(opData, "author"), getString(opData, "permlink")),
		Author:        getString(opData, "author"),
		Permlink:      getString(opData, "permlink"),
		SBDPayout:     parseAmountValue(getString(opData, "sbd_payout")),
		SteemPayout:   parseAmountValue(getString(opData, "steem_payout")),
		VestingPayout: parseAmountValue(getString(opData, "vesting_payout")),
		Timestamp:     op.Operation.Timestamp,
		BlockNum:      op.Block.Number,
	}

	collection := p.db.Collection("author_reward")
	_, err := collection.InsertOne(ctx, reward)
	if err != nil {
		return fmt.Errorf("failed to save author reward: %w", err)
	}

	return nil
}

// handleCurationReward processes curation reward operations
func (p *OperationProcessor) handleCurationReward(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid curation reward operation data")
	}

	reward := &database.CurationReward{
		ID:              fmt.Sprintf("%d/%s/%s/%s", op.Block.Number, getString(opData, "curator"), getString(opData, "comment_author"), getString(opData, "comment_permlink")),
		Curator:         getString(opData, "curator"),
		CommentAuthor:   getString(opData, "comment_author"),
		CommentPermlink: getString(opData, "comment_permlink"),
		Reward:          parseAmountValue(getString(opData, "reward")),
		Timestamp:       op.Operation.Timestamp,
		BlockNum:        op.Block.Number,
	}

	collection := p.db.Collection("curation_reward")
	_, err := collection.InsertOne(ctx, reward)
	if err != nil {
		return fmt.Errorf("failed to save curation reward: %w", err)
	}

	return nil
}

// handleVestingDeposit processes transfer to vesting operations
func (p *OperationProcessor) handleVestingDeposit(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid vesting deposit operation data")
	}

	deposit := &database.VestingDeposit{
		ID:        fmt.Sprintf("%d/%s/%s", op.Block.Number, getString(opData, "from"), getString(opData, "to")),
		From:      getString(opData, "from"),
		To:        getString(opData, "to"),
		Amount:    parseAmountValue(getString(opData, "amount")),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	collection := p.db.Collection("vesting_deposit")
	_, err := collection.InsertOne(ctx, deposit)
	if err != nil {
		return fmt.Errorf("failed to save vesting deposit: %w", err)
	}

	return nil
}

// handleVestingWithdraw processes vesting withdraw operations
func (p *OperationProcessor) handleVestingWithdraw(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid vesting withdraw operation data")
	}

	withdraw := &database.VestingWithdraw{
		ID:          fmt.Sprintf("%d/%s/%s", op.Block.Number, getString(opData, "from_account"), getString(opData, "to_account")),
		FromAccount: getString(opData, "from_account"),
		ToAccount:   getString(opData, "to_account"),
		Deposited:   parseAmountValue(getString(opData, "deposited")),
		Withdrawn:   parseAmountValue(getString(opData, "withdrawn")),
		Timestamp:   op.Operation.Timestamp,
		BlockNum:    op.Block.Number,
	}

	collection := p.db.Collection("vesting_withdraw")
	_, err := collection.InsertOne(ctx, withdraw)
	if err != nil {
		return fmt.Errorf("failed to save vesting withdraw: %w", err)
	}

	return nil
}

// handleConvert processes convert operations
func (p *OperationProcessor) handleConvert(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid convert operation data")
	}

	amountStr := getString(opData, "amount")
	amount, currency := parseAmount(amountStr)

	convert := &database.Convert{
		ID:        fmt.Sprintf("%d/%d", op.Block.Number, int(getFloat64(opData, "requestid"))),
		Owner:     getString(opData, "owner"),
		RequestID: int(getFloat64(opData, "requestid")),
		Amount:    amount,
		Type:      currency,
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	collection := p.db.Collection("convert")
	_, err := collection.InsertOne(ctx, convert)
	if err != nil {
		return fmt.Errorf("failed to save convert: %w", err)
	}

	return nil
}

// handleFeedPublish processes feed publish operations
func (p *OperationProcessor) handleFeedPublish(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid feed publish operation data")
	}

	feed := &database.FeedPublish{
		ID:        fmt.Sprintf("%d|%s", op.Block.Number, getString(opData, "publisher")),
		Publisher: getString(opData, "publisher"),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	// Parse exchange rate
	if exchangeRate, ok := opData["exchange_rate"].(map[string]interface{}); ok {
		feed.ExchangeRate = exchangeRate
	}

	collection := p.db.Collection("feed_publish")
	filter := map[string]interface{}{"_id": feed.ID}
	update := map[string]interface{}{"$set": feed}
	opts := options.Update().SetUpsert(true)

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save feed publish: %w", err)
	}

	return nil
}

// handleWitnessVote processes witness vote operations
func (p *OperationProcessor) handleWitnessVote(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid witness vote operation data")
	}

	vote := &database.WitnessVote{
		ID:        fmt.Sprintf("%d/%s/%s", op.Block.Number, getString(opData, "account"), getString(opData, "witness")),
		Account:   getString(opData, "account"),
		Witness:   getString(opData, "witness"),
		Approve:   getBool(opData, "approve"),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	collection := p.db.Collection("witness_vote")
	_, err := collection.InsertOne(ctx, vote)
	if err != nil {
		return fmt.Errorf("failed to save witness vote: %w", err)
	}

	return nil
}

// handleCustomJson processes custom JSON operations
func (p *OperationProcessor) handleCustomJson(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid custom json operation data")
	}

	jsonStr := getString(opData, "json")
	if jsonStr == "" {
		return nil
	}

	var data []interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil // Skip invalid JSON
	}

	if len(data) == 0 {
		return nil
	}

	opType, ok := data[0].(string)
	if !ok {
		return nil
	}

	switch opType {
	case "follow":
		return p.handleFollow(ctx, op, data)
	case "reblog":
		return p.handleReblog(ctx, op, data)
	}

	return nil
}

// handleFollow processes follow operations from custom JSON
func (p *OperationProcessor) handleFollow(ctx context.Context, op *Operation, data []interface{}) error {
	if len(data) < 2 {
		return nil
	}

	followData, ok := data[1].(map[string]interface{})
	if !ok {
		return nil
	}

	follow := &database.Follow{
		ID:        fmt.Sprintf("%d/%s/%s", op.Block.Number, getString(followData, "follower"), getString(followData, "following")),
		Follower:  getString(followData, "follower"),
		Following: getString(followData, "following"),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
		Data:      followData,
	}

	// Parse what array
	if whatInterface, ok := followData["what"].([]interface{}); ok {
		for _, w := range whatInterface {
			if what, ok := w.(string); ok {
				follow.What = append(follow.What, what)
			}
		}
	}

	collection := p.db.Collection("follow")
	filter := map[string]interface{}{"_id": follow.ID}
	update := map[string]interface{}{"$set": follow}
	opts := options.Update().SetUpsert(true)

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save follow: %w", err)
	}

	return nil
}

// handleReblog processes reblog operations from custom JSON
func (p *OperationProcessor) handleReblog(ctx context.Context, op *Operation, data []interface{}) error {
	if len(data) < 2 {
		return nil
	}

	reblogData, ok := data[1].(map[string]interface{})
	if !ok {
		return nil
	}

	reblog := &database.Reblog{
		ID:        fmt.Sprintf("%d/%s/%s/%s", op.Block.Number, getString(reblogData, "account"), getString(reblogData, "author"), getString(reblogData, "permlink")),
		Account:   getString(reblogData, "account"),
		Author:    getString(reblogData, "author"),
		Permlink:  getString(reblogData, "permlink"),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	collection := p.db.Collection("reblog")
	_, err := collection.InsertOne(ctx, reblog)
	if err != nil {
		return fmt.Errorf("failed to save reblog: %w", err)
	}

	return nil
}

// handlePow processes proof of work operations
func (p *OperationProcessor) handlePow(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid pow operation data")
	}

	pow := &database.Pow{
		ID:        fmt.Sprintf("%d/%s", op.Block.Number, getString(opData, "worker")),
		Worker:    getString(opData, "worker"),
		Signature: getString(opData, "signature"),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	// Parse input and work as maps
	if input, ok := opData["input"].(map[string]interface{}); ok {
		pow.Input = input
	}
	if work, ok := opData["work"].(map[string]interface{}); ok {
		pow.Work = work
	}

	collection := p.db.Collection("pow")
	_, err := collection.InsertOne(ctx, pow)
	if err != nil {
		return fmt.Errorf("failed to save pow: %w", err)
	}

	return nil
}

// handleCommentOptions processes comment options operations
func (p *OperationProcessor) handleCommentOptions(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid comment options operation data")
	}

	// Update the comment with options
	commentID := fmt.Sprintf("%s/%s", getString(opData, "author"), getString(opData, "permlink"))
	
	update := map[string]interface{}{
		"$set": map[string]interface{}{
			"last_update": op.Operation.Timestamp,
		},
	}

	// Add specific options if present
	if maxPayout := getString(opData, "max_accepted_payout"); maxPayout != "" {
		update["$set"].(map[string]interface{})["max_accepted_payout"] = parseAmountValue(maxPayout)
	}
	if percentSBD, ok := opData["percent_steem_dollars"].(float64); ok {
		update["$set"].(map[string]interface{})["percent_steem_dollars"] = int(percentSBD)
	}
	if allowVotes, ok := opData["allow_votes"].(bool); ok {
		update["$set"].(map[string]interface{})["allow_votes"] = allowVotes
	}
	if allowCurationRewards, ok := opData["allow_curation_rewards"].(bool); ok {
		update["$set"].(map[string]interface{})["allow_curation_rewards"] = allowCurationRewards
	}

	collection := p.db.Collection("comment")
	_, err := collection.UpdateOne(ctx, map[string]interface{}{"_id": commentID}, update)
	if err != nil {
		return fmt.Errorf("failed to update comment options: %w", err)
	}

	return nil
}

// handleBenefactorReward processes benefactor reward operations
func (p *OperationProcessor) handleBenefactorReward(ctx context.Context, op *Operation) error {
	opData, ok := op.Operation.Op[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid benefactor reward operation data")
	}

	reward := &database.BenefactorReward{
		ID:              fmt.Sprintf("%d/%s/%s/%s", op.Block.Number, getString(opData, "benefactor"), getString(opData, "author"), getString(opData, "permlink")),
		Benefactor:      getString(opData, "benefactor"),
		Author:          getString(opData, "author"),
		Permlink:        getString(opData, "permlink"),
		SBDPayout:       parseAmountValue(getString(opData, "sbd_payout")),
		SteemPayout:     parseAmountValue(getString(opData, "steem_payout")),
		VestingPayout:   parseAmountValue(getString(opData, "vesting_payout")),
		Timestamp:       op.Operation.Timestamp,
		BlockNum:        op.Block.Number,
	}

	collection := p.db.Collection("benefactor_reward")
	_, err := collection.InsertOne(ctx, reward)
	if err != nil {
		return fmt.Errorf("failed to save benefactor reward: %w", err)
	}

	return nil
}

// Utility functions
func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getFloat64(data map[string]interface{}, key string) float64 {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
	}
	return 0
}

func getBool(data map[string]interface{}, key string) bool {
	if val, ok := data[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

func parseAmount(amountStr string) (float64, string) {
	return utils.ParseAmount(amountStr)
}

func parseAmountValue(amountStr string) float64 {
	return utils.ParseAmountValue(amountStr)
}
