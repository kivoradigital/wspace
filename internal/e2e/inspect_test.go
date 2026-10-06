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

type fileDiff struct {
	Path      string `json:"path"`
	OrigPath  string `json:"origPath"`
	Status    string `json:"status"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Hunks     []struct {
		Lines []string `json:"lines"`
	} `json:"hunks"`
}

func (u updateFixture) call(t *testing.T, method string, params map[string]any) json.RawMessage {
	t.Helper()
	p := map[string]any{"workspace": "feat", "repo": "api"}
	for k, v := range params {
		p[k] = v
	}
	responses, _ := rpcSession(t, u.fx, req("x", method, p))
	return mustResult(t, responses, "x")
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return v
}

func writeFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestInspect_ChangesDiffsCommitsAndStashAgainstRealGit covers the
// read-only inspector over staged, unstaged, untracked, renamed and binary
// files, the branch's commits ahead of its base and a stash holding an
// untracked file.
func TestInspect_ChangesDiffsCommitsAndStashAgainstRealGit(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	commitFile(t, wt, "rename-me.txt", strings.Repeat("same line\n", 20))

	// a stash holding a tracked change and an untracked file
	writeFile(t, wt, "README.md", "stashed edit\n")
	writeFile(t, wt, "stashed-new.txt", "only in the stash\n")
	runGit(t, wt, "stash", "push", "-q", "-u", "-m", "half done")

	writeFile(t, wt, "feat.txt", "local work\nstaged line\n")
	runGit(t, wt, "add", "feat.txt")
	writeFile(t, wt, "feat.txt", "local work\nstaged line\nunstaged line\n")
	runGit(t, wt, "mv", "rename-me.txt", "renamed.txt")
	writeFile(t, wt, "new file.txt", "brand new\n")
	writeFile(t, wt, "logo.bin", "\x00\x01\x02\x03binary")
	before := runGit(t, wt, "status", "--porcelain=v1") + runGit(t, wt, "stash", "list")

	type entry struct {
		Path, OrigPath, Status string
	}
	cs := decode[struct {
		Staged, Unstaged, Untracked, Conflicted []entry
	}](t, u.call(t, "repos.changes", nil))
	if len(cs.Staged) != 2 || len(cs.Unstaged) != 1 || len(cs.Untracked) != 2 || len(cs.Conflicted) != 0 {
		t.Fatalf("changes = %+v", cs)
	}
	if cs.Staged[1] != (entry{Path: "renamed.txt", OrigPath: "rename-me.txt", Status: "renamed"}) && cs.Staged[0] != (entry{Path: "renamed.txt", OrigPath: "rename-me.txt", Status: "renamed"}) {
		t.Fatalf("staged = %+v", cs.Staged)
	}

	staged := decode[fileDiff](t, u.call(t, "repos.diff", map[string]any{"path": "feat.txt", "staged": true}))
	if staged.Additions != 1 || staged.Hunks[0].Lines[len(staged.Hunks[0].Lines)-1] != "+staged line" {
		t.Fatalf("staged diff = %+v", staged)
	}
	unstaged := decode[fileDiff](t, u.call(t, "repos.diff", map[string]any{"path": "feat.txt"}))
	if unstaged.Additions != 1 || unstaged.Deletions != 0 {
		t.Fatalf("unstaged diff = %+v", unstaged)
	}
	renamed := decode[fileDiff](t, u.call(t, "repos.diff", map[string]any{"path": "renamed.txt", "staged": true}))
	if renamed.Status != "renamed" || renamed.OrigPath != "rename-me.txt" {
		t.Fatalf("rename diff = %+v", renamed)
	}
	untracked := decode[fileDiff](t, u.call(t, "repos.diff", map[string]any{"path": "new file.txt"}))
	if untracked.Status != "added" || untracked.Hunks[0].Lines[0] != "+brand new" {
		t.Fatalf("untracked diff = %+v", untracked)
	}
	bin := decode[fileDiff](t, u.call(t, "repos.diff", map[string]any{"path": "logo.bin"}))
	if !bin.Binary || len(bin.Hunks) != 0 {
		t.Fatalf("binary diff = %+v", bin)
	}
	responses, _ := rpcSession(t, u.fx, req("bad", "repos.diff", map[string]any{"workspace": "feat", "repo": "api", "path": "../../outside.txt"}))
	if e := responses["bad"].Error; e == nil || e.Code != "not_found" {
		t.Fatalf("outside path = %+v", responses["bad"])
	}

	commits := decode[struct {
		Range   string
		Total   int
		Commits []struct{ Hash, ShortHash, Subject, Author string }
	}](t, u.call(t, "repos.commits", nil))
	if commits.Range != "origin/develop..HEAD" || commits.Total != 2 || len(commits.Commits) != 2 ||
		commits.Commits[0].Subject != "change rename-me.txt" || commits.Commits[0].Author != "wspace e2e" {
		t.Fatalf("commits = %+v", commits)
	}
	raw := u.call(t, "repos.commit", map[string]any{"hash": commits.Commits[1].ShortHash})
	if strings.Contains(string(raw), "example.invalid") {
		t.Fatalf("commit detail exposes the author e-mail: %s", raw)
	}
	detail := decode[struct {
		Subject string
		Files   []fileDiff
	}](t, raw)
	if detail.Subject != "change feat.txt" || len(detail.Files) != 1 || detail.Files[0].Path != "feat.txt" || detail.Files[0].Status != "added" {
		t.Fatalf("commit detail = %+v", detail)
	}

	stashes := decode[[]struct {
		Index           int
		Branch, Message string
	}](t, u.call(t, "repos.stashes", nil))
	if len(stashes) != 1 || stashes[0].Branch != "feat" || stashes[0].Message != "half done" {
		t.Fatalf("stashes = %+v", stashes)
	}
	stash := decode[struct {
		Files             []fileDiff
		IncludesUntracked bool
	}](t, u.call(t, "repos.stash", map[string]any{"index": 0}))
	var paths []string
	for _, f := range stash.Files {
		paths = append(paths, f.Path)
	}
	if !stash.IncludesUntracked || strings.Join(paths, ",") != "README.md,stashed-new.txt" {
		t.Fatalf("stash content = %v (untracked %v)", paths, stash.IncludesUntracked)
	}

	branch := decode[struct {
		Branch struct {
			Branch     string
			BaseRef    string
			BaseAhead  *int
			BaseBehind *int
		}
		Staged, Unstaged, Untracked, Stashes int
	}](t, u.call(t, "repos.inspect", nil))
	if branch.Branch.Branch != "feat" || branch.Branch.BaseRef != "origin/develop" || *branch.Branch.BaseAhead != 2 || branch.Stashes != 1 || branch.Untracked != 2 {
		t.Fatalf("inspect = %+v", branch)
	}

	if after := runGit(t, wt, "status", "--porcelain=v1") + runGit(t, wt, "stash", "list"); after != before {
		t.Fatalf("a read-only query changed the repo:\nbefore %q\nafter  %q", before, after)
	}
}

// pullFixture is updateFixture whose workspace branch is pushed with an
// upstream (origin/feat), plus a second clone that can push to feat.
func newPullFixture(t *testing.T) (updateFixture, string) {
	t.Helper()
	u := newUpdateFixture(t)
	runGit(t, u.worktree, "push", "-q", "-u", "origin", "feat")
	other := filepath.Join(u.fx.Root, "other")
	runGit(t, u.fx.Root, "clone", "-q", "-b", "feat", u.api.Remote, other)
	runGit(t, other, "config", "user.email", "e2e@example.invalid")
	runGit(t, other, "config", "user.name", "wspace e2e")
	commitFile(t, other, "teammate.txt", "from a teammate\n")
	runGit(t, other, "push", "-q", "origin", "feat")
	return u, other
}

type pullResult struct {
	Upstream      string
	BeforeHead    string
	AfterHead     string
	UpToDate      bool
	CommitsPulled int
	Ahead, Behind int
	Error         *struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
}

func TestPull_FastForwardsFromTheUpstream(t *testing.T) {
	u, _ := newPullFixture(t)
	before := runGit(t, u.worktree, "rev-parse", "HEAD")

	res := decode[pullResult](t, u.call(t, "repos.pull", nil))

	if res.Error != nil || res.Upstream != "origin/feat" || res.CommitsPulled != 1 || res.BeforeHead != before || res.AfterHead == before {
		t.Fatalf("pull = %+v", res)
	}
	if got := runGit(t, u.worktree, "rev-parse", "HEAD"); got != runGit(t, u.worktree, "rev-parse", "origin/feat") {
		t.Fatalf("HEAD %s is not origin/feat", got)
	}
	if parents := strings.Fields(runGit(t, u.worktree, "rev-list", "--parents", "-n", "1", "HEAD")); len(parents) != 2 {
		t.Fatalf("HEAD parents = %v: a merge commit was created", parents)
	}
	again := decode[pullResult](t, u.call(t, "repos.pull", nil))
	if again.Error != nil || !again.UpToDate {
		t.Fatalf("second pull = %+v", again)
	}
}

func TestPull_DivergedIsRefusedAndTheRepoIsUnchanged(t *testing.T) {
	u, _ := newPullFixture(t)
	commitFile(t, u.worktree, "mine.txt", "local only\n")
	before := u.state(t)

	res := decode[pullResult](t, u.call(t, "repos.pull", nil))

	if res.Error == nil || res.Error.Code != "conflict" || res.Error.Data["domainCode"] != "diverged" || res.Ahead != 1 || res.Behind != 1 {
		t.Fatalf("pull = %+v", res)
	}
	if after := u.state(t); after != before {
		t.Fatalf("repo changed:\nbefore %q\nafter  %q", before, after)
	}
	u.assertNothingInProgress(t)
}

func TestFetch_UpdatesRemoteRefsOnlyThroughTheCLI(t *testing.T) {
	u, _ := newPullFixture(t)
	head := runGit(t, u.worktree, "rev-parse", "HEAD")

	out, _ := u.fx.MustRun("", "repo", "feat", "api", "fetch")

	if !strings.Contains(out, "api: fetched origin") || !strings.Contains(out, "upstream origin/feat: 0 ahead, 1 behind") {
		t.Fatalf("stdout = %q", out)
	}
	if runGit(t, u.worktree, "rev-parse", "HEAD") != head {
		t.Fatal("fetch moved HEAD")
	}
	log, _ := u.fx.MustRun("", "repo", "feat", "api", "log")
	if !strings.Contains(log, "change feat.txt") {
		t.Fatalf("log = %q", log)
	}
}

// A branch that only fast-forwarded to its base after it was pushed is
// ahead of its upstream but has no commits of its own since the base:
// range "upstream" lists what a push would send, range "base" lists none.
func TestInspect_CommitsAheadOfTheUpstreamButNotOfTheBase(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	runGit(t, wt, "branch", "-q", "--unset-upstream")
	if code, _ := u.callErr(t, "repos.commits", map[string]any{"range": "upstream"}); code != "conflict" {
		t.Fatalf("without an upstream: code = %q", code)
	}
	runGit(t, wt, "reset", "-q", "--hard", "HEAD~1")
	runGit(t, wt, "push", "-q", "-u", "origin", "feat")
	runGit(t, wt, "merge", "-q", "--ff-only", "origin/develop")

	type page struct {
		Range, Base, Upstream string
		Total                 int
		Commits               []struct{ Subject string }
	}
	up := decode[page](t, u.call(t, "repos.commits", map[string]any{"range": "upstream"}))
	if up.Range != "origin/feat..HEAD" || up.Upstream != "origin/feat" || up.Base != "" || up.Total != 1 ||
		len(up.Commits) != 1 || up.Commits[0].Subject != "change upstream.txt" {
		t.Fatalf("upstream range = %+v", up)
	}
	base := decode[page](t, u.call(t, "repos.commits", nil))
	if base.Range != "origin/develop..HEAD" || base.Total != 0 || len(base.Commits) != 0 || base.Upstream != "" {
		t.Fatalf("base range = %+v", base)
	}
	if code, _ := u.callErr(t, "repos.commits", map[string]any{"range": "everything"}); code != "invalid_params" {
		t.Fatalf("unknown range: code = %q", code)
	}
}
