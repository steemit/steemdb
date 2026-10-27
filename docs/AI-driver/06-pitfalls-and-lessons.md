# 06 — Pitfalls and Lessons (paid for already; do not pay again)

Each entry: what happened → root cause → the rule to internalize.
Incident details live in `~/workspace/agent-share/notes/steemdb/`;
review details in `~/workspace/agent-share/steemdb/next/`.

## Incidents (production/test-stack, with PRs)

### 1. Batcher O(N) sweep + never-deleted maps (PR #51)
**What**: cold-ingest degraded from ~1250 to ~105 ops/s over hours; first
misdiagnosed as "old image spinning" (restart "fixed" it). **Root cause**:
`blocks`/`blocksWritten` maps grew forever; a 1s ticker scanned all of it
under read locks, colliding with every request's write lock — goroutines
spun on futexes. **Lessons**: (a) in-memory state needs a death — "present
in map" must mean "not yet persisted"; one source of truth beats two
bookkeeping maps; (b) throughput that decays with runtime = suspect
unbounded growth under a lock (`grep delete(` is a valid health check);
(c) a restart that cures something is evidence about *state*, not
*binary* — don't attribute it to versions.

### 2. benefactor_reward COLLSCAN (PR #65)
**What**: processor flush 414ms → 6203ms, rate 7850 → 250 blocks/min on
EC2; Mongo CPU eaten by `planSummary: COLLSCAN, docsExamined: 846k`. **Root
cause**: Pattern-B index led with `_ts` while the handler filter led with
`_block` — planner couldn't use it. One added index: flush 6203→98ms. **Lessons**: Pattern-B filter and its index are one contract (leading order
included); slow-query log (`COLLSCAN` lines) is the first tool; watch the
window fetch/dispatch/flush breakdown, not a single blocks/min number.
Side-finding: `GetMaxBlockSeen` was 0 on the batch path because a second
entry point bypassed the single funnel — every recording invariant needs
one funnel.

### 3. Misdiagnosis theater: "Mongo is slow" during 2kw replay
**What**: 28.5h at 19.5% replay; producer CPU 0.02%, middle service 146%,
storage 7.6% with 21ms writes. **Root cause**: see #1 (the middle layer).
**Lessons**: three-layer CPU snapshot (producer/service/storage) + storage
latency immediately localizes the bottleneck; the bounded plugin queue is
deliberate backpressure — "upstream starved" is the symptom, not the
disease.

### 4. Asset NAI format (PR #33)
Cold plugin pushed NAI objects, live RPC pushes strings — one `ParseAsset`
now handles both. **Lesson**: every ingest source has its own wire quirks;
convert at the boundary in one function, never per-callsite.

### 5. go-diff PatchApply panics (PR #62)
Malformed chain patches crashed the processor; now recovered per-op.
**Lesson**: chain data is hostile input; wrap third-party parsers.

### 6. live_sync head-following + AH-lag empty ops (PRs #93, #97)
Single-block follow near the head wrote block headers for seconds-old
blocks whose account_history entries were not yet indexed
(`get_ops_in_block` returns empty, no error) — a 19-day ops gap
(558k blocks) while blocks/transactions kept landing. Also surfaced:
duplicate creation ops from the same trx in two blocks (orphan pushes).
**Lessons**: (a) never trust "empty ops" from a fresh block — every block
carries at least producer_reward, so empty means not-yet-indexed, retry;
(b) follow LIB, not the head; (c) blocks/transactions advancing while
operations stand still is a detectable stall pattern — add a coverage
health check rather than eyeballing block progress.

## Review-confirmed defect patterns (2026-09 five-pass review)

Each pattern below occurred ≥2 times independently — they are systemic,
not one-offs. A6 is the exception: it was found once, and is recorded for its
category (a pre-filter mistaken for a guarantee), not its frequency. The fix
direction is named; until fixed, work around.

### A. Rigid decoding of open-shaped documents
labs ×5 endpoints structurally 500 (string→int64, double→[]string,
string→map decode aborts the whole cursor). **Rule**: bson.M lenient
projection for anything sync writes (02-data-model).

### A2. Unbounded labs aggregation / wrong display units (2026-10 audit)
`/labs/flags` and `/labs/benefactors` returned a permanent 500: both grouped
their whole collection (`vote` 7.7M negative docs, `benefactor_reward` 7.1M)
with no `$match`, exceeding the 100MB `$group` budget (code 292), and
`allowDiskUse` turned that into a 145-293s request. Alongside it, three
endpoints printed VESTS through a STEEM formatter (power down overstated
~1610x, curation and benefactors the same class). **Rules**: a per-request
aggregation is bounded by a `$match` the leading index serves, or it moves to
a refresher snapshot; `allowDiskUse` is a safety net, not a fix; and every
figure has a unit contract checked against the writer (02-data-model,
"VESTS vs STEEM"). A second-order trap from the same audit: `$sort` +
`$limit` with an ascending date sort returned the *oldest* 10 days of a
9-year collection — pin the direction a test can assert.

### A3. Shape heuristics, unit mixing and truncation (2026-10 audit, batch 3)
Three separate ways a board can be confidently wrong without erroring.

- **A string shape is not a fact.** `/labs/author` classified a payout as a
  reply when its permlink started with `re-`. That is a client convention: on
  one day 164 of 876 payouts (18.7%) were replies judged as posts, replies were
  under-reported by 87%, and 88 of 622 authors were split wrong. The truth was
  already written next door — `comment.parent_author` (written by the comment
  handler for every op, so no rescan needed). **Rule**: classify on a written
  field, joined by `comment._id = author/permlink`; never on the spelling of an
  identifier.
- **Rank by the unit you display.** The same board sorted by `vest` while its
  "Total Rewards" column showed only steem+sbd, so the #1 card read `0.000
  STEEM / 0.000 SBD` and the true leader was ranked 2nd. With three payout legs
  there is one comparable unit: STEEM, using the props vesting ratio for VESTS
  and the **median price feed** for SBD (SBD is not at parity — 2026-10: 1 SBD ≈
  9.6 STEEM; `get_feed_history.current_median_history`, quote/base, cached
  ~1min). **Rule**: one ranking key, derived from the same legs the row
  displays.
- **Truncation without server-side totals corrupts every percentage.** A
  `$limit` added to `/labs/rshares` would have left the SPA dividing by the
  truncated sum (top-200 of 1,400 rows: shares of ~700%). Totals come from a
  `$facet` computed before the limit. And a snapshot that cannot state its own
  freshness is unauditable: `status.clients-snapshot` stored no `updated_at`
  and no coverage counters, so "is this stale?" and "how many posts have no
  client?" were both unanswerable from the API. In the same ticker, `$add`
  returned null whenever one payout field was missing, which is how 70 of 91
  days carried reward 0 while post counts looked right — `$sum` over
  `$ifNull`-guarded legs instead.

### A4. A multi-key `$sort` built from `bson.M` has no precedence (2026-10, batch 3)

MongoDB reads the **field order of a `$sort` document as sort precedence**, and
`bson.M` is an unordered Go map: marshalling `bson.M{"total_steem": -1, "_id": 1}`
emits the two keys in a random order. When the tie-breaker lands first it becomes
the primary key — and because it was a tie-breaker precisely because it is
unique (`_id`, `_id.voter`), the ranking key is then ignored entirely. The board
did not error; it just came back alphabetical, in roughly one run out of five
(measured 5/40 and 7/40 on a local mongo 4.4 with identical input).

It was found by accident: the labs integration tests assert row order, and they
were run repeatedly. A single pass passes 80% of the time, which is exactly why
this shape survives review.

- **Rule**: any `$sort` with more than one key is a `bson.D` (`bson.D{{Key:...,
  Value:...}, ...}`), never a map. Same for `$sort` inside `$facet` sub-pipelines
  and on the sync side (`refresher/clients.go` ordered each day's client list by
  a four-key map).
- **Corollary for tests**: a board's ordering must be asserted more than once.
  The regression test for this runs the same aggregation 20 times, because one
  call cannot distinguish "correct precedence" from "lucky map order".

A single-key `$sort` may stay a map — one key has no precedence problem.

### A5. A field with two writers is not a snapshot time (2026-10, batch 3)

`/labs/rshares` reported `data_as_of` from `max(comment.scanned)` among the
day's posts, documented as "the comment snapshot behind the numbers". But
`scanned` has two writers: the comment handler sets it to `time.Now()` on
**every** comment op (edits included), and the rescanner sets it again when it
refreshes `get_content`. So on an ongoing day the maximum is dominated by the
newest document write — ~now — while the last ~6h of votes are still missing,
and the page said both things at once: "data as of <now>" and "the most recent
hours are not counted yet". On a historical day it degenerated into "the last
time some post of that day was edited or rescanned".

The fix is a field with one writer, not a better query: the rescanner also
writes `rescanned_at`, nobody else touches it, and the response field
(`votes_complete_as_of`) is read from it. A day with no `rescanned_at` reports
**no** timestamp, and the SPA then makes no freshness claim — the missing field
is the honest answer, not `now`.

Swapping the field is only half of it. The good field still has to be reduced in
the right direction: the rescanner works a **rolling** queue, so on an ongoing
day some post is refreshed every few minutes and `max(rescanned_at)` is ~now
again — the same overstatement with a different field name. The only single
instant that is true for a whole day is `min(rescanned_at)`: every post's votes
have been refreshed at least that recently, so the day is complete **up to**
that point. The response computes it with a `$group`/`$min` aggregation rather
than `find().sort()`: nothing indexes `rescanned_at`, so the sort is an
unindexed blocking SORT over whole documents (bodies plus `active_votes`) that
can hit the 100MB `internalQueryMaxBlockingSortMemoryUsageBytes` cap on a busy
day — and the resulting error looked exactly like "this day was never
rescanned".

- **Rule**: before publishing a timestamp as "the data is as of X", count its
  writers (`grep` the collection name in the handler, the rescanner and the
  repair tools). A field bumped by a write path that does not refresh the data
  in question answers "when was this row touched", never "how fresh is it".
- **Second rule**: a per-row timestamp aggregated over a set is a bound, not an
  instant. `max` claims the whole set is as fresh as its freshest row (never
  true under a rolling refresh); `min` claims only what every row satisfies.
- **Corollary for tests**: assert the discriminating case, not the happy path.
  The regression test seeds one post whose `scanned` is the newest write while
  its `rescanned_at` is older, a second post rescanned at a different instant
  (the two values are what make `min` and `max` disagree about the answer), and
  a third day whose documents have a fresh `scanned` and no `rescanned_at` at
  all — which must report nil.
- **Same-shaped hazard, not fixed here**: the rescanner's staleness cursor is
  still `scanned`, so an actively edited post postpones its own vote rescan for
  up to `stale_hours`. Switching that cursor to `rescanned_at` needs a backfill
  plan (a `$lt` on a missing field matches every legacy document at once).

### A6. The `$type` query operator is not a type guard in an aggregation (2026-10, found once in review)

The clients snapshot refuses `json_metadata.app` values that are not non-empty
strings by matching `{"json_metadata.app": {"$type": "string", "$ne": ""}}`.
That match does **not** exclude an array: MongoDB matches a field against a
`$type` when **an element** has that type, so `{"app": ["steemx/1.0"]}` passes.
The next stage, `$split` on the same field, requires a string and throws
`(Location40085) $split requires an expression that evaluates to a string as a
first argument, found: array` (reproduced on mongo 4.4).

The blast radius is not the bad document. The aggregation aborts, `tick` returns
before `status.clients-snapshot` is written, and the only trace is one log line:
a single comment whose `app` is an array holding a string, anywhere in the 90-day
window, freezes the whole clients page for as long as it stays in the window.
Other foreign shapes (object, number, boolean, null, an array of numbers) never
reach the projection — the pre-filter drops them — so this one shape is the whole
risk. This is not corruption anyone can clean up: `json_metadata` is the op's
on-chain JSON string unmarshalled as-is (`handlers/comment.go`), so the shape is
user-controlled input that any account can post. A background refresher must
survive its worst input — the value that a stage cannot accept has to be
narrowed in the projection, not assumed away by the pre-filter.

- **Rule**: `$type` inside a `$match` is a pre-filter, not a guarantee. Any
  expression that requires a specific type (`$split`, `$trim`, `$toLower`,
  arithmetic) gets an `$isArray`/`$type` guard feeding it, so one dirty document
  costs one row instead of the whole tick.
- **Which fields get the guard — ask who shapes the value.** `json_metadata` is
  arbitrary JSON parsed from the on-chain string
  (`handlers/comment.go`: `json.Unmarshal` of the op's `json_metadata`), and the
  chain only asserts that the string is valid JSON (`comment_operation::validate`
  → `fc::json::is_valid`), so any account can post `{"app": ["x/1.0"]}` — or a
  top-level array — a user-reachable shape, hence the guard. `created` is a
  `time.Time` written by our own handlers on insert
  (`setOnInsert` of the block timestamp) and by the rescanner's string→time
  conversion, so no writer produces a non-date there and the `$group` date
  extractors need no guard. A range match does *not* make that safe by itself —
  `[date]` passes `{$gte, $lte}` and `$dayOfYear` throws `can't convert from
  BSON type array to Date` — the writer inventory is what makes it safe.
- **Any array shape, not just the one-element case.** The guard sits on the
  value, so it is total over the traversal. `["x/1.0"]`, `["x/1.0","y/2.0"]`, a
  mixed array that holds a string, and a top-level array
  (`json_metadata: [{"app": "x/1.0"}]` — dot notation descends into it, so its
  `app` also reads as an array) all pass the pre-filter and project an empty
  client name: counted in `posts_scanned`, skipped in `posts_with_app`. Arrays
  the pre-filter already excludes (all-number, all-object, or nested —
  `[["steemx/1.0"]]` has array elements, so no string element) never reach the
  projection and are not attributed either, so the verdict is the same however
  deep the array sits. A one-element array that *does* name a client
  (`["steemx/1.0"]`) is the deliberate cost of that rule: the attribution is
  dropped, because inventing a naming rule for an undocumented shape is a worse
  error than under-attributing it. If such a client is ever observed in real
  data, taking the first element becomes a decision with evidence behind it
  rather than a guess.
- **Corollary for tests**: seed the *foreign* type, not just the empty value.
  The clients test carries an array-of-strings app, an array-of-strings app with
  two elements, and an app reached through a top-level `json_metadata` array;
  without the guard it fails at "snapshot not written", not at a count
  assertion — which is what a broken tick actually looks like.

### B. Reader imagining writer's fields
`Vote.Time`, `Reblog.BlockNum`, phantom indexes on `timestamp`/
`last_update`, frontend types declaring string amounts. Every imagined
field = permanently zero. **Rule**: open the writer's handler before
naming a bson tag or index.

### C. Parameter-name drift across three tiers (fixed in PR #80)
accounts/posts ignore `limit/sort/order/search`; one handler implemented
aliases, others didn't; frontend standardized on the short names nothing
reads. **Rule**: alias fallback in every list handler + one canonical
name from the frontend, changed in the same PR across tiers.

### D. Steem scale/unit constants
Four shipped 100× display errors (voting_power, weight,
sbd_interest_rate, VESTS double-unit), fixed in PR #72. **Rule**: see 05; every `%` and
every currency label needs a scale check against chain semantics.

### E. Fallbacks that fabricate data
`time.Now()` on timestamp parse failure (pollutes `_ts` chains, breaks
witness_vote dedup), placeholder stats endpoints returning hardcoded
numbers, `404` masking decode errors. **Rule**: a parse failure is an
error to retry, never a value to invent; an unknown error is not "not
found".

### F. Unprotected side-writes around commit points
comment direct-write bypass loses edits on transient errors while the
window still commits; mid-flush buffer clear loses a bucket the same
way; `last_applied_op` equality breaks multi-diff replay. **Rule**: the
commit point protects only what FlushAll replays — anything writing
outside it needs its own failure semantics; idempotency markers must
order-compare (numerically parsed coordinates, never op.ID strings).

### G. Decorative configuration
env tags without implementations, Redis/cache/rate-limit/JWT config with
zero consumers, README endpoints that don't exist (`SERVER_MODE` was in
this group too until it got an explicit `BindEnv`). **Rule**: config must be wired in the same PR that introduces it,
or deleted; docs that describe nonexistent behavior get fixed, not
extended. (The rule was executed for the Redis/cache pair in 2026-09:
web's redis client, its config sections, and the compose services were
removed; rate-limit/JWT/metrics sections remain declared-but-unwired.)

### H. Security posture assumed but absent
nginx add_header inheritance silently dropped all security headers on the
SPA (now fixed via a shared include in every add_header location); ingest
endpoint unauthenticated (mitigated by loopback-default bind); mongo/redis
published without auth; regex injection via unescaped user input (now
QuoteMeta'd). **Rule**: verify the posture
end-to-end (curl -I the actual responses) before calling it configured;
QuoteMeta every user regex.

### I. Scanners don't scale
repair scanner = per-block double query; rescanner queues = unindexed
full scans; empty-block false positives bury the real-hole signal.
**Rule**: any "walk everything" component needs a range-aggregation
design review at today's data size, and a way to distinguish signal from
noise.

## Process lessons (how these got found/missed)

- Single-op unit tests cannot catch window/replay-order bugs — the
  write-ordering rule exists because review found what tests couldn't.
- Scale × time is a dimension tests don't cover: both production
  incidents (#1, #2) only manifested at millions of blocks/documents.
- The 20M-block dry run is the real test suite: replay equivalence +
  kill -9 + zero-gap checks validated the core; nothing comparable
  exists for the read side — which is exactly where all seven P0s live.
