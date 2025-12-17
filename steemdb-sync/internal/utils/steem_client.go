package utils

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	sdkapi "github.com/steemit/steemgosdk/api"
	protocolapi "github.com/steemit/steemutil/protocol/api"
)

// SteemClient wraps steemgosdk API with retry and node switching
type SteemClient struct {
	nodes       []string
	currentNode int
	mutex       sync.RWMutex
	logger      Logger
	apis        []*sdkapi.API
}

// NewSteemClient creates a new Steem RPC client
func NewSteemClient(nodes []string, logger Logger) *SteemClient {
	if len(nodes) == 0 {
		nodes = []string{"https://api.steemit.com"}
	}

	apis := make([]*sdkapi.API, len(nodes))
	for i, node := range nodes {
		apis[i] = sdkapi.NewAPI(node)
		apis[i].SetMaxRetry(3)
	}

	return &SteemClient{
		nodes:       nodes,
		currentNode: rand.Intn(len(nodes)),
		logger:      logger,
		apis:        apis,
	}
}

func (c *SteemClient) getCurrentAPI() *sdkapi.API {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.apis[c.currentNode]
}

func (c *SteemClient) switchNode() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.currentNode = (c.currentNode + 1) % len(c.nodes)
	c.logger.Debug("Switched to node", String("node", c.nodes[c.currentNode]))
}

// GetDynamicGlobalProperties gets the dynamic global properties
func (c *SteemClient) GetDynamicGlobalProperties(ctx context.Context) (*protocolapi.DynamicGlobalProperties, error) {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		dgp, err := api.GetDynamicGlobalProperties()
		if err == nil {
			return dgp, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			String("method", "get_dynamic_global_properties"),
			Int("attempt", attempt+1),
			Error(err),
		)

		c.switchNode()

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
// Returns blocks and their block numbers
func (c *SteemClient) GetBlocksRange(ctx context.Context, startBlock, endBlock int64) ([]*protocolapi.Block, []int64, error) {
	if startBlock >= endBlock {
		return nil, nil, fmt.Errorf("invalid block range: startBlock (%d) must be less than endBlock (%d)", startBlock, endBlock)
	}

	const maxBatchSize = 100
	if endBlock-startBlock > maxBatchSize {
		return nil, nil, fmt.Errorf("block range too large: %d blocks (max: %d)", endBlock-startBlock, maxBatchSize)
	}

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()

		var wrapBlocks []*sdkapi.WrapBlock
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic in GetBlocks: %v", r)
				}
			}()
			wrapBlocks, err = api.GetBlocks(uint(startBlock), uint(endBlock))
		}()

		if err == nil && wrapBlocks != nil {
			blocks := make([]*protocolapi.Block, 0, len(wrapBlocks))
			blockNums := make([]int64, 0, len(wrapBlocks))
			for _, wrapBlock := range wrapBlocks {
				if wrapBlock == nil || wrapBlock.Block == nil {
					return nil, nil, fmt.Errorf("received nil block")
				}
				blocks = append(blocks, wrapBlock.Block)
				blockNums = append(blockNums, int64(wrapBlock.BlockNum))
			}
			return blocks, blockNums, nil
		}

		if err == nil && wrapBlocks == nil {
			err = fmt.Errorf("GetBlocks returned nil without error")
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			String("method", "get_blocks_range"),
			Int64("start_block", startBlock),
			Int64("end_block", endBlock),
			Int("attempt", attempt+1),
			Error(err),
		)

		c.switchNode()

		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return nil, nil, fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// CallWithResult performs an RPC call and unmarshals the result
func (c *SteemClient) CallWithResult(ctx context.Context, apiName, method string, params []interface{}, result interface{}) error {
	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		err := api.CallWithResult(apiName, method, params, result)
		if err == nil {
			return nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			String("method", fmt.Sprintf("%s.%s", apiName, method)),
			Int("attempt", attempt+1),
			Error(err),
		)

		c.switchNode()

		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}

	return fmt.Errorf("RPC call failed after %d attempts: %w", maxRetries+1, lastErr)
}

// GetAccounts gets account information
func (c *SteemClient) GetAccounts(ctx context.Context, names []string) ([]Account, error) {
	var accounts []Account
	err := c.CallWithResult(ctx, "condenser_api", "get_accounts", []interface{}{names}, &accounts)
	return accounts, err
}

// GetWitnessesByVote gets witnesses by vote
func (c *SteemClient) GetWitnessesByVote(ctx context.Context, from string, limit int) ([]Witness, error) {
	var witnesses []Witness
	err := c.CallWithResult(ctx, "condenser_api", "get_witnesses_by_vote", []interface{}{from, limit}, &witnesses)
	return witnesses, err
}

// GetRewardFund gets reward fund information
func (c *SteemClient) GetRewardFund(ctx context.Context, name string) (*RewardFund, error) {
	var fund RewardFund
	err := c.CallWithResult(ctx, "condenser_api", "get_reward_fund", []interface{}{name}, &fund)
	if err != nil {
		return nil, err
	}
	return &fund, nil
}

// GetContent gets post/comment content
func (c *SteemClient) GetContent(ctx context.Context, author, permlink string) (*Content, error) {
	var content Content
	err := c.CallWithResult(ctx, "condenser_api", "get_content", []interface{}{author, permlink}, &content)
	if err != nil {
		return nil, err
	}
	return &content, nil
}

// LookupAccounts looks up accounts by name pattern
func (c *SteemClient) LookupAccounts(ctx context.Context, lowerBoundName string, limit int) ([]string, error) {
	var accounts []string
	err := c.CallWithResult(ctx, "condenser_api", "lookup_accounts", []interface{}{lowerBoundName, limit}, &accounts)
	return accounts, err
}
