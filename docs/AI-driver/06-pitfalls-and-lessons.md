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

## Review-confirmed defect patterns (2026-09 five-pass review)

Each pattern below occurred ≥2 times independently — they are systemic,
not one-offs. The fix direction is named; until fixed, work around.

### A. Rigid decoding of open-shaped documents
labs ×5 endpoints structurally 500 (string→int64, double→[]string,
string→map decode aborts the whole cursor). **Rule**: bson.M lenient
projection for anything sync writes (02-data-model).

### B. Reader imagining writer's fields
`Vote.Time`, `Reblog.BlockNum`, phantom indexes on `timestamp`/
`last_update`, frontend types declaring string amounts. Every imagined
field = permanently zero. **Rule**: open the writer's handler before
naming a bson tag or index.

### C. Parameter-name drift across three tiers
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
env tags without implementations, `SERVER_MODE` unwired, Redis/cache/
rate-limit/JWT config with zero consumers, README endpoints that don't
exist. **Rule**: config must be wired in the same PR that introduces it,
or deleted; docs that describe nonexistent behavior get fixed, not
extended.

### H. Security posture assumed but absent
nginx add_header inheritance silently drops all security headers on the
SPA; ingest endpoint unauthenticated; mongo/redis published without auth;
regex injection via unescaped user input. **Rule**: verify the posture
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
