// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// TestParseWorktreeList_DetachedAndPrunable is a white-box table test
// against captured real `git worktree list --porcelain` output (tasks.md
// 2.14), covering the detached and prunable markers that are awkward to
// provoke reliably in an end-to-end test.
func TestParseWorktreeList_DetachedAndPrunable(t *testing.T) {
	// Captured from a real `git worktree list --porcelain` run: one main
	// clone on "main", one detached worktree, and one prunable worktree
	// (its directory was removed without `worktree remove`).
	const captured = "worktree /repo\n" +
		"HEAD 1111111111111111111111111111111111111111\n" +
		"branch refs/heads/main\n" +
		"\n" +
		"worktree /repo-detached\n" +
		"HEAD 2222222222222222222222222222222222222222\n" +
		"detached\n" +
		"\n" +
		"worktree /repo-gone\n" +
		"HEAD 3333333333333333333333333333333333333333\n" +
		"branch refs/heads/feature-gone\n" +
		"prunable gitdir file points to non-existent location\n" +
		"\n"

	refs := parseWorktreeList(captured)
	if len(refs) != 3 {
		t.Fatalf("expected 3 records, got %d: %+v", len(refs), refs)
	}

	if refs[0].Path != domain.Path("/repo") || refs[0].Branch != domain.BranchName("main") || refs[0].Detached || refs[0].Prunable {
		t.Fatalf("unexpected main-clone record: %+v", refs[0])
	}
	if refs[1].Path != domain.Path("/repo-detached") || !refs[1].Detached || refs[1].Branch != "" {
		t.Fatalf("unexpected detached record: %+v", refs[1])
	}
	if refs[2].Path != domain.Path("/repo-gone") || refs[2].Branch != domain.BranchName("feature-gone") || !refs[2].Prunable {
		t.Fatalf("unexpected prunable record: %+v", refs[2])
	}
}

// TestParseStatusZ_RenameConsumesTwoFields is a white-box test against
// captured `status --porcelain=v1 -z` output for a staged rename, whose
// record consumes two NUL-separated fields (new path, then old path) rather
// than the "old -> new" arrow form used without -z (design.md §7).
func TestParseStatusZ_RenameConsumesTwoFields(t *testing.T) {
	// "R  new.txt\0old.txt\0M  other.txt\0"
	const captured = "R  new.txt\x00old.txt\x00M  other.txt\x00"

	entries := parseStatusZ(captured)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (rename consumes its extra field), got %d: %+v", len(entries), entries)
	}
	if entries[0].RelPath != "new.txt" || entries[0].X != 'R' {
		t.Fatalf("expected the rename entry to report the new path, got %+v", entries[0])
	}
	if entries[0].OrigPath != "old.txt" {
		t.Fatalf("expected the rename entry to keep its source path, got %+v", entries[0])
	}
	if entries[1].RelPath != "other.txt" || entries[1].X != 'M' {
		t.Fatalf("expected the second entry to be the unrelated modification, got %+v", entries[1])
	}
}

func TestParseLeftRightCount(t *testing.T) {
	behind, ahead, err := parseLeftRightCount("1\t2\n")
	if err != nil {
		t.Fatalf("parseLeftRightCount: %v", err)
	}
	if behind != 1 || ahead != 2 {
		t.Fatalf("expected behind=1 ahead=2, got behind=%d ahead=%d", behind, ahead)
	}
}
