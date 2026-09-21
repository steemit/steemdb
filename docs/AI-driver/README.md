# AI-Driver Documentation

Architecture reference for AI agents developing features in this repository.
Written after a five-pass full-codebase review (2026-09-21/22, branch `next`
@ cd21059). These documents exist so that agent-driven development **does not
break the existing architecture**, **reuses what is already there**, and
**avoids pitfalls that have already been paid for**.

## Reading order

1. [01-architecture-overview.md](01-architecture-overview.md) — system shape,
   data flow, design philosophy. Read this first, always.
2. [02-data-model.md](02-data-model.md) — MongoDB collections: who writes,
   who reads, `_id` schemes, naming conventions, index authority.
3. [03-sync-service.md](03-sync-service.md) — the five sync binaries,
   processor internals, invariants, known failure edges.
4. [04-web-service.md](04-web-service.md) — web layering, dual-track data
   access, decoding rules, WebSocket, endpoint conventions.
5. [05-frontend.md](05-frontend.md) — frontend structure, data-flow patterns,
   Steem numeric scales.
6. [06-pitfalls-and-lessons.md](06-pitfalls-and-lessons.md) — incident
   history and review findings, each with the lesson that must be internalized.
7. [07-agent-playbook.md](07-agent-playbook.md) — the constraint list and
   per-task checklists. **Consult this before and after every change.**

## The three rules that prevent most damage

1. **Never decode sync-written collections into rigid Go structs from the web
   service.** Sync documents are open-shaped (RPC pass-through fields). Use
   `bson.M` / lenient projection (`AccountService.GetAccount` is the pattern).
   Rigid decoding has caused five structurally-broken endpoints (labs P0s).
2. **Any new write path into `operations`/`blocks`, and any new processor
   handler, must pass the write-ordering classification
   (`docs/rules/processor-write-ordering.md`), reuse the op-coordinate
   renumbering, and carry a matching index.** Violations lose data silently.
3. **The database is the contract.** Field names, types, and units are defined
   by what the sync side writes (see 02-data-model.md), not by what a reader
   wishes were there. When in doubt, read the writer's code first.

## Relationship to other docs

- `docs/rules/` — mandatory conventions (language, commits, write-ordering
  invariant). Rules there are enforced; this directory explains the system.
- READMEs in each subproject describe intent but have drifted in places.
  **Where a README conflicts with code, code wins**; where this directory
  conflicts with code, fix this directory in the same PR.
- The full review evidence lives outside the repo in
  `~/workspace/agent-share/steemdb/next/` (report + nine pass documents).

## Maintenance

These documents reflect the state at review time. When a change alters any
fact stated here (collection shape, endpoint list, invariants, constraints),
update the relevant file in the same PR. Stale reference docs are worse than
none — an agent that trusts a wrong map breaks things confidently.
