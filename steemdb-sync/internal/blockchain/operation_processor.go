package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/steemdb/sync/internal/database"
	"github.com/steemdb/sync/internal/utils"
)

// Operation represents a blockchain operation with context
type Operation struct {
	Block     *utils.Block
	Operation *utils.Operation
}

// Collection interface for collection operations
type Collection interface {
	InsertOne(ctx context.Context, document any) (*mongo.InsertOneResult, error)
	InsertMany(ctx context.Context, documents []any) (*mongo.InsertManyResult, error)
	UpdateOne(ctx context.Context, filter any, update any, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error)
	FindOne(ctx context.Context, filter any) *mongo.SingleResult
}

// mongoCollectionAdapter adapts *mongo.Collection to Collection interface
type mongoCollectionAdapter struct {
	*mongo.Collection
}

func (a *mongoCollectionAdapter) InsertOne(ctx context.Context, document any) (*mongo.InsertOneResult, error) {
	return a.Collection.InsertOne(ctx, document)
}

func (a *mongoCollectionAdapter) InsertMany(ctx context.Context, documents []any) (*mongo.InsertManyResult, error) {
	return a.Collection.InsertMany(ctx, documents)
}

func (a *mongoCollectionAdapter) UpdateOne(ctx context.Context, filter any, update any, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	return a.Collection.UpdateOne(ctx, filter, update, opts...)
}

func (a *mongoCollectionAdapter) FindOne(ctx context.Context, filter any) *mongo.SingleResult {
	return a.Collection.FindOne(ctx, filter)
}

// Database interface for database operations
type Database interface {
	Collection(name string) Collection
	MarkAccountNeedsUpdate(ctx context.Context, accountName string) error
}

// mongoDatabaseAdapter adapts *database.MongoDB to Database interface
type mongoDatabaseAdapter struct {
	db *database.MongoDB
}

func (a *mongoDatabaseAdapter) Collection(name string) Collection {
	return &mongoCollectionAdapter{Collection: a.db.Collection(name)}
}

func (a *mongoDatabaseAdapter) MarkAccountNeedsUpdate(ctx context.Context, accountName string) error {
	return a.db.MarkAccountNeedsUpdate(ctx, accountName)
}

// OperationProcessor processes blockchain operations
type OperationProcessor struct {
	db       Database
	logger   utils.Logger
	handlers map[string]OperationHandler

	// Batch buffers for account_operations
	accountOpBuffer []any
	bufferMutex     sync.Mutex
	bufferSize      int
}

// OperationHandler is a function that processes a specific operation type
type OperationHandler func(ctx context.Context, op *Operation) error

// NewOperationProcessor creates a new operation processor
func NewOperationProcessor(db *database.MongoDB, logger utils.Logger) *OperationProcessor {
	p := &OperationProcessor{
		db:              &mongoDatabaseAdapter{db: db},
		logger:          logger,
		handlers:        make(map[string]OperationHandler),
		accountOpBuffer: make([]any, 0, 100), // Initial capacity for batch writes
		bufferSize:      100,                 // Flush when buffer reaches this size
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Save operation to operations collection first
	opID, err := p.saveOperation(ctx, op, opType)
	if err != nil {
		p.logger.Error("Failed to save operation",
			utils.String("type", opType),
			utils.Int64("block", op.Block.Number),
			utils.Error(err),
		)
		// Continue processing even if save fails
	}

	// Process with handler
	handler, exists := p.handlers[opType]
	if !exists {
		p.logger.Debug("Unknown operation type", utils.String("type", opType))
		// Still add to account_operations buffer even if no handler
		if opID != nil {
			if err := p.addAccountOperationsToBuffer(ctx, op, opType, *opID); err != nil {
				p.logger.Error("Failed to add account operations to buffer",
					utils.String("type", opType),
					utils.Int64("block", op.Block.Number),
					utils.Error(err),
				)
			}
		}
		return nil
	}

	if err := handler(ctx, op); err != nil {
		return err
	}

	// Add to account_operations buffer (will be flushed in batch)
	if opID != nil {
		if err := p.addAccountOperationsToBuffer(ctx, op, opType, *opID); err != nil {
			p.logger.Error("Failed to add account operations to buffer",
				utils.String("type", opType),
				utils.Int64("block", op.Block.Number),
				utils.Error(err),
			)
			// Don't fail the whole operation if buffer add fails
		}
	}

	return nil
}

// FlushAccountOperationsBuffer flushes the account_operations buffer
func (p *OperationProcessor) FlushAccountOperationsBuffer(ctx context.Context) error {
	p.bufferMutex.Lock()
	if len(p.accountOpBuffer) == 0 {
		p.bufferMutex.Unlock()
		return nil
	}

	buffer := make([]any, len(p.accountOpBuffer))
	copy(buffer, p.accountOpBuffer)
	p.accountOpBuffer = p.accountOpBuffer[:0]
	p.bufferMutex.Unlock()

	if len(buffer) > 0 {
		collection := p.db.Collection("account_operations")
		_, err := collection.InsertMany(ctx, buffer)
		if err != nil {
			return fmt.Errorf("failed to flush account operations buffer: %w", err)
		}
		p.logger.Debug("Flushed account operations buffer",
			utils.Int("count", len(buffer)),
		)
	}

	return nil
}

// saveOperation saves operation to operations collection
func (p *OperationProcessor) saveOperation(ctx context.Context, op *Operation, opType string) (*primitive.ObjectID, error) {
	opData, err := getOperationData(op)
	if err != nil {
		// Log detailed error information for debugging
		if op != nil && op.Block != nil && op.Operation != nil {
			p.logger.Error("Invalid operation data",
				utils.String("type", opType),
				utils.Int64("block", op.Block.Number),
				utils.String("trx_id", op.Operation.TrxID),
				utils.Int("op_index", op.Operation.OpInTrx),
				utils.Error(err),
			)
		} else {
			p.logger.Error("Invalid operation data",
				utils.String("type", opType),
				utils.Error(err),
			)
		}
		return nil, fmt.Errorf("invalid operation data: %w", err)
	}

	// Extract accounts from operation
	accounts := p.extractAccounts(opType, opData)
	primaryAccount := ""
	if len(accounts) > 0 {
		primaryAccount = accounts[0]
	}

	// Calculate date and hour indices
	dateIndex := op.Operation.Timestamp.Format("2006-01-02")
	hourIndex := op.Operation.Timestamp.Hour()

	// Determine op_index (operation index in transaction)
	opIndex := op.Operation.OpInTrx
	if opIndex < 0 {
		opIndex = 0
	}

	// Determine trx_in_block (transaction index in block)
	trxInBlock := op.Operation.TrxInBlock
	if trxInBlock < 0 {
		trxInBlock = 0
	}

	dbOp := &database.Operation{
		ID:             primitive.NewObjectID(),
		BlockNum:       op.Block.Number,
		BlockTime:      op.Operation.Timestamp,
		TrxID:          op.Operation.TrxID,
		TrxInBlock:     trxInBlock,
		OpType:         opType,
		OpIndex:        opIndex,
		Data:           opData,
		Accounts:       accounts,
		PrimaryAccount: primaryAccount,
		DateIndex:      dateIndex,
		HourIndex:      hourIndex,
	}

	collection := p.db.Collection("operations")
	_, err = collection.InsertOne(ctx, dbOp)
	if err != nil {
		return nil, fmt.Errorf("failed to save operation: %w", err)
	}

	return &dbOp.ID, nil
}

// addAccountOperationsToBuffer adds account operations to buffer for batch writing
func (p *OperationProcessor) addAccountOperationsToBuffer(ctx context.Context, op *Operation, opType string, opID primitive.ObjectID) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid operation data: %w", err)
	}

	// Extract accounts
	accounts := p.extractAccounts(opType, opData)
	if len(accounts) == 0 {
		return nil // No accounts to index
	}

	// Create summary (key information to avoid JOIN)
	summary := p.createOperationSummary(opType, opData)

	// Add account_operations to buffer
	p.bufferMutex.Lock()
	defer p.bufferMutex.Unlock()

	for _, account := range accounts {
		accountOp := &database.AccountOperation{
			ID:        primitive.NewObjectID(),
			Account:   account,
			BlockNum:  op.Block.Number,
			BlockTime: op.Operation.Timestamp,
			OpType:    opType,
			OpID:      opID,
			TrxID:     op.Operation.TrxID,
			Summary:   summary,
		}
		p.accountOpBuffer = append(p.accountOpBuffer, accountOp)

		// Flush buffer if it reaches the threshold
		if len(p.accountOpBuffer) >= p.bufferSize {
			buffer := make([]any, len(p.accountOpBuffer))
			copy(buffer, p.accountOpBuffer)
			p.accountOpBuffer = p.accountOpBuffer[:0]
			p.bufferMutex.Unlock()

			// Flush buffer asynchronously
			go func(buf []any) {
				flushCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				collection := p.db.Collection("account_operations")
				if _, err := collection.InsertMany(flushCtx, buf); err != nil {
					p.logger.Error("Failed to flush account operations buffer",
						utils.Int("count", len(buf)),
						utils.Error(err),
					)
				}
			}(buffer)

			p.bufferMutex.Lock()
		}
	}

	return nil
}

// extractAccounts extracts account names from operation data
func (p *OperationProcessor) extractAccounts(opType string, opData map[string]any) []string {
	accounts := make([]string, 0)

	switch opType {
	case "transfer":
		if from := getString(opData, "from"); from != "" {
			accounts = append(accounts, from)
		}
		if to := getString(opData, "to"); to != "" {
			accounts = append(accounts, to)
		}
	case "vote":
		if voter := getString(opData, "voter"); voter != "" {
			accounts = append(accounts, voter)
		}
		if author := getString(opData, "author"); author != "" {
			accounts = append(accounts, author)
		}
	case "comment":
		if author := getString(opData, "author"); author != "" {
			accounts = append(accounts, author)
		}
	case "transfer_to_vesting":
		if from := getString(opData, "from"); from != "" {
			accounts = append(accounts, from)
		}
		if to := getString(opData, "to"); to != "" {
			accounts = append(accounts, to)
		}
	case "fill_vesting_withdraw":
		if fromAccount := getString(opData, "from_account"); fromAccount != "" {
			accounts = append(accounts, fromAccount)
		}
		if toAccount := getString(opData, "to_account"); toAccount != "" {
			accounts = append(accounts, toAccount)
		}
	case "account_witness_vote":
		if account := getString(opData, "account"); account != "" {
			accounts = append(accounts, account)
		}
	case "author_reward":
		if author := getString(opData, "author"); author != "" {
			accounts = append(accounts, author)
		}
	case "curation_reward":
		if curator := getString(opData, "curator"); curator != "" {
			accounts = append(accounts, curator)
		}
	case "comment_benefactor_reward":
		if benefactor := getString(opData, "benefactor"); benefactor != "" {
			accounts = append(accounts, benefactor)
		}
	case "convert":
		if owner := getString(opData, "owner"); owner != "" {
			accounts = append(accounts, owner)
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, acc := range accounts {
		if !seen[acc] {
			seen[acc] = true
			result = append(result, acc)
		}
	}

	return result
}

// createOperationSummary creates a summary of operation for account_operations
func (p *OperationProcessor) createOperationSummary(opType string, opData map[string]any) bson.M {
	summary := bson.M{
		"op_type": opType,
	}

	switch opType {
	case "transfer":
		summary["from"] = getString(opData, "from")
		summary["to"] = getString(opData, "to")
		summary["amount"] = getString(opData, "amount")
	case "vote":
		summary["voter"] = getString(opData, "voter")
		summary["author"] = getString(opData, "author")
		summary["permlink"] = getString(opData, "permlink")
		summary["weight"] = getFloat64(opData, "weight")
	case "comment":
		summary["author"] = getString(opData, "author")
		summary["permlink"] = getString(opData, "permlink")
		summary["title"] = getString(opData, "title")
	case "author_reward":
		summary["author"] = getString(opData, "author")
		summary["permlink"] = getString(opData, "permlink")
		summary["sbd_payout"] = getString(opData, "sbd_payout")
		summary["steem_payout"] = getString(opData, "steem_payout")
	case "curation_reward":
		summary["curator"] = getString(opData, "curator")
		summary["reward"] = getString(opData, "reward")
	}

	return summary
}

// handleComment processes comment operations
func (p *OperationProcessor) handleComment(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid comment operation data: %w", err)
	}

	author := getString(opData, "author")
	permlink := getString(opData, "permlink")
	parentPermlink := getString(opData, "parent_permlink")

	comment := &database.Comment{
		ID:             fmt.Sprintf("%s/%s", author, permlink),
		Author:         author,
		Permlink:       permlink,
		Title:          getString(opData, "title"),
		Body:           getString(opData, "body"),
		ParentAuthor:   getString(opData, "parent_author"),
		ParentPermlink: parentPermlink,
		Created:        op.Operation.Timestamp,
		LastUpdate:     op.Operation.Timestamp,
		BlockNum:       op.Block.Number,
		Scanned:        time.Now(),
		AuthorLower:    strings.ToLower(author),
		CategoryLower:  strings.ToLower(parentPermlink),
		DateIndex:      op.Operation.Timestamp.Format("2006-01-02"),
	}

	// Parse JSON metadata
	if jsonMeta := getString(opData, "json_metadata"); jsonMeta != "" {
		var metadata map[string]any
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
		parentCollection := p.db.Collection("comments")
		var parent database.Comment
		err := parentCollection.FindOne(ctx, map[string]any{
			"_id": fmt.Sprintf("%s/%s", comment.ParentAuthor, comment.ParentPermlink),
		}).Decode(&parent)
		if err == nil {
			comment.Depth = parent.Depth + 1
		} else {
			comment.Depth = 1
		}
	}

	// Save comment to comments collection
	collection := p.db.Collection("comments")
	filter := map[string]any{"_id": comment.ID}
	update := map[string]any{"$set": comment}
	opts := options.Update().SetUpsert(true)

	_, err = collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save comment: %w", err)
	}

	// Mark author account needs update
	if author := getString(opData, "author"); author != "" {
		if err := p.db.MarkAccountNeedsUpdate(ctx, author); err != nil {
			p.logger.Debug("Failed to mark account needs update",
				utils.String("account", author),
				utils.Error(err),
			)
		}
	}

	return nil
}

// handleVote processes vote operations
func (p *OperationProcessor) handleVote(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid vote operation data: %w", err)
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
	_, err = collection.InsertOne(ctx, vote)
	if err != nil {
		return fmt.Errorf("failed to save vote: %w", err)
	}

	// Mark voter and author accounts need update
	if voter := getString(opData, "voter"); voter != "" {
		p.db.MarkAccountNeedsUpdate(ctx, voter)
	}
	if author := getString(opData, "author"); author != "" {
		p.db.MarkAccountNeedsUpdate(ctx, author)
	}

	return nil
}

// handleTransfer processes transfer operations
func (p *OperationProcessor) handleTransfer(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid transfer operation data: %w", err)
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
	_, err = collection.InsertOne(ctx, transfer)
	if err != nil {
		return fmt.Errorf("failed to save transfer: %w", err)
	}

	// Mark from and to accounts need update
	if from := getString(opData, "from"); from != "" {
		p.db.MarkAccountNeedsUpdate(ctx, from)
	}
	if to := getString(opData, "to"); to != "" {
		p.db.MarkAccountNeedsUpdate(ctx, to)
	}

	return nil
}

// handleAuthorReward processes author reward operations
func (p *OperationProcessor) handleAuthorReward(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid author reward operation data: %w", err)
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
	_, err = collection.InsertOne(ctx, reward)
	if err != nil {
		return fmt.Errorf("failed to save author reward: %w", err)
	}

	// Mark author account needs update
	if author := getString(opData, "author"); author != "" {
		p.db.MarkAccountNeedsUpdate(ctx, author)
	}

	return nil
}

// handleCurationReward processes curation reward operations
func (p *OperationProcessor) handleCurationReward(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid curation reward operation data: %w", err)
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
	_, err = collection.InsertOne(ctx, reward)
	if err != nil {
		return fmt.Errorf("failed to save curation reward: %w", err)
	}

	// Mark curator account needs update
	if curator := getString(opData, "curator"); curator != "" {
		p.db.MarkAccountNeedsUpdate(ctx, curator)
	}

	return nil
}

// handleVestingDeposit processes transfer to vesting operations
func (p *OperationProcessor) handleVestingDeposit(ctx context.Context, op *Operation) error {
	if len(op.Operation.Op) < 2 {
		p.logger.Error("Invalid operation format: Op array too short",
			utils.Int("length", len(op.Operation.Op)),
			utils.Int64("block", op.Block.Number),
			utils.String("trx_id", op.Operation.TrxID),
		)
		return fmt.Errorf("invalid vesting deposit operation data: Op array too short")
	}

	opDataRaw := op.Operation.Op[1]
	opData, ok := opDataRaw.(map[string]any)
	if !ok {
		// Try to convert to map via JSON marshaling/unmarshaling
		// This handles cases where op.Data() returns a struct instead of a map
		jsonBytes, err := json.Marshal(opDataRaw)
		if err != nil {
			p.logger.Error("Invalid vesting deposit operation data: failed to marshal",
				utils.String("type", fmt.Sprintf("%T", opDataRaw)),
				utils.Int64("block", op.Block.Number),
				utils.String("trx_id", op.Operation.TrxID),
				utils.Int("op_index", op.Operation.OpInTrx),
				utils.Error(err),
			)
			return fmt.Errorf("invalid vesting deposit operation data: failed to marshal %T: %w", opDataRaw, err)
		}

		if err := json.Unmarshal(jsonBytes, &opData); err != nil {
			p.logger.Error("Invalid vesting deposit operation data: failed to unmarshal",
				utils.String("type", fmt.Sprintf("%T", opDataRaw)),
				utils.Int64("block", op.Block.Number),
				utils.String("trx_id", op.Operation.TrxID),
				utils.Int("op_index", op.Operation.OpInTrx),
				utils.Error(err),
			)
			return fmt.Errorf("invalid vesting deposit operation data: failed to unmarshal: %w", err)
		}
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

	// Mark from and to accounts need update
	if from := getString(opData, "from"); from != "" {
		p.db.MarkAccountNeedsUpdate(ctx, from)
	}
	if to := getString(opData, "to"); to != "" {
		p.db.MarkAccountNeedsUpdate(ctx, to)
	}

	return nil
}

// handleVestingWithdraw processes vesting withdraw operations
func (p *OperationProcessor) handleVestingWithdraw(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid vesting withdraw operation data: %w", err)
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
	_, err = collection.InsertOne(ctx, withdraw)
	if err != nil {
		return fmt.Errorf("failed to save vesting withdraw: %w", err)
	}

	// Mark from_account and to_account need update
	if fromAccount := getString(opData, "from_account"); fromAccount != "" {
		p.db.MarkAccountNeedsUpdate(ctx, fromAccount)
	}
	if toAccount := getString(opData, "to_account"); toAccount != "" {
		p.db.MarkAccountNeedsUpdate(ctx, toAccount)
	}

	return nil
}

// handleConvert processes convert operations
func (p *OperationProcessor) handleConvert(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid convert operation data: %w", err)
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
	_, err = collection.InsertOne(ctx, convert)
	if err != nil {
		return fmt.Errorf("failed to save convert: %w", err)
	}

	// Mark owner account needs update
	if owner := getString(opData, "owner"); owner != "" {
		p.db.MarkAccountNeedsUpdate(ctx, owner)
	}

	return nil
}

// handleFeedPublish processes feed publish operations
func (p *OperationProcessor) handleFeedPublish(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid feed publish operation data: %w", err)
	}

	feed := &database.FeedPublish{
		ID:        fmt.Sprintf("%d|%s", op.Block.Number, getString(opData, "publisher")),
		Publisher: getString(opData, "publisher"),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	// Parse exchange rate
	if exchangeRate, ok := opData["exchange_rate"].(map[string]any); ok {
		feed.ExchangeRate = exchangeRate
	}

	collection := p.db.Collection("feed_publish")
	filter := map[string]any{"_id": feed.ID}
	update := map[string]any{"$set": feed}
	opts := options.Update().SetUpsert(true)

	_, err = collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save feed publish: %w", err)
	}

	return nil
}

// handleWitnessVote processes witness vote operations
func (p *OperationProcessor) handleWitnessVote(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid witness vote operation data: %w", err)
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
	_, err = collection.InsertOne(ctx, vote)
	if err != nil {
		return fmt.Errorf("failed to save witness vote: %w", err)
	}

	// Mark account needs update
	if account := getString(opData, "account"); account != "" {
		p.db.MarkAccountNeedsUpdate(ctx, account)
	}

	return nil
}

// handleCustomJson processes custom JSON operations
func (p *OperationProcessor) handleCustomJson(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid custom json operation data: %w", err)
	}

	jsonStr := getString(opData, "json")
	if jsonStr == "" {
		return nil
	}

	var data []any
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
func (p *OperationProcessor) handleFollow(ctx context.Context, op *Operation, data []any) error {
	if len(data) < 2 {
		return nil
	}

	followData, ok := data[1].(map[string]any)
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
	if whatInterface, ok := followData["what"].([]any); ok {
		for _, w := range whatInterface {
			if what, ok := w.(string); ok {
				follow.What = append(follow.What, what)
			}
		}
	}

	collection := p.db.Collection("follow")
	filter := map[string]any{"_id": follow.ID}
	update := map[string]any{"$set": follow}
	opts := options.Update().SetUpsert(true)

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save follow: %w", err)
	}

	// Mark follower account needs update
	if follower := getString(followData, "follower"); follower != "" {
		p.db.MarkAccountNeedsUpdate(ctx, follower)
	}

	return nil
}

// handleReblog processes reblog operations from custom JSON
func (p *OperationProcessor) handleReblog(ctx context.Context, op *Operation, data []any) error {
	if len(data) < 2 {
		return nil
	}

	reblogData, ok := data[1].(map[string]any)
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

	// Mark account needs update
	if account := getString(reblogData, "account"); account != "" {
		p.db.MarkAccountNeedsUpdate(ctx, account)
	}

	return nil
}

// handlePow processes proof of work operations
func (p *OperationProcessor) handlePow(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid pow operation data: %w", err)
	}

	pow := &database.Pow{
		ID:        fmt.Sprintf("%d/%s", op.Block.Number, getString(opData, "worker")),
		Worker:    getString(opData, "worker"),
		Signature: getString(opData, "signature"),
		Timestamp: op.Operation.Timestamp,
		BlockNum:  op.Block.Number,
	}

	// Parse input and work as maps
	if input, ok := opData["input"].(map[string]any); ok {
		pow.Input = input
	}
	if work, ok := opData["work"].(map[string]any); ok {
		pow.Work = work
	}

	collection := p.db.Collection("pow")
	_, err = collection.InsertOne(ctx, pow)
	if err != nil {
		return fmt.Errorf("failed to save pow: %w", err)
	}

	return nil
}

// handleCommentOptions processes comment options operations
func (p *OperationProcessor) handleCommentOptions(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid comment options operation data: %w", err)
	}

	// Update the comment with options
	commentID := fmt.Sprintf("%s/%s", getString(opData, "author"), getString(opData, "permlink"))

	update := map[string]any{
		"$set": map[string]any{
			"last_update": op.Operation.Timestamp,
		},
	}

	// Add specific options if present
	if maxPayout := getString(opData, "max_accepted_payout"); maxPayout != "" {
		update["$set"].(map[string]any)["max_accepted_payout"] = parseAmountValue(maxPayout)
	}
	if percentSBD, ok := opData["percent_steem_dollars"].(float64); ok {
		update["$set"].(map[string]any)["percent_steem_dollars"] = int(percentSBD)
	}
	if allowVotes, ok := opData["allow_votes"].(bool); ok {
		update["$set"].(map[string]any)["allow_votes"] = allowVotes
	}
	if allowCurationRewards, ok := opData["allow_curation_rewards"].(bool); ok {
		update["$set"].(map[string]any)["allow_curation_rewards"] = allowCurationRewards
	}

	collection := p.db.Collection("comments")
	_, err = collection.UpdateOne(ctx, map[string]any{"_id": commentID}, update)
	if err != nil {
		return fmt.Errorf("failed to update comment options: %w", err)
	}

	// Mark author account needs update
	if author := getString(opData, "author"); author != "" {
		p.db.MarkAccountNeedsUpdate(ctx, author)
	}

	return nil
}

// handleBenefactorReward processes benefactor reward operations
func (p *OperationProcessor) handleBenefactorReward(ctx context.Context, op *Operation) error {
	opData, err := getOperationData(op)
	if err != nil {
		return fmt.Errorf("invalid benefactor reward operation data: %w", err)
	}

	reward := &database.BenefactorReward{
		ID:            fmt.Sprintf("%d/%s/%s/%s", op.Block.Number, getString(opData, "benefactor"), getString(opData, "author"), getString(opData, "permlink")),
		Benefactor:    getString(opData, "benefactor"),
		Author:        getString(opData, "author"),
		Permlink:      getString(opData, "permlink"),
		SBDPayout:     parseAmountValue(getString(opData, "sbd_payout")),
		SteemPayout:   parseAmountValue(getString(opData, "steem_payout")),
		VestingPayout: parseAmountValue(getString(opData, "vesting_payout")),
		Timestamp:     op.Operation.Timestamp,
		BlockNum:      op.Block.Number,
	}

	collection := p.db.Collection("benefactor_reward")
	_, err = collection.InsertOne(ctx, reward)
	if err != nil {
		return fmt.Errorf("failed to save benefactor reward: %w", err)
	}

	// Mark benefactor account needs update
	if benefactor := getString(opData, "benefactor"); benefactor != "" {
		p.db.MarkAccountNeedsUpdate(ctx, benefactor)
	}

	return nil
}

// Utility functions

// getOperationData extracts operation data from Op array, handling both map and struct types
// According to steemutil README:
// - Known operations: op.Data() returns the operation struct itself (e.g., *VoteOperation, *TransferOperation)
// - Unknown operations: op.Data() returns *json.RawMessage
func getOperationData(op *Operation) (map[string]any, error) {
	if op == nil || op.Operation == nil {
		return nil, fmt.Errorf("invalid operation: operation is nil")
	}

	if op.Operation.Op == nil {
		return nil, fmt.Errorf("invalid operation format: Op array is nil")
	}

	if len(op.Operation.Op) < 2 {
		return nil, fmt.Errorf("invalid operation format: Op array too short (length: %d, expected: >= 2)", len(op.Operation.Op))
	}

	opDataRaw := op.Operation.Op[1]
	if opDataRaw == nil {
		return nil, fmt.Errorf("invalid operation format: Op[1] is nil")
	}

	// Try direct type assertion first (for map[string]any)
	opData, ok := opDataRaw.(map[string]any)
	if ok {
		return opData, nil
	}

	// Check if it's *json.RawMessage (unknown operations from steemutil)
	// According to steemutil README, unknown operations return *json.RawMessage
	if rawJSON, ok := opDataRaw.(*json.RawMessage); ok {
		var data map[string]any
		if err := json.Unmarshal(*rawJSON, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal json.RawMessage: %w", err)
		}
		return data, nil
	}

	// Try to convert to map via JSON marshaling/unmarshaling
	// This handles cases where op.Data() returns a struct (known operations) instead of a map
	jsonBytes, err := json.Marshal(opDataRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal operation data %T: %w", opDataRaw, err)
	}

	if err := json.Unmarshal(jsonBytes, &opData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal operation data: %w", err)
	}

	return opData, nil
}

func getString(data map[string]any, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getFloat64(data map[string]any, key string) float64 {
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

func getBool(data map[string]any, key string) bool {
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
