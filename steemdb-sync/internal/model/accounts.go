package model

// accountFields lists op_value field names that hold Steem account names.
// The scan is type-aware: only string values (or arrays of strings) are
// collected, so same-named non-account fields (e.g. the "owner" authority
// object in account_update) are skipped naturally.
var accountFields = []string{
	"account",
	"owner",
	"from",
	"to",
	"author",
	"voter",
	"curator",
	"publisher",
	"worker_account",
	"creator",
	"new_account_name",
	"benefactor",
	"from_account",
	"to_account",
	"producer",
	"witness",
	"delegator",
	"delegatee",
	"current_owner",
	"open_owner",
	"comment_author",
	"parent_author",
	"recovering_account",
	"required_posting_auths",
	"required_auths",
}

// ExtractAccounts derives the list of accounts involved in an operation by
// scanning op_value for well-known account fields. The result is deduplicated
// with first-seen order preserved. It works for every op_type (including
// future ones) without a per-type switch.
func ExtractAccounts(opValue map[string]interface{}) []string {
	if opValue == nil {
		return nil
	}

	seen := make(map[string]bool, 4)
	accounts := make([]string, 0, 4)

	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		accounts = append(accounts, name)
	}

	for _, field := range accountFields {
		raw, ok := opValue[field]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			add(v)
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					add(s)
				}
			}
		}
	}

	// pow/pow2 carry the miner account nested inside "work"; the account
	// fields above only see top-level keys.
	if worker := ExtractWorkerAccount(opValue); worker != "" {
		add(worker)
	}

	return accounts
}

// ExtractWorkerAccount gets the worker (miner) account from a pow/pow2
// operation's op_value. Three shapes exist in the wild:
//
//   - pow: top-level worker_account field (work is a map of proof data)
//   - pow2, condenser serialization: work is a list [which, value] with
//     value.input.worker_account
//   - pow2, appbase/plugin serialization: work is a map
//     {type: "pow2"|"equihash_pow", value: {input: {worker_account}}}
//
// Returns "" when no worker can be located.
func ExtractWorkerAccount(v map[string]interface{}) string {
	if v == nil {
		return ""
	}
	switch work := v["work"].(type) {
	case []interface{}:
		if len(work) >= 2 {
			if inner, ok := work[1].(map[string]interface{}); ok {
				if account := workerFromPowValue(inner); account != "" {
					return account
				}
			}
		}
	case map[string]interface{}:
		if val, ok := work["value"].(map[string]interface{}); ok {
			if account := workerFromPowValue(val); account != "" {
				return account
			}
		}
	}
	// pow (legacy): top-level worker_account.
	if s, ok := v["worker_account"].(string); ok {
		return s
	}
	return ""
}

// workerFromPowValue reads value.input.worker_account from an unwrapped
// pow2 work variant.
func workerFromPowValue(val map[string]interface{}) string {
	if input, ok := val["input"].(map[string]interface{}); ok {
		if account, ok := input["worker_account"].(string); ok {
			return account
		}
	}
	return ""
}
