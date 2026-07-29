package handlers

import (
	"strings"
	"testing"
)

func TestIsDiffBody(t *testing.T) {
	tests := []struct {
		body   string
		expect bool
	}{
		{"Hello world", false},
		{"@@ -1,5 +1,10 @@\n hello\n+ world\n", true},
		{"@@@ something", false},
		{"", false},
		{"@ mention", false},
		{"@@ ", true},
		{"@@@ ", false},
	}

	for _, tt := range tests {
		label := tt.body
		if len(label) > 30 {
			label = label[:30] + "..."
		}
		t.Run(label, func(t *testing.T) {
			got := IsDiffBody(tt.body)
			if got != tt.expect {
				t.Errorf("IsDiffBody(%q) = %v, want %v", tt.body, got, tt.expect)
			}
		})
	}
}

func TestApplySteemDiff_PlainText(t *testing.T) {
	// Real data: red/fast-reply edit chain from Steem chain
	// block 653461: full body "Just fixed posting form performance.... ;)"
	// block 820398: diff removes one "."  from the middle
	// block 820498: diff removes ".."     from the middle

	base := "Just fixed posting form performance.... ;)"

	// First diff: "@@ -31,12 +31,11 @@\n mance...\n-.\n ;)"
	// DMP PatchApply is fuzzy — verify the NET effect (one "." removed) rather than exact string
	diff1 := "@@ -31,12 +31,11 @@\n mance...\n-.\n ;)"
	patched1, ok := ApplySteemDiff(base, diff1)
	if !ok {
		t.Fatal("first diff apply failed")
	}
	// One "." should have been removed (4 dots → 3 dots)
	dotCount1 := strings.Count(patched1, ".")
	if dotCount1 != 3 {
		t.Errorf("after diff1: expected 3 dots, got %d in %q", dotCount1, patched1)
	}

	// Second diff: "@@ -32,10 +32,7 @@\n ance\n-...\n ;)"
	// Removes "..." (3 dots)
	diff2 := "@@ -32,10 +32,7 @@\n ance\n-...\n ;)"
	patched2, ok := ApplySteemDiff(patched1, diff2)
	if !ok {
		t.Fatal("second diff apply failed")
	}
	dotCount2 := strings.Count(patched2, ".")
	if dotCount2 != 0 {
		t.Errorf("after diff2: expected 0 dots, got %d in %q", dotCount2, patched2)
	}
	// Should still contain the core text
	if !strings.Contains(patched2, "performance") {
		t.Errorf("after diff2: lost 'performance' in %q", patched2)
	}
}

func TestApplySteemDiff_UrlEncoded(t *testing.T) {
	// Real data: dantheman/re-red-fast-reply diff with %0A (newline)
	// Patch header @@ -36,8 +36,41 @@ means context " testing." starts at position 36.
	// Construct a base text where " testing." is at position 36.
	base := "abcdefghijklmnopqrstuvwxyz0123456789 testing."
	diff := "@@ -36,8 +36,41 @@\n testing.\n+%0A%0AAdding a new line in this edit.\n"

	patched, ok := ApplySteemDiff(base, diff)
	if !ok {
		t.Fatal("url-encoded diff apply failed")
	}

	// The result should contain "Adding a new line"
	if !strings.Contains(patched, "Adding a new line") {
		t.Errorf("patched text does not contain expected addition: %q", patched)
	}
	if !strings.Contains(patched, "testing.") {
		t.Errorf("patched text lost original content: %q", patched)
	}
}

func TestApplySteemDiff_UrlEncodedChinese(t *testing.T) {
	// Real data: red/re-abit-... diff replacing text with Chinese characters
	// %E4%B8%AD = UTF-8 for "中"
	diff := "@@ -1,40 +1,7 @@\n-Trying to make a reply on the spam post1\n+%E4%B8%AD patch"
	base := "Trying to make a reply on the spam post1"

	patched, ok := ApplySteemDiff(base, diff)
	if !ok {
		t.Fatal("chinese-encoded diff apply failed")
	}

	if !strings.Contains(patched, "中") {
		t.Errorf("patched text should contain Chinese char 中: %q", patched)
	}
	if !strings.Contains(patched, "patch") {
		t.Errorf("patched text should contain 'patch': %q", patched)
	}
}

func TestApplySteemDiff_EmptyBase(t *testing.T) {
	// Orphan diff with empty base — should fail gracefully
	diff := "@@ -1,5 +1,10 @@\n hello\n+ world\n"
	patched, ok := ApplySteemDiff("", diff)
	if ok {
		t.Errorf("expected apply to fail on empty base, got: %q", patched)
	}
}

func TestApplySteemDiff_InvalidDiff(t *testing.T) {
	// Garbage that's not a valid diff
	patched, ok := ApplySteemDiff("hello", "not a diff at all")
	if ok {
		t.Errorf("expected apply to fail on invalid diff, got: %q", patched)
	}
	// Should return base unchanged
	if patched != "hello" {
		t.Errorf("expected base returned on failure, got: %q", patched)
	}
}

func TestApplySteemDiff_NoChange(t *testing.T) {
	// Diff that results in no change
	base := "hello world"
	diff := "@@ -1,11 +1,11 @@\n hello world\n"
	patched, ok := ApplySteemDiff(base, diff)
	// An identity patch should apply successfully
	if ok && patched != "hello world" {
		t.Errorf("identity diff changed text: %q", patched)
	}
}
