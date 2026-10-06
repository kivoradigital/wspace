// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// worktreeRecorder scripts FakeGit so every WorktreeAdd materializes its
// target directory in FakeFS (the real `git worktree add` does) and every
// WorktreeRemove/DeleteBranch is captured for assertion.
type worktreeRecorder struct {
	removed         []domain.Path
	deletedBranches []domain.BranchName
}

func newWorktreeRecorder(fs *portstest.FakeFS, git *portstest.FakeGit) *worktreeRecorder {
	rec := &worktreeRecorder{}
	git.WorktreeAddFunc = func(_ domain.Path, spec ports.WorktreeSpec) error {
		return fs.MkdirAll(spec.Target)
	}
	git.WorktreeRemoveFunc = func(_, worktree domain.Path, _ bool) error {
		rec.removed = append(rec.removed, worktree)
		return nil
	}
	git.DeleteBranchFunc = func(_ domain.Path, b domain.BranchName, _ bool) error {
		rec.deletedBranches = append(rec.deletedBranches, b)
		return nil
	}
	return rec
}

func TestCreateWorkspace_MutateFailureRollsBackEverythingThisRunCreated(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	c := testContext(t, fs)
	rec := newWorktreeRecorder(fs, git)

	webSource := c.Projects[1].SourceDir
	boom := domain.NewOpError("git.fetch", domain.CodeGitFailed, string(webSource), "", nil)
	git.FetchFunc = func(repo domain.Path, _ string) error {
		if repo == webSource {
			return boom
		}
		return nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	_, err := app.CreateWorkspace(context.Background(), deps, app.CreateWorkspaceInput{Context: c, Branch: "feature-x", Name: "ws1"})
	if !errors.Is(err, boom) {
		t.Fatalf("CreateWorkspace() error = %v, want the original mutate failure", err)
	}

	wsRoot := c.WorkspacesRoot.Join("ws1")
	if len(rec.removed) != 1 || rec.removed[0] != wsRoot.Join("api") {
		t.Fatalf("removed worktrees = %v, want only the api worktree this run created", rec.removed)
	}
	if len(rec.deletedBranches) != 1 || rec.deletedBranches[0] != "feature-x" {
		t.Fatalf("deleted branches = %v, want the feature-x branch this run created", rec.deletedBranches)
	}
	if exists, _ := fs.Exists(wsRoot); exists {
		t.Fatalf("workspace root %s still exists after rollback", wsRoot)
	}
	if _, err := store.LoadManifest(context.Background(), wsRoot); domain.Code(err) != domain.CodeWorkspaceNotFound {
		t.Fatalf("LoadManifest() error = %v, want workspace_not_found (no manifest written)", err)
	}
}

func TestCreateWorkspace_RollbackKeepsABranchThatAlreadyExisted(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	c := testContext(t, fs)
	rec := newWorktreeRecorder(fs, git)

	apiSource := c.Projects[0].SourceDir
	git.BranchExistsFunc = func(repo domain.Path, _ domain.BranchName) (bool, error) {
		return repo == apiSource, nil
	}
	webSource := c.Projects[1].SourceDir
	git.WorktreeAddFunc = func(repo domain.Path, spec ports.WorktreeSpec) error {
		if repo == webSource {
			return errors.New("worktree add failed")
		}
		return fs.MkdirAll(spec.Target)
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: portstest.NewRecordingReporter()}
	if _, err := app.CreateWorkspace(context.Background(), deps, app.CreateWorkspaceInput{Context: c, Branch: "feature-x", Name: "ws1"}); err == nil {
		t.Fatal("CreateWorkspace() error = nil, want the worktree add failure")
	}

	if len(rec.removed) != 1 {
		t.Fatalf("removed worktrees = %v, want the api worktree removed", rec.removed)
	}
	if len(rec.deletedBranches) != 0 {
		t.Fatalf("deleted branches = %v, want none: api's branch pre-existed this run", rec.deletedBranches)
	}
}

func TestAddRepo_FailureAfterWorktreeAddRollsBackOnlyThatRepo(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	wsRoot, m := seededWorkspace(t, fs, store)
	rec := newWorktreeRecorder(fs, git)

	webSource := fs.Paths().Home.Join("src", "web")
	if err := fs.WriteFile(webSource.Join(".env"), []byte("A=1"), 0o644); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	git.IsIgnoredFunc = func(domain.Path, string) (bool, error) { return true, nil }
	// Fail the env copy step, which runs after the worktree already exists.
	fs.DenyWrite = func(domain.Path) bool { return true }
	_ = fs.MkdirAll(wsRoot.Join("api"))

	c := domain.Context{
		Name: "work",
		Projects: []domain.Project{
			{Key: "api", SourceDir: fs.Paths().Home.Join("src", "api")},
			{Key: "web", SourceDir: webSource},
		},
	}
	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: portstest.NewRecordingReporter()}
	if _, err := app.AddRepo(context.Background(), deps, app.AddRepoInput{WorkspaceRoot: wsRoot, Context: c, ProjectKey: "web"}); err == nil {
		t.Fatal("AddRepo() error = nil, want the env copy failure")
	}

	if len(rec.removed) != 1 || rec.removed[0] != wsRoot.Join("web") {
		t.Fatalf("removed worktrees = %v, want only the web worktree", rec.removed)
	}
	if exists, _ := fs.Exists(wsRoot); !exists {
		t.Fatal("workspace root was removed; AddRepo must leave the pre-existing workspace intact")
	}
	if exists, _ := fs.Exists(wsRoot.Join("api")); !exists {
		t.Fatal("pre-existing api worktree was removed by AddRepo's rollback")
	}
	if exists, _ := fs.Exists(wsRoot.Join("web")); exists {
		t.Fatal("web worktree directory still exists after rollback")
	}
	after, err := store.LoadManifest(context.Background(), wsRoot)
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}
	if len(after.Workspace.Repos) != len(m.Workspace.Repos) {
		t.Fatalf("manifest repos = %+v, want unchanged %+v", after.Workspace.Repos, m.Workspace.Repos)
	}
}

func TestCreateWorkspace_ReportsRepoProgressToAProgressAwareReporter(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	c := testContext(t, fs)
	_ = newWorktreeRecorder(fs, git)

	webSource := c.Projects[1].SourceDir
	git.WorktreeAddFunc = func(repo domain.Path, spec ports.WorktreeSpec) error {
		if repo == webSource {
			return errors.New("boom")
		}
		return fs.MkdirAll(spec.Target)
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	_, _ = app.CreateWorkspace(context.Background(), deps, app.CreateWorkspaceInput{Context: c, Branch: "feature-x", Name: "ws1"})

	want := []ports.RepoEvent{
		{Op: app.OpCreateWorkspace, Repo: "api", Phase: ports.RepoStarted},
		{Op: app.OpCreateWorkspace, Repo: "api", Phase: ports.RepoFinished},
		{Op: app.OpCreateWorkspace, Repo: "web", Phase: ports.RepoStarted},
		{Op: app.OpCreateWorkspace, Repo: "web", Phase: ports.RepoFailed},
		{Op: app.OpCreateWorkspace, Repo: "api", Phase: ports.RepoRolledBack},
	}
	if len(reporter.RepoEvents) != len(want) {
		t.Fatalf("RepoEvents = %+v, want %+v", reporter.RepoEvents, want)
	}
	for i, w := range want {
		got := reporter.RepoEvents[i]
		if got.Op != w.Op || got.Repo != w.Repo || got.Phase != w.Phase {
			t.Fatalf("RepoEvents[%d] = %+v, want %+v", i, got, w)
		}
		if w.Phase == ports.RepoFailed && got.Err == nil {
			t.Fatalf("RepoEvents[%d] is a failure with no Err", i)
		}
	}
}

// TestCreateWorkspace_PlainReporterStillWorks guards the optional nature of
// ports.RepoProgressReporter: a Reporter that does not implement it (the
// CLI's HumanReporter, the tray's reporter) must keep working unchanged.
func TestCreateWorkspace_PlainReporterStillWorks(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	c := testContext(t, fs)

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: plainReporter{}}
	if _, err := app.CreateWorkspace(context.Background(), deps, app.CreateWorkspaceInput{Context: c, Branch: "feature-x", Name: "ws1"}); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
}

// plainReporter is a ports.Reporter with no RepoProgress method.
type plainReporter struct{}

func (plainReporter) Step(messages.Key, ...any) {}
func (plainReporter) Info(messages.Key, ...any) {}
func (plainReporter) Warn(messages.Key, ...any) {}
func (plainReporter) Result(any)                {}
