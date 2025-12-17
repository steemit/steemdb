package steem

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	sdkapi "github.com/steemit/steemgosdk/api"
	protocolapi "github.com/steemit/steemutil/protocol/api"

	"github.com/steemit/steemdb/web/pkg/utils"
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
func (c *Client) GetDynamicGlobalProperties() (*DynamicGlobalProperties, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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
func (c *Client) GetBlock(blockNum int64) (*Block, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

// GetAccounts gets account information
func (c *Client) GetAccounts(names []string) ([]Account, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

// GetWitnessesByVote gets witnesses by vote
func (c *Client) GetWitnessesByVote(from string, limit int) ([]Witness, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

// GetWitnessSchedule gets the witness schedule
func (c *Client) GetWitnessSchedule() (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		var schedule map[string]interface{}
		err := api.CallWithResult("condenser_api", "get_witness_schedule", []interface{}{}, &schedule)
		if err == nil {
			return schedule, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_witness_schedule"),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
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

// GetActiveWitnesses gets the active witnesses
func (c *Client) GetActiveWitnesses() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var lastErr error
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		api := c.getCurrentAPI()
		var witnesses []string
		err := api.CallWithResult("condenser_api", "get_active_witnesses", []interface{}{}, &witnesses)
		if err == nil {
			return witnesses, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "get_active_witnesses"),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
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

// Helper functions to convert steemgosdk types to our types

func convertDynamicGlobalProperties(dgp *protocolapi.DynamicGlobalProperties) *DynamicGlobalProperties {
	if dgp == nil {
		return nil
	}

	return &DynamicGlobalProperties{
		HeadBlockNumber:              int64(dgp.HeadBlockNumber),
		HeadBlockID:                  dgp.HeadBlockId,
		Time:                         dgp.Time, // Keep as string in web version
		CurrentWitness:               dgp.CurrentWitness,
		TotalPow:                     int64(dgp.TotalPow),
		NumPowWitnesses:              int(dgp.NumPowWitnesses),
		VirtualSupply:                dgp.VirtualSupply,
		CurrentSupply:                dgp.CurrentSupply,
		ConfidentialSupply:           dgp.ConfidentialSupply,
		CurrentSBDSupply:             dgp.CurrentSbdSupply,
		ConfidentialSBDSupply:        dgp.ConfidentialSbdSupply,
		TotalVestingFundSteem:        dgp.TotalVestingFundSteem,
		TotalVestingShares:           dgp.TotalVestingShares,
		TotalRewardFundSteem:         dgp.TotalRewardFundSteem,
		TotalRewardShares2:           dgp.TotalRewardShares2,
		PendingRewardedVestingShares: dgp.PendingRewardedVestingShares,
		PendingRewardedVestingSteem:  dgp.PendingRewardedVestingSteem,
		SBDInterestRate:              int(dgp.SbdInterestRate),
		SBDPrintRate:                 int(dgp.SbdPrintRate),
		MaximumBlockSize:             int(dgp.MaximumBlockSize),
		CurrentAslot:                 int64(dgp.CurrentAslot), // int64 in web version
		RecentSlotsFilled:            dgp.RecentSlotsFilled,
		ParticipationCount:           int(dgp.ParticipationCount),
		LastIrreversibleBlockNum:     int64(dgp.LastIrreversibleBlockNum),
		VotePowerReserveRate:         int(dgp.VotePowerReserveRate),
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
		Number:           blockNum,
		Previous:         block.Previous,
		Timestamp:        timestamp,
		Witness:          block.Witness,
		TransactionRoot:  block.TransactionMerkleRoot,
		Extensions:       block.Extensions,
		WitnessSignature: block.WitnessSignature,
		Transactions:     transactions,
		BlockID:          block.BlockId,
		SigningKey:       block.SigningKey,
		TransactionIDs:   block.TransactionIds,
	}
}

func convertTransaction(tx *protocolapi.Transaction, blockNum int64, txNum int) Transaction {
	var expiration time.Time
	if tx.Expiration != nil && tx.Expiration.Time != nil {
		expiration = *tx.Expiration.Time
	}

	// Convert operations to []Operation
	ops := make([]Operation, len(tx.Operations))
	for i, op := range tx.Operations {
		// Get operation type name as string (OpType is already a string type)
		opTypeStr := string(op.Type())
		ops[i] = Operation{
			Type:  opTypeStr,
			Value: op.Data(),
		}
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
