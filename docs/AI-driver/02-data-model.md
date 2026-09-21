# 02 — Data Model (MongoDB, database `steemdb`)

The database is the contract between sync (writer) and web/frontend
(readers). This file is the field-level map. **Rule: writers define,
readers conform.** Before adding a field, find its owner below; before
reading a field, verify the writer actually writes it.

## Two-layer naming convention (intentional, know it or break things)

| Layer | Collections | Height field | Time field | `_id` style |
|---|---|---|---|---|
| Raw layer | operations, blocks, transactions | `block_num` | `timestamp` | `{block}:{trx}:{op}` etc. |
| Derived layer (legacy sync.py alignment) | vote, transfer, comment, rewards, follow, reblog, witness_vote, vesting_*, pow, feed_publish | `_block` | `_ts` | composite strings, see table |

`_ts`/`_block` look like inconsistencies but are the legacy-compatible
schema by design. Web model `bson` tags that map to non-existent names
(e.g. `Vote.Time → "time"`, `Reblog.BlockNum → "block_num"`) are simply
bugs — every such mismatch found in review produced permanently-zero
fields.

## Amount and unit conventions

- **Amounts are stored as float64** (sync converts asset strings via
  `AssetValue`/`transformAssetValue`). Web code that decodes amounts as
  `"100.000 STEEM"` strings is legacy-era code and structurally broken.
- Exception that proves the rule: `account_refresher.processAccount`
  converts only reputation, to_withdraw, 8 asset fields, and dates —
  `withdrawn`, `curation_rewards`, `posting_rewards`,
  `proxied_vsf_votes`, `last_account_update` remain RPC strings. These
  poison any rigid decode (the labs P0s). Do not "fix" them by parsing in
  the reader; extend the writer's conversion list (and bump the review
  docs).
- Steem numeric scales (frontend display): `voting_power` and vote
  `weight` are ±10000-based (divide by 100 for %), `sbd_interest_rate`
  is basis points. Never concatenate `%` onto raw values.
- Account identity is **`account._id`** (the name). The `name` field
  exists only on refreshed documents — never use it as a join key,
  search filter, or sort key.

## Collection reference

### Raw layer

| Collection | Writers | Readers | Key facts |
|---|---|---|---|
| `operations` | cold_ingest, live_sync, repair | processor; web account history (`accounts[]` + `block_num`), block ops, stats | `_id={block}:{trx}:{op}`; `accounts[]` fan-out with `{accounts:1, block_num:-1}` index; virtual ops use `trx=0xFFFFFFFF` (plugin & live_sync convention); plugin-source virtual ops have `trx_id=""` while RPC-source may carry all-zero trx_id (known stat skew) |
| `blocks` | cold_ingest (header only, no witness/previous, transaction_count=0), live_sync (full header), repair | processor window, web blocks/dashboard/stats | `_id`=block number; witness/previous only exist for live_sync-written range (i.e. the cold/history range shows empty witness in UI — known, documented) |
| `transactions` | live_sync only | none (block detail enriches via RPC) | effectively dead data for the cold range |
| `meta` | cold_ingest, live_sync, repair | sync internals | `sync_state.max_block` watermark = live_sync resume point |

### Derived layer (processor handlers)

| Collection | Writer handler | `_id` | Notes |
|---|---|---|---|
| `vote` | VoteHandler | `{block}/{voter}/{author}/{permlink}` | fields: `_ts/_block/voter/author/permlink/weight` (+op passthrough). **No rshares/percent/time here** — those live in `comment.active_votes` |
| `transfer` | TransferHandler | `{block}/{from}/{to}` | ⚠️ same-block same-pair collisions overwrite (legacy-inherited; known) |
| `vesting_deposit` / `vesting_withdraw` | VestingHandlers | `{block}/{from}/{to}` / `{block}/{from_account}/{to_account}` | amounts float64; ⚠️ same-block collisions |
| `convert` | ConvertHandler | `{block}/{requestid}` | ⚠️ requestid is per-account counter — cross-account same-block collisions |
| `curation_reward` / `author_reward` | RewardsHandlers | `{block}/{curator}/...` / `{block}/{author}/...` | author_reward also writes back `comment.reward` |
| `benefactor_reward` | BenefactorRewardHandler | filter-upsert `{_block, benefactor, permlink, author}` | Pattern B; has both `_ts`- and `_block`-led indexes (post-incident) |
| `feed_publish` / `pow` | MiscHandlers | `{block}|{publisher}` / `{block}-{worker}` | ⚠️ same-block collisions |
| `follow` / `reblog` | CustomJSONHandler | filter-upsert `{_block, follower, following}` / `{_block, permlink, account}` | Pattern B; currently zero web readers |
| `witness_vote` | WitnessVoteHandler | filter-upsert `{_ts, account, witness}` | filter contains `_ts` — same-block vote+unvote is the canonical Pattern-B conflict case |
| `comment` | CommentHandler (+comment_options, author_reward writeback) + **comment_rescanner** (get_content snapshots) | `{author}/{permlink}` | the only collection on the unbuffered direct-write bypass (diff read-modify-write + matched semantics); `last_applied_op` dedup marker; dynamic fields (active_votes/depth/payout/…) only exist after rescan; `json_metadata` stays a raw string when the chain data is invalid (poison for rigid decoders) |
| `account` | handlers (`_dirty` stubs: `{_id, _dirty:true}`) + **account_refresher** (full docs, `$unset _dirty`) | web everywhere | stub→full two-phase protocol; `scanned` = last refresh time |

### Snapshot layer (refresher)

`status` (processor cursor `processor_height`; stats counters; clients
snapshot), `witness` (30s top-100 rebuild; votes float), `witness_history`
(daily, `_id=owner|YYYYMMDD`), `witness_misses`, `funds_history` (hourly),
`clients_history`. `status` is cleanly partitioned by `_id` across writers —
the model multi-writer collection.

`stats_cache` is written by **web** (stats_service background counter) — the
only web write to Mongo; self-contained.

## Index authority

- **The single authority for indexes on sync-written collections is
  `steemdb-sync/internal/mongo/mongodb.go` `createIndexes` (backed by
  `indexInventory`), run at sync service startup. steemdb-web creates no
  indexes** — its former `CreateIndexes` was removed: it built phantom
  indexes on non-existent fields (`vote.timestamp`, `transfer.timestamp`,
  `account.last_update`; writers use `_ts` / `scanned`) and duplicated
  several sync-side indexes. Every legitimate entry was migrated into
  sync's inventory, and sync's `dropLegacyWebIndexes` removes the legacy
  web-built phantom/unused indexes from existing deployments at startup
  (online, lossless).
- The inventory and the drop list are guarded by `TestIndexInventory` /
  `TestPatternBFiltersIndexed` / `TestLegacyWebIndexesAreNotRecreated` in
  `steemdb-sync/internal/mongo/` (golden-list assertions; index changes
  must update the tests).
- Pattern-B collections **must** have an index matching the handler filter
  including leading order. The benefactor_reward COLLSCAN incident
  (flush 6203ms → 98ms after index) proved this is a hard coupling, not
  tuning; `TestPatternBFiltersIndexed` now guards it.
- Read-side indexes were backfilled for the hot paths (review F3/F9/F11/N1):
  `curation_reward/author_reward/vesting_deposit/vesting_withdraw {_ts:1}`
  (labs time-range aggregations), `vote {author,permlink,_ts}` +
  partial `{weight:1}` on `weight<0` (post votes, flags),
  `reblog {author,permlink,_ts}`, `comment {depth,created}`,
  `{parent_author,parent_permlink,created}`, `{scanned,created}` and
  `{depth,pending_payout_value}` (rescanner queues), plus the posts-list
  sort columns (`created`/`net_votes`/`pending_payout_value`/
  `total_payout_value`/`category`). Account: `name` (labs `$lookup`),
  `reputation`/`vesting_shares` (list sorts), `next_vesting_withdrawal`
  (power-down schedule). Witness: `owner`/`votes`/`total_missed`.
  `transfer` has **zero readers** — no indexes (add one with the reader).
- Index creation runs on a dedicated context without the connect deadline:
  builds on production-size collections take minutes-to-hours and would
  otherwise time out at startup and block the whole sync service.

## Write-ordering / buffering classes (summary; full rule in docs/rules/)

1. Block-unique `_id` upserts → buffered, unordered bulk-safe. **But "unique"
   requires (block, trx, op) or a chain-level uniqueness proof** — six
   handlers currently violate this (see 03 §known defects).
2. Filter-based upserts (Pattern B) → buffered with same-filter conflict
   detection (flush bucket before append, later write wins).
3. Read-modify-write (comment diff) → unbuffered direct write +
   `last_applied_op` idempotency marker, order-compared as a numeric
   (block, trx, op) tuple (`<=` marker → skip) so multi-diff window
   replays stay exactly-once.

## Dead/unread collections (at review time)

`follow`, `transactions`, `witness_misses`, `clients_history`,
`feed_publish`, `pow`, `witness_vote` have writers but zero readers. Do not
add data to them without also adding a consumer, and do not assume their
content is load-bearing.
