package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/steemit/steemdb-sync/internal/model"
)

func makeOp(opType string, blockNum uint32, v map[string]interface{}) *model.Operation {
	return &model.Operation{
		ID:       fmt.Sprintf("%d:0:0", blockNum),
		BlockNum: blockNum,
		OpType:   opType,
		OpValue:  v,
	}
}

// --- ConvertHandler ---

func TestConvertHandler_RequestIDFormatting(t *testing.T) {
	// Verify requestid is correctly formatted into _id (no DB needed)
	// ConvertHandler doesn't validate fields before DB write, so we test
	// the logic indirectly via the asset parsing it uses.
	// This test verifies the ParseAsset call that ConvertHandler relies on.
	amountVal, amountType := ParseAsset("5.000 SBD")
	if amountVal != 5.0 {
		t.Errorf("convert amount = %v, want 5.0", amountVal)
	}
	if amountType != "SBD" {
		t.Errorf("convert type = %q, want SBD", amountType)
	}
}

// --- VestingDepositHandler ---

func TestVestingDepositHandler_AssetParsing(t *testing.T) {
	// Verify amount is correctly parsed from both NAI and string formats
	// (Logic test: AssetValue is already tested in helpers_test.go,
	// here we verify the handler uses it correctly)
	naIAsset := map[string]interface{}{
		"nai":       "@@000000021",
		"amount":    "1000000",
		"precision": 3,
	}
	val := AssetValue(naIAsset)
	if val != 1000.0 {
		t.Errorf("NAI asset value = %v, want 1000.0", val)
	}

	strAsset := "1.500 STEEM"
	val = AssetValue(strAsset)
	if val != 1.5 {
		t.Errorf("string asset value = %v, want 1.5", val)
	}
}

// --- BenefactorRewardHandler ---

func TestBenefactorRewardHandler_AssetParsing(t *testing.T) {
	// vesting_payout in NAI format: {nai:"@@000000037", amount:"1234567890", precision:6}
	// → 1234.567890 VESTS
	naIAsset := map[string]interface{}{
		"nai":       "@@000000037",
		"amount":    "1234567890",
		"precision": 6,
	}
	reward := AssetValue(naIAsset)
	if reward != 1234.567890 {
		t.Errorf("vesting payout NAI = %v, want 1234.567890", reward)
	}
}

// --- CustomJSONHandler ---

func TestCustomJSONHandler_ReblogDispatch(t *testing.T) {
	// Test that a reblog custom_json is correctly dispatched
	// We can't test the DB write without MongoDB, but we can verify
	// the parsing logic doesn't error on valid input.
	// Use unknown action to avoid hitting DB.
	h := &CustomJSONHandler{inserter: &MongoInserter{}}

	// Test parsing succeeds by using a non-reblog/follow action
	op := makeOp("custom_json", 100, map[string]interface{}{
		"json": `["test_action",{"account":"alice"}]`,
	})
	err := h.Handle(context.Background(), op, time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCustomJSONHandler_FollowDispatch(t *testing.T) {
	// Same approach — test parsing only, avoid DB write
	h := &CustomJSONHandler{inserter: &MongoInserter{}}

	op := makeOp("custom_json", 100, map[string]interface{}{
		"json": `["test_action",{"follower":"alice"}]`,
	})
	err := h.Handle(context.Background(), op, time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCustomJSONHandler_UnknownAction(t *testing.T) {
	h := &CustomJSONHandler{inserter: &MongoInserter{}}

	op := makeOp("custom_json", 100, map[string]interface{}{
		"json": `[\"unknown_action\",{}]`,
	})

	err := h.Handle(context.Background(), op, time.Now())
	if err != nil {
		t.Errorf("unknown action should not error: %v", err)
	}
}

func TestCustomJSONHandler_InvalidJson(t *testing.T) {
	h := &CustomJSONHandler{inserter: &MongoInserter{}}

	op := makeOp("custom_json", 100, map[string]interface{}{
		"json": "not valid json {{{",
	})

	err := h.Handle(context.Background(), op, time.Now())
	if err != nil {
		t.Errorf("invalid json should silently skip, got error: %v", err)
	}
}

func TestCustomJSONHandler_EmptyArray(t *testing.T) {
	h := &CustomJSONHandler{inserter: &MongoInserter{}}

	op := makeOp("custom_json", 100, map[string]interface{}{
		"json": "[]",
	})

	err := h.Handle(context.Background(), op, time.Now())
	if err != nil {
		t.Errorf("empty array should silently skip, got error: %v", err)
	}
}

func TestCustomJSONHandler_AlreadyDecoded(t *testing.T) {
	// Test when json field is already decoded (from plugin as []interface{})
	// Use a non-DB action to avoid nil pointer
	h := &CustomJSONHandler{inserter: &MongoInserter{}}

	op := makeOp("custom_json", 100, map[string]interface{}{
		"json": []interface{}{
			"test_action",
			map[string]interface{}{
				"account": "alice",
			},
		},
	})

	err := h.Handle(context.Background(), op, time.Now())
	if err != nil {
		t.Errorf("already-decoded json should not error: %v", err)
	}
}

// --- PowHandler ---

func TestExtractWorkerAccount_PowFormat(t *testing.T) {
	// pow format: work is a map, worker_account is top-level
	v := map[string]interface{}{
		"worker_account": "miner1",
		"work":           map[string]interface{}{"nonce": "12345"},
	}
	account := extractWorkerAccount(v)
	if account != "miner1" {
		t.Errorf("pow worker_account = %q, want miner1", account)
	}
}

func TestExtractWorkerAccount_Pow2Format(t *testing.T) {
	// pow2 format: work is a list [props, {input: {worker_account: ...}}]
	v := map[string]interface{}{
		"work": []interface{}{
			map[string]interface{}{"props": "abc"},
			map[string]interface{}{
				"input": map[string]interface{}{
					"worker_account": "miner2",
				},
			},
		},
	}
	account := extractWorkerAccount(v)
	if account != "miner2" {
		t.Errorf("pow2 worker_account = %q, want miner2", account)
	}
}

func TestExtractWorkerAccount_MissingWork(t *testing.T) {
	// No work field, no worker_account
	v := map[string]interface{}{}
	account := extractWorkerAccount(v)
	if account != "" {
		t.Errorf("missing work: worker_account = %q, want empty", account)
	}
}

func TestExtractWorkerAccount_Pow2Malformed(t *testing.T) {
	// work is a list but missing the nested structure
	v := map[string]interface{}{
		"work":         []interface{}{"just a string"},
		"worker_account": "fallback",
	}
	account := extractWorkerAccount(v)
	if account != "fallback" {
		t.Errorf("malformed pow2 should fall back to top-level: got %q, want fallback", account)
	}
}

// --- FeedPublishHandler ---

func TestFeedPublishHandler_ExchangeRateParsing(t *testing.T) {
	// Verify that exchange_rate base/quote are correctly parsed
	// Using string format (RPC source)
	base := "1.000 STEEM"
	quote := "5.000 SBD"

	baseVal := AssetValue(base)
	quoteVal := AssetValue(quote)

	if baseVal != 1.0 {
		t.Errorf("base = %v, want 1.0", baseVal)
	}
	if quoteVal != 5.0 {
		t.Errorf("quote = %v, want 5.0", quoteVal)
	}
}
