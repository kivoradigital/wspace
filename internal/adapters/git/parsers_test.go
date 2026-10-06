// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/git"
	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TestParseWorktreeList exercises the `worktree list --porcelain` parser
// end-to-end against real git output for ordinary, blank-line-separated
// records (tasks.md 2.14). The detached/prunable cases are covered by the
// white-box TestParseWorktreeList_DetachedAndPrunable in parsers_internal_test.go,
// since reliably producing "prunable" requires deleting a worktree's
// directory out from under git rather than removing it properly.
func TestParseWorktreeList(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}
	ctx := context.Background()

	// An ordinary attached worktree.
	feature := domain.Path(string(clone) + "-feature")
	if err := a.WorktreeAdd(ctx, clone, ports.WorktreeSpec{
		Target: feature, Branch: "feature-a", StartPoint: "main",
	}); err != nil {
		t.Fatalf("WorktreeAdd feature-a: %v", err)
	}

	refs, err := a.WorktreeList(ctx, clone)
	if err != nil {
		t.Fatalf("WorktreeList: %v", err)
	}

	var main, feat *ports.WorktreeRef
	for i := range refs {
		switch refs[i].Branch {
		case "main":
			main = &refs[i]
		case "feature-a":
			feat = &refs[i]
		}
	}
	if main == nil {
		t.Fatalf("expected a record for main, got %+v", refs)
	}
	if feat == nil {
		t.Fatalf("expected a record for feature-a, got %+v", refs)
	}
	// Compare base names only: git canonicalizes worktree paths (resolving
	// symlinks, e.g. macOS's /var -> /private/var), so the reported path
	// need not be byte-identical to what was requested.
	if filepath.Base(string(feat.Path)) != filepath.Base(string(feature)) {
		t.Fatalf("expected feature-a's path to end in %q, got %q", feature, feat.Path)
	}
}

// TestParseStatus_PathWithNewline is a named §13 threat-matrix test: -z
// output means a newline embedded in a path cannot forge a record boundary.
func TestParseStatus_PathWithNewline(t *testing.T) {
	gitfix.RequireGit(t)
	weirdName := "odd\nname.txt"
	gitfix.RequireValidFileName(t, weirdName)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)
	gitfix.Untracked(t, clone, weirdName)

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	entries, err := a.Status(context.Background(), clone)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.RelPath == weirdName {
			found = true
			if e.X != '?' || e.Y != '?' {
				t.Fatalf("expected an untracked entry, got X=%q Y=%q", e.X, e.Y)
			}
		}
	}
	if !found {
		t.Fatalf("expected an entry for the newline-containing path, got %+v", entries)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one status entry (the newline must not split into two), got %d: %+v", len(entries), entries)
	}
}

// TestAheadBehind_ParsesLeftRightCount is a named §13 threat-matrix test:
// `rev-list --left-right --count` output is behind<TAB>ahead, and the
// adapter must not swap the two.
func TestAheadBehind_ParsesLeftRightCount(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)
	other := gitfix.NewClone(t, origin)

	// clone gets 2 local commits not on origin/main.
	gitfix.Commit(t, clone, "local-1.txt", "one\n")
	gitfix.Commit(t, clone, "local-2.txt", "two\n")

	// other advances origin/main by 1 commit clone does not have.
	gitfix.Commit(t, other, "remote-1.txt", "remote\n")
	gitfix.Push(t, other, "main")

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}
	if err := a.Fetch(context.Background(), clone, "origin"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	ahead, behind, err := a.AheadBehind(context.Background(), clone, "origin/main")
	if err != nil {
		t.Fatalf("AheadBehind: %v", err)
	}
	if ahead != 2 {
		t.Fatalf("expected ahead=2, got %d", ahead)
	}
	if behind != 1 {
		t.Fatalf("expected behind=1, got %d", behind)
	}
}
