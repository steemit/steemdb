package steem

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/steemit/steemdb/web/pkg/utils"
)

// RCAccount represents the Resource Credit state of one account as reported
// by rc_api.find_rc_accounts. The plugin serializes uint64 mana values as
// JSON strings, hence the string fields.
type RCAccount struct {
	Account   string  `json:"account"`
	RCManabar Manabar `json:"rc_manabar"`
	MaxRC     string  `json:"max_rc"`
}

// rcFindAccounts posts one pre-marshaled rc_api.find_rc_accounts request to
// the given node and returns the first rc_account, or (nil, nil) when the
// node answers with an empty rc_accounts array (account does not exist on
// chain). ctx cancels the in-flight HTTP request when the caller's context
// is done (e.g. a disconnected browser request), so the node stops working
// for nobody.
func rcFindAccounts(ctx context.Context, client *http.Client, node string, body []byte) (*RCAccount, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, node, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) // drain so the connection can be reused
		return nil, fmt.Errorf("rc_api http status %d", resp.StatusCode)
	}

	var payload struct {
		Result *struct {
			RCAccounts []*RCAccount `json:"rc_accounts"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Error != nil {
		return nil, fmt.Errorf("rc_api error: %s", payload.Error.Message)
	}
	if payload.Result == nil || len(payload.Result.RCAccounts) == 0 {
		return nil, nil
	}
	return payload.Result.RCAccounts[0], nil
}

// GetRCAccount fetches the live Resource Credit state of one account from the
// rc_api plugin.
//
// rc_api exists only as an appbase plugin API and requires NAMED JSON-RPC
// params ({"accounts": ["name"]}) — positional arrays are rejected with
// "Could not find method find_rc_accounts" (verified against api.steemit.com).
// The steemgosdk Call surface only emits array params, so this call posts the
// JSON-RPC body directly. Node rotation, per-attempt timeout and backoff
// mirror rpcCall's semantics (same constants, same abandon-and-rotate shape).
// ctx bounds the whole call: a cancelled context fails the in-flight attempt
// and short-circuits the remaining retries (the backoff sleeps are bounded
// to 6 units total, so cancellation latency stays small regardless).
func (c *Client) GetRCAccount(ctx context.Context, account string) (*RCAccount, error) {
	body, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "rc_api.find_rc_accounts",
		"params":  map[string]interface{}{"accounts": []string{account}},
		"id":      1,
	})
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= rpcMaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		node, _ := c.currentNodeAndAPI()
		result, err := callWithAttemptTimeout(c.attemptTimeout, func() (*RCAccount, error) {
			return rcFindAccounts(ctx, c.rcHTTP, node, body)
		})
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrAttemptTimeout) {
			err = fmt.Errorf("%w (node %s)", ErrAttemptTimeout, node)
		}

		lastErr = err
		c.logger.Warn("RPC call failed, retrying",
			utils.String("method", "rc_api.find_rc_accounts"),
			utils.Int("attempt", attempt+1),
			utils.String("account", account),
			utils.Error(err),
		)

		c.switchNode()

		if attempt < rpcMaxRetries {
			time.Sleep(time.Duration(attempt+1) * c.backoffUnit)
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", rpcMaxRetries+1, lastErr)
}
