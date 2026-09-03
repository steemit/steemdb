package handlers

import (
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// ApplySteemDiff applies a Steem-style diff patch to a base text.
//
// Steem diffs use Google's diff-match-patch Unidiff format. Percent-encoded
// characters (%0A, %E4%B8%AD, etc.) are part of the DMP patch format itself —
// PatchFromText handles them natively. Do NOT URL-decode the body before parsing,
// as that would break the patch structure (real newlines would split add-lines).
//
// Returns the patched text and true on success, or (baseText, false) if the patch
// could not be applied (e.g. orphan diff with no base text, or patch mismatch).
func ApplySteemDiff(baseText, diffBody string) (result string, ok bool) {
	// PatchApply panics on pathological patches (index out of range inside
	// go-diff) — observed live on a 2017-era edit. Treat a panic exactly like
	// an unappliable patch so the caller stores the raw diff instead of
	// taking down the process.
	defer func() {
		if r := recover(); r != nil {
			result, ok = baseText, false
		}
	}()

	// Parse the diff body into Patches using diff-match-patch.
	// No URL decode needed — DMP PatchFromText handles %XX encoding natively.
	dmp := diffmatchpatch.New()
	patches, err := dmp.PatchFromText(diffBody)
	if err != nil || len(patches) == 0 {
		return baseText, false
	}

	// Apply patches to the base text.
	// PatchApply returns the patched text and a []bool indicating which patches applied.
	patched, applied := dmp.PatchApply(patches, baseText)

	// Check that all patches applied successfully.
	for _, ok := range applied {
		if !ok {
			return baseText, false
		}
	}

	return patched, true
}

// IsDiffBody returns true if the body string is a Steem diff patch (starts with "@@ ").
func IsDiffBody(body string) bool {
	return strings.HasPrefix(body, "@@ ")
}
