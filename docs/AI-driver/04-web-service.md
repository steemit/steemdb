# 04 — web Service (steemdb-web)

Single Go binary (Gin + Gorilla WebSocket) shipped in one image with Nginx
(frontend dist + reverse proxy) and supervisord. Nginx :80 →
`/api/`, `/ws`, `/health`, `/ready` → 127.0.0.1:8080.

## Layering

`cmd/web/main.go` (wiring: config, Mongo, graceful shutdown; web
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

## Labs endpoint data shapes (aggregation rules)

Labs is the read-amplification surface: every endpoint is an aggregation over
a processor-written collection, so three rules apply and are cheap to get
wrong.

1. **A per-request aggregation must fit the index that leads its `$match`.**
   `benefactor_reward` (7.1M docs) and `vote` (941M docs / 7.7M negative) both
   broke the endpoint with MongoDB code 292
   `QueryExceededMemoryLimitNoDiskUseAllowed` when aggregated unbounded; the
   fix is a `$match` window that the leading `_ts` index can serve (benefactors:
   10 UTC days, ~18ms) or a precomputed snapshot (flags — `vote`'s only index
   on the negative slice is the partial `{weight:1}`, which carries no
   author/voter/permlink, so any request over it must FETCH ~7.7M documents and
   measured 145-293s even with `allowDiskUse`). `allowDiskUse` alone is not a
   fix: it converts a fast 500 into a multi-minute request that competes with
   everything else on the hot cache.
2. **A snapshot is written by the refresher, read as a point lookup.** The
   precedent is `status.clients-snapshot`; `status.flags-snapshot` follows it
   (daily, `refresher.flags` config). Snapshot writers overwrite, so a bad run
   self-heals on the next tick — do not build them append-only.
3. **Render chain units, don't relabel them.** `reward` on
   `curation_reward`/`benefactor_reward` and `withdrawn` on
   `vesting_withdraw` are VESTS; the SPA shows them via `formatVests`, or the
   service converts with the props ratio when the economic question is STEEM
   (labs/powerdown). Never feed VESTS to `formatCurrency` — that is how the
   power down page displayed 348.8M STEEM for 216.5k STEEM of real withdrawals.

Also: a labs response must stay small. The old `/labs/flags` shipped every
voter name and every flagged permlink (~40MB for 200 accounts) and the SPA read
neither — the API now exposes `count`, `voter_count`, `post_count` and the top
10 voters only.

4. **Truncate after ranking, and cap what a row carries.** `$limit` ahead of
   `$sort` keeps an arbitrary slice ($group output order is unspecified), so a
   "top N" claim must be backed by a sort that runs first — labs/powerup
   shipped `$limit 1000` with no sort at all, and powerup's `total` was derived
   from an `$addToSet` of amounts, so identical deposits collapsed and the
   figures were short while `count` stayed right. Caps on a per-row array
   belong in a `$project` with `$slice` (curation keeps 20 permlinks per
   curator instead of every post, which was 96.9-99.8% of a ~4.7MB monthly
   response).
5. **Copy must state the window, the unit and the cap.** Rolling windows are
   not calendar days ("Day" meant 24h), a capped list must say "top N", and
   the unit in the label must match the formatter (`formatVests` vs
   `formatCurrency`). Replies vs posts is a filter (`depth: 0`), not a
   subtitle — 14.5% of the pending-payout rows used to be replies on a page
   titled "Posts".
6. **A capped board ships its own totals.** `labs/rshares` used to return every
   voter of the day (290-405KB, unbounded) while top-10 held 65% of the
   rshares; it is now a `$facet` of top-N rows plus `total_rshares` /
   `total_votes` / `voters`. Never let the SPA derive a share from a truncated
   array. The same endpoint reports `votes_complete_as_of` (the OLDEST
   `comment.rescanned_at` among the day's posts — the instant up to which the
   whole day's votes have been refreshed) because the rescanner refreshes
   `active_votes` on a ~6h cycle — an ongoing day is systematically short of its
   last hours (36% of votes, 42% of rshares at audit time) and the copy says so.
   It must not read `comment.scanned`: that field is also bumped by the comment
   handler on every op, so its maximum over an ongoing day is "newest document
   write" (~now), and the board ended up announcing freshness and lag on the same
   screen (pitfall A5). The newest `rescanned_at` fails the same way — the
   rescanner's queue is rolling, so some post is always refreshed ~now. Only the
   oldest snapshot is a bound the whole day satisfies.
7. **Classify on written fields, not on identifier spelling.** A reply is a
   payout whose `comment.parent_author` is non-empty, never a payout whose
   permlink starts with `re-` (client convention; 18.7% wrong on one day). The
   join is `comment._id = author/permlink`. Cost, measured on a local 4.4 with
   34k monthly payouts: the join roughly doubles the monthly request (~5s vs
   1s in the probe) — acceptable for a labs page, and the cheap long-term exit
   is a `parent_author`-derived flag written by the rewards handler (which then
   needs the processor write-ordering classification plus a backfill).
8. **A snapshot states its freshness and coverage.** `status.clients-snapshot`
   now carries `updated_at`, `window_from`, `window_to`, `posts_scanned` and
   `posts_with_app`; `GET /labs/clients` exposes them and only ships the
   per-day series on request (`?include=daily`, it was 98% of the payload and
   nothing rendered it). `rewards` is SBD, the sum of the three payout legs
   (`total_payout_value` + `pending_payout_value` +
   `total_pending_payout_value`), not STEEM. Both counters count comments
   (posts + replies), and `posts_with_app` re-applies the attribution
   predicate instead of testing `json_metadata.app != ""` — the latter counted
   app strings like `/1.2.3` as attributed after the aggregation had already
   dropped them, so the coverage share covered documents the lists did not
   contain.
9. **A cached RPC value must not queue requests behind it.** `labs/author`
   needs props + the median price feed to rank payouts in one unit. The cache
   is refreshed under singleflight; only the request that owns the refresh
   waits (≤3s), every other request takes the last good rate immediately, and a
   failure suppresses the next attempt for 15s and is logged once per attempt.
   A hanging node costs up to two 46s retry loops (`pkg/steem`), so the naive
   "hold a mutex across the call" shape turns one bad node into an
   endpoint-wide pile-up. On a cold cache the endpoint fails fast instead of
   ranking on a fabricated rate.
10. **A multi-key `$sort` is a `bson.D`.** MongoDB reads the field order of a
   `$sort` document as precedence and `bson.M` has none, so
   `{"total_steem": -1, "_id": 1}` occasionally makes the unique tie-breaker
   primary — alphabetical boards, no error (pitfall A4).

### Response-shape changes (labs audit batch, 2026-10)

These changed on `/api/v1/labs/*` and nothing outside this repo depends on the
old shapes yet (the batch is not deployed), so no compatibility shims were kept.

| Endpoint | Change |
|---|---|
| `GET /labs/rshares` | array → object: `{allocations, total_rshares, total_votes, voters, limit, votes_complete_as_of}`. Rows are capped (`?limit`, default 200, max 2000) while the totals cover every voter of the day, so a share must be derived from `total_rshares`. `votes_complete_as_of` is the OLDEST `comment.rescanned_at` among the day's posts (the instant the whole day is complete up to), absent when no post of that day has been rescanned. |
| `GET /labs/flags` | `flaggers` (every voter) and `posts` (every permlink) removed — neither was ever rendered and together they made the response ~29MB; `voter_count` and `post_count` added. `count` is still the downvote **op** count. |
| `GET /labs/author` | `permlinks` removed (never rendered); `total_steem` added — the STEEM equivalent of all three payout legs, and the value the board is ranked by. `?limit` is honoured (default 200, max 500). |
| `GET /labs/clients` | `dates` is opt-in (`?include=daily`, it was 98% of a 236KB payload); each daily entry gained `total`/`reward`; the response gained `updated_at`, `window_from`, `window_to`, `posts_scanned`, `posts_with_app`. Both counters count comments (posts + replies), and the per-client `posts` map counts comments attributed with the same predicate — the SPA labels them accordingly. |
| `GET /labs/benefactors` | window narrowed to the 10 daily buckets ending today (it returned the oldest 10 days — April 2017) and the reward unit is documented as VESTS. |

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

## Output escaping (XSS) contract

Every string this service serves and the SPA displays comes from the
chain — post bodies, titles, categories, witness URLs, account profile
metadata, memo keys, operation payloads — so the attacker writes the
bytes. The posture is "escape, never interpret", verified 2026-10
end-to-end (built SPA bundle + stub API + real browser, plus the Go
test `internal/api/json_output_test.go`):

- **API side**: responses are JSON only (no HTML templating anywhere in
  the service), and gin's default encoder HTML-escapes `<`, `>`, `&` to
  `\u003c`, `\u003e`, `\u0026` with a correct `application/json`
  content type. A client that inlines a response into an HTML document
  therefore cannot break out of it. Pinned by the test above — failing
  it means the renderer was swapped for a non-escaping encoder
  (`jsoniter`, `c.PureJSON`, `SetEscapeHTML(false)`). Because that test
  only pins gin's default and a handler can opt out one at a time,
  `TestNoUnescapedJSONRenderer` scans this package's non-test sources
  for the same three markers.
- **Client side**: React escapes text children, and escaped text is the
  only rendering mode in `steemdb-frontend` — there is no
  `dangerouslySetInnerHTML`, no `innerHTML` assignment, no markdown or
  HTML renderer. `PostDetail` shows `post.body` in a
  `whitespace-pre-wrap` text node, so a post containing
  `<script>…</script>` displays those characters literally (the body
  is therefore also *not* rendered as markdown; that is a display
  limitation, not a hole). `steemdb-frontend/eslint.config.js` bans the
  four DOM-reachable raw-HTML sinks with `no-restricted-syntax` —
  `dangerouslySetInnerHTML`, `innerHTML`/`outerHTML` assignment (dot and
  bracket form), and `insertAdjacentHTML` — so the fence is enforced on
  `pnpm lint`, not remembered by reviewers.
- **URLs are the one place escaping is not enough** (`<a href>` /
  `<img src>` do not run the escaper). `src` values are built by
  prefixing a fixed origin (`getAvatarUrl`), and `href` values from
  chain data (`witness.url` in Accounts/Witnesses) are sanitized by
  React 19's `sanitizeURL`, which rewrites `javascript:` to a throwing
  stub — verified in the browser. That protection lives in the
  framework, so re-verify it before any React downgrade, and prefer an
  explicit `http(s)`-only check when adding new chain-sourced links.
- Missing hardening (deliberate, not a bug): no CSP is sent. If one is
  added it must allow `googletagmanager.com` and the configured Umami
  origin (see `src/lib/analytics.ts`), plus `images.hive.blog` /
  `ui-avatars.com` avatars.

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
  `SERVER_MODE`, `AUTH_JWT_SECRET`, `LOG_LEVEL`, …
  (unmarshal resolves each known key through env first). An explicit
  `viper.BindEnv` is only required for keys declared nowhere (not in
  defaults, not in the config file — those are invisible to viper's key
  set); the existing BindEnv call also adds the legacy `MONGODB_URI`
  alias. (Before the replacer, `AutomaticEnv`
  looked up `SERVER.MODE` for `server.mode`, which never exists — why
  `SERVER_MODE` used to be dead.)
- Redis was removed from web entirely (2026-09): the client was wired
  only into `/ready` and three service structs that never called it, so
  a Redis outage took web out of rotation behind a dependency nothing
  used. `/ready` now pings Mongo only (`internal/api/health_handler.go`);
  the redis/cache config sections and the `go-redis` dependency are gone.
  Rate-limit/JWT/metrics config sections remain declared-but-unwired.
  If a real cache is ever built, reintroduce Redis alongside the feature
  that uses it — including its readiness gate — not before.
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
- Root compose runs web+mongo(4.4)+prometheus+grafana+refresher.
  Prometheus has no scrape config; grafana ships admin123 — monitoring
  is decorative until configured.
- Ports: web 80/8080/9090(metrics, unpublished), processor 9092,
  refresher 9093, live_sync 9091, cold_ingest 8080/9090 — cold_ingest's
  8080 collides with web's if run on one host.
