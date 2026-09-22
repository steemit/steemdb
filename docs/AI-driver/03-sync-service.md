# 03 — sync Service (steemdb-sync)

Five deployable processes share `internal/` (config / mongo / rpc / metrics /
model). Binaries build to `../bin/`:

```bash
cd steemdb-sync && go build -o ../bin/cold_ingest ./cmd/cold_ingest
# same for: live_sync, processor, refresher, repair (+ test_tools)
```

## cmd/cold_ingest + internal/pipeline — cold-start receiver

- HTTP server (loopback-default `127.0.0.1:8080`; the endpoint is
  unauthenticated, so wildcard-bind only on a trusted network) receiving
  plugin batches at `/ingest/applied/applied_ops`; synchronous ACK:
  `Batcher.
  FlushOperationsAndBlocks` writes ops (unordered bulk upsert) then blocks,
  then 200. Plugin retries 5×3s on failure.
- `AddBlockInfo` registers per-block metadata and tracks `maxBlockSeen`
  (target-height exit condition — keep it the single funnel for all ingested
  records; a past bug had the batch path bypass it).
- `flushUnwrittenBlocks` (1s ticker + post-success sweeps) writes block-only
  docs for empty blocks, then deletes map entries (write-then-delete keeps
  the map bounded — PR #51; "present in map" == "not yet written" is the
  invariant; two bookkeeping maps were the incident).
- Known edges (review-confirmed): the async channel path (`AddOperation`/
  `run()`) is dead in production (ACK path only) — its metrics never fire;
  signal channel has two consumers (monitor goroutine + main) so ~50% of
  SIGTERMs hang the process; ticker sweep writes block headers for blocks
  whose ops are still being retried, which can hand the processor a
  "header without ops" window (see failure chain below).

## cmd/live_sync — real-time follower

- Resume point: `max(meta.max_block, highest block in blocks)`. Behind by
  > follow_threshold (100) → chunked concurrent fetch (256/chunk); near head
  → single-block follow. Same loop, no mode switch.
- **Per-chunk write order is the crash-consistency contract:
  ops → transactions → blocks → meta.max_block.** The comment in
  `cmd/live_sync/main.go` says exactly why: a block header must never land
  before its operations. Any new ingest path must copy this order.
- Condenser quirks compensated in `rpc.ConvertBlockOps` (`internal/rpc/
  blockops.go`, shared by live_sync and repair — reuse this, do not
  re-derive): in all-ops listings `op_in_trx` is 0 for every op of a
  multi-op transaction → renumber by array position; virtual op coordinates
  are unreliable → normalize to `trx=0xFFFFFFFF, op=1..n` so ids match the
  plugin's convention.
- RPC client wrapper (`internal/rpc/client.go`) accepts ctx but does not
  propagate it into SDK calls (known gap — cancellation cannot interrupt an
  in-flight RPC).

## cmd/processor + internal/processor — the derivation engine

Reads `operations` in windows (default 64, env `PROCESSOR_WINDOW_SIZE`;
buffer limit 5000/collection, `PROCESSOR_BUFFER_LIMIT`):

1. One query for window block metadata (existence + timestamps), one for
   all window ops sorted by (block_num, trx_index, op_index). The window is
   truncated at the first header gap; a gap at the window **head** holds the
   window (no dispatch, no cursor advance — retry until ingest/repair lands
   the header), and ops whose block header is missing are never dispatched
   (holding beats dispatching with a zero timestamp).
2. Dispatch per op (panic-safe per op) to 16 handlers. Ops that fail their
   handler are collected by op id; dispatch continues past them (one bad op
   never stops the block).
3. Writes split in three classes (see 02); comment family is entirely
   unbuffered (direct-write bypass) for diff read-modify-write and
   author_reward matched semantics.
4. `FlushAll` (per-collection unordered BulkWrite + batched `_dirty`
   marking) — executed even when the window is about to be held, so
   successfully-dispatched writes land (idempotent upserts make the later
   replay safe). A failed flush KEEPS its buffered models: an unordered
   bulk write may have partially landed, and re-writing landed upserts is
   harmless while dropping unlanded ones is permanent loss (review N2).
5. Error gate before the commit: any op that failed its handler, or any
   flush error, holds the window — the cursor does NOT advance and the next
   iteration replays the whole window (review F5). Replay safety comes from
   the same invariants as crash replay (below).

**Poison-op escape hatch**: `processor.skip_error_ops_after_retries`
(env `PROCESSOR_SKIP_ERROR_OPS_AFTER_RETRIES`, default 0 = never skip). An
op whose handler has failed this many *consecutive* window attempts is
skipped (not dispatched) with a loud `SKIP op ...` log naming the op and the
config, and the window may commit past it. Default 0 keeps the safe side: a
stalled cursor is loudly visible (repeated per-attempt error logs, frozen
`status.processor_height`) while silently skipped ops are not. State is
in-memory per process and cleared on every window commit.

**Invariants that make crash-replay safe** — keep them true or fix the docs:
window writes are deterministic functions of ops; same-filter conflicts
flush the bucket (later wins); a failed flush keeps the bucket's models so
the retry re-delivers them (idempotent upserts make the re-write harmless);
comment writes carry `last_applied_op` in the same UpdateOne as the body
patch, and the replay guard compares it as a numeric (block, trx, op) tuple
— `<=` skips, `>` applies — so a replayed window with two diffs to one
comment skips both instead of double-patching (unparseable markers degrade
to exact string equality). These are what make BOTH crash replay AND the
error-gate hold-and-retry window replay safe recovery strategies.

Two workers run in-process (paused while catching up >1000 blocks):
- `account_refresher` — batches `_dirty` accounts through `get_accounts`
  (500/batch), converts fields (see 02), `$unset _dirty` on success.
- `comment_rescanner` — three queues (recent rescan / expired payout /
  bootstrap-by-missing-depth), `get_content` backfill of dynamic fields.

## cmd/refresher + internal/refresher — legacy Python replacement

`runTicker` schedules: witness (30s; rebuild top-100, daily witness_history,
misses delta), stats (5m; `status.transactions-*` counters using
`op_index:0` counting — NOT distinct, which hits the 16MB cap at scale),
clients (1h), funds (1h), optional account rescan (24h, off by default).
Known edges: no `recover` in ticker (a panic kills the process);
`witness` rebuild deletes `$nin` top-N — guards only against an empty list,
a truncated non-empty list would delete live witnesses; funds inserts a
duplicate snapshot on every restart.

## cmd/repair + internal/checker — gap filler

Scanner walks 1..maxBlock marking missing headers and zero-op blocks;
repair re-fetches via RPC. Repair shares live_sync's exact ingest
conventions: op ids come from `rpc.ConvertBlockOps` (renumbered `op_in_trx`,
virtual ops at `trx=-1, op=1..n`) and the write order is the same
crash-consistency contract (ops → transactions → blocks → max_block), so a
crashed repair leaves either nothing or a gap the next scan re-repairs —
never a header that hides missing ops. (Both were review findings F3/F2:
repair used to write header-first and keep raw RPC coordinates,
deterministically collapsing multi-op transactions onto one `_id`.)
Repair does not rewind the processor cursor; blocks repaired behind the
cursor need a manual `status.processor_height` rewind to re-derive. The
scanner itself is per-block double-query (unusable at tens of millions of
blocks without the proposed range-aggregation redesign).

## The worst known failure chain (why the invariants matter)

A Mongo blip >15s during cold ingest → plugin gives up on a batch →
batcher's ticker sweep writes the batch's block **headers** → processor
reaches that height, sees header-without-ops, dispatches nothing, advances
cursor → plugin retry lands the ops later, but the cursor has passed:
**ops permanently underived, no alarm**. This end of the chain — header
**present**, ops **missing** — is **not** closed by the window guards: both
guards trigger on a *missing block header*, so planWindow proceeds
normally, the ops query simply returns nothing for the block, the flush
succeeds over (partly) empty buffers, and the cursor advances as if the
block had been processed. Nothing is logged and the cursor keeps moving:
unlike the head-gap hold there is no visible signal.

What the guards do close (sync review finding F1):

- **Window head, header missing** (partial repair failure or a raced
  ops-first write): planWindow yields an empty window and the processor
  holds the cursor instead of permanently skipping the head block's ops.
  Visible: a "holding cursor, waiting for ingest/repair" log line on every
  retry and a stalled `status.processor_height`.
- **Ops present, header missing**: the window is held before dispatch.
  Previously such ops were dispatched with a zero timestamp, polluting
  `_ts` across every derived document of the block and silently dropping
  witness_vote documents through the `_ts` same-filter upsert path.

The header-present/ops-missing mode remains open: silent dispatch loss is
still possible. Fixes under consideration: block-only markers,
empty-block-aware scanning (scanner's zero-op signal is still drowned out by
legitimate empty blocks). Until one lands, treat "block header present, ops
empty" after any infra hiccup as an incident — manually compare
`status.processor_height` with the ops actually present and rewind the
cursor if it has passed.

## Other review-confirmed sharp edges (short list)

- `_id` collisions in same block: transfer/vesting_deposit/convert/
  vesting_withdraw/feed_publish/pow (legacy-inherited; fix = include
  trx/op index, needs legacy-data compatibility assessment).
- ~~Handler errors do not block cursor advance; mid-window flush failure
  clears the buffer and the window can still commit~~ — fixed: handler
  errors and flush errors both hold the window (no cursor advance, full
  replay), a failed flush keeps its buffered models for the retry, and the
  `skip_error_ops_after_retries` escape hatch (default 0) is the only way
  an op is deliberately passed over (loud `SKIP op` log).
- ~~`last_applied_op` equality check breaks on two diffs to the same comment
  in one window + replay (double patch)~~ — fixed: the guard now compares
  (block,trx,op) numerically (op.ID strings do not sort:
  `"100:10:0" < "100:2:0"` lexicographically); unparseable markers fall
  back to exact string equality.
- Timestamp parse failures fall back to `time.Now()` (ingest handler and
  RPC converter) — a deterministic-error retry would be correct instead.
- rescanner `$set`s the whole get_content result, which can overwrite
  processor-maintained fields (body/created) for old content; restrict to
  a dynamic-field whitelist before relying on it for historical data.
- Config: `configs/config.yaml` lacks processor/live_sync sections (env
  vars + defaults work); many `env:` tags are decorative (not implemented
  in `loadFromEnv`).

## Testing

`test/docker-compose/` runs the real stack (mongo with auth, cold-ingest,
processor, refresher, live-sync profile, steemd `ety001/steemd:with-ingest`
replaying a 383GB block_log). e2e (`test/e2e`) requires prebuilt binaries
in `../bin` and the steemd image; it skips otherwise. ⚠️ e2e kills whatever
occupies host port 8080 — do not run it next to anything you value on 8080.
`docker-compose.db.test.yml` (repo root) is a different, auth-less db stack;
sync's default config URI points at the auth'd test stack instead — always
check which stack you are combining.

## Production

The resident production topology (processor + live-sync + refresher + web
family + mongo on one box) is codified in the repo root
`docker-compose.production.yml`; `steemdb-sync/Dockerfile.sync` builds the
one image its sync services run from (all five binaries; `cold_ingest` and
`repair` are run ad hoc from it). The env names it sets are the
`loadFromEnv`-verified ones listed in `internal/config/config.go`. Its
header documents the cold-start handoff order (replay → processor → enable
the `live` profile) and the shared-Mongo contract.
