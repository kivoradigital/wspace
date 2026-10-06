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

func TestAddRepo_ValidatesAgainstExistingWorkspace(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, m := seededWorkspace(t, fs, store)

	ctx := domain.Context{
		Name: "work",
		Projects: []domain.Project{
			{Key: "api", SourceDir: fs.Paths().Home.Join("src", "api")},
			{Key: "web", SourceDir: fs.Paths().Home.Join("src", "web")},
		},
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}

	// api is already mounted (seededWorkspace put it there): adding it
	// again must be rejected.
	_, err := app.AddRepo(context.Background(), deps, app.AddRepoInput{
		WorkspaceRoot: wsRoot, Context: ctx, ProjectKey: "api",
	})
	if err == nil {
		t.Fatal("AddRepo() error = nil for an already-mounted project, want an error")
	}

	// web is not mounted yet: adding it must succeed and extend the
	// manifest.
	entry, err := app.AddRepo(context.Background(), deps, app.AddRepoInput{
		WorkspaceRoot: wsRoot, Context: ctx, ProjectKey: "web",
	})
	if err != nil {
		t.Fatalf("AddRepo(web) unexpected error: %v", err)
	}
	if entry.Alias != "web" {
		t.Fatalf("entry.Alias = %q, want %q", entry.Alias, "web")
	}

	updated, err := store.LoadManifest(context.Background(), wsRoot)
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}
	if len(updated.Workspace.Repos) != len(m.Workspace.Repos)+1 {
		t.Fatalf("manifest repos = %+v, want %d entries", updated.Workspace.Repos, len(m.Workspace.Repos)+1)
	}
}

func TestRemoveRepo_RequiresSafeToRemove(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: 'M', Y: ' ', RelPath: "dirty.txt"}}, nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}

	err := app.RemoveRepo(context.Background(), deps, app.RemoveRepoInput{WorkspaceRoot: wsRoot, Alias: "api"})
	if err == nil {
		t.Fatal("RemoveRepo() error = nil, want CodeUnsafeTeardown")
	}
	if got := domain.Code(err); got != domain.CodeUnsafeTeardown {
		t.Fatalf("domain.Code(err) = %q, want %q", got, domain.CodeUnsafeTeardown)
	}

	git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) { return nil, nil }
	if err := app.RemoveRepo(context.Background(), deps, app.RemoveRepoInput{WorkspaceRoot: wsRoot, Alias: "api", Force: true}); err != nil {
		t.Fatalf("RemoveRepo(force) unexpected error: %v", err)
	}

	m, err := store.LoadManifest(context.Background(), wsRoot)
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}
	for _, r := range m.Workspace.Repos {
		if r.Alias == "api" {
			t.Fatalf("expected api to be removed from the manifest, got %+v", m.Workspace.Repos)
		}
	}
}

// TestRemoveRepo_EnvCopyChangesNeverBlock_EvenWithoutBusinessForce is
// RemoveRepo's counterpart to
// TestDestroy_EnvCopyChangesNeverBlock_EvenWithoutBusinessForce: real
// git's own `worktree remove` refuses on any untracked file (including
// one this tool copied in itself) unless told --force at the git level,
// with no concept of "this is just an env copy" — RemoveRepo shares
// DestroyWorkspace's exact same classification-then-remove shape and had
// the exact same gap.
func TestRemoveRepo_EnvCopyChangesNeverBlock_EvenWithoutBusinessForce(t *testing.T) {
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
	in := app.RemoveRepoInput{WorkspaceRoot: wsRoot, Alias: "api"} // no --force from the caller
	if err := app.RemoveRepo(context.Background(), deps, in); err != nil {
		t.Fatalf("RemoveRepo() unexpected error: %v (an env-copy-only worktree must never need the caller's own --force)", err)
	}
}
