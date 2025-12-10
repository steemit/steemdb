package steem

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	sdkapi "github.com/steemit/steemgosdk/api"
	protocolapi "github.com/steemit/steemutil/protocol/api"

	"github.com/steemdb/sync/internal/utils"
)

type Client struct {
	nodes       []string
	currentNode int
	mutex       sync.RWMutex
	logger      utils.Logger
	apis        []*sdkapi.API // One API instance per node
}

// NewClient creates a new Steem RPC client using steemgosdk
func NewClient(nodes []string, logger utils.Logger) *Client {
	if len(nodes) == 0 {
		nodes = []string{"https://api.steemit.com"}
	}

	// Create API instances for each node
	apis := make([]*sdkapi.API, len(nodes))
	for i, node := range nodes {
		apis[i] = sdkapi.NewAPI(node)
		apis[i].SetMaxRetry(3)
	}

	return &Client{
		nodes:       nodes,
		currentNode: rand.Intn(len(nodes)),
		logger:      logger,
		apis:        apis,
	}
}

// getCurrentAPI returns the current API instance
func (c *Client) getCurrentAPI() *sdkapi.API {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.apis[c.currentNode]
}

// switchNode switches to the next available node
func (c *Client) switchNode() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.currentNode = (c.currentNode + 1) % len(c.nodes)
	c.logger.Debug("Switched to node", utils.String("node", c.nodes[c.currentNode]))
}

// GetDynamicGlobalProperties gets the dynamic global properties
func (c *Client) GetDynamicGlobalProperties(ctx context.Context) (*DynamicGlobalProperties, error) {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		dgp, err := api.GetDynamicGlobalProperties()
		if err == nil {
			return convertDynamicGlobalProperties(dgp), nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_dynamic_global_properties"),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// GetBlock gets a block by number
func (c *Client) GetBlock(ctx context.Context, blockNum int64) (*Block, error) {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		block, err := api.GetBlock(uint(blockNum))
		if err == nil {
			return convertBlock(block, blockNum), nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_block"),
			utils.Int64("block_num", blockNum),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// GetOpsInBlock gets operations in a block
func (c *Client) GetOpsInBlock(ctx context.Context, blockNum int64, onlyVirtual bool) ([]Operation, error) {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		result, err := api.Call("condenser_api", "get_ops_in_block", []interface{}{blockNum, onlyVirtual})
		if err == nil {
			// Convert result to our Operation type
			var ops []Operation
			resultBytes, err := json.Marshal(result.Result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal operations: %w", err)
			}
			if err := json.Unmarshal(resultBytes, &ops); err != nil {
				return nil, fmt.Errorf("failed to unmarshal operations: %w", err)
			}
			return ops, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_ops_in_block"),
			utils.Int64("block_num", blockNum),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// GetBlocksRange gets multiple blocks in the range [startBlock, endBlock)
// Uses steemgosdk's GetBlocks which fetches blocks concurrently
func (c *Client) GetBlocksRange(ctx context.Context, startBlock, endBlock int64) ([]*Block, error) {
	// Validate parameters
	if startBlock >= endBlock {
		return nil, fmt.Errorf("invalid block range: startBlock (%d) must be less than endBlock (%d)", startBlock, endBlock)
	}

	// Limit batch size to prevent excessive memory usage
	maxBatchSize := int64(100)
	if endBlock-startBlock > maxBatchSize {
		return nil, fmt.Errorf("block range too large: %d blocks (max: %d)", endBlock-startBlock, maxBatchSize)
	}

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		
		// Call steemgosdk's GetBlocks which uses concurrent goroutines
		wrapBlocks, err := api.GetBlocks(uint(startBlock), uint(endBlock))
		if err == nil {
			// Convert WrapBlock to Block
			blocks := make([]*Block, len(wrapBlocks))
			for i, wrapBlock := range wrapBlocks {
				if wrapBlock == nil || wrapBlock.Block == nil {
					return nil, fmt.Errorf("received nil block at index %d (blockNum: %d)", i, startBlock+int64(i))
				}
				blocks[i] = convertBlock(wrapBlock.Block, int64(wrapBlock.BlockNum))
			}
			return blocks, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_blocks_range"),
			utils.Int64("start_block", startBlock),
			utils.Int64("end_block", endBlock),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// GetAccounts gets account information
func (c *Client) GetAccounts(ctx context.Context, names []string) ([]Account, error) {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		var accounts []Account
		err := api.CallWithResult("condenser_api", "get_accounts", []interface{}{names}, &accounts)
		if err == nil {
			return accounts, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_accounts"),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// LookupAccounts looks up accounts by name pattern
func (c *Client) LookupAccounts(ctx context.Context, lowerBoundName string, limit int) ([]string, error) {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		var accounts []string
		err := api.CallWithResult("condenser_api", "lookup_accounts", []interface{}{lowerBoundName, limit}, &accounts)
		if err == nil {
			return accounts, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "lookup_accounts"),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// GetWitnessesByVote gets witnesses by vote
func (c *Client) GetWitnessesByVote(ctx context.Context, from string, limit int) ([]Witness, error) {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		var witnesses []Witness
		err := api.CallWithResult("condenser_api", "get_witnesses_by_vote", []interface{}{from, limit}, &witnesses)
		if err == nil {
			return witnesses, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_witnesses_by_vote"),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// GetRewardFund gets reward fund information
func (c *Client) GetRewardFund(ctx context.Context, name string) (*RewardFund, error) {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		var fund RewardFund
		err := api.CallWithResult("condenser_api", "get_reward_fund", []interface{}{name}, &fund)
		if err == nil {
			return &fund, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_reward_fund"),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// GetContent gets content (post/comment)
func (c *Client) GetContent(ctx context.Context, author, permlink string) (*Content, error) {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		var content Content
		err := api.CallWithResult("condenser_api", "get_content", []interface{}{author, permlink}, &content)
		if err == nil {
			return &content, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_content"),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// BatchRequest performs multiple RPC calls in a single HTTP request
// Note: steemgosdk doesn't directly support batch requests, so we'll make individual calls
func (c *Client) BatchRequest(ctx context.Context, requests []RPCRequest) ([]RPCResponse, error) {
	// For now, we'll make individual calls and combine results
	// This is not ideal but maintains compatibility
	responses := make([]RPCResponse, len(requests))
	api := c.getCurrentAPI()

	for i, req := range requests {
		// Parse method name (e.g., "condenser_api.get_block" -> apiName="condenser_api", method="get_block")
		dotIndex := strings.Index(req.Method, ".")
		if dotIndex == -1 {
			responses[i] = RPCResponse{
				JSONRPC: "2.0",
				Error: &RPCError{
					Code:    -1,
					Message: "invalid method format",
				},
				ID: req.ID,
			}
			continue
		}

		apiName := req.Method[:dotIndex]
		method := req.Method[dotIndex+1:]

		result, err := api.Call(apiName, method, req.Params)
		if err != nil {
			responses[i] = RPCResponse{
				JSONRPC: "2.0",
				Error: &RPCError{
					Code:    -1,
					Message: err.Error(),
				},
				ID: req.ID,
			}
		} else {
			resultBytes, _ := json.Marshal(result.Result)
			responses[i] = RPCResponse{
				JSONRPC: "2.0",
				Result:  resultBytes,
				ID:      req.ID,
			}
		}
	}

	return responses, nil
}

// Helper functions to convert steemgosdk types to our types

func convertDynamicGlobalProperties(dgp *protocolapi.DynamicGlobalProperties) *DynamicGlobalProperties {
	if dgp == nil {
		return nil
	}

	// Convert time string to time.Time
	var timeVal time.Time
	if dgp.Time != "" {
		timeVal, _ = time.Parse(time.RFC3339, dgp.Time)
	}

	return &DynamicGlobalProperties{
		HeadBlockNumber:            int64(dgp.HeadBlockNumber),
		HeadBlockID:                dgp.HeadBlockId,
		Time:                       timeVal,
		CurrentWitness:             dgp.CurrentWitness,
		TotalPow:                   int64(dgp.TotalPow),
		NumPowWitnesses:            int(dgp.NumPowWitnesses),
		VirtualSupply:              dgp.VirtualSupply,
		CurrentSupply:              dgp.CurrentSupply,
		ConfidentialSupply:         dgp.ConfidentialSupply,
		CurrentSBDSupply:           dgp.CurrentSbdSupply,
		ConfidentialSBDSupply:      dgp.ConfidentialSbdSupply,
		TotalVestingFundSteem:      dgp.TotalVestingFundSteem,
		TotalVestingShares:         dgp.TotalVestingShares,
		TotalRewardFundSteem:       dgp.TotalRewardFundSteem,
		TotalRewardShares2:         dgp.TotalRewardShares2,
		PendingRewardedVestingShares: dgp.PendingRewardedVestingShares,
		PendingRewardedVestingSteem:  dgp.PendingRewardedVestingSteem,
		SBDInterestRate:            int(dgp.SbdInterestRate),
		SBDPrintRate:               int(dgp.SbdPrintRate),
		MaximumBlockSize:           int(dgp.MaximumBlockSize),
		CurrentAslot:               int(dgp.CurrentAslot),
		RecentSlotsFilled:          dgp.RecentSlotsFilled,
		ParticipationCount:         int(dgp.ParticipationCount),
		LastIrreversibleBlockNum:   int64(dgp.LastIrreversibleBlockNum),
		VotePowerReserveRate:       int(dgp.VotePowerReserveRate),
	}
}

func convertBlock(block *protocolapi.Block, blockNum int64) *Block {
	if block == nil {
		return nil
	}

	// Convert transactions
	transactions := make([]Transaction, len(block.Transactions))
	for i, tx := range block.Transactions {
		transactions[i] = convertTransaction(&tx, blockNum, i)
	}

	// Convert time to time.Time
	var timestamp time.Time
	if block.Timestamp != nil && block.Timestamp.Time != nil {
		timestamp = *block.Timestamp.Time
	}

	return &Block{
		Number:          blockNum,
		Previous:        block.Previous,
		Timestamp:       timestamp,
		Witness:         block.Witness,
		TransactionRoot: block.TransactionMerkleRoot,
		Extensions:      block.Extensions,
		WitnessSignature: block.WitnessSignature,
		Transactions:    transactions,
		BlockID:         block.BlockId,
		SigningKey:      block.SigningKey,
		TransactionIDs:  block.TransactionIds,
	}
}

func convertTransaction(tx *protocolapi.Transaction, blockNum int64, txNum int) Transaction {
	var expiration time.Time
	if tx.Expiration != nil && tx.Expiration.Time != nil {
		expiration = *tx.Expiration.Time
	}

	// Convert operations to [][]interface{}
	ops := make([][]interface{}, len(tx.Operations))
	for i, op := range tx.Operations {
		// Get operation type name as string (OpType is already a string type)
		opTypeStr := string(op.Type())
		ops[i] = []interface{}{opTypeStr, op.Data()}
	}

	return Transaction{
		RefBlockNum:    int(tx.RefBlockNum),
		RefBlockPrefix: int64(tx.RefBlockPrefix),
		Expiration:     expiration,
		Operations:     ops,
		Extensions:     tx.Extensions,
		Signatures:     tx.Signatures,
		TransactionID:  tx.TransactionId,
		BlockNum:       blockNum,
		TransactionNum: txNum,
	}
}

// RPCRequest and RPCResponse types for batch requests (kept for compatibility)
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
	ID      int             `json:"id"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
