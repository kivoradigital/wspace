// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// update regenerates every golden file this package's tests compare
// against (design.md §12: "regenerated only via -update").
var update = flag.Bool("update", false, "update golden files")

// tmpDirPattern normalizes any real t.TempDir()-derived path a fake port
// might leak into rendered output (design.md §12: "time, paths, and
// version are normalized before comparison"). Fixtures in this package use
// fixed literal paths for the common case, so this rarely fires — it
// exists for the one golden test (TestCLI_List_JSONContract) that
// deliberately exercises a FakeFS-derived root, proving the helper is
// real, not decorative.
// On Windows the path is matched both as os.TempDir() returns it
// (backslashes) and in the slash form domain.Path uses.
var tmpDirPattern = regexp.MustCompile(`(?:` +
	regexp.QuoteMeta(strings.TrimRight(os.TempDir(), `/\`)) + `|` +
	regexp.QuoteMeta(strings.TrimRight(filepath.ToSlash(os.TempDir()), "/")) +
	`)[^"\s]*`)

// timePattern normalizes RFC3339-ish timestamps.
var timePattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?`)

// versionPattern normalizes a semver-ish version string.
var versionPattern = regexp.MustCompile(`\bv?\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?\b`)

// normalizeGolden is the one shared normalization helper every CLI golden
// test in this package reuses (tasks.md 4b.19).
func normalizeGolden(s string) string {
	s = tmpDirPattern.ReplaceAllString(s, "<TMPDIR>")
	s = timePattern.ReplaceAllString(s, "<TIME>")
	s = versionPattern.ReplaceAllString(s, "<VERSION>")
	return s
}

// compareGolden compares got (after normalization) against
// testdata/<name>, rewriting the golden file instead when -update is set.
func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	got = normalizeGolden(got)
	path := filepath.Join("testdata", name)

	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", name, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, string(want))
	}
}
