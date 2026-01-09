package rpc

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	sdkapi "github.com/steemit/steemgosdk/api"
	protocolapi "github.com/steemit/steemutil/protocol/api"
	"github.com/steemit/steemutil/protocol"
	"github.com/steemit/steemdb-sync/internal/metrics"
)

// Client wraps steemgosdk API for RPC communication
type Client struct {
	api      *sdkapi.API
	endpoint string
	maxRetry int
	timeout  time.Duration
}

// NewClient creates a new RPC client
func NewClient(endpoint string, maxRetry int, timeout time.Duration) *Client {
	api := sdkapi.NewAPI(endpoint)
	api.SetMaxRetry(maxRetry)

	return &Client{
		api:      api,
		endpoint: endpoint,
		maxRetry: maxRetry,
		timeout:  timeout,
	}
}

// GetBlock retrieves a block by block number
func (c *Client) GetBlock(ctx context.Context, blockNum uint32) (*protocolapi.Block, error) {
	startTime := time.Now()
	block, err := c.api.GetBlock(uint(blockNum))
	duration := time.Since(startTime)
	
	// Record metrics
	metrics.RecordRPCCall("get_block", duration, err)
	
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get block %d", blockNum)
	}
	return block, nil
}

// GetOpsInBlock retrieves operations in a block
// If onlyVirtual is false, returns all operations (both regular and virtual)
// If onlyVirtual is true, returns only virtual operations
func (c *Client) GetOpsInBlock(ctx context.Context, blockNum uint32, onlyVirtual bool) ([]*protocol.OperationObject, error) {
	startTime := time.Now()
	ops, err := c.api.GetOpsInBlock(uint(blockNum), onlyVirtual)
	duration := time.Since(startTime)
	
	// Record metrics
	method := "get_ops_in_block"
	if onlyVirtual {
		method = "get_ops_in_block_virtual"
	}
	metrics.RecordRPCCall(method, duration, err)
	
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get ops in block %d", blockNum)
	}
	return ops, nil
}

// GetBlockWithOps retrieves both block and all operations (regular + virtual)
func (c *Client) GetBlockWithOps(ctx context.Context, blockNum uint32) (*protocolapi.Block, []*protocol.OperationObject, []*protocol.OperationObject, error) {
	// Get block
	block, err := c.GetBlock(ctx, blockNum)
	if err != nil {
		return nil, nil, nil, err
	}

	// Get regular operations
	regularOps, err := c.GetOpsInBlock(ctx, blockNum, false)
	if err != nil {
		return nil, nil, nil, err
	}

	// Get virtual operations
	virtualOps, err := c.GetOpsInBlock(ctx, blockNum, true)
	if err != nil {
		return nil, nil, nil, err
	}

	// Filter: regular ops = all ops - virtual ops
	regularOpsMap := make(map[string]bool)
	for _, op := range regularOps {
		// Use a unique key for each operation
		key := fmt.Sprintf("%d:%d:%d", op.BlockNumber, op.TransactionInBlock, op.OperationInTransaction)
		regularOpsMap[key] = true
	}

	var filteredRegularOps []*protocol.OperationObject
	for _, op := range regularOps {
		key := fmt.Sprintf("%d:%d:%d", op.BlockNumber, op.TransactionInBlock, op.OperationInTransaction)
		if regularOpsMap[key] {
			// Check if it's not in virtual ops
			isVirtual := false
			for _, vop := range virtualOps {
				if vop.BlockNumber == op.BlockNumber &&
					vop.TransactionInBlock == op.TransactionInBlock &&
					vop.OperationInTransaction == op.OperationInTransaction {
					isVirtual = true
					break
				}
			}
			if !isVirtual {
				filteredRegularOps = append(filteredRegularOps, op)
			}
		}
	}

	return block, filteredRegularOps, virtualOps, nil
}
