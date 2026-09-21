# 04 — web Service (steemdb-web)

Single Go binary (Gin + Gorilla WebSocket) shipped in one image with Nginx
(frontend dist + reverse proxy) and supervisord. Nginx :80 →
`/api/`, `/ws`, `/health`, `/ready` → 127.0.0.1:8080.

## Layering

`cmd/web/main.go` (wiring: config, Mongo/Redis, CreateIndexes, graceful
shutdown) → `internal/api/` (handlers + routes.go — parameter parsing and
response wrapping only) → `internal/services/` (business + queries) →
`internal/database/` + `pkg/steem` (steemgosdk client with node rotation
and retry). Handlers stay thin; business logic lives in services.

## The one decoding rule (learned from five broken endpoints)

**Reading sync-written collections: never `cursor.All` into rigid
structs.** Sync documents carry open-shaped, partially-converted RPC data
(string amounts that should be floats, string dates, string int-arrays).
Use `bson.M` decode + explicit projection, following
`AccountService.GetAccount` / `accountSummaryFromMap`. The labs service's
former `[]models.Account` and `[]string` decodes were the counter-example
that produced P0s (`/labs/powerup|powerdown|rshares|curation|author` failed
on non-empty results; `/labs/pending` had an inverted time window); all five
joins now decode as `[]bson.M` + `accountSummaryFromLookup` and the pending
window spans [12.5 days ago, 7 days ago] (fixed 2026-09). Same rule
for `json_metadata`: it may be a raw string (invalid chain JSON), decode
leniently and never let one bad document 500 a whole page.

## Dual-track data access (local Mongo vs steem RPC)

Five dual-track points exist: block-detail enrich, virtual-ops (pure RPC —
though `operations` already holds all virtual ops), witness fallback
(local first, RPC if empty), dashboard branch (upstream if local head
lags chain head by >10 blocks), props (pure RPC).

Boundary rule going forward: **immutable history (blocks/ops/posts/
account history) — local Mongo is the authority, RPC only as enrich
fallback; current-state (props/head/witness-now) — RPC is fine, but a
single response must be same-source/same-instant (one page mixing local
head, chain-head props, and 5-minute-old snapshots is the anti-pattern).**

## Endpoint surface (routes.go is the truth)

`/api/v1/`: accounts (+search/stats/top/:name/:name/history), blocks
(+latest/stats/:number/:number/virtual-ops), operations/stats, dashboard,
posts (+daily/:author/:permlink/replies/votes/reblogs), labs
(powerup/powerdown/rshares/curation/author/flags/clients/benefactors/
pending + index), witnesses (+top/:username), stats (global/props),
search, charts (accounts/growth, blocks/production, transactions/volume,
witnesses/voting), status. Legacy `/api/{supply,props,...}` 302-redirect
to v1 (semantics differ from the old endpoints — known debt; prefer 410
for incompatible ones when touched). `/health`, `/ready`, `/ws`.

Conventions:
- **Pagination/sort aliases**: block_handler implements the canonical
  fallback pattern (`page_size`←`limit`, `sort_by`←`sort`,
  `sort_order`←`order`); account history supports `limit`. accounts/posts
  handlers lack it (frontend sends the short names — known break).
  New endpoints: implement the alias set and a sort-field whitelist
  (`witness_service` is the whitelist pattern).
- **User input into `$regex` must be `regexp.QuoteMeta`'d**; `sort_by`
  must be whitelisted.
- **Counting large collections**: `EstimatedDocumentCount` for empty
  filters (dashboard does; per-second WS state counts do not — known).
- Error mapping: decode errors are not "not found" — distinguish
  `ErrNoDocuments` from corruption before returning 404.

## WebSocket (`websocket_service.go`)

Single-producer model: `fetchData` goroutine polls RPC every 1s
(props + blocks after LIB), pushes into a 1024-buffered broadcast
channel; `run()` hub fans out to per-client 256-buffered send channels
(slow clients evicted). Channels: `blocks`, `props`, `state`,
`operation`, `@account` (mentions via regex on comment bodies —
legacy-parity). New connections get the three defaults + replay of last
10 irreversible blocks (currently synchronous in the hub loop — known
head-of-line blocker; move off the hot path before extending).

Known gaps (fix before scaling, don't copy): per-second exact
CountDocuments on account/comment; RPC ctx not propagated (a hung node
freezes the pump); no recover in the pump goroutine; CheckOrigin always
true; subscription channels unvalidated/unbounded; `lastBlockProcessed`
read across goroutines without atomics; replay advances lastBlock on
fetch failure (gap in feed).

## Config reality check

- Viper binds env only for Mongo/Redis URIs. `SERVER_MODE=production`
  (compose sets it) is **not** wired — the container runs gin debug.
  Adding env overrides requires explicit `BindEnv`.
- Redis is connected and `/ready` pings it, but no service uses it
  (cache/rate-limit/JWT/metrics config sections are dead). Don't build
  on the assumption that caching exists; don't add Redis dependencies to
  readiness without using them.
- `steem.timeout`/retry and WS tuning knobs in config.yaml are not wired
  (values hardcoded in `pkg/steem/client.go` — 10s ctx between retries,
  4 attempts × SDK retry 3, node rotation each failure).

## Deployment notes

- Image: multi-stage (frontend dist + Go binary + nginx + supervisord).
  Configs mount at `/app/configs` (hot-edit + restart).
- Root compose runs web+mongo(4.4)+redis+prometheus+grafana+refresher.
  Prometheus has no scrape config; grafana ships admin123 — monitoring
  is decorative until configured.
- Ports: web 80/8080/9090(metrics, unpublished), processor 9092,
  refresher 9093, live_sync 9091, cold_ingest 8080/9090 — cold_ingest's
  8080 collides with web's if run on one host.
