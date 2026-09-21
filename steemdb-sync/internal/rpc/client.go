package rpc

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/metrics"
	sdkapi "github.com/steemit/steemgosdk/api"
	"github.com/steemit/steemutil/protocol"
	protocolapi "github.com/steemit/steemutil/protocol/api"
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

// GetBlockWithOps was removed: its coordinate-based regular/virtual
// filtering could not distinguish the ops of a multi-op transaction (which
// share op_in_trx=0) from virtual ops reported at a triggering transaction's
// coordinates. Callers should fetch the two listings via GetBlock +
// GetOpsInBlock(false/true) and normalize coordinates with ConvertBlockOps.

// GetAccounts retrieves full account data for multiple accounts in a single RPC call.
// Uses condenser_api.get_accounts with a batch of names (up to 100 per call).
func (c *Client) GetAccounts(names []string) ([]*protocolapi.ExtendedAccount, error) {
	if len(names) == 0 {
		return nil, nil
	}
	startTime := time.Now()
	accounts, err := c.api.GetAccounts(names)
	duration := time.Since(startTime)

	metrics.RecordRPCCall("get_accounts", duration, err)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to get accounts (batch size=%d)", len(names))
	}
	return accounts, nil
}

// GetContent retrieves the full content of a post or comment via condenser_api.get_content.
// Uses steemgosdk's typed GetContent wrapper (returns *protocolapi.Content).
func (c *Client) GetContent(author, permlink string) (*protocolapi.Content, error) {
	startTime := time.Now()

	content, err := c.api.GetContent(author, permlink)

	duration := time.Since(startTime)
	metrics.RecordRPCCall("get_content", duration, err)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to get content %s/%s", author, permlink)
	}
	return content, nil
}

// GetWitnessesByVote retrieves witnesses ordered by vote count starting after
// `from` (empty string = from the top). Returns raw condenser_api maps so the
// caller controls field conversion.
func (c *Client) GetWitnessesByVote(ctx context.Context, from string, limit int) ([]map[string]interface{}, error) {
	startTime := time.Now()

	var witnesses []map[string]interface{}
	err := c.api.CallWithResult("condenser_api", "get_witnesses_by_vote", []interface{}{from, limit}, &witnesses)

	duration := time.Since(startTime)
	metrics.RecordRPCCall("get_witnesses_by_vote", duration, err)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to get witnesses by vote (from=%q, limit=%d)", from, limit)
	}
	return witnesses, nil
}

// GetRewardFund retrieves a reward fund object (e.g. "post") as a raw map.
func (c *Client) GetRewardFund(ctx context.Context, name string) (map[string]interface{}, error) {
	startTime := time.Now()

	var fund map[string]interface{}
	err := c.api.CallWithResult("condenser_api", "get_reward_fund", []interface{}{name}, &fund)

	duration := time.Since(startTime)
	metrics.RecordRPCCall("get_reward_fund", duration, err)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to get reward fund %q", name)
	}
	return fund, nil
}

// LookupAccounts retrieves up to `limit` account names lexicographically
// greater than `after` (condenser_api.lookup_accounts). Used for full rescan
// paging; -1 as after returns from the start.
func (c *Client) LookupAccounts(ctx context.Context, after string, limit int) ([]string, error) {
	startTime := time.Now()

	var names []string
	err := c.api.CallWithResult("condenser_api", "lookup_accounts", []interface{}{after, limit}, &names)

	duration := time.Since(startTime)
	metrics.RecordRPCCall("lookup_accounts", duration, err)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to lookup accounts (after=%q, limit=%d)", after, limit)
	}
	return names, nil
}
