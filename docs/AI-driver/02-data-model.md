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
- **VESTS vs STEEM is a per-field fact, not a per-collection one.**
  `curation_reward.reward`, `author_reward.vesting_payout`,
  `benefactor_reward.reward` (= `comment_benefactor_reward.vesting_payout`),
  `fill_vesting_withdraw.withdrawn` and `account.vesting_withdraw_rate` are
  **VESTS**; `vesting_deposit.amount`,
  `fill_vesting_withdraw.deposited`, `author_reward.steem_payout` are STEEM.
  Readers must render VESTS as VESTS (`formatVests`) or convert with
  `total_vesting_fund_steem / total_vesting_shares` from the same props
  snapshot — never hand a VESTS number to a STEEM formatter (the labs power
  down page was overstated ~1610x that way until 2026-10).
- **`comment.pending_payout_value` is SBD and is the only pending figure.**
  `total_pending_payout_value` is written as 0 by this chain, so a reader that
  displays it renders `0.000` for every row. `depth` distinguishes top-level
  posts (0) from replies; labs boards whose copy says "posts" must filter it.

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
| `comment` | CommentHandler (+comment_options, author_reward writeback) + **comment_rescanner** (get_content snapshots) | `{author}/{permlink}` | the only collection on the unbuffered direct-write bypass (diff read-modify-write + matched semantics); `last_applied_op` dedup marker; dynamic fields (active_votes/depth/payout/…) only exist after rescan; `parent_author`/`parent_permlink` are written by the handler on every op, so reply classification needs no rescan; `scanned` = last **write** to the document (the handler bumps it on every op, edits included — it is the rescanner's staleness cursor, *not* a snapshot time); `rescanned_at` = last `get_content` refresh, written only by the rescanner, so it is the only field that answers "when were these active_votes snapshotted"; `json_metadata` stays a raw string when the chain data is invalid (poison for rigid decoders) |
| `account` | creation ops (stub upsert `{_id, _dirty:true}` via `QueueAccountCreate`; dirty marks via `QueueAccountDirty` never upsert — `docs/rules/account-doc-creation.md`) + **account_refresher** (full docs, `$unset _dirty`) | web everywhere | stub→full two-phase protocol; `scanned` = last refresh time |

### Snapshot layer (refresher)

`status` (processor cursor `processor_height`; stats counters; clients
snapshot; flags snapshot), `witness` (30s top-100 rebuild; votes float),
`witness_history` (daily, `_id=owner|YYYYMMDD`), `witness_misses`,
`funds_history` (hourly), `clients_history`. `status` is cleanly
partitioned by `_id` across writers — the model multi-writer collection.

`status.flags-snapshot` (`_id=flags-snapshot`, daily, written by the
refresher's flags ticker) holds the labs/flags leaderboard:
`data[] = {author, count, voter_count, post_count, voters[{voter, count}]}` —
top 200 authors plus their 10 heaviest downvoters. It exists because the
underlying aggregation over `vote` (`weight<0`, ~7.7M documents) cannot run
on a request path (see 04-web-service.md); `steemdb-web` only reads it.

`status.clients-snapshot` (`_id=clients-snapshot`, 90-day window, written by
the refresher's clients ticker) holds the labs/clients board:
`data[] = {_id: {doy, year, month, day, dow}, clients: [{client, count,
reward}], reward, total}` plus the snapshot metadata `updated_at`,
`window_from`, `window_to`, `posts_scanned`, `posts_with_app`. `client` is the
part of `comment.json_metadata.app` before the first `/` (a version suffix is
optional — it must not gate attribution), `count` is attributed comments
(posts and replies — the same set `posts_scanned` counts) and
`reward` is **SBD**, the sum of all three payout legs SteemDB stores per
comment: `total_payout_value + pending_payout_value + total_pending_payout_value`
(summed with `$sum`, which ignores a missing leg; the older `$add` returned null
as soon as one leg was absent and zeroed the whole day).
The legs overlap in meaning; they are summed as written, so the figure is "the
payout legs summed", not "SBD already paid out". The metadata is what makes the
snapshot auditable: `posts_scanned` counts every **comment** in the window
(posts *and* replies — it is a coverage denominator, not a post count),
`posts_with_app` counts the ones the per-client lists actually attribute (same
predicate, so an app string like `/1.2.3` that trims to an empty client name is
scanned but not attributed, and so is a non-string `app` such as an array —
the projection narrows the field to a string rather than letting `$split` abort
the tick), `posts_scanned - posts_with_app` is the share with
no client signal, and `updated_at` is the only way to tell a stalled refresher
from a fresh one.

`stats_cache` is written by **web** (stats_service background counter) —
self-contained. `tx_cache` is also written by **web** (tx_service read-through
cache for `GET /api/v1/tx/:id`): `_id`=txid, full
`condenser_api.get_transaction` document nested under `tx`, `_ts`=fetch time.
The endpoint only serves irreversible data: a transaction is returned (and
admitted to the cache) only once its block is at or below the chain's
`last_irreversible_block_num`, fetched from the public node
(get_dynamic_global_properties, cached in memory for a few seconds); anything
still reversible answers 425 and is never cached, so a fork can never pin
wrong data. Landed transactions are immutable, so entries are never refreshed
and there is no TTL; a txid is refetched upstream only until it passes LIB,
then served locally forever after. No secondary index —
`_id` lookups only, so the sync index inventory is not involved.

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
