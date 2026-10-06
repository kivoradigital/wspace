// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func testContext(t *testing.T, fs *portstest.FakeFS) domain.Context {
	t.Helper()
	return domain.Context{
		Name:           "work",
		WorkspacesRoot: fs.Paths().Home.Join("workspaces"),
		Projects: []domain.Project{
			{Key: "api", SourceDir: fs.Paths().Home.Join("src", "api")},
			{Key: "web", SourceDir: fs.Paths().Home.Join("src", "web")},
		},
	}
}

func TestCreateWorkspace_PreflightBeforeAnyMutation(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	ctx := testContext(t, fs)

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	in := app.CreateWorkspaceInput{Context: ctx, Branch: "feature-x", Name: "ws1"}

	if _, err := app.CreateWorkspace(context.Background(), deps, in); err != nil {
		t.Fatalf("CreateWorkspace() unexpected error: %v", err)
	}

	preflight := map[string]bool{"IsMainClone": true, "ResolveBase": true, "WorktreeList": true}
	mutate := map[string]bool{"Fetch": true, "SyncLocalBase": true, "WorktreeAdd": true}

	lastPreflight, firstMutate := -1, -1
	for i, call := range git.Calls {
		if preflight[call.Method] {
			lastPreflight = i
		}
		if mutate[call.Method] && firstMutate == -1 {
			firstMutate = i
		}
	}
	if lastPreflight == -1 || firstMutate == -1 {
		t.Fatalf("expected both preflight and mutate calls to occur, got %+v", git.Calls)
	}
	if lastPreflight > firstMutate {
		t.Fatalf("expected every preflight call to precede the first mutation; last preflight index %d, first mutate index %d, calls=%+v", lastPreflight, firstMutate, git.Calls)
	}
}

// A local-only repository has no remote to fetch: creation must warn and
// carry on from the local base instead of failing the whole workspace.
func TestCreateWorkspace_RepoWithoutTheRemoteSkipsFetchWithAWarning(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	git.FetchFunc = func(domain.Path, string) error {
		return domain.NewOpError("git.fetch", domain.CodeRemoteMissing, "origin", "", nil)
	}
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}

	_, err := app.CreateWorkspace(context.Background(), deps, app.CreateWorkspaceInput{Context: testContext(t, fs), Branch: "feature-x", Name: "ws1"})
	if err != nil {
		t.Fatalf("CreateWorkspace() = %v, want success without a remote", err)
	}
	skipped := 0
	for _, w := range reporter.Warnings {
		if w.Key == messages.FetchSkippedNoRemote {
			skipped++
		}
	}
	if skipped != 2 {
		t.Fatalf("Warnings = %+v, want one FetchSkippedNoRemote per project", reporter.Warnings)
	}
}

func TestCreateWorkspace_MutatesAndCopiesEnv(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()

	ctx := domain.Context{
		Name:           "work",
		WorkspacesRoot: fs.Paths().Home.Join("workspaces"),
		Projects: []domain.Project{
			{Key: "api", SourceDir: fs.Paths().Home.Join("src", "api")},
		},
	}

	envSrc := ctx.Projects[0].SourceDir.Join(".env")
	if err := fs.WriteFile(envSrc, []byte("A=1"), 0o644); err != nil {
		t.Fatalf("seed env file: %v", err)
	}

	git.IsIgnoredFunc = func(domain.Path, string) (bool, error) { return false, nil }

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	in := app.CreateWorkspaceInput{Context: ctx, Branch: "feature-x", Name: "ws1"}

	if _, err := app.CreateWorkspace(context.Background(), deps, in); err != nil {
		t.Fatalf("CreateWorkspace() unexpected error: %v", err)
	}

	wantOrder := []string{"IsMainClone", "ResolveBase", "WorktreeList", "Fetch", "SyncLocalBase", "BranchExists", "WorktreeAdd", "IsIgnored"}
	if len(git.Calls) != len(wantOrder) {
		t.Fatalf("git.Calls = %+v, want %d calls in order %v", git.Calls, len(wantOrder), wantOrder)
	}
	for i, want := range wantOrder {
		if git.Calls[i].Method != want {
			t.Fatalf("git.Calls[%d] = %q, want %q (full: %+v)", i, git.Calls[i].Method, want, git.Calls)
		}
	}

	if len(reporter.Warnings) != 1 || reporter.Warnings[0].Key != messages.EnvCopyNotIgnored {
		t.Fatalf("reporter.Warnings = %+v, want exactly one EnvCopyNotIgnored warning", reporter.Warnings)
	}

	m, err := store.LoadManifest(context.Background(), ctx.WorkspacesRoot.Join("ws1"))
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}
	if len(m.Workspace.Repos) != 1 || m.Workspace.Repos[0].Alias != "api" {
		t.Fatalf("manifest repos = %+v, want one repo aliased api", m.Workspace.Repos)
	}
	if len(m.EnvCopies) != 1 || m.EnvCopies[0] != "api/.env" {
		t.Fatalf("manifest env copies = %+v, want [api/.env]", m.EnvCopies)
	}
}

func TestCreateWorkspace_RejectsBranchCheckedOutElsewhere(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	ctx := testContext(t, fs)

	git.WorktreeListFunc = func(repo domain.Path) ([]ports.WorktreeRef, error) {
		return []ports.WorktreeRef{{Path: repo.Join("..", "elsewhere"), Branch: "feature-x"}}, nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	in := app.CreateWorkspaceInput{Context: ctx, Branch: "feature-x", Name: "ws1"}

	_, err := app.CreateWorkspace(context.Background(), deps, in)
	if err == nil {
		t.Fatal("CreateWorkspace() error = nil, want CodeBranchCheckedOut")
	}
	if got := domain.Code(err); got != domain.CodeBranchCheckedOut {
		t.Fatalf("domain.Code(err) = %q, want %q", got, domain.CodeBranchCheckedOut)
	}

	for _, call := range git.Calls {
		if call.Method == "WorktreeAdd" {
			t.Fatalf("expected no WorktreeAdd call after a preflight rejection, got calls=%+v", git.Calls)
		}
	}
}

// TestCreateWorkspace_ReportsAStepPerProject covers the GUI wizard's own
// progress-feedback defect: creating a workspace fetches and adds a git
// worktree per repository, seconds of work the product owner reported as
// "no loading, no progress" — the tray window simply closed with nothing
// to show while CreateWorkspace ran. deps.Reporter.Step must announce
// each project as its mutation begins, exactly like app.Exec already
// announces each repo through the identical CLIExecRepoHeader-shaped
// call, so a caller driving a progress view (internal/gui/wizard's
// project window) has something substantive to render as the operation
// actually progresses, not only a final Result.
func TestCreateWorkspace_ReportsAStepPerProject(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	ctx := testContext(t, fs)

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	in := app.CreateWorkspaceInput{Context: ctx, Branch: "feature-x", Name: "ws1"}

	if _, err := app.CreateWorkspace(context.Background(), deps, in); err != nil {
		t.Fatalf("CreateWorkspace() unexpected error: %v", err)
	}

	if len(reporter.Steps) != len(ctx.Projects) {
		t.Fatalf("reporter.Steps = %+v, want exactly one Step per project (%d)", reporter.Steps, len(ctx.Projects))
	}
	for i, p := range ctx.Projects {
		step := reporter.Steps[i]
		if step.Key != messages.WorkspaceCreateProject {
			t.Fatalf("Steps[%d].Key = %v, want %v", i, step.Key, messages.WorkspaceCreateProject)
		}
		if len(step.Args) != 1 || step.Args[0] != string(p.Key) {
			t.Fatalf("Steps[%d].Args = %+v, want [%q]", i, step.Args, p.Key)
		}
	}
}

// A context branch_prefix configured without a trailing slash ("feature")
// must still produce "feature/<name>", for the workspace and every repo.
func TestCreateWorkspace_PrefixWithoutSlashGetsOneSeparator(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	ctx := testContext(t, fs)
	prefix := "feature"
	ctx.Defaults.BranchPrefix = &prefix

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: portstest.NewRecordingReporter()}
	res, err := app.CreateWorkspace(context.Background(), deps, app.CreateWorkspaceInput{Context: ctx, Name: "test3"})
	if err != nil {
		t.Fatalf("CreateWorkspace() unexpected error: %v", err)
	}
	if res.Workspace.Branch != "feature/test3" {
		t.Fatalf("workspace branch = %q, want feature/test3", res.Workspace.Branch)
	}
	for _, r := range res.Workspace.Repos {
		if r.Branch != "feature/test3" {
			t.Fatalf("repo %s branch = %q, want feature/test3", r.Alias, r.Branch)
		}
	}
}

// A per-workspace prefix override (the app's branch type selector sends
// options.branchPrefix) wins over the context's prefix.
func TestCreateWorkspace_FlagPrefixOverridesContextPrefix(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	ctx := testContext(t, fs)
	ctxPrefix, flagPrefix := "feature", "hotfix"
	ctx.Defaults.BranchPrefix = &ctxPrefix

	deps := app.Deps{Store: portstest.NewFakeConfigStore(), Git: git, FS: fs, Reporter: portstest.NewRecordingReporter()}
	res, err := app.CreateWorkspace(context.Background(), deps, app.CreateWorkspaceInput{
		Context: ctx, Name: "login", Flags: domain.Options{BranchPrefix: &flagPrefix},
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() unexpected error: %v", err)
	}
	if res.Workspace.Branch != "hotfix/login" {
		t.Fatalf("workspace branch = %q, want hotfix/login", res.Workspace.Branch)
	}
}
