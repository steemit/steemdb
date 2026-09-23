# 05 — Frontend (steemdb-frontend)

React 19 + TypeScript + Vite + Tailwind + Zustand + TanStack Query +
recharts. ~7k lines. Dev: `pnpm install && pnpm run dev` (proxies to
:8080 per vite config; prod build served by web's nginx with `/api` and
`/ws` reverse proxy — `VITE_API_URL='/api'` is baked by define).

## Structure

- `src/App.tsx` — all routes registered under `<Layout/>` (Header +
  Sidebar + Outlet). Pages: Dashboard (the former LiveFeed page was merged
  into it; `/live` redirects to `/`), Blocks(+detail), Accounts(+detail),
  Posts(+detail), Labs index + 9 subpages, Witnesses(+detail), Statistics,
  Settings, 404 catch-all.
- `src/lib/api.ts` — hand-rolled fetch client, single ApiClient with 35
  methods (Proxy-bound exports). `src/lib/websocket.ts` — WS client
  (reconnect, subscription Set, event emitter). `src/lib/utils.ts` —
  formatting helpers. `src/types/index.ts` — contract types (partially
  drifted from reality — treat as documentation, verify against payloads).
- `src/store/index.ts` — Zustand stores (theme/websocket/blockchain/
  navigation/favorites+notification).

## Two data-flow patterns (by page type)

1. **List/detail pages**: TanStack Query, queryKey includes all params
   (correct), global staleTime 5min / gcTime 10min. Override staleTime to
   ~0 for block-feed-like pages (Blocks currently doesn't — known staleness
   bug).
2. **Dashboard**: REST preload (`getDashboard`) + WebSocket
   incremental writes into `useBlockchainStore`; 10s fallback polling only
   when disconnected AND data empty (too narrow — dashboard can freeze
   after WS gives up reconnecting; known). Its activity stream subscribes
   to `blocks`/`props`/`state`/`operation` and renders live WS items
   followed by the store's `latestBlocks` (deduplicated by block number),
   so it is populated even before the first live event.

## Hard rules for anything touching Steem values

- **Scales**: `voting_power` 0–10000, vote `weight` ±10000,
  `sbd_interest_rate` basis points → divide by 100 before appending `%`.
  VESTS and STEEM are different units — never `formatCurrency(vest)` (which
  defaults to STEEM) with a " VESTS" suffix. These four display bugs were
  fixed in PR #72; the scale rules above remain mandatory and fix-adjacent
  code must not copy the old pattern.
- Account amounts arrive as **numbers** from the backend (sync converts);
  types claiming `string` are stale. Use lenient formatting (existing
  utils accept both) and placeholder rendering for missing fields
  (stub accounts legitimately have no fields — never render "undefined").
- Avatars: current source is `images.hive.blog` (Hive, not Steem) with
  onError fallback — known semantic drift.

## API contract conventions

- Pagination params: backend canonical is `page/page_size/sort_by/
  sort_order`; blocks/accounts/posts also accept the `limit/sort/order`
  aliases, and accounts additionally supports `search=` (name-prefix
  filter; posts rejects `search` with 400). `src/lib/api.ts` sends the
  canonical names for accounts/posts/blocks (witnesses is the exception —
  that handler reads the short names only). **When touching any list
  page, make the param names match the handler actually implemented** —
  this is the single most-recurring contract break (before 2026-09 the
  accounts page's sort/search/page-size were all silently dropped).
- Sortable columns must stay within the backend sort-key whitelist
  (index-backed fields only — e.g. accounts: name/reputation/vests; the
  Balance column is display-only on purpose).
- Labs pages: `date` format must match `grouping` (`YYYY-MM-DD` for daily,
  `YYYY-MM` for monthly) — convert on toggle or the backend 400s.
- WS: channels `blocks/props/state/operation/@name`; server auto-subscribes
  the first three and replays 10 blocks per connection; `operation`
  requires explicit subscribe. Client's unsubscribe on page unmount has no
  refcounting — pages must not unsubscribe channels another page may rely
  on.

## Client-side quality bar (current debt, do not extend)

- Subscribe with selectors (`useStore(s => s.x)`); whole-store
  subscriptions in App/Sidebar re-render the tree per WS message.
- Guard WS `connect()` against CONNECTING (double-socket duplication);
  `onclose` must null the reference only if it owns the current socket.
- Wrap `JSON.parse(localStorage…)` in try/catch at module top level.
- Use `useParams` (not searchParams/pathname splitting) on detail pages.
- AbortController (or sequence guard) on debounced search-as-you-type.
- `debounce` util exists — use it instead of per-keystroke queries.

## Verification

`pnpm exec tsc --noEmit` passes clean. `pnpm test` / lint via eslint
config. The TypeScript types are advisory (they lie in places); the real
contract is the JSON each endpoint returns — check handler response
structs in web when a field's existence matters.
