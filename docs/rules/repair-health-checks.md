# Repair / Health-Check Pairing Rule

Every `repair -mode=<x>` must ship with a corresponding data health check
registered in `checker.DefaultChecks` (`steemdb-sync/internal/checker/
healthcheck.go`) whose `RepairMode()` returns `"<x>"`. A repair mode without
its check is incomplete work.

## Why this rule exists

Repair modes accumulate over time. Months later nobody remembers which ones a
given database still needs — and a repair written for an old data bug is
usually accompanied by a fix to the ingest/cold-start path, so freshly
synced databases never had the bug. Running every repair "just in case" is
wasteful (RPC pressure) and risky (some modes delete data). The health check
answers the only question that matters: **does THIS database need THIS
repair?** It also serves as the post-repair verification: re-run the check,
expect green.

## Contract

- **Checks are read-only.** A check must never write to any collection.
- **Checks are cheap by default.** Prefer count/sample based probes (index
  ranges, sampled windows) over full scans; when only a deep scan is
  authoritative (e.g. per-block gap scan), the check says so in its details
  and points at the deep mode.
- **Repairs stay idempotent.** A repair run on healthy data must converge to
  a no-op (upserts, field-existence guards, dry-run defaults for destructive
  modes). The check is the gate; idempotency is the safety net.
- **A repair may green its check indirectly.** Some repairs unblock a
  background process instead of fixing the measured value themselves (e.g.
  verify-accounts deletes the never-created phantoms blocking the
  AccountRefresher queue head; the phantom-account-stubs check then drains
  to green over hours as real stubs refresh). In that case the check's
  details must say so, so nobody re-runs the repair expecting instant green.
- **Cold-start fixes land together.** When a data bug is fixed, fix the
  ingest/processor path in the same change so new databases never fail the
  check — the repair mode then exists only for legacy data.

## Operator workflow

```bash
repair -mode=check          # read-only; exit 1 if any check fails
# run only the -mode named by each FAIL line
repair -mode=check          # verify all green
```

Adding a check: implement `checker.HealthCheck` against the narrow
`checker.HealthStore` interface (unit-testable without MongoDB), add the
MongoDB-backed query to `checker.MongoHealthStore`, register in
`DefaultChecks`.
