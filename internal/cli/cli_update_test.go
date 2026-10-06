// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// updateCLIFixture mounts svc and web in ws1 (base develop), both three
// commits behind origin/develop, with an Integrate that records its spec.
type updateCLIFixture struct {
	*fixture
	wsRoot domain.Path
	specs  map[domain.Path]ports.IntegrateSpec
	dirty  map[domain.Path]bool
}

func newUpdateCLIFixture(t *testing.T) *updateCLIFixture {
	t.Helper()
	fx := &updateCLIFixture{fixture: newFixture(t), wsRoot: "/fixture/workspaces/ws1", specs: map[domain.Path]ports.IntegrateSpec{}, dirty: map[domain.Path]bool{}}
	base := domain.BranchName("develop")
	fx.Store.PutManifest(fx.wsRoot, domain.Manifest{Workspace: domain.Workspace{
		Name: "ws1", Root: fx.wsRoot, Context: "work", Branch: "feature-x",
		Options: domain.Options{BaseBranch: &base},
		Repos: []domain.RepoEntry{
			{Alias: "svc", Project: "svc", SourceDir: "/fixture/src/svc", Branch: "feature-x"},
			{Alias: "web", Project: "web", SourceDir: "/fixture/src/web", Branch: "feature-x"},
		},
	}})
	fx.Git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feature-x", false, nil }
	fx.Git.BehindCountFunc = func(wt domain.Path, _ string) (int, error) {
		if _, done := fx.specs[wt]; done {
			return 0, nil
		}
		return 3, nil
	}
	fx.Git.StatusFunc = func(wt domain.Path) ([]domain.PorcelainEntry, error) {
		if fx.dirty[wt] {
			return []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "main.go"}}, nil
		}
		return nil, nil
	}
	fx.Git.IntegrateFunc = func(wt domain.Path, spec ports.IntegrateSpec) error { fx.specs[wt] = spec; return nil }
	return fx
}

func TestCLI_Update_MergesEveryRepo(t *testing.T) {
	fx := newUpdateCLIFixture(t)

	stdout, stderr, code := run(fx.RT, "", "update", "ws1")

	if code != 0 {
		t.Fatalf("exit code = %d; stderr=%q", code, stderr)
	}
	for _, alias := range []string{"svc", "web"} {
		spec, ok := fx.specs[fx.wsRoot.Join(alias)]
		if !ok || spec.Strategy != domain.UpdateMerge || spec.Ref != "origin/develop" || spec.Autostash {
			t.Fatalf("%s spec = %+v, %v", alias, spec, ok)
		}
		if !strings.Contains(stdout, alias+": integrated 3 commit(s) from origin/develop (merge)") {
			t.Fatalf("stdout = %q, want a line for %s", stdout, alias)
		}
	}

	// Run again: nothing left to integrate.
	stdout, _, code = run(fx.RT, "", "update", "ws1")
	if code != 0 || !strings.Contains(stdout, "svc: already up to date with origin/develop") {
		t.Fatalf("second run: code %d stdout %q", code, stdout)
	}
}

func TestCLI_Update_OneRepoWithRebaseAndAutostash(t *testing.T) {
	fx := newUpdateCLIFixture(t)
	fx.dirty[fx.wsRoot.Join("web")] = true

	_, stderr, code := run(fx.RT, "", "update", "ws1", "--repo", "web", "--rebase", "--autostash")

	if code != 0 {
		t.Fatalf("exit code = %d; stderr=%q", code, stderr)
	}
	if _, touched := fx.specs[fx.wsRoot.Join("svc")]; touched {
		t.Fatal("--repo web also updated svc")
	}
	if spec := fx.specs[fx.wsRoot.Join("web")]; spec.Strategy != domain.UpdateRebase || !spec.Autostash {
		t.Fatalf("web spec = %+v, want rebase with autostash", spec)
	}
}

func TestCLI_Update_DirtyRefusalFailsAndExplains(t *testing.T) {
	fx := newUpdateCLIFixture(t)
	fx.dirty[fx.wsRoot.Join("svc")] = true

	stdout, stderr, code := run(fx.RT, "", "update", "ws1")

	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero when a repo was not updated; stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "svc: not updated, uncommitted changes in: main.go") || !strings.Contains(stdout, "--autostash") {
		t.Fatalf("stdout = %q, want the dirty files and the --autostash hint", stdout)
	}
	if !strings.Contains(stdout, "web: integrated 3 commit(s)") {
		t.Fatalf("stdout = %q, want web still updated", stdout)
	}
	if !strings.Contains(stderr, "1 of 2") {
		t.Fatalf("stderr = %q, want the failure count", stderr)
	}
}

func TestCLI_Update_ConflictIsReportedAsAborted(t *testing.T) {
	fx := newUpdateCLIFixture(t)
	stopped := domain.UpdateStrategy("")
	fx.Git.IntegrateFunc = func(domain.Path, ports.IntegrateSpec) error {
		stopped = domain.UpdateMerge
		return domain.NewOpError("git.integrate", domain.CodeGitFailed, "", "", nil)
	}
	fx.Git.IntegrationInProgressFunc = func(domain.Path) (domain.UpdateStrategy, error) { return stopped, nil }
	fx.Git.ConflictedPathsFunc = func(domain.Path) ([]string, error) { return []string{"a.go", "b.go"}, nil }
	fx.Git.AbortIntegrationFunc = func(domain.Path, domain.UpdateStrategy) error { stopped = ""; return nil }

	stdout, _, code := run(fx.RT, "", "update", "ws1", "--repo", "svc")

	if code == 0 || !strings.Contains(stdout, "svc: merge with origin/develop conflicted in: a.go, b.go") || !strings.Contains(stdout, "unchanged") {
		t.Fatalf("code %d stdout %q, want the conflict explained", code, stdout)
	}
}

func TestCLI_Update_JSON(t *testing.T) {
	fx := newUpdateCLIFixture(t)
	fx.dirty[fx.wsRoot.Join("web")] = true

	stdout, _, code := run(fx.RT, "", "update", "ws1", "--json")

	if code == 0 {
		t.Fatal("exit code = 0, want non-zero: web was refused")
	}
	for _, want := range []string{`"repo": "svc"`, `"commits_integrated": 3`, `"before_head"`, `"up_to_date": false`, `"repo": "web"`, `"code": "worktree_dirty"`, `"dirty_files": [`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %s, want %s", stdout, want)
		}
	}
}

func TestCLI_Update_DefaultsToTheWorkspaceAroundTheCwd(t *testing.T) {
	fx := newUpdateCLIFixture(t)
	fx.FS.Chdir(fx.wsRoot)

	_, stderr, code := run(fx.RT, "", "update")

	if code != 0 || len(fx.specs) != 2 {
		t.Fatalf("code %d stderr %q specs %+v, want ws1 updated", code, stderr, fx.specs)
	}

	fx.FS.Chdir("/elsewhere")
	if _, stderr, code := run(fx.RT, "", "update"); code == 0 || !strings.Contains(stderr, "wspace update <workspace>") {
		t.Fatalf("outside a workspace: code %d stderr %q", code, stderr)
	}
}

func TestCLI_Update_UnknownRepo(t *testing.T) {
	fx := newUpdateCLIFixture(t)

	if _, stderr, code := run(fx.RT, "", "update", "ws1", "--repo", "ghost"); code == 0 || !strings.Contains(stderr, "ghost") {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
}
