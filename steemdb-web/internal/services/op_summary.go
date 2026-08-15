package services

// summaryFields maps op_type to the op_value fields surfaced in account
// history responses. The summary is generated at read time instead of being
// persisted, so it can evolve without storage migrations.
var summaryFields = map[string][]string{
	"transfer":                  {"from", "to", "amount", "memo"},
	"vote":                      {"voter", "author", "permlink", "weight"},
	"comment":                   {"author", "permlink", "title", "parent_author", "parent_permlink"},
	"curation_reward":           {"curator", "reward", "comment_author", "comment_permlink"},
	"author_reward":             {"author", "permlink", "sbd_payout", "steem_payout", "vesting_payout"},
	"comment_benefactor_reward": {"benefactor", "author", "permlink", "reward"},
	"transfer_to_vesting":       {"from", "to", "amount"},
	"fill_vesting_withdraw":     {"from_account", "to_account", "withdrawn", "deposited"},
	"convert":                   {"owner", "amount", "requestid"},
	"feed_publish":              {"publisher", "exchange_rate"},
	"account_witness_vote":      {"account", "witness", "approve"},
	"witness_update":            {"owner", "url"},
	"account_create":            {"creator", "new_account_name"},
	"custom_json":               {"id", "required_posting_auths"},
	"producer_reward":           {"producer", "vesting_shares"},
	"pow":                       {"worker_account"},
	"pow2":                      {"worker_account"},
}

// defaultSummaryFields is used for op_types without an explicit mapping.
var defaultSummaryFields = []string{"amount", "reward", "permlink", "weight"}

// buildOpSummary extracts salient op_value fields for the history UI.
func buildOpSummary(opType string, opValue map[string]interface{}) map[string]interface{} {
	if opValue == nil {
		return nil
	}

	fields, ok := summaryFields[opType]
	if !ok {
		fields = defaultSummaryFields
	}

	summary := make(map[string]interface{}, len(fields))
	for _, field := range fields {
		if v, ok := opValue[field]; ok {
			summary[field] = v
		}
	}

	if len(summary) == 0 {
		return nil
	}
	return summary
}
