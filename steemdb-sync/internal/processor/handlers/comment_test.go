package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/steemit/steemdb-sync/internal/model"
)

// makeCommentOp builds a comment Operation for testing.
func makeCommentOp(blockNum uint32, v map[string]interface{}) *model.Operation {
	return &model.Operation{
		ID:       fmt.Sprintf("%d:0:0", blockNum),
		BlockNum: blockNum,
		OpType:   "comment",
		OpValue:  v,
	}
}

// TestCommentHandler_MissingAuthor checks error handling for malformed ops.
func TestCommentHandler_MissingAuthor(t *testing.T) {
	h := &CommentHandler{inserter: &MongoInserter{}}

	op := makeCommentOp(100, map[string]interface{}{
		"permlink": "test", // missing author
	})

	err := h.Handle(context.Background(), op, time.Now())
	if err == nil {
		t.Fatal("expected error for missing author, got nil")
	}
	if !strings.Contains(err.Error(), "missing author") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestCommentHandler_MissingPermlink checks error handling for missing permlink.
func TestCommentHandler_MissingPermlink(t *testing.T) {
	h := &CommentHandler{inserter: &MongoInserter{}}

	op := makeCommentOp(100, map[string]interface{}{
		"author": "alice", // missing permlink
	})

	err := h.Handle(context.Background(), op, time.Now())
	if err == nil {
		t.Fatal("expected error for missing permlink, got nil")
	}
	if !strings.Contains(err.Error(), "missing author") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestCommentOptionsHandler_MissingFields checks error for missing author/permlink.
func TestCommentOptionsHandler_MissingFields(t *testing.T) {
	h := &CommentOptionsHandler{inserter: &MongoInserter{}}

	op := &model.Operation{
		ID:       "200:0:0",
		BlockNum: 200,
		OpType:   "comment_options",
		OpValue:  map[string]interface{}{"author": "alice"}, // missing permlink
	}

	err := h.Handle(context.Background(), op, time.Now())
	if err == nil {
		t.Fatal("expected error for missing permlink, got nil")
	}
}

// TestParseJsonMetadata_logic verifies the json_metadata parsing behavior
// that the comment handler relies on.
func TestParseJsonMetadata_logic(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid json", `{"tags":["test"],"app":"steemit/1.0"}`, false},
		{"empty object", `{}`, false},
		{"empty string", "", true},
		{"invalid json", "not json {{{", true},
		{"trailing comma", `{"tags":["a"],}`, true},
		{"nested object", `{"tags":["a","b"],"users":["alice"],"app":{"name":"x","ver":"1"}}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result interface{}
			err := json.Unmarshal([]byte(tt.input), &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestAlreadyApplied(t *testing.T) {
	tests := []struct {
		name        string
		opID        string
		lastApplied string
		want        bool
	}{
		// Baseline behavior retained from the equality guard.
		{"exact match skips replay", "9306617:0:0", "9306617:0:0", true},
		{"different op applies", "9306617:0:0", "9306616:4:0", false},
		{"no marker (legacy doc) applies", "9306617:0:0", "", false},

		// Numeric tuple comparison (op IDs do not sort lexicographically).
		{"older op in same block skips (100:10:0 vs 100:2:0 lexicographic trap)", "100:2:0", "100:10:0", true},
		{"newer op in same block applies (100:2:0 vs 100:10:0 lexicographic trap)", "100:10:0", "100:2:0", false},
		{"same block same trx smaller op_index skips", "100:5:2", "100:5:10", true},
		{"same block same trx larger op_index applies", "100:5:10", "100:5:2", false},
		{"same block same trx equal op_index skips", "100:5:7", "100:5:7", true},
		{"earlier block skips", "99:0:0", "100:0:0", true},
		{"later block applies", "101:0:0", "100:0:0", false},
		{"virtual-op trx 0xFFFFFFFF orders above any int32 trx", "100:2147483647:0", "100:4294967295:0", true},
		{"negative trx (repair convention) orders below zero", "100:-2:0", "100:-1:0", true},
		{"negative trx orders below trx zero", "100:-1:3", "100:0:0", true},

		// Unparseable ids degrade to exact string equality (no new skip surface).
		{"unparseable marker, distinct op applies", "100:0:0", "legacy-marker", false},
		{"unparseable marker, identical op skips", "legacy-marker", "legacy-marker", true},
		{"two-segment marker, identical op skips", "100:0", "100:0", true},
		{"two-segment marker, distinct op applies", "100:0:0", "100:0", false},
		{"four-segment marker, identical op skips", "1:2:3:4", "1:2:3:4", true},
		{"non-numeric marker, distinct op applies", "100:0:0", "a:b:c", false},
		{"unparseable current op id, distinct marker applies", "not:an:op", "100:0:0", false},
		{"overflowing block number does not parse, identical op skips", "99999999999:0:0", "99999999999:0:0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := alreadyApplied(tt.opID, tt.lastApplied); got != tt.want {
				t.Errorf("alreadyApplied(%q, %q) = %v, want %v", tt.opID, tt.lastApplied, got, tt.want)
			}
		})
	}
}

// --- Window-replay sequence tests ---
//
// commentDoc models the persisted comment document exactly as
// CommentHandler's single upsert maintains it: body and last_applied_op are
// written atomically in the same $set (the "comment" collection is
// unbuffered), so the model advances them together — never one without the
// other.
type commentDoc struct {
	body          string
	lastAppliedOp string
}

// dispatch mimics the processor dispatching one comment op through
// CommentHandler.Handle in window order: the replay guard, the diff patch,
// and the atomic (body, marker) write. It reports whether the op took effect
// (false = skipped as already applied).
func (d *commentDoc) dispatch(opID, body string) bool {
	if IsDiffBody(body) {
		res := resolveDiffOp(commentState{Body: d.body, LastAppliedOp: d.lastAppliedOp}, opID, body)
		if res.skip {
			return false
		}
		d.body = res.body
	} else {
		// Non-diff ops $set the full body straight from the op — replay-safe
		// by construction, no guard needed.
		d.body = body
	}
	d.lastAppliedOp = opID
	return true
}

// makeDiff builds a Steem-style diff body (DMP unidiff, "@@ " prefixed)
// transforming base into next — the same format the chain emits for edits.
func makeDiff(base, next string) string {
	dmp := diffmatchpatch.New()
	return dmp.PatchToText(dmp.PatchMake(base, next))
}

// TestCommentReplayGuard_SameWindowDoubleDiff reproduces review finding sync
// F4: two edits of the same comment inside one window, a crash between the
// second write and the cursor advance, then a window replay. The older diff
// must be skipped by the numeric-tuple guard; the body must survive the
// replay byte-identical. The previous equality-only guard re-applied the
// older diff and corrupted the body into B(A(B(A(body0)))).
func TestCommentReplayGuard_SameWindowDoubleDiff(t *testing.T) {
	// Comment created in a previous window; this window holds the two edits.
	// diffA is an insertion: DMP's fuzzy apply provably duplicates it when
	// applied twice ("INSERTED INSERTED"), which is the visible corruption
	// mode the guard must prevent.
	base := "alpha beta gamma"
	v1 := "alpha INSERTED beta gamma"
	v2 := "alpha INSERTED beta GAMMA"
	diffA := makeDiff(base, v1) // op 101:0:0
	diffB := makeDiff(v1, v2)   // op 102:0:0

	var d commentDoc
	if !d.dispatch("99:0:0", base) {
		t.Fatal("initial post from the previous window must apply")
	}
	if !d.dispatch("101:0:0", diffA) {
		t.Fatal("first diff must apply on first run")
	}
	if !d.dispatch("102:0:0", diffB) {
		t.Fatal("second diff must apply on first run")
	}
	if d.body != v2 {
		t.Fatalf("after first run body = %q, want %q", d.body, v2)
	}
	if d.lastAppliedOp != "102:0:0" {
		t.Fatalf("marker after first run = %q, want 102:0:0", d.lastAppliedOp)
	}

	// Crash before the cursor advance: the whole window replays, re-dispatching
	// both diffs. Idempotence must hold across repeated replays.
	for round := 1; round <= 2; round++ {
		if d.dispatch("101:0:0", diffA) {
			t.Fatalf("replay round %d: older diff A must be skipped (101 <= 102)", round)
		}
		if d.dispatch("102:0:0", diffB) {
			t.Fatalf("replay round %d: diff B must be skipped (102 <= 102)", round)
		}
		if d.body != v2 {
			t.Fatalf("replay round %d corrupted body: got %q, want %q", round, d.body, v2)
		}
		if d.lastAppliedOp != "102:0:0" {
			t.Fatalf("replay round %d changed marker: got %q", round, d.lastAppliedOp)
		}
	}

	// Sanity: the scenario is a real corruption case — unguarded re-apply of
	// diff A on the final body would change it (double patch).
	if repatched, ok := ApplySteemDiff(v2, diffA); ok && repatched == v2 {
		t.Fatal("vacuous scenario: unguarded re-apply of diff A does not change the body")
	}
}

// TestCommentReplayGuard_NormalOrderApplication checks the non-crash path:
// ascending ops apply in order, the marker tracks the newest op, and an exact
// re-dispatch of the newest op (the single-diff replay the guard was
// originally written for) stays a no-op.
func TestCommentReplayGuard_NormalOrderApplication(t *testing.T) {
	base := "one two three"
	v1 := "one TWO three"
	v2 := "one TWO three four"
	diffA := makeDiff(base, v1)
	diffB := makeDiff(v1, v2)

	var d commentDoc
	if !d.dispatch("200:1:0", base) {
		t.Fatal("initial post must apply")
	}
	if !d.dispatch("201:0:0", diffA) {
		t.Fatal("diff A must apply in order")
	}
	if d.body != v1 || d.lastAppliedOp != "201:0:0" {
		t.Fatalf("after diff A: body=%q marker=%q", d.body, d.lastAppliedOp)
	}
	if !d.dispatch("202:3:5", diffB) {
		t.Fatal("diff B must apply in order")
	}
	if d.body != v2 || d.lastAppliedOp != "202:3:5" {
		t.Fatalf("after diff B: body=%q marker=%q", d.body, d.lastAppliedOp)
	}

	// Single-diff replay (crash right after this op's write): the identical
	// op re-dispatched must be skipped, leaving state untouched.
	if d.dispatch("202:3:5", diffB) {
		t.Fatal("exact re-dispatch of the newest op must be skipped")
	}
	if d.body != v2 || d.lastAppliedOp != "202:3:5" {
		t.Fatalf("skipped op must not touch state: body=%q marker=%q", d.body, d.lastAppliedOp)
	}
}

// TestCommentReplayGuard_FullWindowReplay checks the window-scale scenario:
// the marker sits at the replayed window's last op (written before the
// crash), so a full replay of the window skips every diff of this comment and
// leaves the body untouched; the next window's newer ops still apply.
func TestCommentReplayGuard_FullWindowReplay(t *testing.T) {
	base := "v0 content"
	v1 := "v1 content"
	v2 := "v2 content"
	v3 := "v3 content"
	v4 := "v4 content"
	diffA := makeDiff(base, v1) // block 100 (window start)
	diffB := makeDiff(v1, v2)   // block 130
	diffC := makeDiff(v2, v3)   // block 163 (window end, 64-block window)
	diffD := makeDiff(v3, v4)   // block 164 (next window)

	// Comment created in the previous window; the current window holds three
	// edits, the last one at the window's final block.
	var d commentDoc
	if !d.dispatch("99:0:0", base) {
		t.Fatal("initial post from the previous window must apply")
	}
	windowDiffs := [][2]string{
		{"100:6:0", diffA},
		{"130:0:0", diffB},
		{"163:7:1", diffC},
	}
	for _, w := range windowDiffs {
		if !d.dispatch(w[0], w[1]) {
			t.Fatalf("first run: op %s must apply", w[0])
		}
	}
	if d.body != v3 || d.lastAppliedOp != "163:7:1" {
		t.Fatalf("after first run: body=%q marker=%q", d.body, d.lastAppliedOp)
	}

	// Crash before cursor advance: the ENTIRE window replays. Every diff of
	// this comment is <= the marker → all skipped, state unchanged.
	for _, w := range windowDiffs {
		if d.dispatch(w[0], w[1]) {
			t.Fatalf("window replay must skip op %s (<= marker 163:7:1)", w[0])
		}
	}
	if d.body != v3 || d.lastAppliedOp != "163:7:1" {
		t.Fatalf("window replay must not change state: body=%q marker=%q", d.body, d.lastAppliedOp)
	}

	// The next window's op is newer than the marker → applies normally.
	if !d.dispatch("164:0:0", diffD) {
		t.Fatal("op from the next window must not be skipped")
	}
	if d.body != v4 {
		t.Fatalf("after next-window op: body=%q, want %q", d.body, v4)
	}
}

// TestCommentReplayGuard_MixedWindowReplayConvergence checks a window that
// mixes a non-diff full-body op with diffs to the same comment. The non-diff
// op re-applies on replay (idempotent full-body write) and atomically resets
// both body and marker; the diffs then re-derive forward, so the replay
// converges to the same final state. This is why the marker must live in the
// same $set as the body — a split write would leave marker and body
// disagreeing and break this convergence.
func TestCommentReplayGuard_MixedWindowReplayConvergence(t *testing.T) {
	fullBody := "full body from op at 100:5:2"
	v1 := "full body from op at 100:5:2, patched once"
	v2 := "full body from op at 100:5:2, patched once and twice"
	diffA := makeDiff(fullBody, v1) // op 100:6:0, after the full-body op
	diffB := makeDiff(v1, v2)       // op 130:0:0

	windowOps := [][2]string{
		{"100:5:2", fullBody}, // non-diff full-body op
		{"100:6:0", diffA},
		{"130:0:0", diffB},
	}

	run := func(d *commentDoc) {
		for _, w := range windowOps {
			d.dispatch(w[0], w[1])
		}
	}

	var firstRun commentDoc
	run(&firstRun)
	if firstRun.body != v2 || firstRun.lastAppliedOp != "130:0:0" {
		t.Fatalf("first run: body=%q marker=%q", firstRun.body, firstRun.lastAppliedOp)
	}

	// The crash persists the first run's final state (body and marker were
	// written atomically); the window then replays over that state.
	replay := commentDoc{body: firstRun.body, lastAppliedOp: firstRun.lastAppliedOp}
	run(&replay)
	if replay.body != firstRun.body || replay.lastAppliedOp != firstRun.lastAppliedOp {
		t.Fatalf("window replay must converge: got body=%q marker=%q, want body=%q marker=%q",
			replay.body, replay.lastAppliedOp, firstRun.body, firstRun.lastAppliedOp)
	}
}
