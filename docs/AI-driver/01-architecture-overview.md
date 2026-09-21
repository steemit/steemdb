# 01 — Architecture Overview

## What SteemDB is

A blockchain explorer for Steem, rewritten from PHP/Python into a Go + React
monorepo. One machine ingests the entire chain into MongoDB and serves a REST
+ WebSocket API with a React SPA. Roughly 30k lines: `steemdb-sync` (~14k Go),
`steemdb-web` (~8.5k Go), `steemdb-frontend` (~7k TS/TSX), plus `legacy/`
(PHP, reference only — never extend, never import).

## System data flow

```
                          ┌──────────────────────────────────────────────┐
                          │                MongoDB (steem db)             │
  steemd                  │                                              │
  (ingest plugin,         │  RAW LAYER            DERIVED LAYER           │
   replay + push) ──HTTP──► operations  ──proc──► vote, transfer,         │
                          │ blocks                curation_reward, ...    │
  steemd / public RPC     │ transactions           (14+ collections)      │
   ──RPC── live_sync ─────► meta (watermarks)     account (stub→refresh)  │
   ──RPC── repair ────────► status (cursor+snap)  comment (diff+rescan)   │
                          │                        witness/funds/clients  │
                          └───────┬──────────────────────┬───────────────┘
                                  │ read-only            │ read-only
                          ┌───────▼───────┐      ┌───────▼────────┐
                          │  steemdb-web  │◄─────│ steemdb-frontend│
                          │  Gin REST+WS  │      │  React 19 SPA   │
                          │  +Nginx       │      └─────────────────┘
                          │  +steem RPC   │  (web also calls steem RPC
                          └───────────────┘   for block detail / props)
```

Four-stage pipeline:

1. **Cold ingest** (`cmd/cold_ingest`, HTTP, loopback-default
   `127.0.0.1:8080` — the endpoint is unauthenticated, so the bind must
   stay loopback or inside a trusted network): receives applied-op
   batches pushed by the steemd ingest plugin during `--replay-blockchain`.
   Synchronous ACK: batch is flushed to Mongo (unordered upsert) before 200;
   the plugin retries 5×3s on failure. Empty blocks land as block-only docs
   via a ticker sweep. Exits at target height.
2. **Live sync** (`cmd/live_sync`, steemgosdk v2): resumes from
   `max(meta.max_block, highest block)`; chunked concurrent catch-up, then
   single-block follow at head. Per-chunk write order is fixed:
   **ops → transactions → blocks → meta.max_block** (the last is the commit
   point). Compensates two condenser quirks (see 03).
3. **Processor** (`cmd/processor`): the only consumer of `operations`.
   Windowed (default 64 blocks) dispatch to 16 handlers writing derived
   collections; `status.processor_height` advances only after the whole
   window flushes — that is the commit point. Crash ⇒ replay whole window.
4. **Refresher** (`cmd/refresher`, independent process): tickers replacing
   legacy Python jobs — witness snapshots (30s), stats counters (5m),
   clients aggregation (1h), funds snapshots (1h), optional account rescan.

Inside the processor process also run two workers: `account_refresher`
(batch `get_accounts` for accounts marked `_dirty` by handlers) and
`comment_rescanner` (`get_content` backfill of dynamic fields).

## Design philosophy (the five pillars)

These are the load-bearing ideas. Do not work against them; if a change
conflicts with one, stop and surface the conflict.

1. **Idempotent upserts with deterministic `_id` are the only recovery
   primitive.** Every raw-layer write is `{block}:{trx}:{op}`-addressed;
   every derived doc is a deterministic function of its op. This is what
   makes crash-replay, cursor rewind, hot container replacement, and
   multi-source backfill all safe without transactions or coordination.
   The whole system was validated at 20M-block scale on this property
   (replay equivalence, kill -9 recovery, zero gaps).
2. **`operations` is the single source of truth for op data; everything
   else is a materialized projection.** The implicit second truth source —
   "op decides history, RPC decides current state" (refresher/rescanner
   write chain-head snapshots) — is real but was never written down with
   per-field ownership. When touching a field, know which side owns it.
3. **sync is the schema authority; web aligns to sync.** Field names, types,
   and units are defined by writer code in `steemdb-sync`. Web/frontend read
   what exists. (The original one-line rule under-specified decoding style,
   index authority, and amount types — see 02 and 07 for the full contract.)
4. **Windows/chunks are commit points.** processor window, live_sync chunk.
   Writes inside a commit point must be replayable; writes outside the
   protected path (comment direct-write bypass, mid-flush buffer loss) are
   known sharp edges — do not add new ones.
5. **Single machine, single node, accept it.** No distributed coordination.
   The compensations this philosophy requires — monitoring and documented
   manual recovery — are still gaps; do not silently assume they exist.

## Deployment topology (as actually run in production)

- Root `docker-compose.yml`: web + mongo + redis + prometheus + grafana +
  **refresher**. It does **not** include processor/live_sync/cold_ingest.
- The full sync pipeline runs on the ingest machine against the **same
  MongoDB database** (`steemdb`). This shared-Mongo contract is implicit —
  keep it true: every sync service supports `MONGO_URI` env override.
- Production steady state (traffic cutover plan stage C): live_sync +
  processor + refresher + web family + Mongo on one box; cold_ingest/steemd
  retired after replay. A production compose with all services does not
  exist in-repo yet — treat "add it" as a known gap, and never assume the
  root compose alone yields a working explorer (derived collections would
  have no writer).

## The review's top-level verdict (2026-09)

**The core is coherent; the seams are not.** Code quality correlates with
proximity to the operations write path: ingest → processor → cursor is the
strongest, most-tested part of the system. Drift concentrates at four seams:
web's rigid read-side decoding, the third ingest path (repair), the
current-state writers (rescanner/refresher) lacking field ownership, and a
production topology that lives only in people's heads. When developing,
expect the seams to be where both the reuse opportunities and the landmines
are.

## External dependencies

- `steemgosdk` (Go) — all Steem RPC from Go code, both modules. Never
  introduce another RPC client (enforced rule, `docs/rules/project.md`).
- `steemutil` — protocol types, crypto, asset handling.
- steemd ingest plugin (steem repo, PR #3708, branch `ingest_plugin`) —
  the cold-start producer. Batches of 100 ops / 1s, bounded queue of 100k
  (deliberate backpressure: slow consumer throttles replay).
- Keep `steemgosdk`/`steemutil` versions aligned across `steemdb-web` and
  `steemdb-sync` (currently drifted v0.0.15 vs v0.0.31 — known debt).
