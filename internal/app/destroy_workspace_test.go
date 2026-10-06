// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func seededWorkspace(t *testing.T, fs *portstest.FakeFS, store *portstest.FakeConfigStore) (domain.Path, domain.Manifest) {
	t.Helper()
	wsRoot := fs.Paths().Home.Join("workspaces", "ws1")
	m := domain.Manifest{
		SchemaVersion: 1,
		Workspace: domain.Workspace{
			Name:   "ws1",
			Root:   wsRoot,
			Branch: "feature-x",
			Repos: []domain.RepoEntry{
				{Alias: "api", Project: "api", SourceDir: fs.Paths().Home.Join("src", "api"), Branch: "feature-x"},
			},
		},
	}
	store.PutManifest(wsRoot, m)
	if err := fs.MkdirAll(wsRoot); err != nil {
		t.Fatalf("seed workspace dir: %v", err)
	}
	return wsRoot, m
}

func TestDestroy_AbortsWhenCwdInsideWorkspace(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	fs.Chdir(wsRoot.Join("api"))

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	// Force cannot help here (the shell holds the directory), so the error
	// must say what to do instead of pointing to --force.
	err := app.DestroyWorkspace(context.Background(), deps, app.DestroyWorkspaceInput{WorkspaceRoot: wsRoot, Force: true})
	if err == nil {
		t.Fatal("DestroyWorkspace() error = nil, want CodeCwdInsideWorkspace")
	}
	if got := domain.Code(err); got != domain.CodeCwdInsideWorkspace {
		t.Fatalf("domain.Code(err) = %q, want %q", got, domain.CodeCwdInsideWorkspace)
	}
	if len(git.Calls) != 0 {
		t.Fatalf("expected no git calls once cwd-safety aborts, got %+v", git.Calls)
	}
}

func TestDestroy_BlocksOnUnpushedBranchWithoutUpstream(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	git.AheadBehindFunc = func(domain.Path, string) (int, int, error) {
		return 0, 0, domain.NewOpError("git.ahead_behind", domain.CodeRefNotFound, "", "", nil)
	}
	git.UnpushedCountFunc = func(domain.Path, string) (int, error) { return 2, nil }

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	err := app.DestroyWorkspace(context.Background(), deps, app.DestroyWorkspaceInput{WorkspaceRoot: wsRoot})
	if err == nil {
		t.Fatal("DestroyWorkspace() error = nil, want CodeUnsafeTeardown")
	}
	if got := domain.Code(err); got != domain.CodeUnsafeTeardown {
		t.Fatalf("domain.Code(err) = %q, want %q", got, domain.CodeUnsafeTeardown)
	}
	for _, call := range git.Calls {
		if call.Method == "WorktreeRemove" {
			t.Fatalf("expected no WorktreeRemove without --force when unpushed, got calls=%+v", git.Calls)
		}
	}

	if _, err := store.LoadManifest(context.Background(), wsRoot); err != nil {
		t.Fatalf("manifest should not have been archived away: %v", err)
	}
}

func TestDestroy_BlocksOnForeignFileWithoutForce(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: "new-file.txt"}}, nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	err := app.DestroyWorkspace(context.Background(), deps, app.DestroyWorkspaceInput{WorkspaceRoot: wsRoot})
	if err == nil {
		t.Fatal("DestroyWorkspace() error = nil, want CodeUnsafeTeardown")
	}
	if got := domain.Code(err); got != domain.CodeUnsafeTeardown {
		t.Fatalf("domain.Code(err) = %q, want %q", got, domain.CodeUnsafeTeardown)
	}
}

func TestDestroy_EnvCopyChangesNeverBlock(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, m := seededWorkspace(t, fs, store)
	m.EnvCopies = []string{"api/.env"}
	store.PutManifest(wsRoot, m)

	git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: ".env"}}, nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	err := app.DestroyWorkspace(context.Background(), deps, app.DestroyWorkspaceInput{WorkspaceRoot: wsRoot})
	if err != nil {
		t.Fatalf("DestroyWorkspace() unexpected error: %v", err)
	}
}

// TestDestroy_EnvCopyChangesNeverBlock_EvenWithoutBusinessForce guards the
// git-level half of the guarantee TestDestroy_EnvCopyChangesNeverBlock
// checks at the classification level. Real git's own `worktree remove`
// refuses on ANY untracked file — including one this tool copied in
// itself — unless told --force at the git level; it has no concept of
// "this untracked file is just an env copy". A fake whose WorktreeRemove
// always returns nil regardless of the force argument it received (as
// every other fake in this file does) cannot see that gap. This test
// simulates real git's actual refusal instead, reproducing exactly what
// internal/e2e's TestFailure_DestroyIgnoresItsOwnEnvCopies found against
// the real binary and a real git repository: DestroyWorkspace passed the
// caller's own --force flag straight through to WorktreeRemove, so an
// env-copy-only worktree that the classification above already ruled
// "safe without --force" still failed to actually come down.
func TestDestroy_EnvCopyChangesNeverBlock_EvenWithoutBusinessForce(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, m := seededWorkspace(t, fs, store)
	m.EnvCopies = []string{"api/.env"}
	store.PutManifest(wsRoot, m)

	git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: ".env"}}, nil
	}
	git.WorktreeRemoveFunc = func(_, _ domain.Path, force bool) error {
		if !force {
			return domain.NewOpError("git.worktree_remove", domain.CodeWorktreeDirty, "", "", nil)
		}
		return nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	in := app.DestroyWorkspaceInput{WorkspaceRoot: wsRoot} // no --force from the caller
	if err := app.DestroyWorkspace(context.Background(), deps, in); err != nil {
		t.Fatalf("DestroyWorkspace() unexpected error: %v (an env-copy-only worktree must never need the caller's own --force)", err)
	}
}

func TestDestroy_ForceOverridesBlockingButArchivesFirst(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: 'M', Y: ' ', RelPath: "dirty.txt"}}, nil
	}

	var removed, pruned bool

	// FakeConfigStore.ArchiveManifest deletes the in-memory manifest
	// (portstest.FakeConfigStore.ArchiveManifest), so "the manifest is
	// already gone from the store" is exactly "ArchiveManifest already ran"
	// — checking that from inside WorktreeRemoveFunc proves the ordering
	// design.md §8.2 requires (ArchiveManifest before any WorktreeRemove).
	git.WorktreeRemoveFunc = func(repo, worktree domain.Path, force bool) error {
		if _, err := store.LoadManifest(context.Background(), wsRoot); err == nil {
			t.Fatal("WorktreeRemove called before ArchiveManifest")
		}
		if !force {
			t.Fatal("WorktreeRemove called without force under --force")
		}
		removed = true
		return nil
	}
	git.WorktreePruneFunc = func(domain.Path) error {
		if !removed {
			t.Fatal("WorktreePrune called before WorktreeRemove")
		}
		pruned = true
		return nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	in := app.DestroyWorkspaceInput{WorkspaceRoot: wsRoot, Force: true, DeleteBranches: true}

	if err := app.DestroyWorkspace(context.Background(), deps, in); err != nil {
		t.Fatalf("DestroyWorkspace() unexpected error: %v", err)
	}
	if !removed || !pruned {
		t.Fatalf("expected WorktreeRemove and WorktreePrune to run, removed=%v pruned=%v", removed, pruned)
	}
	if _, err := store.LoadManifest(context.Background(), wsRoot); err == nil {
		t.Fatal("expected manifest to be archived (removed from the live store) after destroy")
	}

	var deletedBranch bool
	for _, call := range git.Calls {
		if call.Method == "DeleteBranch" {
			deletedBranch = true
		}
	}
	if !deletedBranch {
		t.Fatalf("expected DeleteBranch to run when DeleteBranches=true, calls=%+v", git.Calls)
	}
}
