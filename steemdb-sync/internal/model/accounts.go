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

	return accounts
}
