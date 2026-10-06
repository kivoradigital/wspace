// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/git"
	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// updateRepo is a clone on branch "feat" (one local commit touching
// feat.txt) whose origin/main has since advanced by one commit touching
// main.txt, already fetched.
type updateRepo struct {
	a     *git.Adapter
	repo  domain.Path
	other domain.Path // a second clone that pushes to origin/main
}

func newUpdateRepo(t *testing.T) updateRepo {
	t.Helper()
	gitfix.RequireGit(t)
	origin := gitfix.NewOrigin(t)
	repo := gitfix.NewClone(t, origin)
	other := gitfix.NewClone(t, origin)
	gitfix.Git(t, repo, "checkout", "-q", "-b", "feat")
	gitfix.Commit(t, repo, "feat.txt", "feat\n")
	gitfix.Commit(t, other, "main.txt", "main\n")
	gitfix.Push(t, other, "main")
	a, err := git.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Fetch(context.Background(), repo, "origin"); err != nil {
		t.Fatal(err)
	}
	return updateRepo{a: a, repo: repo, other: other}
}

// conflictOnBase makes origin/main and the local branch both change
// README.md differently.
func (u updateRepo) conflictOnBase(t *testing.T) {
	t.Helper()
	gitfix.Commit(t, u.other, "README.md", "from main\n")
	gitfix.Push(t, u.other, "main")
	gitfix.Commit(t, u.repo, "README.md", "from feat\n")
	if err := u.a.Fetch(context.Background(), u.repo, "origin"); err != nil {
		t.Fatal(err)
	}
}

func TestGitAdapter_HeadCommitAndBehindCount(t *testing.T) {
	u := newUpdateRepo(t)
	ctx := context.Background()

	head, err := u.a.HeadCommit(ctx, u.repo)
	if err != nil || head != gitfix.Git(t, u.repo, "rev-parse", "HEAD") {
		t.Fatalf("HeadCommit = %q, %v", head, err)
	}
	n, err := u.a.BehindCount(ctx, u.repo, "origin/main")
	if err != nil || n != 1 {
		t.Fatalf("BehindCount = %d, %v; want 1", n, err)
	}
	if _, err := u.a.BehindCount(ctx, u.repo, "origin/nope"); domain.Code(err) != domain.CodeRefNotFound {
		t.Fatalf("BehindCount(missing ref) code = %q, want ref_not_found", domain.Code(err))
	}
}

func TestGitAdapter_IntegrateMergeAndRebase(t *testing.T) {
	for _, s := range []domain.UpdateStrategy{domain.UpdateMerge, domain.UpdateRebase} {
		t.Run(string(s), func(t *testing.T) {
			u := newUpdateRepo(t)
			ctx := context.Background()
			if err := u.a.Integrate(ctx, u.repo, ports.IntegrateSpec{Ref: "origin/main", Strategy: s}); err != nil {
				t.Fatalf("Integrate: %v", err)
			}
			if n, _ := u.a.BehindCount(ctx, u.repo, "origin/main"); n != 0 {
				t.Fatalf("still %d behind after %s", n, s)
			}
			if b, _, _ := u.a.CurrentBranch(ctx, u.repo); b != "feat" {
				t.Fatalf("branch = %q, want feat", b)
			}
			parents := gitfix.Git(t, u.repo, "rev-list", "--parents", "-n", "1", "HEAD")
			isMerge := len(strings.Fields(parents)) == 3
			if isMerge != (s == domain.UpdateMerge) {
				t.Fatalf("%s produced HEAD parents %q", s, parents)
			}
		})
	}
}

func TestGitAdapter_ConflictIsInProgressUntilAborted(t *testing.T) {
	for _, s := range []domain.UpdateStrategy{domain.UpdateMerge, domain.UpdateRebase} {
		t.Run(string(s), func(t *testing.T) {
			u := newUpdateRepo(t)
			u.conflictOnBase(t)
			ctx := context.Background()
			before, _ := u.a.HeadCommit(ctx, u.repo)

			if err := u.a.Integrate(ctx, u.repo, ports.IntegrateSpec{Ref: "origin/main", Strategy: s}); err == nil {
				t.Fatal("Integrate succeeded despite a conflict")
			}
			got, err := u.a.IntegrationInProgress(ctx, u.repo)
			if err != nil || got != s {
				t.Fatalf("IntegrationInProgress = %q, %v; want %q", got, err, s)
			}
			paths, err := u.a.ConflictedPaths(ctx, u.repo)
			if err != nil || len(paths) != 1 || paths[0] != "README.md" {
				t.Fatalf("ConflictedPaths = %v, %v", paths, err)
			}
			if err := u.a.AbortIntegration(ctx, u.repo, got); err != nil {
				t.Fatalf("AbortIntegration: %v", err)
			}
			if got, _ := u.a.IntegrationInProgress(ctx, u.repo); got != "" {
				t.Fatalf("still in progress after abort: %q", got)
			}
			if after, _ := u.a.HeadCommit(ctx, u.repo); after != before {
				t.Fatalf("HEAD = %s after abort, want %s", after, before)
			}
			if entries, _ := u.a.Status(ctx, u.repo); len(entries) != 0 {
				t.Fatalf("status after abort = %+v, want clean", entries)
			}
		})
	}
}

func TestGitAdapter_ConflictedPathsKeepsOddNames(t *testing.T) {
	u := newUpdateRepo(t)
	odd := "dir with space/quote\"d.txt"
	gitfix.Commit(t, u.other, odd, "main\n")
	gitfix.Push(t, u.other, "main")
	gitfix.Commit(t, u.repo, odd, "feat\n")
	ctx := context.Background()
	_ = u.a.Fetch(ctx, u.repo, "origin")
	_ = u.a.Integrate(ctx, u.repo, ports.IntegrateSpec{Ref: "origin/main", Strategy: domain.UpdateMerge})
	paths, err := u.a.ConflictedPaths(ctx, u.repo)
	if err != nil || len(paths) != 1 || paths[0] != odd {
		t.Fatalf("ConflictedPaths = %q, %v; want [%q]", paths, err, odd)
	}
	_ = u.a.AbortIntegration(ctx, u.repo, domain.UpdateMerge)
}

func TestGitAdapter_IntegrateWithAutostashKeepsLocalChanges(t *testing.T) {
	for _, s := range []domain.UpdateStrategy{domain.UpdateMerge, domain.UpdateRebase} {
		t.Run(string(s), func(t *testing.T) {
			u := newUpdateRepo(t)
			ctx := context.Background()
			gitfix.Dirty(t, u.repo, "feat.txt", "edited\n")
			if err := u.a.Integrate(ctx, u.repo, ports.IntegrateSpec{Ref: "origin/main", Strategy: s, Autostash: true}); err != nil {
				t.Fatalf("Integrate with autostash: %v", err)
			}
			entries, _ := u.a.Status(ctx, u.repo)
			if len(entries) != 1 || entries[0].RelPath != "feat.txt" {
				t.Fatalf("status = %+v, want the local edit restored", entries)
			}
			if ref, _ := u.a.StashRef(ctx, u.repo); ref != "" {
				t.Fatalf("stash = %q, want the autostash consumed", ref)
			}
		})
	}
}

func TestGitAdapter_StashRefResetHardAndStashPop(t *testing.T) {
	u := newUpdateRepo(t)
	ctx := context.Background()
	if ref, err := u.a.StashRef(ctx, u.repo); err != nil || ref != "" {
		t.Fatalf("StashRef on an empty stash = %q, %v", ref, err)
	}
	before, _ := u.a.HeadCommit(ctx, u.repo)
	gitfix.Dirty(t, u.repo, "feat.txt", "staged\n")
	gitfix.Git(t, u.repo, "add", "feat.txt")
	gitfix.Git(t, u.repo, "stash", "push", "-q")
	ref, err := u.a.StashRef(ctx, u.repo)
	if err != nil || ref == "" {
		t.Fatalf("StashRef = %q, %v; want a commit", ref, err)
	}
	gitfix.Commit(t, u.repo, "x.txt", "x\n")
	if err := u.a.ResetHard(ctx, u.repo, before); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}
	if head, _ := u.a.HeadCommit(ctx, u.repo); head != before {
		t.Fatalf("HEAD = %s, want %s", head, before)
	}
	if err := u.a.StashPop(ctx, u.repo); err != nil {
		t.Fatalf("StashPop: %v", err)
	}
	entries, _ := u.a.Status(ctx, u.repo)
	if len(entries) != 1 || entries[0].X != 'M' || entries[0].Y != ' ' {
		t.Fatalf("status = %+v, want the staged change restored to the index", entries)
	}
}
