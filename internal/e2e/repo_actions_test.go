// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type stashApplyResult struct {
	IndexRestored bool     `json:"indexRestored"`
	Dropped       bool     `json:"dropped"`
	Conflicts     []string `json:"conflicts"`
	Warnings      []string `json:"warnings"`
	Error         *struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	} `json:"error"`
}

// callErr runs one request that must fail and returns its error.
func (u updateFixture) callErr(t *testing.T, method string, params map[string]any) (code string, data map[string]any) {
	t.Helper()
	p := map[string]any{"workspace": "feat", "repo": "api"}
	for k, v := range params {
		p[k] = v
	}
	responses, _ := rpcSession(t, u.fx, req("x", method, p))
	r := responses["x"]
	if r.Error == nil {
		t.Fatalf("%s succeeded: %s", method, r.Result)
	}
	return r.Error.Code, r.Error.Data
}

func stashCount(t *testing.T, wt string) int {
	t.Helper()
	out := runGit(t, wt, "stash", "list")
	if out == "" {
		return 0
	}
	return len(strings.Split(out, "\n"))
}

func stashTop(t *testing.T, wt string) string { return runGit(t, wt, "rev-parse", "stash@{0}") }

// TestRepoActions_StashApplyPopDropAgainstRealGit applies (index and
// untracked file restored, entry kept), pops (entry dropped), pops into a
// conflict (entry kept, markers left, nothing reset) and drops (confirm
// required) real stash entries.
func TestRepoActions_StashApplyPopDropAgainstRealGit(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	commitFile(t, wt, "app.txt", "1\n2\n3\n")

	writeFile(t, wt, "app.txt", "1\nstaged\n3\n")
	runGit(t, wt, "add", "app.txt")
	writeFile(t, wt, "new.txt", "untracked\n")
	runGit(t, wt, "stash", "push", "-q", "-u", "-m", "work")
	hash := stashTop(t, wt)

	r := decode[stashApplyResult](t, u.call(t, "repos.stashApply", map[string]any{"index": 0, "hash": "0000000"}))
	if r.Error == nil || r.Error.Data["domainCode"] != "stash_changed" {
		t.Fatalf("wrong hash = %+v", r)
	}

	r = decode[stashApplyResult](t, u.call(t, "repos.stashApply", map[string]any{"index": 0, "hash": hash[:10]}))
	if r.Error != nil || !r.IndexRestored || r.Dropped {
		t.Fatalf("apply = %+v", r)
	}
	if st := runGit(t, wt, "status", "--porcelain=v1"); !strings.Contains(st, "M  app.txt") || !strings.Contains(st, "?? new.txt") || stashCount(t, wt) != 1 {
		t.Fatalf("after apply: status %q, %d stash entries", st, stashCount(t, wt))
	}

	// An untracked file of the stash already on disk is refused up front.
	r = decode[stashApplyResult](t, u.call(t, "repos.stashPop", map[string]any{"index": 0, "hash": hash}))
	if r.Error == nil || r.Error.Data["domainCode"] != "worktree_dirty" || stashCount(t, wt) != 1 {
		t.Fatalf("pop over existing files = %+v", r)
	}

	runGit(t, wt, "reset", "-q", "--hard")
	runGit(t, wt, "clean", "-fq", "--", "new.txt")
	r = decode[stashApplyResult](t, u.call(t, "repos.stashPop", map[string]any{"index": 0, "hash": hash}))
	if r.Error != nil || !r.Dropped || stashCount(t, wt) != 0 {
		t.Fatalf("pop = %+v, %d entries left", r, stashCount(t, wt))
	}

	// A conflicting pop keeps the entry and leaves the markers.
	runGit(t, wt, "reset", "-q", "--hard")
	runGit(t, wt, "clean", "-fq", "--", "new.txt")
	writeFile(t, wt, "app.txt", "1\nmine\n3\n")
	runGit(t, wt, "stash", "push", "-q", "-m", "conflicting")
	commitFile(t, wt, "app.txt", "1\ntheirs\n3\n")
	hash = stashTop(t, wt)
	r = decode[stashApplyResult](t, u.call(t, "repos.stashPop", map[string]any{"index": 0, "hash": hash}))
	if r.Error == nil || r.Error.Code != "conflict" || r.Error.Data["domainCode"] != "stash_conflict" || !slices.Equal(r.Conflicts, []string{"app.txt"}) || r.Dropped {
		t.Fatalf("conflicting pop = %+v", r)
	}
	if stashCount(t, wt) != 1 {
		t.Fatal("a conflicting pop dropped the stash entry")
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "app.txt")); !strings.Contains(string(b), "<<<<<<<") {
		t.Fatalf("app.txt = %q, want conflict markers left for the user", b)
	}

	// Drop needs confirm, then removes only that entry.
	runGit(t, wt, "reset", "-q", "--hard")
	if code, data := u.callErr(t, "repos.stashDrop", map[string]any{"index": 0, "hash": hash}); code != "needs_confirmation" || data["files"] != float64(1) {
		t.Fatalf("unconfirmed drop = %s %v", code, data)
	}
	if stashCount(t, wt) != 1 {
		t.Fatal("unconfirmed drop removed the entry")
	}
	u.call(t, "repos.stashDrop", map[string]any{"index": 0, "hash": hash, "confirm": true})
	if stashCount(t, wt) != 0 {
		t.Fatal("confirmed drop kept the entry")
	}
}

// TestRepoActions_DiscardUntrackedDeletesOnlyUntrackedFiles checks that
// tracked and ignored files are refused or left alone.
func TestRepoActions_DiscardUntrackedDeletesOnlyUntrackedFiles(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	commitFile(t, wt, ".gitignore", "*.log\n")
	writeFile(t, wt, "feat.txt", "edited tracked file\n")
	writeFile(t, wt, "scratch file.txt", "untracked\n")
	writeFile(t, wt, "debug.log", "ignored\n")

	for _, bad := range []string{"feat.txt", "debug.log", "../outside.txt"} {
		code, data := u.callErr(t, "repos.discardUntracked", map[string]any{"paths": []string{"scratch file.txt", bad}, "confirm": true})
		if code != "not_found" || data["domainCode"] != "path_not_untracked" {
			t.Fatalf("discard %s = %s %v", bad, code, data)
		}
	}
	if code, _ := u.callErr(t, "repos.discardUntracked", map[string]any{"paths": []string{"scratch file.txt"}}); code != "needs_confirmation" {
		t.Fatalf("unconfirmed discard = %s", code)
	}
	v := decode[struct {
		Paths []struct{ Path, AbsolutePath string }
	}](t, u.call(t, "repos.validateUntracked", map[string]any{"paths": []string{"scratch file.txt"}}))
	if len(v.Paths) != 1 || v.Paths[0].AbsolutePath != filepath.Join(wt, "scratch file.txt") {
		t.Fatalf("validate = %+v", v)
	}

	res := decode[struct{ Removed, Kept []string }](t, u.call(t, "repos.discardUntracked", map[string]any{"paths": []string{"scratch file.txt"}, "confirm": true}))
	if !slices.Equal(res.Removed, []string{"scratch file.txt"}) || len(res.Kept) != 0 {
		t.Fatalf("discard = %+v", res)
	}
	for name, want := range map[string]bool{"scratch file.txt": false, "debug.log": true, "feat.txt": true} {
		if _, err := os.Stat(filepath.Join(wt, name)); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", name, err == nil, want)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "feat.txt")); string(b) != "edited tracked file\n" {
		t.Fatalf("tracked edit lost: %q", b)
	}
}

// TestRepoActions_AddableProjectsExcludeTheWorkspacesOwn lists only
// projects not already in the workspace, and addRepo refuses a present one.
func TestRepoActions_AddableProjectsExcludeTheWorkspacesOwn(t *testing.T) {
	u := newUpdateFixture(t)
	web := NewGitProject(t, filepath.Join(u.fx.Root, "projects"), "web", "develop")
	responses, _ := rpcSession(t, u.fx,
		req("r", "projects.register", map[string]any{"key": "web", "sourceDir": web.Main}),
		req("a", "workspaces.addableProjects", map[string]any{"workspace": "feat"}),
		req("dup", "workspaces.addRepo", map[string]any{"workspace": "feat", "project": "api"}),
	)
	mustResult(t, responses, "r")
	list := decode[[]struct{ Key string }](t, mustResult(t, responses, "a"))
	if len(list) != 1 || list[0].Key != "web" {
		t.Fatalf("addable = %+v, want only web", list)
	}
	if e := responses["dup"].Error; e == nil || e.Code != "already_exists" || e.Data["domainCode"] != "already_in_workspace" {
		t.Fatalf("adding api again = %+v", responses["dup"])
	}
}
