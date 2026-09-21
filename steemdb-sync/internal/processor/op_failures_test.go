package processor

import "testing"

// TestFailedOpTrackerLifecycle exercises the poison-op tracker's contract:
// streaks count CONSECUTIVE failures, recovered ops reset, skipped ops keep
// their streak, and the threshold gates skipping (0 disables).
func TestFailedOpTrackerLifecycle(t *testing.T) {
	tr := newFailedOpTracker(2)

	if tr.shouldSkip("a") {
		t.Fatal("fresh tracker must not skip")
	}

	// Attempt 1: a fails, b fails, c succeeds.
	tr.recordAttempt([]string{"a", "b", "c"}, []string{"a", "b"})
	if got := tr.streak("a"); got != 1 {
		t.Fatalf("streak(a) = %d, want 1", got)
	}
	if tr.shouldSkip("a") {
		t.Fatal("must not skip before the threshold is reached")
	}

	// Attempt 2: a fails again (streak 2), b recovers (streak reset).
	tr.recordAttempt([]string{"a", "b"}, []string{"a"})
	if got := tr.streak("a"); got != 2 {
		t.Fatalf("streak(a) = %d, want 2", got)
	}
	if got := tr.streak("b"); got != 0 {
		t.Fatalf("streak(b) = %d, want 0 (recovered ops reset)", got)
	}
	if !tr.shouldSkip("a") {
		t.Fatal("a must be skipped after reaching the threshold")
	}
	if tr.shouldSkip("b") {
		t.Fatal("recovered b must never be skipped")
	}

	// Attempt 3: a is skipped — recordAttempt must not touch its streak —
	// and the reset after a window commit clears everything.
	tr.recordAttempt([]string{"b"}, nil)
	if got := tr.streak("a"); got != 2 {
		t.Fatalf("streak(a) = %d, want 2 (skipped ops keep their streak)", got)
	}
	tr.reset()
	if got := tr.streak("a"); got != 0 {
		t.Fatalf("streak(a) after reset = %d, want 0", got)
	}
	if tr.shouldSkip("a") {
		t.Fatal("reset tracker must not skip")
	}
}

// TestFailedOpTrackerDisabledThreshold verifies threshold 0 (the default)
// never skips regardless of streak length.
func TestFailedOpTrackerDisabledThreshold(t *testing.T) {
	tr := newFailedOpTracker(0)
	for i := 0; i < 10; i++ {
		tr.recordAttempt([]string{"a"}, []string{"a"})
	}
	if tr.shouldSkip("a") {
		t.Fatal("threshold 0 must disable skipping entirely")
	}
}

// TestFailedOpTrackerNilSafety verifies the nil tracker (Processor literals
// in tests) degrades to "never skip, no bookkeeping".
func TestFailedOpTrackerNilSafety(t *testing.T) {
	var tr *failedOpTracker
	if tr.shouldSkip("a") {
		t.Fatal("nil tracker must not skip")
	}
	if skip, streak, threshold := tr.skipInfo("a"); skip || streak != 0 || threshold != 0 {
		t.Fatalf("nil skipInfo = %v/%d/%d, want false/0/0", skip, streak, threshold)
	}
	tr.recordAttempt([]string{"a"}, []string{"a"}) // must not panic
	tr.reset()                                     // must not panic
}
