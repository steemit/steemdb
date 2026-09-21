package processor

// failedOpTracker counts, per operation, how many consecutive window attempts
// ended with that op's handler failing, and decides when a persistently
// failing ("poison") op is skipped so one deterministic op error cannot stall
// the processor forever (sync review finding F5 — the error gate itself never
// gives up on a window by default).
//
// The retry threshold is processor.skip_error_ops_after_retries (env
// PROCESSOR_SKIP_ERROR_OPS_AFTER_RETRIES). The default 0 DISABLES skipping: a
// stalled cursor is loudly visible (per-attempt error logs, frozen
// status.processor_height), while silently skipped ops are not. Operators opt
// in when they judge a poison op must be sacrificed to keep deriving.
//
// State is in-memory only — a restart resets the counters, so every op re-earns
// its retries after a process restart. Counters are cleared whenever a window
// commits; they are per-window-retry state and meaningless across windows.
type failedOpTracker struct {
	// threshold is the number of consecutive failed attempts after which an
	// op is skipped; 0 never skips.
	threshold int
	failures  map[string]int
}

// newFailedOpTracker builds a tracker; threshold <= 0 disables skipping.
func newFailedOpTracker(threshold int) *failedOpTracker {
	return &failedOpTracker{
		threshold: threshold,
		failures:  make(map[string]int),
	}
}

// shouldSkip reports whether opID has exhausted its consecutive retries.
// Safe on a nil tracker (never skips).
func (t *failedOpTracker) shouldSkip(opID string) bool {
	if t == nil || t.threshold <= 0 {
		return false
	}
	return t.failures[opID] >= t.threshold
}

// skipInfo is shouldSkip plus the data needed to log a skip loudly: the op's
// current streak and the configured threshold. Safe on a nil tracker.
func (t *failedOpTracker) skipInfo(opID string) (skip bool, streak, threshold int) {
	if t == nil {
		return false, 0, 0
	}
	return t.shouldSkip(opID), t.failures[opID], t.threshold
}

// recordAttempt updates the counters after one window attempt: dispatched ops
// that failed again extend their streak, dispatched ops that recovered reset
// it (transient errors must not accumulate towards the threshold), and
// skipped ops — which were not dispatched — keep their streak so they stay
// skipped. Safe on a nil tracker (no-op).
func (t *failedOpTracker) recordAttempt(dispatched, failed []string) {
	if t == nil {
		return
	}
	failedSet := make(map[string]struct{}, len(failed))
	for _, id := range failed {
		failedSet[id] = struct{}{}
	}
	for _, id := range dispatched {
		if _, ok := failedSet[id]; ok {
			t.failures[id]++
		} else {
			delete(t.failures, id)
		}
	}
}

// reset clears all state after a window commit. Safe on a nil tracker.
func (t *failedOpTracker) reset() {
	if t == nil {
		return
	}
	t.failures = make(map[string]int)
}

// streak returns the current consecutive-failure count of opID (0 when none).
func (t *failedOpTracker) streak(opID string) int {
	if t == nil {
		return 0
	}
	return t.failures[opID]
}
