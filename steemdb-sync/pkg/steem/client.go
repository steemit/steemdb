package steem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/steemdb/sync/internal/utils"
)

type Client struct {
	nodes       []string
	httpClient  *http.Client
	currentNode int
	mutex       sync.RWMutex
	logger      utils.Logger
	retryPolicy RetryPolicy
}

type RetryPolicy struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

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

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// NewClient creates a new Steem RPC client
func NewClient(nodes []string, logger utils.Logger) *Client {
	if len(nodes) == 0 {
		nodes = []string{"https://api.steemit.com"}
	}

	return &Client{
		nodes: nodes,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		currentNode: rand.Intn(len(nodes)),
		logger:      logger,
		retryPolicy: RetryPolicy{
			MaxRetries:   3,
			InitialDelay: 1 * time.Second,
			MaxDelay:     10 * time.Second,
			Multiplier:   2.0,
		},
	}
}

// call makes an RPC call to the Steem node
func (c *Client) call(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	request := RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	var lastErr error
	for attempt := 0; attempt <= c.retryPolicy.MaxRetries; attempt++ {
		result, err := c.makeRequest(ctx, request)
		if err == nil {
			return result, nil
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", method),
			utils.Int("attempt", attempt+1),
			utils.Error(err),
		)

		// Switch to next node on error
		c.switchNode()

		// Wait before retry (except on last attempt)
		if attempt < c.retryPolicy.MaxRetries {
			delay := time.Duration(float64(c.retryPolicy.InitialDelay) * 
				(1 << attempt)) // Exponential backoff
			if delay > c.retryPolicy.MaxDelay {
				delay = c.retryPolicy.MaxDelay
			}
			
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", c.retryPolicy.MaxRetries+1, lastErr)
}

func (c *Client) makeRequest(ctx context.Context, request RPCRequest) (json.RawMessage, error) {
	// Get current node
	c.mutex.RLock()
	nodeURL := c.nodes[c.currentNode]
	c.mutex.RUnlock()

	// Marshal request
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", nodeURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Make HTTP request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	// Parse RPC response
	var rpcResp RPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal RPC response: %w", err)
	}

	// Check for RPC error
	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}

	return rpcResp.Result, nil
}

func (c *Client) switchNode() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.currentNode = (c.currentNode + 1) % len(c.nodes)
	c.logger.Debug("Switched to node", utils.String("node", c.nodes[c.currentNode]))
}

// GetDynamicGlobalProperties gets the dynamic global properties
func (c *Client) GetDynamicGlobalProperties(ctx context.Context) (*DynamicGlobalProperties, error) {
	result, err := c.call(ctx, "condenser_api.get_dynamic_global_properties", []interface{}{})
	if err != nil {
		return nil, err
	}

	var props DynamicGlobalProperties
	if err := json.Unmarshal(result, &props); err != nil {
		return nil, fmt.Errorf("failed to unmarshal dynamic global properties: %w", err)
	}

	return &props, nil
}

// GetBlock gets a block by number
func (c *Client) GetBlock(ctx context.Context, blockNum int64) (*Block, error) {
	result, err := c.call(ctx, "condenser_api.get_block", []interface{}{blockNum})
	if err != nil {
		return nil, err
	}

	var block Block
	if err := json.Unmarshal(result, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	block.Number = blockNum
	return &block, nil
}

// GetOpsInBlock gets operations in a block
func (c *Client) GetOpsInBlock(ctx context.Context, blockNum int64, onlyVirtual bool) ([]Operation, error) {
	result, err := c.call(ctx, "condenser_api.get_ops_in_block", []interface{}{blockNum, onlyVirtual})
	if err != nil {
		return nil, err
	}

	var ops []Operation
	if err := json.Unmarshal(result, &ops); err != nil {
		return nil, fmt.Errorf("failed to unmarshal operations: %w", err)
	}

	return ops, nil
}

// GetAccounts gets account information
func (c *Client) GetAccounts(ctx context.Context, names []string) ([]Account, error) {
	result, err := c.call(ctx, "condenser_api.get_accounts", []interface{}{names})
	if err != nil {
		return nil, err
	}

	var accounts []Account
	if err := json.Unmarshal(result, &accounts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal accounts: %w", err)
	}

	return accounts, nil
}

// LookupAccounts looks up accounts by name pattern
func (c *Client) LookupAccounts(ctx context.Context, lowerBoundName string, limit int) ([]string, error) {
	result, err := c.call(ctx, "condenser_api.lookup_accounts", []interface{}{lowerBoundName, limit})
	if err != nil {
		return nil, err
	}

	var accounts []string
	if err := json.Unmarshal(result, &accounts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal account names: %w", err)
	}

	return accounts, nil
}

// GetWitnessesByVote gets witnesses by vote
func (c *Client) GetWitnessesByVote(ctx context.Context, from string, limit int) ([]Witness, error) {
	result, err := c.call(ctx, "condenser_api.get_witnesses_by_vote", []interface{}{from, limit})
	if err != nil {
		return nil, err
	}

	var witnesses []Witness
	if err := json.Unmarshal(result, &witnesses); err != nil {
		return nil, fmt.Errorf("failed to unmarshal witnesses: %w", err)
	}

	return witnesses, nil
}

// GetRewardFund gets reward fund information
func (c *Client) GetRewardFund(ctx context.Context, name string) (*RewardFund, error) {
	result, err := c.call(ctx, "condenser_api.get_reward_fund", []interface{}{name})
	if err != nil {
		return nil, err
	}

	var fund RewardFund
	if err := json.Unmarshal(result, &fund); err != nil {
		return nil, fmt.Errorf("failed to unmarshal reward fund: %w", err)
	}

	return &fund, nil
}

// GetContent gets content (post/comment)
func (c *Client) GetContent(ctx context.Context, author, permlink string) (*Content, error) {
	result, err := c.call(ctx, "condenser_api.get_content", []interface{}{author, permlink})
	if err != nil {
		return nil, err
	}

	var content Content
	if err := json.Unmarshal(result, &content); err != nil {
		return nil, fmt.Errorf("failed to unmarshal content: %w", err)
	}

	return &content, nil
}

// BatchRequest performs multiple RPC calls in a single HTTP request
func (c *Client) BatchRequest(ctx context.Context, requests []RPCRequest) ([]RPCResponse, error) {
	// Get current node
	c.mutex.RLock()
	nodeURL := c.nodes[c.currentNode]
	c.mutex.RUnlock()

	// Marshal requests
	requestBody, err := json.Marshal(requests)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", nodeURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Make HTTP request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	// Parse RPC responses
	var rpcResponses []RPCResponse
	if err := json.Unmarshal(body, &rpcResponses); err != nil {
		return nil, fmt.Errorf("failed to unmarshal batch RPC response: %w", err)
	}

	return rpcResponses, nil
}
