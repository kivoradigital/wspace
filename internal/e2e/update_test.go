// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateResult mirrors workspaces.updateRepo's result.
type updateResult struct {
	Repo              string   `json:"repo"`
	Strategy          string   `json:"strategy"`
	Base              string   `json:"base"`
	BeforeHead        string   `json:"beforeHead"`
	AfterHead         string   `json:"afterHead"`
	UpToDate          bool     `json:"upToDate"`
	CommitsIntegrated int      `json:"commitsIntegrated"`
	Conflicts         []string `json:"conflicts"`
	Error             *struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	} `json:"error"`
}

// updateFixture is a workspace "feat" (base develop) mounting api, with
// one local commit on the workspace branch, created before origin/develop
// advanced by one more commit.
type updateFixture struct {
	fx       *Fixture
	api      GitProject
	worktree string
}

func newUpdateFixture(t *testing.T) updateFixture {
	t.Helper()
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	api := NewGitProject(t, projectsRoot, "api", "develop")
	responses, _ := rpcSession(t, fx,
		req("ctx", "contexts.create", map[string]any{"name": "upd", "workspacesRoot": workspacesRoot, "activate": true, "defaults": map[string]any{"baseBranch": "develop"}}),
		req("r", "projects.register", map[string]any{"key": "api", "sourceDir": api.Main}),
		req("c", "workspaces.create", map[string]any{"name": "feat", "branch": "feat"}),
	)
	mustResult(t, responses, "c")
	u := updateFixture{fx: fx, api: api, worktree: filepath.Join(workspacesRoot, "feat", "api")}
	commitFile(t, u.worktree, "feat.txt", "local work\n")
	commitFile(t, api.Main, "upstream.txt", "from develop\n")
	runGit(t, api.Main, "push", "-q", "origin", "develop")
	return u
}

func commitFile(t *testing.T, dir, file, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "--", file)
	runGit(t, dir, "commit", "-q", "-m", "change "+file)
}

func (u updateFixture) updateRepo(t *testing.T, params map[string]any) updateResult {
	t.Helper()
	p := map[string]any{"workspace": "feat", "repo": "api"}
	for k, v := range params {
		p[k] = v
	}
	responses, events := rpcSession(t, u.fx, req("u", "workspaces.updateRepo", p))
	var res updateResult
	if err := json.Unmarshal(mustResult(t, responses, "u"), &res); err != nil {
		t.Fatal(err)
	}
	if len(events["u"]) < 2 || events["u"][0]["op"] != "workspace.update" {
		t.Fatalf("progress events = %+v, want per-repo update events", events["u"])
	}
	return res
}

// state is what an aborted update must leave untouched.
func (u updateFixture) state(t *testing.T) string {
	t.Helper()
	return runGit(t, u.worktree, "rev-parse", "HEAD") + "\n" + runGit(t, u.worktree, "status", "--porcelain=v1", "--untracked-files=all") +
		"\nstash:" + runGit(t, u.worktree, "stash", "list")
}

func (u updateFixture) assertNothingInProgress(t *testing.T) {
	t.Helper()
	for _, p := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply"} {
		path := runGit(t, u.worktree, "rev-parse", "--path-format=absolute", "--git-path", p)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("%s exists: the repo was left mid-merge/rebase", p)
		}
	}
}

func TestUpdate_MergeIntegratesTheAdvancedBaseThenIsUpToDate(t *testing.T) {
	u := newUpdateFixture(t)
	before := runGit(t, u.worktree, "rev-parse", "HEAD")

	res := u.updateRepo(t, nil)

	if res.Error != nil || res.Strategy != "merge" || res.Base != "origin/develop" || res.CommitsIntegrated != 1 ||
		res.BeforeHead != before || res.AfterHead == before || res.UpToDate {
		t.Fatalf("result = %+v", res)
	}
	if got := runGit(t, u.worktree, "rev-parse", "HEAD"); got != res.AfterHead {
		t.Fatalf("HEAD = %s, want afterHead %s", got, res.AfterHead)
	}
	if parents := strings.Fields(runGit(t, u.worktree, "rev-list", "--parents", "-n", "1", "HEAD")); len(parents) != 3 {
		t.Fatalf("HEAD parents = %v, want a merge commit", parents)
	}
	if _, err := os.Stat(filepath.Join(u.worktree, "upstream.txt")); err != nil {
		t.Fatal("the base's file was not integrated")
	}
	if b := CurrentBranch(t, u.worktree); b != "feat" {
		t.Fatalf("branch = %q, want feat", b)
	}

	again := u.updateRepo(t, nil)
	if again.Error != nil || !again.UpToDate || again.AfterHead != res.AfterHead || again.CommitsIntegrated != 0 {
		t.Fatalf("second update = %+v, want up to date", again)
	}
}

func TestUpdate_RebaseThroughTheCLI(t *testing.T) {
	u := newUpdateFixture(t)

	out, _ := u.fx.MustRun("", "update", "feat", "--rebase")

	if !strings.Contains(out, "api: integrated 1 commit(s) from origin/develop (rebase)") {
		t.Fatalf("stdout = %q", out)
	}
	if parents := strings.Fields(runGit(t, u.worktree, "rev-list", "--parents", "-n", "1", "HEAD")); len(parents) != 2 {
		t.Fatalf("HEAD parents = %v, want a linear history", parents)
	}
	runGit(t, u.worktree, "merge-base", "--is-ancestor", "origin/develop", "HEAD")
	if got := runGit(t, u.worktree, "log", "-1", "--format=%s"); got != "change feat.txt" {
		t.Fatalf("HEAD subject = %q, want the local commit replayed on top", got)
	}
}

func TestUpdate_DirtyWorktreeIsRefusedThenAutostashed(t *testing.T) {
	u := newUpdateFixture(t)
	if err := os.WriteFile(filepath.Join(u.worktree, "README.md"), []byte("edited, not committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := u.state(t)

	res := u.updateRepo(t, nil)

	if res.Error == nil || res.Error.Code != "conflict" || res.Error.Data["domainCode"] != "worktree_dirty" {
		t.Fatalf("result = %+v, want a worktree_dirty refusal", res)
	}
	if files, _ := res.Error.Data["files"].([]any); len(files) != 1 || files[0] != "README.md" {
		t.Fatalf("error data = %+v, want files [README.md]", res.Error.Data)
	}
	if after := u.state(t); after != before {
		t.Fatalf("a refused update changed the repo:\n%s\n--- want ---\n%s", after, before)
	}

	res = u.updateRepo(t, map[string]any{"autostash": true})

	if res.Error != nil || res.CommitsIntegrated != 1 {
		t.Fatalf("autostash result = %+v", res)
	}
	if body, _ := os.ReadFile(filepath.Join(u.worktree, "README.md")); string(body) != "edited, not committed\n" {
		t.Fatalf("README.md = %q, want the local edit kept", body)
	}
	if st := runGit(t, u.worktree, "status", "--porcelain=v1"); st != "M README.md" {
		t.Fatalf("status = %q, want only the local edit", st)
	}
	if stash := runGit(t, u.worktree, "stash", "list"); stash != "" {
		t.Fatalf("stash = %q, want the autostash consumed", stash)
	}
}

func TestUpdate_ConflictIsAbortedAndTheRepoRestored(t *testing.T) {
	for _, strategy := range []string{"merge", "rebase"} {
		t.Run(strategy, func(t *testing.T) {
			u := newUpdateFixture(t)
			commitFile(t, u.api.Main, "README.md", "develop's version\n")
			runGit(t, u.api.Main, "push", "-q", "origin", "develop")
			commitFile(t, u.worktree, "README.md", "the workspace's version\n")
			if err := os.WriteFile(filepath.Join(u.worktree, "scratch.txt"), []byte("untracked\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			before := u.state(t)

			res := u.updateRepo(t, map[string]any{"strategy": strategy})

			if res.Error == nil || res.Error.Data["domainCode"] != "update_conflict" || res.Error.Data["restored"] != true {
				t.Fatalf("result = %+v, want a restored update_conflict", res)
			}
			if len(res.Conflicts) != 1 || res.Conflicts[0] != "README.md" {
				t.Fatalf("conflicts = %v, want [README.md]", res.Conflicts)
			}
			if after := u.state(t); after != before {
				t.Fatalf("repo not restored:\n%s\n--- want ---\n%s", after, before)
			}
			u.assertNothingInProgress(t)
		})
	}
}

// git's own autostash re-apply can conflict after a successful merge; the
// update is then undone too, keeping the uncommitted edit.
func TestUpdate_AutostashThatConflictsOnReapplyIsUndone(t *testing.T) {
	u := newUpdateFixture(t)
	commitFile(t, u.api.Main, "README.md", "develop's version\n")
	runGit(t, u.api.Main, "push", "-q", "origin", "develop")
	if err := os.WriteFile(filepath.Join(u.worktree, "README.md"), []byte("uncommitted edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := u.state(t)

	res := u.updateRepo(t, map[string]any{"autostash": true})

	if res.Error == nil || res.Error.Data["domainCode"] != "update_conflict" || res.Error.Data["restored"] != true {
		t.Fatalf("result = %+v, want a restored update_conflict", res)
	}
	if after := u.state(t); after != before {
		t.Fatalf("repo not restored:\n%s\n--- want ---\n%s", after, before)
	}
	u.assertNothingInProgress(t)
}

func TestUpdate_WorkspaceReportsEveryRepoAsJSON(t *testing.T) {
	u := newUpdateFixture(t)

	out, _ := u.fx.MustRun("", "update", "feat", "--json")

	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("stdout %q: %v", out, err)
	}
	if len(items) != 1 || items[0]["repo"] != "api" || items[0]["commits_integrated"] != float64(1) {
		t.Fatalf("items = %+v", items)
	}

	responses, _ := rpcSession(t, u.fx, req("w", "workspaces.update", map[string]any{"workspace": "feat"}))
	var res struct {
		Repos []updateResult `json:"repos"`
	}
	if err := json.Unmarshal(mustResult(t, responses, "w"), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 1 || !res.Repos[0].UpToDate {
		t.Fatalf("workspaces.update = %+v, want api up to date", res)
	}
}
