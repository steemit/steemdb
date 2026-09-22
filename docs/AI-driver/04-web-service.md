# 04 — web Service (steemdb-web)

Single Go binary (Gin + Gorilla WebSocket) shipped in one image with Nginx
(frontend dist + reverse proxy) and supervisord. Nginx :80 →
`/api/`, `/ws`, `/health`, `/ready` → 127.0.0.1:8080.

## Layering

`cmd/web/main.go` (wiring: config, Mongo/Redis, graceful shutdown; web
creates no indexes — sync's `createIndexes` is the single index
authority, see 02-data-model.md) → `internal/api/` (handlers +
routes.go — parameter parsing and response wrapping only) →
`internal/services/` (business + queries) → `internal/database/` +
`pkg/steem` (steemgosdk client with node rotation and retry). Handlers
stay thin; business logic lives in services.

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
window spans [7 days ago, 156 hours ago] — the legacy 12-hour pre-cashout
review window (fixed 2026-09). Same rule
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
- **Pagination/sort aliases**: every collection listing (blocks, accounts,
  posts; account history) accepts the canonical `page`/`page_size`/
  `sort_by`/`sort_order` plus the short aliases `limit`/`sort`/`order`
  (canonical wins; shared parsing in `internal/api/params.go`). accounts
  additionally supports `search=` — a case-insensitive, QuoteMeta-escaped
  name-prefix filter (same matcher as `/accounts/search`). posts rejects
  `search` with 400: it has no text-search semantics, tag-scoped listings
  live on `/posts/daily?tag=`. Sort keys are whitelisted to index-backed
  fields (`accountSortField`/`postSortField`, following the
  `witnessSortField` pattern — balance is deliberately not sortable on
  accounts, no index). New endpoints: implement the alias set and a
  sort-field whitelist. (History: before 2026-09 only blocks had the
  aliases; the frontend's short names were silently dropped on
  accounts/posts — the sort/search/page-size triple break.)
- **User input into `$regex` must be `regexp.QuoteMeta`'d**; `sort_by`
  must be whitelisted.
- **Counting large collections**: `EstimatedDocumentCount` for empty
  filters (dashboard and the WS `state` channel both do).
- Error mapping: decode errors are not "not found" — distinguish
  `ErrNoDocuments` from corruption before returning 404.

## WebSocket (`websocket_service.go`)

Single-producer model: `fetchData` goroutine polls RPC every 1s
(props + blocks after LIB), pushes into a 1024-buffered broadcast
channel; `run()` hub fans out to per-client 256-buffered send channels
(slow clients evicted). Channels: `blocks`, `props`, `state`,
`operation`, `@account` (mentions via regex on comment bodies —
legacy-parity). New connections get the three defaults + replay of last
10 irreversible blocks; the replay runs on a dedicated worker fed by a
bounded queue (capacity 128, drop-oldest on overflow) so a slow or hung
replay cannot stall the hub loop or back-pressure the data pump (fixed
2026-09; it used to run synchronously inside the hub loop). The `state`
channel runs on its own 10s ticker with `EstimatedDocumentCount` (same
rationale as dashboard); on count errors the last known counts are kept
and a frame with no known value is skipped — zeros are never broadcast.
All long-lived service goroutines (hub, replay worker, data pump) run
through `runRecoverable`/`safeRun`: panics are logged and the goroutine
keeps serving instead of killing the process. Every logical RPC in
`pkg/steem/client.go` bounds each attempt with a 10s per-attempt timeout
(the SDK calls take no context, so a hung node is abandoned, not waited
out); retries rotate nodes as before.

Known gaps (fix before scaling, don't copy): CheckOrigin always true;
subscription channels unvalidated/unbounded; replay advances lastBlock on
fetch failure (gap in feed); `register`/`unregister` are unbuffered
channels, so post-`Stop()` connect/disconnect hangs (benign while main
exits right after); `writePump` does not watch the service context.

## Config reality check

- Env overrides: `viper.AutomaticEnv()` plus the dot-to-underscore
  `EnvKeyReplacer` makes **every key already declared in `setDefaults()`
  or config.yaml** overridable by its uppercase-underscore form —
  `SERVER_MODE`, `AUTH_JWT_SECRET`, `LOG_LEVEL`, `CACHE_ENABLED`, …
  (unmarshal resolves each known key through env first). An explicit
  `viper.BindEnv` is only required for keys declared nowhere (not in
  defaults, not in the config file — those are invisible to viper's key
  set); the existing BindEnv calls also add the legacy
  `MONGODB_URI`/`REDIS_URI` aliases. (Before the replacer, `AutomaticEnv`
  looked up `SERVER.MODE` for `server.mode`, which never exists — why
  `SERVER_MODE` used to be dead.)
- Redis is connected and `/ready` pings it, but no service uses it
  (cache/rate-limit/JWT/metrics config sections are dead). Don't build
  on the assumption that caching exists; don't add Redis dependencies to
  readiness without using them.
- `steem.timeout`/retry and WS tuning knobs in config.yaml are not wired
  (values hardcoded in `pkg/steem/client.go` — 10s per-attempt timeout,
  4 attempts with node rotation each failure, backoff 1-3s with no shared
  budget; worst case ~46s per logical call). The env override coverage
  above reaches them too
  (`STEEM_TIMEOUT`, `STEEM_RETRY_ATTEMPTS`), but only `steem.nodes` is
  consumed (`cmd/web/main.go`) — the knobs stay dead until the client
  reads the config.

## Deployment notes

- Image: multi-stage (frontend dist + Go binary + nginx + supervisord).
  Configs mount at `/app/configs` (hot-edit + restart).
- Root compose runs web+mongo(4.4)+redis+prometheus+grafana+refresher.
  Prometheus has no scrape config; grafana ships admin123 — monitoring
  is decorative until configured.
- Ports: web 80/8080/9090(metrics, unpublished), processor 9092,
  refresher 9093, live_sync 9091, cold_ingest 8080/9090 — cold_ingest's
  8080 collides with web's if run on one host.
