# Account Document Creation Rule

**Only account-creation operations may create account documents.** Any other
operation that carries an account name is NOT an account creation and must
never insert into the `account` collection.

## The complete list of account creators

Verified against `steem` (`libraries/chain/steem_evaluator.cpp`,
`database.cpp` — all `create<account_object>` sites):

| Source | Era | Notes |
|--------|-----|-------|
| `account_create` | all | paid creation |
| `account_create_with_delegation` | HF17+ | delegation + fee |
| `create_claimed_account` | HF20+ | spends a claimed-account slot |
| `pow` | pre-HF0.13 | mining; creates only when the worker does not exist |
| `pow2` | HF0.13–HF0.17 | same |
| genesis | block 0 | `miners`, `null`, `temp`, `initminer` (no op) |

`claim_account` only increments `pending_claimed_accounts` — it does NOT
create an account. Steem accounts are never deleted, so the set of created
accounts equals the set of existing accounts.

## Why this rule exists

`custom_json` follow payloads carry a user-controlled `following` string the
chain never validates as an account. Dirty-marking it with upsert created
~1M phantom stub documents (`{_id, _dirty}`) from 2016-era garbage
(`'followmyvote.'`, `'ubmit.html'`, …). The AccountRefresher fetches dirty
ids in natural order and RPC never returns phantoms, so the same 500
phantoms blocked the head of the refresh queue permanently and real account
snapshots went stale.

## Enforcement (steemdb-sync)

- `MongoInserter.QueueAccountDirty` — marks `_dirty` with **upsert=false**;
  a mark for a name not in the collection is a no-op.
- `MongoInserter.QueueAccountCreate` — upserts the stub with `_dirty`;
  called ONLY by `AccountCreateHandler` (the three creation ops) and
  `PowHandler` (mined workers).
- The `custom_json` follow handler marks no accounts at all: follow state
  changes no field of the chain `account_object` (follower counts belong to
  follow_api / hivemind, not `get_accounts`).
- Historical gaps are backfilled by repair (`discover-accounts`), phantom
  stubs are removed by repair (`verify-accounts`) — both one-off modes,
  since new data cannot violate the rule anymore.
