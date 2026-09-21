package steem

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"

	sdkapi "github.com/steemit/steemgosdk/api"
	steemprotocol "github.com/steemit/steemutil/protocol"
	protocolapi "github.com/steemit/steemutil/protocol/api"

	"github.com/steemit/steemdb/web/pkg/utils"
)

// Per-attempt RPC timeout. The steemgosdk methods take no context, so a hung
// node (TCP half-open, stalled response) can only be bounded by wrapping each
// attempt with its own timer — see callWithAttemptTimeout. The timeout is
// deliberately PER ATTEMPT, not a shared budget across retries: a single
// shared budget would be consumed entirely by the first hung node, defeating
// the node-rotation retry design (we would never reach a healthy node).
// Worst case per logical call is therefore attempts*timeout + backoff
// (4*10s + 6s = 46s) — bounded, instead of the previous behavior where the
// 10s ctx only covered the backoff sleeps and a hung node stalled the caller
// until the SDK-internal HTTP timeout fired (30s per attempt, ~126s total,
// or forever if the SDK ever regresses).
const rpcAttemptTimeout = 10 * time.Second

// Backoff between retries: attempt n waits (n+1)*rpcBackoffUnit, capped by
// rpcBackoffBudget overall (the budget only guards the sleeps; attempts are
// bounded by rpcAttemptTimeout).
const (
	rpcMaxRetries    = 3
	rpcBackoffUnit   = 1 * time.Second
	rpcBackoffBudget = 10 * time.Second
)

// ErrAttemptTimeout is returned when a single RPC attempt does not complete
// within rpcAttemptTimeout. The abandoned attempt keeps running in the
// background until the SDK call returns on its own (steemutil's jsonrpc2
// carries a 30s http.Client timeout); the buffered result channel makes the
// wrapper goroutine exit cleanly then, so abandoned attempts do not leak.
var ErrAttemptTimeout = errors.New("steem rpc: attempt timed out")

type Client struct {
	nodes       []string
	currentNode int
	mutex       sync.RWMutex
	logger      utils.Logger
	apis        []*sdkapi.API // One API instance per node

	// Test seams: override the per-attempt timeout and the backoff unit to
	// keep timeout/retry tests fast. Production code uses the constants.
	attemptTimeout time.Duration
	backoffUnit    time.Duration
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
		nodes:          nodes,
		currentNode:    rand.Intn(len(nodes)),
		logger:         logger,
		apis:           apis,
		attemptTimeout: rpcAttemptTimeout,
		backoffUnit:    rpcBackoffUnit,
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

// callWithAttemptTimeout runs fn, a context-unaware SDK call, and abandons it
// if it does not finish within timeout. The result channel is buffered so a
// late fn result is delivered without blocking (and the wrapper goroutine can
// exit) even though nobody reads it anymore.
func callWithAttemptTimeout[T any](timeout time.Duration, fn func() (T, error)) (T, error) {
	type attemptResult struct {
		val T
		err error
	}
	ch := make(chan attemptResult, 1)
	go func() {
		val, err := fn()
		ch <- attemptResult{val: val, err: err}
	}()

	select {
	case res := <-ch:
		return res.val, res.err
	case <-time.After(timeout):
		var zero T
		return zero, ErrAttemptTimeout
	}
}

// rpcCall executes one logical RPC: up to rpcMaxRetries+1 attempts, each
// bounded by c.attemptTimeout (per attempt — see rpcAttemptTimeout for why
// this is not one shared budget), rotating to the next node after every
// failure and backing off (attempt+1)*backoffUnit between attempts, with the
// sleeps capped by rpcBackoffBudget. It is a free function (not a method)
// because Go does not allow methods to declare type parameters.
func rpcCall[T any](c *Client, method string, call func(*sdkapi.API) (T, error), extra ...zap.Field) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rpcBackoffBudget)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= rpcMaxRetries; attempt++ {
		api := c.getCurrentAPI()
		result, err := callWithAttemptTimeout(c.attemptTimeout, func() (T, error) {
			return call(api)
		})
		if err == nil {
			return result, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			append(append([]zap.Field{
				utils.String("method", method),
				utils.Int("attempt", attempt+1),
			}, extra...), utils.Error(err))...,
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < rpcMaxRetries {
			select {
			case <-ctx.Done():
				var zero T
				return zero, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * c.backoffUnit):
			}
		}
	}

	var zero T
	return zero, fmt.Errorf("RPC call failed after %d attempts: %w", rpcMaxRetries+1, lastErr)
}

// GetDynamicGlobalProperties gets the dynamic global properties
func (c *Client) GetDynamicGlobalProperties() (*DynamicGlobalProperties, error) {
	dgp, err := rpcCall(c, "get_dynamic_global_properties", func(api *sdkapi.API) (*protocolapi.DynamicGlobalProperties, error) {
		return api.GetDynamicGlobalProperties()
	})
	if err != nil {
		return nil, err
	}
	return convertDynamicGlobalProperties(dgp), nil
}

// GetBlock gets a block by number
func (c *Client) GetBlock(blockNum int64) (*Block, error) {
	block, err := rpcCall(c, "get_block",
		func(api *sdkapi.API) (*protocolapi.Block, error) {
			return api.GetBlock(uint(blockNum))
		},
		utils.Int64("block_num", blockNum))
	if err != nil {
		return nil, err
	}
	return convertBlock(block, blockNum), nil
}

// GetOpsInBlock gets the operations in a block. When onlyVirtual is true only
// virtual operations are returned; otherwise all operations are returned.
func (c *Client) GetOpsInBlock(blockNum int64, onlyVirtual bool) ([]*steemprotocol.OperationObject, error) {
	return rpcCall(c, "get_ops_in_block",
		func(api *sdkapi.API) ([]*steemprotocol.OperationObject, error) {
			return api.GetOpsInBlock(uint(blockNum), onlyVirtual)
		},
		utils.Int64("block_num", blockNum))
}

// GetAccounts gets account information
func (c *Client) GetAccounts(names []string) ([]Account, error) {
	return rpcCall(c, "get_accounts", func(api *sdkapi.API) ([]Account, error) {
		var accounts []Account
		if err := api.CallWithResult("condenser_api", "get_accounts", []interface{}{names}, &accounts); err != nil {
			return nil, err
		}
		return accounts, nil
	})
}

// GetWitnessesByVote gets witnesses by vote
func (c *Client) GetWitnessesByVote(from string, limit int) ([]Witness, error) {
	return rpcCall(c, "get_witnesses_by_vote", func(api *sdkapi.API) ([]Witness, error) {
		var witnesses []Witness
		if err := api.CallWithResult("condenser_api", "get_witnesses_by_vote", []interface{}{from, limit}, &witnesses); err != nil {
			return nil, err
		}
		return witnesses, nil
	})
}

// GetWitnessByAccount gets a single witness by account name
func (c *Client) GetWitnessByAccount(account string) (*Witness, error) {
	witness, err := rpcCall(c, "get_witness_by_account",
		func(api *sdkapi.API) (*Witness, error) {
			var witness Witness
			if err := api.CallWithResult("condenser_api", "get_witness_by_account", []interface{}{account}, &witness); err != nil {
				return nil, err
			}
			return &witness, nil
		},
		utils.String("account", account))
	if err != nil {
		return nil, err
	}
	if witness.Owner == "" {
		return nil, nil
	}
	return witness, nil
}

// GetTransaction gets a transaction by transaction ID
func (c *Client) GetTransaction(txID string) (map[string]interface{}, error) {
	return rpcCall(c, "get_transaction",
		func(api *sdkapi.API) (map[string]interface{}, error) {
			var tx map[string]interface{}
			if err := api.CallWithResult("condenser_api", "get_transaction", []interface{}{txID}, &tx); err != nil {
				return nil, err
			}
			return tx, nil
		},
		utils.String("tx_id", txID))
}

// GetWitnessSchedule gets the witness schedule
func (c *Client) GetWitnessSchedule() (map[string]interface{}, error) {
	return rpcCall(c, "get_witness_schedule", func(api *sdkapi.API) (map[string]interface{}, error) {
		var schedule map[string]interface{}
		if err := api.CallWithResult("condenser_api", "get_witness_schedule", []interface{}{}, &schedule); err != nil {
			return nil, err
		}
		return schedule, nil
	})
}

// GetActiveWitnesses gets the active witnesses
func (c *Client) GetActiveWitnesses() ([]string, error) {
	return rpcCall(c, "get_active_witnesses", func(api *sdkapi.API) ([]string, error) {
		var witnesses []string
		if err := api.CallWithResult("condenser_api", "get_active_witnesses", []interface{}{}, &witnesses); err != nil {
			return nil, err
		}
		return witnesses, nil
	})
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
