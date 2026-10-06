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

func TestStatus_And_List_AggregateRepoStatus(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feature-x", false, nil }
	git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 2, 1, nil }
	git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: 'M', Y: ' ', RelPath: "dirty.txt"}}, nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}

	status, err := app.Status(context.Background(), deps, app.StatusInput{WorkspaceRoot: wsRoot})
	if err != nil {
		t.Fatalf("Status() unexpected error: %v", err)
	}
	if len(status.Repos) != 1 {
		t.Fatalf("status.Repos = %+v, want exactly one repo", status.Repos)
	}
	rs := status.Repos[0]
	if rs.Alias != "api" || rs.Branch != "feature-x" || rs.Ahead != 2 || rs.Behind != 1 || !rs.Dirty {
		t.Fatalf("status.Repos[0] = %+v, want alias=api branch=feature-x ahead=2 behind=1 dirty=true", rs)
	}

	list, err := app.List(context.Background(), deps, app.ListInput{WorkspacesRoot: wsRoot.Join("..")})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(list) != 1 || list[0].Name != "ws1" {
		t.Fatalf("List() = %+v, want exactly one workspace named ws1", list)
	}
	if len(list[0].Repos) != 1 || list[0].Repos[0].Alias != "api" {
		t.Fatalf("List()[0].Repos = %+v, want one repo aliased api", list[0].Repos)
	}
}

// TestList_FreshContextWithNoWorkspacesRootYetSucceedsEmpty is CRITICAL-3's
// own regression test (verify-report.md): a brand-new context's
// workspaces_root does not exist on disk until CreateWorkspace's mutate
// phase creates it — the normal state of every context before its first
// `ws create`. List must report zero workspaces, not fail. This uses a
// FakeFS on which nothing was ever created (not even WorkspacesRoot
// itself), which the old FakeFS.ListDirs could not distinguish from
// "exists and is empty" — every previous List/Doctor test happened to seed
// at least one workspace first, which is exactly what let this ship.
func TestList_FreshContextWithNoWorkspacesRootYetSucceedsEmpty(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}

	workspacesRoot := fs.Paths().Home.Join("workspaces")
	list, err := app.List(context.Background(), deps, app.ListInput{WorkspacesRoot: workspacesRoot})
	if err != nil {
		t.Fatalf("List() on a never-created workspaces_root = %v, want nil error", err)
	}
	if len(list) != 0 {
		t.Fatalf("List() = %+v, want empty", list)
	}
}

// TestList_SharedWorkspacesRootOnlyReturnsTheContextsOwnWorkspaces: two
// contexts may point at the same workspaces_root. Each must list only the
// workspaces whose manifest names it; a manifest with no context (written
// before contexts were recorded) still belongs to every context sharing the
// root, so nothing an older version created ever disappears.
func TestList_SharedWorkspacesRootOnlyReturnsTheContextsOwnWorkspaces(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()

	root := fs.Paths().Home.Join("workspaces")
	seed := func(name string, owner domain.ContextName) {
		wsRoot := root.Join(name)
		store.PutManifest(wsRoot, domain.Manifest{
			SchemaVersion: 1,
			Workspace:     domain.Workspace{Name: name, Root: wsRoot, Context: owner},
		})
		if err := fs.MkdirAll(wsRoot); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	// acme is a registered context: a workspace it owns stays hidden (an
	// owner that no longer exists makes an orphan instead — see
	// workspace_ownership_test.go).
	store.PutContext(domain.Context{Name: "acme", WorkspacesRoot: root})
	seed("mine", "work")
	seed("theirs", "acme")
	seed("legacy", "")

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	list, err := app.List(context.Background(), deps, app.ListInput{WorkspacesRoot: root, Context: "work"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	got := map[string]bool{}
	for _, ws := range list {
		got[ws.Name] = true
	}
	if len(list) != 2 || !got["mine"] || !got["legacy"] {
		t.Fatalf("List(context=work) = %+v, want exactly mine and legacy", list)
	}
}

// TestList_OneBrokenWorkspaceStillReturnsHealthyOnes covers phase 6's
// disclosed defect: app.List used to abort entirely on the first
// per-workspace collection error, blanking a context's whole tray listing
// over one damaged workspace. List must instead report the damaged
// workspace inline (WorkspaceStatus.Err set, Repos empty) while every
// healthy workspace still renders normally.
func TestList_OneBrokenWorkspaceStillReturnsHealthyOnes(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()

	root := fs.Paths().Home.Join("workspaces")
	healthyRoot := root.Join("healthy")
	brokenRoot := root.Join("broken")

	seedManifest := func(wsRoot domain.Path, name string) {
		store.PutManifest(wsRoot, domain.Manifest{
			SchemaVersion: 1,
			Workspace: domain.Workspace{
				Name: name,
				Root: wsRoot,
				Repos: []domain.RepoEntry{
					{Alias: "api", Project: "api", SourceDir: fs.Paths().Home.Join("src", "api")},
				},
			},
		})
		if err := fs.MkdirAll(wsRoot); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	seedManifest(healthyRoot, "healthy")
	seedManifest(brokenRoot, "broken")

	git.CurrentBranchFunc = func(worktree domain.Path) (domain.BranchName, bool, error) {
		if worktree == brokenRoot.Join("api") {
			return "", false, domain.NewOpError("git.current_branch", domain.CodeWorktreeMissing, "", "", nil)
		}
		return "feature-x", false, nil
	}
	git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) { return nil, nil }
	git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 0, 0, nil }

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	list, err := app.List(context.Background(), deps, app.ListInput{WorkspacesRoot: root})
	if err != nil {
		t.Fatalf("List() unexpected top-level error: %v (a damaged workspace must not abort the whole call)", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() = %+v, want both the healthy and the broken workspace present", list)
	}

	var healthy, broken *app.WorkspaceStatus
	for i := range list {
		switch list[i].Name {
		case "healthy":
			healthy = &list[i]
		case "broken":
			broken = &list[i]
		}
	}
	if healthy == nil || healthy.Err != nil {
		t.Fatalf("healthy workspace = %+v, want no error", healthy)
	}
	if len(healthy.Repos) != 1 {
		t.Fatalf("healthy workspace repos = %+v, want one collected repo", healthy.Repos)
	}
	if broken == nil || broken.Err == nil {
		t.Fatalf("broken workspace = %+v, want a non-nil Err marking it damaged", broken)
	}
	if domain.Code(broken.Err) != domain.CodeWorktreeMissing {
		t.Fatalf("broken.Err code = %v, want CodeWorktreeMissing", domain.Code(broken.Err))
	}
}

// TestList_ReportsLegacyOnlyWorkspacesFlaggedNotHidden covers a directory
// the legacy bash tool created (".ws/workspace.conf") that has no wspace
// manifest yet: it is listed with Legacy set, in every context sharing the
// root (a legacy manifest names no context), and never collected.
func TestList_ReportsLegacyOnlyWorkspacesFlaggedNotHidden(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()

	root := fs.Paths().Home.Join("workspaces")
	legacyRoot := root.Join("old")
	if err := fs.MkdirAll(legacyRoot.Join(".ws")); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile(legacyRoot.Join(".ws", "workspace.conf"), []byte("[repos]\napi|Api|b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fs.MkdirAll(root.Join("plain")); err != nil {
		t.Fatal(err)
	}
	theirs := root.Join("theirs")
	store.PutContext(domain.Context{Name: "other", WorkspacesRoot: root})
	store.PutManifest(theirs, domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{Name: "theirs", Root: theirs, Context: "other"}})
	_ = fs.MkdirAll(theirs)

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: portstest.NewRecordingReporter()}
	list, err := app.List(context.Background(), deps, app.ListInput{WorkspacesRoot: root, Context: "work"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List = %+v, want only the legacy workspace", list)
	}
	got := list[0]
	if !got.Legacy || got.Name != "old" || got.Root != legacyRoot || got.Err != nil || len(got.Repos) != 0 {
		t.Fatalf("legacy entry = %+v", got)
	}
	if len(git.Calls) != 0 {
		t.Errorf("git calls = %+v, a legacy-only workspace must not be collected", git.Calls)
	}
}
