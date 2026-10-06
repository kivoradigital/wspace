// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/git"
	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TestGitAdapter_TerminatesOptionsWithDoubleDash is a named §13 threat-matrix
// test, covering WorktreeAdd/DeleteBranch/BranchExists (tasks.md 2.12): a
// flag-like branch value must never be parsed as an option by the
// underlying git invocation.
//
// domain.NewBranchName already rejects a "-"-leading value, so a
// domain.BranchName carrying one can only reach the adapter if a caller
// bypasses the constructor (BranchName is a plain string alias, not an
// opaque type). This test does exactly that, deliberately, to prove the
// adapter's own "--" placement is a real second layer of defense and not
// merely decorative given the domain-level check.
func TestGitAdapter_TerminatesOptionsWithDoubleDash(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}
	ctx := context.Background()

	//nolint:staticcheck // deliberately bypassing NewBranchName; see doc comment above
	flagLike := domain.BranchName("--force")

	// BranchExists: `show-ref --verify --quiet -- refs/heads/--force` must
	// resolve "--force" as a literal (nonexistent) ref, not as an option —
	// it must return (false, nil), never an error from a misparsed flag.
	exists, err := a.BranchExists(ctx, clone, flagLike)
	if err != nil {
		t.Fatalf("BranchExists must treat --force as a literal, nonexistent ref, got error: %v", err)
	}
	if exists {
		t.Fatal("expected --force to not exist as a branch")
	}

	// DeleteBranch: `branch -d -- --force` must report "not found", not
	// silently no-op or affect an unrelated branch via flag reinterpretation.
	err = a.DeleteBranch(ctx, clone, flagLike, false)
	if err == nil {
		t.Fatal("expected an error deleting a nonexistent literal branch named --force")
	}
	if domain.Code(err) != domain.CodeGitFailed {
		t.Fatalf("expected CodeGitFailed, got %q (err: %v)", domain.Code(err), err)
	}

	// WorktreeAdd: `worktree add -- <target> --force` must attempt to
	// resolve "--force" as a literal ref (and fail, since it does not
	// exist), never as the worktree-add "--force" safety-override flag.
	target := domain.Path(string(clone) + "-flaglike")
	err = a.WorktreeAdd(ctx, clone, ports.WorktreeSpec{Target: target, Branch: flagLike})
	if err == nil {
		t.Fatal("expected an error resolving --force as a literal branch reference")
	}
	if domain.Code(err) != domain.CodeGitFailed {
		t.Fatalf("expected CodeGitFailed, got %q (err: %v)", domain.Code(err), err)
	}
}

func TestGitAdapter_WorktreeLifecycle(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}
	ctx := context.Background()

	target := domain.Path(string(clone) + "-feature")
	err = a.WorktreeAdd(ctx, clone, ports.WorktreeSpec{
		Target:     target,
		Branch:     "feature-x",
		StartPoint: "main",
	})
	if err != nil {
		t.Fatalf("WorktreeAdd (new branch): %v", err)
	}

	refs, err := a.WorktreeList(ctx, clone)
	if err != nil {
		t.Fatalf("WorktreeList: %v", err)
	}
	found := false
	for _, r := range refs {
		if r.Branch == "feature-x" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected feature-x in WorktreeList, got %+v", refs)
	}

	if err := a.WorktreeRemove(ctx, clone, target, false); err != nil {
		t.Fatalf("WorktreeRemove: %v", err)
	}
	if err := a.WorktreePrune(ctx, clone); err != nil {
		t.Fatalf("WorktreePrune: %v", err)
	}
}
