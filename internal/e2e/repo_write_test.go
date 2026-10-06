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

type changeSets struct {
	Staged, Unstaged, Untracked []struct{ Path, OrigPath, Status string }
}

func (c changeSets) paths(side string) []string {
	list := map[string][]struct{ Path, OrigPath, Status string }{"staged": c.Staged, "unstaged": c.Unstaged, "untracked": c.Untracked}[side]
	out := []string{}
	for _, e := range list {
		out = append(out, e.Path)
	}
	return out
}

func (u updateFixture) changes(t *testing.T) changeSets {
	t.Helper()
	return decode[changeSets](t, u.call(t, "repos.changes", nil))
}

type commitResult struct {
	Commit *struct{ Hash, ShortHash, Subject string }
	Error  *struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
}

type pushResult struct {
	Remote, Upstream, Branch string
	SetUpstream, UpToDate    bool
	Pushed                   int
	Error                    *struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
}

// TestRepoWrite_StageUnstageDeletedAndRenamedFiles stages a modification,
// a deletion and an untracked file, unstages a staged rename (both paths)
// and refuses a path status does not list.
func TestRepoWrite_StageUnstageDeletedAndRenamedFiles(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	commitFile(t, wt, "rename-me.txt", strings.Repeat("same line\n", 20))
	commitFile(t, wt, "gone.txt", "bye\n")
	writeFile(t, wt, "feat.txt", "edited\n")
	if err := os.Remove(filepath.Join(wt, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, wt, "new file.txt", "new\n")

	r := decode[struct{ Paths []string }](t, u.call(t, "repos.stage", map[string]any{"paths": []string{"feat.txt", "gone.txt", "new file.txt"}}))
	if len(r.Paths) != 3 {
		t.Fatalf("stage = %+v", r)
	}
	if st := runGit(t, wt, "status", "--porcelain=v1"); !strings.Contains(st, "M  feat.txt") || !strings.Contains(st, "D  gone.txt") || !strings.Contains(st, `A  "new file.txt"`) {
		t.Fatalf("after stage: %q", st)
	}
	if code, data := u.callErr(t, "repos.stage", map[string]any{"paths": []string{"feat.txt"}}); code != "not_found" || data["domainCode"] != "path_not_changed" {
		t.Fatalf("stage of a staged-only path = %s %v", code, data)
	}

	runGit(t, wt, "mv", "rename-me.txt", "renamed.txt")
	u.call(t, "repos.unstage", map[string]any{"paths": []string{"renamed.txt", "gone.txt"}})
	cs := u.changes(t)
	if !slices.Equal(cs.paths("staged"), []string{"feat.txt", "new file.txt"}) ||
		!slices.Contains(cs.paths("unstaged"), "rename-me.txt") || !slices.Contains(cs.paths("unstaged"), "gone.txt") || !slices.Contains(cs.paths("untracked"), "renamed.txt") {
		t.Fatalf("after unstage: %+v", cs)
	}
	u.call(t, "repos.unstage", map[string]any{"all": true})
	if cs := u.changes(t); len(cs.Staged) != 0 {
		t.Fatalf("after unstage all: %+v", cs)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "feat.txt")); string(b) != "edited\n" {
		t.Fatalf("unstage changed the worktree: %q", b)
	}
}

// TestRepoWrite_DiscardWritesABackupThenRestoresTheWorktree discards only
// unstaged changes, keeps staged ones, refuses untracked and staged-only
// paths, and the backup patch brings the discarded changes back.
func TestRepoWrite_DiscardWritesABackupThenRestoresTheWorktree(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	commitFile(t, wt, "app.txt", "1\n2\n3\n")
	writeFile(t, wt, "app.txt", "1\nstaged\n3\n")
	runGit(t, wt, "add", "app.txt")
	writeFile(t, wt, "app.txt", "1\nstaged\n3\nunstaged\n")
	writeFile(t, wt, "feat.txt", "local edit\n")
	writeFile(t, wt, "notes.txt", "untracked\n")
	writeFile(t, wt, "README.md", "staged only\n")
	runGit(t, wt, "add", "README.md")

	for path, domainCode := range map[string]string{"notes.txt": "path_is_untracked", "README.md": "staged_only"} {
		if code, data := u.callErr(t, "repos.discard", map[string]any{"paths": []string{"feat.txt", path}, "confirm": true}); code != "conflict" || data["domainCode"] != domainCode {
			t.Fatalf("discard %s = %s %v", path, code, data)
		}
	}
	code, data := u.callErr(t, "repos.discard", map[string]any{"paths": []string{"app.txt", "feat.txt"}})
	files, _ := data["files"].([]any)
	if code != "needs_confirmation" || len(files) != 2 || files[0].(map[string]any)["additions"] != float64(1) {
		t.Fatalf("unconfirmed discard = %s %v", code, data)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "feat.txt")); string(b) != "local edit\n" {
		t.Fatal("unconfirmed discard changed the worktree")
	}

	r := decode[struct {
		Discarded []string
		Backup    string
	}](t, u.call(t, "repos.discard", map[string]any{"paths": []string{"app.txt", "feat.txt"}, "confirm": true}))
	if !slices.Equal(r.Discarded, []string{"app.txt", "feat.txt"}) || !strings.HasPrefix(r.Backup, u.fx.Home) || !strings.Contains(r.Backup, "/discarded/api-") {
		t.Fatalf("discard = %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "app.txt")); string(b) != "1\nstaged\n3\n" {
		t.Fatalf("app.txt = %q, want the staged version kept", b)
	}
	if st := runGit(t, wt, "status", "--porcelain=v1"); !strings.Contains(st, "M  app.txt") || !strings.Contains(st, "M  README.md") || !strings.Contains(st, "?? notes.txt") || strings.Contains(st, "feat.txt") {
		t.Fatalf("after discard: %q", st)
	}
	if info, err := os.Stat(r.Backup); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup %s: %v %v", r.Backup, info, err)
	}
	runGit(t, wt, "apply", r.Backup)
	if b, _ := os.ReadFile(filepath.Join(wt, "feat.txt")); string(b) != "local edit\n" {
		t.Fatalf("restored feat.txt = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "app.txt")); string(b) != "1\nstaged\n3\nunstaged\n" {
		t.Fatalf("restored app.txt = %q", b)
	}
}

// TestRepoWrite_CommitHappyPathNothingStagedIdentityAndHook commits with
// the repo's own identity, and refuses without staged changes, without an
// identity (the user's git config is never changed) and when a pre-commit
// hook fails.
func TestRepoWrite_CommitHappyPathNothingStagedIdentityAndHook(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	writeFile(t, wt, "feat.txt", "edited\n")

	r := decode[commitResult](t, u.call(t, "repos.commitChanges", map[string]any{"message": "feat: x"}))
	if r.Error == nil || r.Error.Data["domainCode"] != "nothing_staged" {
		t.Fatalf("nothing staged = %+v", r)
	}

	runGit(t, wt, "add", "feat.txt")
	before := runGit(t, wt, "rev-parse", "HEAD")
	r = decode[commitResult](t, u.call(t, "repos.commitChanges", map[string]any{"message": "feat: edit feat.txt\n\nWith a body."}))
	if r.Error != nil || r.Commit == nil || r.Commit.Subject != "feat: edit feat.txt" {
		t.Fatalf("commit = %+v", r)
	}
	if got := runGit(t, wt, "log", "-1", "--format=%H|%an|%ae|%b"); got != r.Commit.Hash+"|wspace e2e|e2e@example.invalid|With a body." || runGit(t, wt, "rev-parse", "HEAD~1") != before {
		t.Fatalf("HEAD = %q", got)
	}

	// A failing pre-commit hook (in the main clone's hooks, shared by
	// its worktrees) refuses the commit and reports its output.
	hook := filepath.Join(u.api.Main, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint: 1 problem' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, wt, "feat.txt", "edited again\n")
	runGit(t, wt, "add", "feat.txt")
	head := runGit(t, wt, "rev-parse", "HEAD")
	r = decode[commitResult](t, u.call(t, "repos.commitChanges", map[string]any{"message": "x"}))
	if r.Error == nil || r.Error.Data["domainCode"] != "hook_failed" || !strings.Contains(r.Error.Data["output"].(string), "lint: 1 problem") || runGit(t, wt, "rev-parse", "HEAD") != head {
		t.Fatalf("hook failure = %+v", r)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}

	// No identity: the repo's local identity removed, auto-detection off,
	// and the engine runs with the fixture's empty HOME (no global config).
	runGit(t, u.api.Main, "config", "--unset", "user.name")
	runGit(t, u.api.Main, "config", "--unset", "user.email")
	runGit(t, u.api.Main, "config", "user.useConfigOnly", "true")
	r = decode[commitResult](t, u.call(t, "repos.commitChanges", map[string]any{"message": "x"}))
	cmds, _ := r.Error.Data["commands"].([]any)
	if r.Error == nil || r.Error.Data["domainCode"] != "identity_missing" || len(cmds) != 2 || runGit(t, wt, "rev-parse", "HEAD") != head {
		t.Fatalf("identity missing = %+v", r)
	}
	if out := runGit(t, u.api.Main, "config", "--local", "--get-regexp", "^user\\."); out != "user.useconfigonly true" {
		t.Fatalf("the engine changed the identity config: %q", out)
	}
	if _, err := os.Stat(filepath.Join(u.fx.Home, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("a global gitconfig was written: %v", err)
	}
}

// TestRepoWrite_PushPublishesAndIsRejectedWithoutForce publishes the
// workspace branch (it tracks origin/develop, so a plain push is refused
// rather than landing on the base), pushes a commit, and is rejected,
// never forced, when the remote moved on.
func TestRepoWrite_PushPublishesAndIsRejectedWithoutForce(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	developBefore := runGit(t, u.api.Remote, "rev-parse", "develop")

	p := decode[pushResult](t, u.call(t, "repos.push", nil))
	if p.Error == nil || p.Error.Data["domainCode"] != "no_upstream" || p.Error.Data["remote"] != "origin" {
		t.Fatalf("push without an own upstream = %+v", p)
	}
	if runGit(t, u.api.Remote, "rev-parse", "develop") != developBefore {
		t.Fatal("the refused push moved the remote's develop")
	}

	p = decode[pushResult](t, u.call(t, "repos.push", map[string]any{"setUpstream": true}))
	if p.Error != nil || !p.SetUpstream || p.Upstream != "origin/feat" || p.Branch != "feat" {
		t.Fatalf("publish = %+v", p)
	}
	if runGit(t, u.api.Remote, "rev-parse", "feat") != runGit(t, wt, "rev-parse", "HEAD") || runGit(t, wt, "rev-parse", "--abbrev-ref", "@{upstream}") != "origin/feat" {
		t.Fatal("publish did not push feat or set its upstream")
	}
	if runGit(t, u.api.Remote, "rev-parse", "develop") != developBefore {
		t.Fatal("publishing moved the remote's develop")
	}

	p = decode[pushResult](t, u.call(t, "repos.push", nil))
	if p.Error != nil || !p.UpToDate {
		t.Fatalf("push with nothing new = %+v", p)
	}
	commitFile(t, wt, "more.txt", "more\n")
	p = decode[pushResult](t, u.call(t, "repos.push", nil))
	if p.Error != nil || p.Pushed != 1 || runGit(t, u.api.Remote, "rev-parse", "feat") != runGit(t, wt, "rev-parse", "HEAD") {
		t.Fatalf("push = %+v", p)
	}

	// Someone else pushes to feat; a local commit is now rejected.
	other := filepath.Join(u.fx.Root, "other")
	runGit(t, u.fx.Root, "clone", "-q", "-b", "feat", u.api.Remote, other)
	runGit(t, other, "config", "user.email", "e2e@example.invalid")
	runGit(t, other, "config", "user.name", "wspace e2e")
	commitFile(t, other, "theirs.txt", "theirs\n")
	runGit(t, other, "push", "-q", "origin", "feat")
	theirs := runGit(t, other, "rev-parse", "HEAD")
	commitFile(t, wt, "mine.txt", "mine\n")
	p = decode[pushResult](t, u.call(t, "repos.push", nil))
	if p.Error == nil || p.Error.Data["domainCode"] != "push_rejected" || !strings.Contains(p.Error.Data["output"].(string), "[rejected]") {
		t.Fatalf("non-fast-forward push = %+v", p)
	}
	if runGit(t, u.api.Remote, "rev-parse", "feat") != theirs {
		t.Fatal("a rejected push overwrote the remote branch")
	}
}

// TestRepoWrite_StashCreateWithAndWithoutUntracked saves tracked changes
// only, then everything with includeUntracked, and refuses when there is
// nothing to save.
func TestRepoWrite_StashCreateWithAndWithoutUntracked(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	writeFile(t, wt, "feat.txt", "edited\n")
	writeFile(t, wt, "notes.txt", "untracked\n")

	s := decode[struct {
		Stash struct{ Ref, Hash, Message string }
	}](t, u.call(t, "repos.stashCreate", map[string]any{"message": "tracked only"}))
	if s.Stash.Ref != "stash@{0}" || s.Stash.Message != "tracked only" || s.Stash.Hash != stashTop(t, wt) {
		t.Fatalf("stash = %+v", s)
	}
	if st := runGit(t, wt, "status", "--porcelain=v1"); st != "?? notes.txt" {
		t.Fatalf("after stash: %q", st)
	}
	if code, data := u.callErr(t, "repos.stashCreate", nil); code != "conflict" || data["domainCode"] != "nothing_to_stash" {
		t.Fatalf("only untracked without includeUntracked = %s %v", code, data)
	}
	u.call(t, "repos.stashCreate", map[string]any{"message": "with untracked", "includeUntracked": true})
	if st := runGit(t, wt, "status", "--porcelain=v1"); st != "" || stashCount(t, wt) != 2 {
		t.Fatalf("after stash -u: %q, %d entries", st, stashCount(t, wt))
	}
	if code, _ := u.callErr(t, "repos.stashCreate", map[string]any{"includeUntracked": true}); code != "conflict" {
		t.Fatalf("clean worktree = %s", code)
	}
}

// TestRepoWrite_CLI drives the same actions through `wspace repo`.
func TestRepoWrite_CLI(t *testing.T) {
	u := newUpdateFixture(t)
	wt := u.worktree
	writeFile(t, wt, "feat.txt", "edited\n")
	u.fx.MustRun("", "repo", "feat", "api", "stage", "feat.txt")
	out, _ := u.fx.MustRun("", "repo", "feat", "api", "commit", "-m", "feat: from the cli")
	if !strings.Contains(out, "api: committed") || runGit(t, wt, "log", "-1", "--format=%s") != "feat: from the cli" {
		t.Fatalf("commit: %q", out)
	}
	if out, _, code := u.fx.Run("", "repo", "feat", "api", "push"); code == 0 || !strings.Contains(out, "--set-upstream") {
		t.Fatalf("push without upstream (%d): %q", code, out)
	}
	out, _ = u.fx.MustRun("", "repo", "feat", "api", "push", "--set-upstream")
	if !strings.Contains(out, "published feat to origin") {
		t.Fatalf("publish: %q", out)
	}
	writeFile(t, wt, "feat.txt", "edited twice\n")
	out, _ = u.fx.MustRun("", "repo", "feat", "api", "discard", "feat.txt", "--yes")
	if !strings.Contains(out, "backup patch:") {
		t.Fatalf("discard: %q", out)
	}
	writeFile(t, wt, "notes.txt", "x\n")
	out, _ = u.fx.MustRun("", "repo", "feat", "api", "stash", "push", "-m", "cli stash", "-u")
	if !strings.Contains(out, "saved stash@{0} (cli stash)") {
		t.Fatalf("stash push: %q", out)
	}
}
