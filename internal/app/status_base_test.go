// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// baseFixture is a two-repo workspace ("api", "hub") whose workspace-wide
// base branch is develop and whose branches have no upstream, so ahead is
// counted against each repo's comparison base.
type baseFixture struct {
	fs     *portstest.FakeFS
	git    *portstest.FakeGit
	store  *portstest.FakeConfigStore
	wsRoot domain.Path
	// refs maps "<worktree>|<branch>" to the ref ResolveBase finds; a
	// missing key means neither the local nor the remote branch exists.
	refs map[string]string
	// defaults maps a worktree to its remote default branch (origin/HEAD).
	defaults map[domain.Path]domain.BranchName
	// unpushed maps the resolved ref passed to UnpushedCount to its count.
	unpushed map[string]int
}

func newBaseFixture(t *testing.T, repos ...domain.RepoEntry) *baseFixture {
	t.Helper()
	fs := portstest.NewFakeFS(t)
	f := &baseFixture{
		fs:       fs,
		git:      portstest.NewFakeGit(),
		store:    portstest.NewFakeConfigStore(),
		wsRoot:   fs.Paths().Home.Join("workspaces", "findings"),
		refs:     map[string]string{},
		defaults: map[domain.Path]domain.BranchName{},
		unpushed: map[string]int{},
	}
	if len(repos) == 0 {
		repos = []domain.RepoEntry{
			{Alias: "api", Project: "api", SourceDir: "/src/api", Branch: "feat"},
			{Alias: "hub", Project: "hub", SourceDir: "/src/hub", Branch: "feat"},
		}
	}
	base := domain.BranchName("develop")
	f.store.PutManifest(f.wsRoot, domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{
		Name: "findings", Root: f.wsRoot, Context: "work", Branch: "feat",
		Options: domain.Options{BaseBranch: &base},
		Repos:   repos,
	}})
	if err := fs.MkdirAll(f.wsRoot); err != nil {
		t.Fatal(err)
	}

	f.git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feat", false, nil }
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) {
		return 0, 0, domain.NewOpError("git.ahead_behind", domain.CodeGitFailed, "", "no upstream", nil)
	}
	f.git.ResolveBaseFunc = func(repo domain.Path, remote string, b domain.BranchName) (ports.BaseRef, error) {
		if ref, ok := f.refs[string(repo)+"|"+string(b)]; ok {
			return ports.BaseRef{Ref: ref, Remote: ref != string(b)}, nil
		}
		return ports.BaseRef{Ref: "HEAD"}, nil
	}
	f.git.RemoteDefaultBranchFunc = func(repo domain.Path, _ string) (domain.BranchName, bool, error) {
		b, ok := f.defaults[repo]
		return b, ok, nil
	}
	f.git.UnpushedCountFunc = func(_ domain.Path, base string) (int, error) {
		n, ok := f.unpushed[base]
		if !ok {
			return 0, domain.NewOpError("git.unpushed_count", domain.CodeRefNotFound, base, "unknown revision", nil)
		}
		return n, nil
	}
	return f
}

func (f *baseFixture) worktree(alias string) domain.Path { return f.wsRoot.Join(alias) }

func (f *baseFixture) has(alias string, branch domain.BranchName, ref string) {
	f.refs[string(f.worktree(alias))+"|"+string(branch)] = ref
}

func (f *baseFixture) deps() app.Deps {
	return app.Deps{Store: f.store, Git: f.git, FS: f.fs, Reporter: portstest.NewRecordingReporter()}
}

func (f *baseFixture) status(t *testing.T) app.WorkspaceStatus {
	t.Helper()
	st, err := app.Status(context.Background(), f.deps(), app.StatusInput{WorkspaceRoot: f.wsRoot})
	if err != nil {
		t.Fatalf("Status() = %v, want the workspace to render", err)
	}
	return st
}

func repoByAlias(t *testing.T, st app.WorkspaceStatus, alias string) domain.RepoStatus {
	t.Helper()
	for _, r := range st.Repos {
		if r.Alias == alias {
			return r
		}
	}
	t.Fatalf("repo %q not in %+v", alias, st.Repos)
	return domain.RepoStatus{}
}

func TestStatus_RepoWithoutTheBaseBranchIsReportedPerRepoNotAsAWorkspaceError(t *testing.T) {
	f := newBaseFixture(t)
	f.has("api", "develop", "origin/develop")
	f.unpushed["origin/develop"] = 3

	st := f.status(t)

	if st.Err != nil {
		t.Fatalf("Err = %v, want nil: one repo's missing base must not fail the workspace", st.Err)
	}
	api := repoByAlias(t, st, "api")
	if api.Ahead != 3 || api.BaseBranch != "develop" || api.BaseMissing {
		t.Errorf("api = %+v, want ahead 3 against develop", api)
	}
	hub := repoByAlias(t, st, "hub")
	if !hub.BaseMissing || hub.BaseBranch != "develop" || hub.Ahead != 0 || hub.Behind != 0 {
		t.Errorf("hub = %+v, want baseMissing against develop with zero counts", hub)
	}
	if hub.Branch != "feat" {
		t.Errorf("hub.Branch = %q, want the rest of the status still collected", hub.Branch)
	}
}

func TestStatus_FallsBackToTheRemoteDefaultBranch(t *testing.T) {
	f := newBaseFixture(t)
	f.has("api", "develop", "develop")
	f.unpushed["develop"] = 1
	f.defaults[f.worktree("hub")] = "master"
	f.has("hub", "master", "origin/master")
	f.unpushed["origin/master"] = 2

	st := f.status(t)

	hub := repoByAlias(t, st, "hub")
	if hub.BaseMissing || hub.BaseBranch != "master" || hub.Ahead != 2 {
		t.Errorf("hub = %+v, want ahead 2 against its default branch master", hub)
	}
	api := repoByAlias(t, st, "api")
	if api.BaseBranch != "develop" || api.Ahead != 1 {
		t.Errorf("api = %+v, want ahead 1 against the local develop", api)
	}
}

func TestStatus_ComparisonBasePrecedence(t *testing.T) {
	master := domain.BranchName("master")
	release := domain.BranchName("release")
	tests := []struct {
		name    string
		project domain.Project
		entry   domain.BranchName
		present []domain.BranchName
		want    domain.BranchName
	}{
		{
			name:    "project origin branch beats everything",
			project: domain.Project{Key: "hub", SourceDir: "/src/hub", OriginBranch: &master, Options: domain.Options{BaseBranch: &release}},
			entry:   "main",
			present: []domain.BranchName{"master", "release", "main", "develop"},
			want:    "master",
		},
		{
			name:    "project base option beats the recorded repo base",
			project: domain.Project{Key: "hub", SourceDir: "/src/hub", Options: domain.Options{BaseBranch: &release}},
			entry:   "main",
			present: []domain.BranchName{"release", "main", "develop"},
			want:    "release",
		},
		{
			name:    "recorded repo base beats the workspace base",
			project: domain.Project{Key: "hub", SourceDir: "/src/hub"},
			entry:   "main",
			present: []domain.BranchName{"main", "develop"},
			want:    "main",
		},
		{
			name:    "a configured base that does not exist falls through to the next",
			project: domain.Project{Key: "hub", SourceDir: "/src/hub", OriginBranch: &master},
			present: []domain.BranchName{"develop"},
			want:    "develop",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newBaseFixture(t, domain.RepoEntry{Alias: "hub", Project: "hub", SourceDir: "/src/hub", Branch: "feat", BaseBranch: tt.entry})
			f.store.PutContext(domain.Context{Name: "work", Projects: []domain.Project{tt.project}})
			for _, b := range tt.present {
				f.has("hub", b, "origin/"+string(b))
				f.unpushed["origin/"+string(b)] = 0
			}

			hub := repoByAlias(t, f.status(t), "hub")

			if hub.BaseBranch != tt.want || hub.BaseMissing {
				t.Errorf("hub = %+v, want base %s", hub, tt.want)
			}
		})
	}
}

func TestStatus_MissingBaseNamesTheFirstCandidate(t *testing.T) {
	master := domain.BranchName("master")
	f := newBaseFixture(t, domain.RepoEntry{Alias: "hub", Project: "hub", SourceDir: "/src/hub", Branch: "feat"})
	f.store.PutContext(domain.Context{Name: "work", Projects: []domain.Project{{Key: "hub", SourceDir: "/src/hub", OriginBranch: &master}}})

	hub := repoByAlias(t, f.status(t), "hub")

	if !hub.BaseMissing || hub.BaseBranch != "master" {
		t.Errorf("hub = %+v, want baseMissing naming master (the configured base)", hub)
	}
}

func TestStatus_UpstreamComparisonNeedsNoBase(t *testing.T) {
	f := newBaseFixture(t)
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 4, 2, nil }

	st := f.status(t)

	for _, r := range st.Repos {
		if r.BaseMissing || r.BaseBranch != "" || r.Ahead != 4 || r.Behind != 2 {
			t.Errorf("%s = %+v, want upstream counts and no base", r.Alias, r)
		}
	}
	for _, c := range f.git.Calls {
		if c.Method == "ResolveBase" || c.Method == "RemoteDefaultBranch" {
			t.Fatalf("resolved a base although every repo has an upstream: %+v", f.git.Calls)
		}
	}
}

func TestStatus_RefVanishingBetweenResolveAndCountIsBaseMissing(t *testing.T) {
	f := newBaseFixture(t, domain.RepoEntry{Alias: "hub", Project: "hub", SourceDir: "/src/hub", Branch: "feat"})
	f.has("hub", "develop", "origin/develop") // resolvable, but rev-list fails

	hub := repoByAlias(t, f.status(t), "hub")

	if !hub.BaseMissing || hub.BaseBranch != "develop" {
		t.Errorf("hub = %+v, want baseMissing", hub)
	}
}

func TestStatus_OtherGitFailuresStillFailTheWorkspace(t *testing.T) {
	f := newBaseFixture(t, domain.RepoEntry{Alias: "hub", Project: "hub", SourceDir: "/src/hub", Branch: "feat"})
	f.has("hub", "develop", "origin/develop")
	boom := errors.New("boom")
	f.git.UnpushedCountFunc = func(domain.Path, string) (int, error) { return 0, boom }

	_, err := app.Status(context.Background(), f.deps(), app.StatusInput{WorkspaceRoot: f.wsRoot})

	if !errors.Is(err, boom) {
		t.Fatalf("Status() = %v, want the git failure", err)
	}
}

func TestList_MixedBasesRenderTheWholeWorkspace(t *testing.T) {
	f := newBaseFixture(t)
	f.has("api", "develop", "origin/develop")
	f.unpushed["origin/develop"] = 0

	list, err := app.List(context.Background(), f.deps(), app.ListInput{WorkspacesRoot: f.wsRoot.Join(".."), Context: "work"})
	if err != nil {
		t.Fatalf("List() = %v", err)
	}
	if len(list) != 1 || list[0].Err != nil || len(list[0].Repos) != 2 {
		t.Fatalf("List() = %+v, want findings with both repos and no error", list)
	}
	if hub := repoByAlias(t, list[0], "hub"); !hub.BaseMissing {
		t.Errorf("hub = %+v, want baseMissing", hub)
	}
}

// Without an upstream, BaseBehind counts the base's commits not yet in the
// branch (what an update would integrate); it stays unknown (nil) with an
// upstream, a missing base or a failed count.
func TestStatus_BaseBehindCountsTheBaseCommitsNotYetIntegrated(t *testing.T) {
	f := newBaseFixture(t)
	f.has("api", "develop", "origin/develop")
	f.unpushed["origin/develop"] = 1
	f.git.BehindCountFunc = func(_ domain.Path, base string) (int, error) {
		if base != "origin/develop" {
			t.Fatalf("BehindCount base = %q, want the resolved ref", base)
		}
		return 5, nil
	}

	st := f.status(t)

	if api := repoByAlias(t, st, "api"); api.BaseBehind == nil || *api.BaseBehind != 5 {
		t.Errorf("api.BaseBehind = %v, want 5", api.BaseBehind)
	}
	if hub := repoByAlias(t, st, "hub"); hub.BaseBehind != nil {
		t.Errorf("hub.BaseBehind = %v, want unknown for a missing base", *hub.BaseBehind)
	}

	f.git.BehindCountFunc = func(domain.Path, string) (int, error) {
		return 0, domain.NewOpError("git.behind_count", domain.CodeGitFailed, "", "", nil)
	}
	if api := repoByAlias(t, f.status(t), "api"); api.BaseBehind != nil {
		t.Errorf("api.BaseBehind = %v after a failed count, want unknown", *api.BaseBehind)
	}

	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 0, 0, nil }
	if api := repoByAlias(t, f.status(t), "api"); api.BaseBehind != nil {
		t.Errorf("api.BaseBehind = %v with an upstream, want unknown", *api.BaseBehind)
	}
}
