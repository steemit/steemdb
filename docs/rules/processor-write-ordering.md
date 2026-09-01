# Processor Write-Ordering Invariant

The steemdb-sync processor (internal/processor) dispatches operations in
windows and buffers derived-collection writes into per-collection buckets that
are flushed as unordered BulkWrite. An unordered bulk write does NOT guarantee
execution order between two operations that target the SAME document — the
final state would be nondeterministic.

**The invariant**: within one window buffer, two buffered writes must never
target the same document unless ordering is explicitly preserved.

## Handler classification checklist (mandatory for new op types)

When adding or modifying a processor handler — including op types introduced
by future chain hardforks — classify its write pattern and follow the matching
rule BEFORE registering the handler:

1. **Block-unique `_id`** — the document id contains the block number (e.g.
   vote's `{block}/{voter}/{author}/{permlink}`). Same document cannot recur
   within a window. Safe to buffer for unordered bulk write.

2. **Filter-based upsert** (`UpsertOneByFilter`) — the write bucket must
   detect a pending write with an identical filter and flush that bucket
   before appending, so the later op wins. Same-block recurrences are real:
   witness_vote can vote and unvote the same witness in one block.

3. **Stateful read-modify-write** — the handler reads a collection before
   writing (e.g. the comment diff path reads the current body). The handler
   must declare its read dependency (ReadBeforeWrite interface) so the
   dispatcher flushes that collection first; the document must carry an
   idempotency marker (`last_applied_op`) so replaying a window after a crash
   cannot double-apply the effect.

## Why this rule exists

Window-level cursor commit means a crash replays the whole window. Ordering
violations surface only under replay or same-window duplicates — unit tests
on single ops will not catch them. This invariant is what makes
"replay the window" a safe recovery strategy.

Design details and audit table:
`agent-share/notes/steemdb/processor-batching-design.md`.
