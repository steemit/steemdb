# 07 — Agent Playbook (constraints + per-task checklists)

Mandatory constraints distilled from the design review, followed by
checklists for the most common change types. C = critical (violation
causes incidents), S = structural (violation causes drift debt).

## Global constraints

**Write path integrity**

1. 【C】New/modified processor handlers must be classified per
   `docs/rules/processor-write-ordering.md` BEFORE registration; a
   "block-unique `_id`" claim requires (block, trx_index, op_index) in the
   `_id` or a chain-level uniqueness proof. Six existing handlers violate
   this — don't add a seventh.
2. 【C】Pattern-B (filter upsert) handlers: the filter's fields AND leading
   order must have a matching index in sync's `createIndexes` in the same
   PR. No test guards this coupling.
3. 【C】Any new writer into `operations`/`blocks` (ingest, backfill, repair
   tools): write order ops → transactions → blocks → meta; reuse
   `live_sync`'s `convertBlockOps` for coordinate renumbering (op_in_trx
   is unreliable; virtual ops = trx 0xFFFFFFFF / op 1..n).
4. 【C】Cursor advance requires window writes all succeeded AND handler
   errCount == 0 (or an explicit, commented exemption). Writes outside
   FlushAll (direct-write bypass, mid-window flushes) need their own
   failure semantics — a transient error must never become permanent loss.
5. 【C】Idempotency markers must compare coordinates numerically;
   `op.ID` strings do not sort (`"100:10:0" < "100:2:0"`).
6. 【C】Replayable writes must be deterministic functions of the op — no
   `time.Now()` inside handler-written fields (e.g. `scanned`).
7. 【C】Parse failures of chain data (timestamps, metadata) are errors to
   retry, never values to fabricate (`time.Now()` fallbacks pollute `_ts`
   chains and break dedup filters).

**Read path integrity**

8. 【C】Web reads of sync-written collections: `bson.M`/lenient projection
   only, never rigid structs (see 04). One unconverted writer field = one
   structurally broken endpoint.
9. 【C】Account identity = `account._id`. `name` is display-only; never a
   join key (`$lookup foreignField`), search filter, or sort key.
10. 【S】Amounts/counts are float64 in the DB; readers decode numbers, and
    any new writer conversion list must cover the fields readers touch
    (extend `processAccount`'s lists rather than parsing in readers).
11. 【S】Field naming: raw layer `block_num`/`timestamp`, derived layer
    `_block`/`_ts` (02-data-model). Verify bson tags against the writer
    before adding fields; imagined fields are silent zeros.
12. 【S】Immutable history: local Mongo is the authority (RPC = enrich
    fallback only, marked); current-state: RPC allowed but one response =
    one source/instant. Don't add "local data exists but call RPC anyway"
    endpoints.
13. 【S】List endpoints: implement the alias fallback set
    (`page_size`/`limit`, `sort_by`/`sort`, `sort_order`/`order`;
    `block_handler` pattern) + sort-field whitelist; frontend sends one
    canonical set; changes land in all three tiers in one PR.
14. 【S】Empty-filter counts on big collections: EstimatedDocumentCount;
    exact counts/aggregations require a verified index first.
15. 【S】User input: `regexp.QuoteMeta` into `$regex`; whitelisted sort
    fields; bounded page/skip; validate numeric ranges.
16. 【S】Steem scales: voting_power/weight ÷100 for %, sbd_interest_rate
    ÷100, VESTS ≠ STEEM units — every displayed `%`/unit gets checked
    (05).
17. 【S】WS channel/message shape is a three-tier contract; subscription
    state is per-connection; changes update web + frontend + docs
    together.

**Structure & ops**

18. 【S】Index authority for sync-written collections = sync's
    `createIndexes` alone; web must not create indexes on them (migrate
    web's existing phantom entries out when touching them).
19. 【S】Multi-writer collections (`blocks`, `comment`, `account`, `meta`,
    `status`): before changing a field, enumerate all writers and their
    semantics (02 §collection reference); new fields declare one owner.
    Rescanner/refresher may only `$set` their dynamic-field whitelist,
    never op-derived fields (body/created).
20. 【S】Every collection/field needs a reader or an explicit "reserved"
    note; don't extend the currently-unread set
    (follow/transactions/witness_misses/clients_history/feed_publish/pow/
    witness_vote).
21. 【S】Shared-Mongo deployment contract: all services take `MONGO_URI`;
    compose changes must not silently break "sync pipeline writes, web
    stack reads, same DB" — and compose files state which services are
    expected where.
22. 【S】steemgosdk/steemutil versions stay aligned across web and sync;
    upgrade together, re-verify decode behavior of shared RPC shapes.
23. 【S】Config triple (env tag ↔ loadFromEnv ↔ config.yaml section) lands
    or dies together; docs describe only real routes/features.
24. 【S】Timestamps persisted in UTC only; no local-timezone `time.Now()`
    into stored fields; day-boundary parsing pins a documented timezone.
25. 【S】Legacy 302 layer: verify old query semantics (e.g. `days`)
    before pointing an old path at a new endpoint; return 410 when
    shapes are incompatible.
26. 【C】(Process) All Steem chain connectivity goes through `steemgosdk`
    — enforced repo rule; never add another RPC client.

## Per-task checklists

### Add a processor handler (new op_type)
- [ ] Classify write pattern (1/2/3) in the PR description; for class 1,
      prove `_id` uniqueness within a block.
- [ ] Class 2: index matching filter (leading order) added to sync
      `createIndexes` in the same PR.
- [ ] Class 3: declare read dependencies; ensure the collection is on the
      unbuffered bypass or flushes precede the read; idempotency marker
      with numeric coordinate comparison.
- [ ] Amount fields via `AssetValue` (float64); names via helpers;
      `_dirty` marking for touched accounts.
- [ ] Test: same op replayed twice converges; same-window duplicate
      semantics defined.

### Add a web endpoint
- [ ] Service reads sync collection via bson.M/projection; no rigid
      structs on sync-written collections; lenient `json_metadata`.
- [ ] Params: alias fallback + whitelist + bounds; QuoteMeta regex;
      EstimatedDocumentCount for totals.
- [ ] Queries checked against 02's index list (or index added sync-side).
- [ ] Error mapping distinguishes missing vs corrupt; no invented data.
- [ ] Route registered in routes.go; README/SUMMARY updated to match
      reality (or not mentioned).
- [ ] If it replaces/introduces a legacy redirect target: check old
      semantics.

### Add/modify a frontend page
- [ ] Query keys include all params; staleTime appropriate (0 for live
      data).
- [ ] Param names match the implemented handler (verify in routes.go /
      handler code, not types).
- [ ] All Steem scales/units per 05; placeholders for missing fields
      (stub accounts).
- [ ] Zustand selectors; cleanup of subscriptions/timers; WS pages
      respect channel ownership.
- [ ] `pnpm exec tsc --noEmit` and lint pass.

### Add a collection / backfill tool
- [ ] `_id` deterministic; writes idempotent; write order rule 3.
- [ ] A reader exists or "reserved" is documented; ownership declared.
- [ ] Indexes through sync's createIndexes — built synchronously at
      startup on a dedicated no-deadline context, so a first startup that
      backfills a new index on a large DB blocks minutes-to-hours by
      design (02 §index authority); pre-creating out-of-band is an
      optional ops shortcut, no longer a timeout workaround.

### Touch compose / deploy
- [ ] Shared-Mongo contract preserved; MONGO_URI overridable everywhere.
- [ ] Which services exist where stays documented; no silent removal of
      a writer for collections web reads.
- [ ] Ports: cold_ingest 8080/9090 collide with web's on a shared host.
- [ ] Don't run e2e next to anything on host port 8080.

### Touch the batcher / processor core
- [ ] Map lifecycle: entries die on success; no second bookkeeping map.
- [ ] Lock scope O(1); nothing scans a growing structure under a lock.
- [ ] Failure paths: what state survives, what replays, what a crash
      between any two statements leaves behind (write the interleaving
      analysis in the PR).
