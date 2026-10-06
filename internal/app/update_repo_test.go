// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// updateFixture is baseFixture ("api" and "hub" on feat, base develop)
// with every repo resolving origin/develop, three commits behind it, a
// clean worktree, and an Integrate that moves HEAD forward.
type updateFixture struct {
	*baseFixture
	reporter   *portstest.RecordingReporter
	head       map[domain.Path]string
	behind     map[domain.Path]int
	entries    map[domain.Path][]domain.PorcelainEntry
	integrated []ports.IntegrateSpec
}

func newUpdateFixture(t *testing.T) *updateFixture {
	t.Helper()
	f := &updateFixture{
		baseFixture: newBaseFixture(t),
		reporter:    portstest.NewRecordingReporter(),
		head:        map[domain.Path]string{},
		behind:      map[domain.Path]int{},
		entries:     map[domain.Path][]domain.PorcelainEntry{},
	}
	for _, alias := range []string{"api", "hub"} {
		f.has(alias, "develop", "origin/develop")
		f.head[f.worktree(alias)] = "before-" + alias
		f.behind[f.worktree(alias)] = 3
	}
	f.git.HeadCommitFunc = func(wt domain.Path) (string, error) { return f.head[wt], nil }
	f.git.BehindCountFunc = func(wt domain.Path, _ string) (int, error) { return f.behind[wt], nil }
	f.git.StatusFunc = func(wt domain.Path) ([]domain.PorcelainEntry, error) { return f.entries[wt], nil }
	f.git.IntegrateFunc = func(wt domain.Path, spec ports.IntegrateSpec) error {
		f.integrated = append(f.integrated, spec)
		f.head[wt] = "after-" + string(spec.Strategy)
		f.behind[wt] = 0
		return nil
	}
	return f
}

func (f *updateFixture) deps() app.Deps {
	return app.Deps{Store: f.store, Git: f.git, FS: f.fs, Reporter: f.reporter}
}

func (f *updateFixture) update(t *testing.T, alias string, s domain.UpdateStrategy, autostash bool) app.RepoUpdate {
	t.Helper()
	res, err := app.UpdateRepo(context.Background(), f.deps(), app.UpdateRepoInput{WorkspaceRoot: f.wsRoot, Alias: alias, Strategy: s, Autostash: autostash})
	if err != nil {
		t.Fatalf("UpdateRepo() = %v, want a per-repo result", err)
	}
	return res
}

func (f *updateFixture) called(method string) bool {
	for _, c := range f.git.Calls {
		if c.Method == method {
			return true
		}
	}
	return false
}

func (f *updateFixture) indexOf(method string) int {
	for i, c := range f.git.Calls {
		if c.Method == method {
			return i
		}
	}
	return -1
}

func wantCode(t *testing.T, err error, code domain.ErrCode) {
	t.Helper()
	if domain.Code(err) != code {
		t.Fatalf("Err = %v, want code %s", err, code)
	}
}

func TestUpdateRepo_MergesTheFetchedBase(t *testing.T) {
	f := newUpdateFixture(t)

	res := f.update(t, "api", domain.UpdateMerge, false)

	if res.Err != nil {
		t.Fatalf("Err = %v", res.Err)
	}
	if res.Alias != "api" || res.Strategy != domain.UpdateMerge || res.Base != "origin/develop" ||
		res.BeforeHead != "before-api" || res.AfterHead != "after-merge" || res.UpToDate || res.CommitsIntegrated != 3 {
		t.Fatalf("result = %+v", res)
	}
	if len(f.integrated) != 1 || f.integrated[0] != (ports.IntegrateSpec{Ref: "origin/develop", Strategy: domain.UpdateMerge}) {
		t.Fatalf("Integrate specs = %+v", f.integrated)
	}
	if fetch, resolve := f.indexOf("Fetch"), f.indexOf("ResolveBase"); fetch < 0 || fetch > resolve {
		t.Fatalf("calls = %+v, want Fetch before the base is resolved", f.git.Calls)
	}
	ev := f.reporter.RepoEvents
	if len(ev) != 2 || ev[0].Op != app.OpUpdate || ev[0].Phase != ports.RepoStarted || ev[1].Phase != ports.RepoFinished {
		t.Fatalf("events = %+v", ev)
	}
}

func TestUpdateRepo_Rebases(t *testing.T) {
	f := newUpdateFixture(t)

	res := f.update(t, "api", domain.UpdateRebase, false)

	if res.Err != nil || res.AfterHead != "after-rebase" || f.integrated[0].Strategy != domain.UpdateRebase {
		t.Fatalf("result = %+v, specs = %+v", res, f.integrated)
	}
}

func TestUpdateRepo_UpToDateTouchesNothingEvenWhenDirty(t *testing.T) {
	f := newUpdateFixture(t)
	f.behind[f.worktree("api")] = 0
	f.entries[f.worktree("api")] = []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "a.go"}}

	res := f.update(t, "api", domain.UpdateMerge, false)

	if res.Err != nil || !res.UpToDate || res.CommitsIntegrated != 0 || res.AfterHead != res.BeforeHead {
		t.Fatalf("result = %+v, want up to date", res)
	}
	if f.called("Integrate") {
		t.Fatal("integrated although already up to date")
	}
}

func TestUpdateRepo_RefusesADirtyWorktreeListingTheFiles(t *testing.T) {
	f := newUpdateFixture(t)
	f.entries[f.worktree("api")] = []domain.PorcelainEntry{
		{X: ' ', Y: 'M', RelPath: "a.go"},
		{X: '?', Y: '?', RelPath: "scratch.txt"},
		{X: 'A', Y: ' ', RelPath: "b.go"},
	}

	res := f.update(t, "api", domain.UpdateMerge, false)

	wantCode(t, res.Err, domain.CodeWorktreeDirty)
	if strings.Join(res.DirtyFiles, ",") != "a.go,b.go" {
		t.Fatalf("DirtyFiles = %v, want the tracked changes only", res.DirtyFiles)
	}
	if f.called("Integrate") || res.AfterHead != res.BeforeHead {
		t.Fatalf("a dirty worktree was touched: %+v", f.git.Calls)
	}
	if ev := f.reporter.RepoEvents; ev[len(ev)-1].Phase != ports.RepoFailed {
		t.Fatalf("events = %+v, want a failed event", ev)
	}
}

func TestUpdateRepo_UntrackedFilesAreNotDirty(t *testing.T) {
	f := newUpdateFixture(t)
	f.entries[f.worktree("api")] = []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: "scratch.txt"}}

	res := f.update(t, "api", domain.UpdateMerge, true)

	if res.Err != nil || f.integrated[0].Autostash {
		t.Fatalf("result = %+v, specs = %+v: untracked files need no stash", res, f.integrated)
	}
}

func TestUpdateRepo_AutostashesADirtyWorktree(t *testing.T) {
	for _, s := range []domain.UpdateStrategy{domain.UpdateMerge, domain.UpdateRebase} {
		t.Run(string(s), func(t *testing.T) {
			f := newUpdateFixture(t)
			f.entries[f.worktree("api")] = []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "a.go"}}

			res := f.update(t, "api", s, true)

			if res.Err != nil || len(f.integrated) != 1 || !f.integrated[0].Autostash {
				t.Fatalf("result = %+v, specs = %+v", res, f.integrated)
			}
		})
	}
}

func TestUpdateRepo_MergeAutostashNeedsGit227(t *testing.T) {
	f := newUpdateFixture(t)
	f.entries[f.worktree("api")] = []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "a.go"}}
	f.git.VersionFunc = func() (ports.Version, error) { return ports.Version{Major: 2, Minor: 26, Patch: 2}, nil }

	res := f.update(t, "api", domain.UpdateMerge, true)
	wantCode(t, res.Err, domain.CodeGitTooOld)
	if f.called("Integrate") {
		t.Fatal("merged with --autostash on a git that lacks it")
	}

	// Rebase has had --autostash far longer than the supported minimum.
	if res := f.update(t, "api", domain.UpdateRebase, true); res.Err != nil {
		t.Fatalf("rebase --autostash on git 2.26 = %v", res.Err)
	}
}

func TestUpdateRepo_RefusesADetachedHead(t *testing.T) {
	f := newUpdateFixture(t)
	f.git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "", true, nil }

	res := f.update(t, "api", domain.UpdateMerge, false)

	wantCode(t, res.Err, domain.CodeDetachedHead)
	if f.called("Fetch") || f.called("Integrate") {
		t.Fatalf("calls = %+v, want nothing done", f.git.Calls)
	}
}

func TestUpdateRepo_NeverTouchesAMergeOrRebaseAlreadyInProgress(t *testing.T) {
	f := newUpdateFixture(t)
	f.git.IntegrationInProgressFunc = func(domain.Path) (domain.UpdateStrategy, error) { return domain.UpdateRebase, nil }

	res := f.update(t, "api", domain.UpdateMerge, false)

	wantCode(t, res.Err, domain.CodeIntegrationInProgress)
	if f.called("Integrate") || f.called("AbortIntegration") {
		t.Fatalf("calls = %+v, want the user's own operation left alone", f.git.Calls)
	}
}

func TestUpdateRepo_MissingBaseIsRefused(t *testing.T) {
	f := newUpdateFixture(t)
	f.refs = map[string]string{}

	res := f.update(t, "api", domain.UpdateMerge, false)

	wantCode(t, res.Err, domain.CodeBaseMissing)
	if res.Base != "develop" || f.called("Integrate") {
		t.Fatalf("result = %+v, want the expected base named and nothing integrated", res)
	}
}

func TestUpdateRepo_FetchFailureIntegratesNothing(t *testing.T) {
	f := newUpdateFixture(t)
	f.git.FetchFunc = func(domain.Path, string) error {
		return domain.NewOpError("git.fetch", domain.CodeGitFailed, "", "offline", nil)
	}

	res := f.update(t, "api", domain.UpdateMerge, false)

	wantCode(t, res.Err, domain.CodeGitFailed)
	if f.called("Integrate") {
		t.Fatal("integrated a base that could not be fetched")
	}
}

// conflicting scripts Integrate to stop in strategy s with conflicts on
// paths, until AbortIntegration restores the previous state.
func (f *updateFixture) conflicting(s domain.UpdateStrategy, paths ...string) *[]domain.UpdateStrategy {
	var aborted []domain.UpdateStrategy
	var inProgress domain.UpdateStrategy
	f.git.IntegrateFunc = func(domain.Path, ports.IntegrateSpec) error {
		inProgress = s
		return domain.NewOpError("git.integrate", domain.CodeGitFailed, "", "CONFLICT", nil)
	}
	f.git.IntegrationInProgressFunc = func(domain.Path) (domain.UpdateStrategy, error) { return inProgress, nil }
	f.git.ConflictedPathsFunc = func(domain.Path) ([]string, error) {
		if inProgress == "" {
			return nil, nil
		}
		return paths, nil
	}
	f.git.AbortIntegrationFunc = func(_ domain.Path, got domain.UpdateStrategy) error {
		aborted = append(aborted, got)
		inProgress = ""
		return nil
	}
	return &aborted
}

func TestUpdateRepo_ConflictIsAbortedAndReported(t *testing.T) {
	for _, s := range []domain.UpdateStrategy{domain.UpdateMerge, domain.UpdateRebase} {
		t.Run(string(s), func(t *testing.T) {
			f := newUpdateFixture(t)
			aborted := f.conflicting(s, "a.go", "dir/b.go")

			res := f.update(t, "api", s, false)

			wantCode(t, res.Err, domain.CodeUpdateConflict)
			if strings.Join(res.Conflicts, ",") != "a.go,dir/b.go" || !res.Restored {
				t.Fatalf("result = %+v, want both conflicts and a restored repo", res)
			}
			if len(*aborted) != 1 || (*aborted)[0] != s {
				t.Fatalf("aborted = %v, want one %s abort", *aborted, s)
			}
			if res.AfterHead != res.BeforeHead {
				t.Fatalf("HEAD moved: %+v", res)
			}
		})
	}
}

func TestUpdateRepo_ConflictReportsAnIncompleteRestore(t *testing.T) {
	f := newUpdateFixture(t)
	f.conflicting(domain.UpdateMerge, "a.go")
	f.git.AbortIntegrationFunc = func(domain.Path, domain.UpdateStrategy) error {
		return domain.NewOpError("git.abort_integration", domain.CodeGitFailed, "", "", nil)
	}

	res := f.update(t, "api", domain.UpdateMerge, false)

	wantCode(t, res.Err, domain.CodeUpdateConflict)
	if res.Restored {
		t.Fatalf("result = %+v, want Restored false when the abort failed", res)
	}
}

func TestUpdateRepo_FailureWithoutAStoppedOperationIsReturnedAsIs(t *testing.T) {
	f := newUpdateFixture(t)
	boom := domain.NewOpError("git.integrate", domain.CodeGitFailed, "", "untracked file would be overwritten", nil)
	f.git.IntegrateFunc = func(domain.Path, ports.IntegrateSpec) error { return boom }

	res := f.update(t, "api", domain.UpdateMerge, false)

	if !errors.Is(res.Err, boom) || f.called("AbortIntegration") {
		t.Fatalf("result = %+v, want the git error and no abort", res)
	}
}

// git re-applies an autostash after a successful merge or rebase; when that
// conflicts it leaves conflict markers and keeps the stash. The update is
// then undone: HEAD reset to where it was and the stash popped back.
func TestUpdateRepo_AutostashThatCannotBeReappliedIsUndone(t *testing.T) {
	f := newUpdateFixture(t)
	wt := f.worktree("api")
	f.entries[wt] = []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "a.go"}}
	stash := ""
	conflicted := false
	f.git.StashRefFunc = func(domain.Path) (string, error) { return stash, nil }
	f.git.IntegrateFunc = func(domain.Path, ports.IntegrateSpec) error {
		f.head[wt] = "after-merge"
		stash, conflicted = "autostash", true
		return nil
	}
	f.git.ConflictedPathsFunc = func(domain.Path) ([]string, error) {
		if conflicted {
			return []string{"a.go"}, nil
		}
		return nil, nil
	}
	var resetTo string
	f.git.ResetHardFunc = func(_ domain.Path, commit string) error {
		resetTo, f.head[wt], conflicted = commit, commit, false
		return nil
	}
	f.git.StashPopFunc = func(domain.Path) error { stash = ""; return nil }

	res := f.update(t, "api", domain.UpdateMerge, true)

	wantCode(t, res.Err, domain.CodeUpdateConflict)
	if resetTo != "before-api" || stash != "" || !res.Restored || strings.Join(res.Conflicts, ",") != "a.go" {
		t.Fatalf("result = %+v, resetTo = %q, stash = %q", res, resetTo, stash)
	}
	if res.AfterHead != "before-api" {
		t.Fatalf("AfterHead = %q, want the original HEAD", res.AfterHead)
	}
}

func TestUpdateRepo_UnknownAliasIsAnError(t *testing.T) {
	f := newUpdateFixture(t)

	_, err := app.UpdateRepo(context.Background(), f.deps(), app.UpdateRepoInput{WorkspaceRoot: f.wsRoot, Alias: "ghost", Strategy: domain.UpdateMerge})

	if domain.Code(err) != domain.CodeRepoNotFound {
		t.Fatalf("err = %v, want repo_not_found", err)
	}
}

func TestUpdateWorkspace_UpdatesEveryRepoAndNeverAllOrNothing(t *testing.T) {
	f := newUpdateFixture(t)
	f.entries[f.worktree("api")] = []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "a.go"}}

	res, err := app.UpdateWorkspace(context.Background(), f.deps(), app.UpdateWorkspaceInput{WorkspaceRoot: f.wsRoot, Strategy: domain.UpdateMerge})

	if err != nil || len(res) != 2 {
		t.Fatalf("UpdateWorkspace() = %+v, %v", res, err)
	}
	if res[0].Alias != "api" || domain.Code(res[0].Err) != domain.CodeWorktreeDirty {
		t.Fatalf("api = %+v, want the dirty refusal", res[0])
	}
	if res[1].Alias != "hub" || res[1].Err != nil || res[1].CommitsIntegrated != 3 {
		t.Fatalf("hub = %+v, want it updated despite api's refusal", res[1])
	}
	var phases []string
	for _, ev := range f.reporter.RepoEvents {
		phases = append(phases, ev.Repo+":"+string(ev.Phase))
	}
	if strings.Join(phases, ",") != "api:started,api:failed,hub:started,hub:finished" {
		t.Fatalf("events = %v, want sequential per-repo progress", phases)
	}
}
