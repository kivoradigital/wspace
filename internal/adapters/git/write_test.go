// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// isolateHome points HOME (the only config location the adapter's
// environment allowlist carries over) at an empty directory, so a
// developer's global gitconfig (identity, hooksPath, gpg signing) can
// never influence a write test.
func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func changes(t *testing.T, a interface {
	Status(context.Context, domain.Path) ([]domain.PorcelainEntry, error)
}, repo domain.Path) domain.ChangeSets {
	t.Helper()
	entries, err := a.Status(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return domain.SplitChanges(entries)
}

func paths(entries []domain.ChangeEntry) []string {
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func TestGitAdapter_StageAndUnstageModifiedDeletedAndUntrackedFiles(t *testing.T) {
	gitfix.RequireGit(t)
	isolateHome(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Commit(t, repo, "gone.txt", "bye\n")
	write(t, repo, "README.md", "edited\n")
	if err := os.Remove(filepath.Join(string(repo), "gone.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, repo, "new file.txt", "new\n")
	write(t, repo, "*.txt", "a file named like a glob\n")

	if err := a.Stage(ctx, repo, []string{"README.md", "gone.txt", "new file.txt", "*.txt"}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	cs := changes(t, a, repo)
	if got := paths(cs.Staged); !slices.Equal(got, []string{"*.txt", "README.md", "gone.txt", "new file.txt"}) || len(cs.Unstaged)+len(cs.Untracked) != 0 {
		t.Fatalf("after stage: %+v", cs)
	}

	if err := a.Unstage(ctx, repo, []string{"README.md", "gone.txt", "new file.txt"}, false); err != nil {
		t.Fatalf("Unstage: %v", err)
	}
	cs = changes(t, a, repo)
	if !slices.Equal(paths(cs.Staged), []string{"*.txt"}) || !slices.Equal(paths(cs.Unstaged), []string{"README.md", "gone.txt"}) ||
		!slices.Equal(paths(cs.Untracked), []string{"new file.txt"}) {
		t.Fatalf("after unstage: %+v", cs)
	}
	if b, _ := os.ReadFile(filepath.Join(string(repo), "README.md")); string(b) != "edited\n" {
		t.Fatalf("unstage touched the worktree: %q", b)
	}
}

func TestGitAdapter_UnstageARenameNeedsBothPaths(t *testing.T) {
	gitfix.RequireGit(t)
	isolateHome(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Commit(t, repo, "old.txt", strings.Repeat("same\n", 10))
	gitfix.Git(t, repo, "mv", "old.txt", "new.txt")

	if err := a.Unstage(ctx, repo, []string{"new.txt", "old.txt"}, false); err != nil {
		t.Fatalf("Unstage: %v", err)
	}
	cs := changes(t, a, repo)
	if len(cs.Staged) != 0 || !slices.Equal(paths(cs.Unstaged), []string{"old.txt"}) || !slices.Equal(paths(cs.Untracked), []string{"new.txt"}) {
		t.Fatalf("after unstaging a rename: %+v", cs)
	}
}

func TestGitAdapter_UnstageInARepoWithoutCommits(t *testing.T) {
	gitfix.RequireGit(t)
	isolateHome(t)
	repo := domain.Path(filepath.ToSlash(t.TempDir()))
	gitfix.Git(t, repo, "init", "-q")
	a, ctx := newAdapter(t), context.Background()
	write(t, repo, "first.txt", "1\n")
	if err := a.Stage(ctx, repo, []string{"first.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Unstage(ctx, repo, []string{"first.txt"}, true); err != nil {
		t.Fatalf("Unstage unborn: %v", err)
	}
	if cs := changes(t, a, repo); !slices.Equal(paths(cs.Untracked), []string{"first.txt"}) {
		t.Fatalf("after unstage: %+v", cs)
	}
}

func TestGitAdapter_UnstagedPatchThenRestoreWorktreeKeepsTheIndex(t *testing.T) {
	gitfix.RequireGit(t)
	isolateHome(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Commit(t, repo, "app.txt", "1\n2\n3\n")
	write(t, repo, "app.txt", "1\nstaged\n3\n")
	gitfix.Git(t, repo, "add", "app.txt")
	write(t, repo, "app.txt", "1\nstaged\n3\nunstaged\n")
	if err := os.Remove(filepath.Join(string(repo), "README.md")); err != nil {
		t.Fatal(err)
	}

	patch, err := a.UnstagedPatch(ctx, repo, []string{"app.txt", "README.md"})
	if err != nil || !strings.Contains(patch, "+unstaged") || strings.Contains(patch, "+staged") || !strings.Contains(patch, "deleted file mode") {
		t.Fatalf("UnstagedPatch = %q, %v", patch, err)
	}
	if err := a.RestoreWorktree(ctx, repo, []string{"app.txt", "README.md"}); err != nil {
		t.Fatalf("RestoreWorktree: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(string(repo), "app.txt")); string(b) != "1\nstaged\n3\n" {
		t.Fatalf("app.txt = %q, want the staged (index) version", b)
	}
	cs := changes(t, a, repo)
	if !slices.Equal(paths(cs.Staged), []string{"app.txt"}) || len(cs.Unstaged) != 0 {
		t.Fatalf("after restore: %+v", cs)
	}

	// The backup patch brings the discarded changes back.
	p := filepath.Join(t.TempDir(), "backup.patch")
	if err := os.WriteFile(p, []byte(patch), 0o600); err != nil {
		t.Fatal(err)
	}
	gitfix.Git(t, repo, "apply", p)
	if b, _ := os.ReadFile(filepath.Join(string(repo), "app.txt")); string(b) != "1\nstaged\n3\nunstaged\n" {
		t.Fatalf("re-applied app.txt = %q", b)
	}
}

func TestGitAdapter_CommitCreatesACommitWithTheRepositoryIdentity(t *testing.T) {
	gitfix.RequireGit(t)
	isolateHome(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	write(t, repo, "README.md", "edited\n")
	gitfix.Git(t, repo, "add", "README.md")

	if ok, err := a.IdentityConfigured(ctx, repo); err != nil || !ok {
		t.Fatalf("IdentityConfigured = %v, %v", ok, err)
	}
	res, err := a.Commit(ctx, repo, "-not an option\n\nbody line\n# kept, not a comment")
	if err != nil || res.Outcome != ports.CommitCreated {
		t.Fatalf("Commit = %+v, %v", res, err)
	}
	if got := gitfix.Git(t, repo, "log", "-1", "--format=%s|%an|%b"); got != "-not an option|ws-test|body line\n# kept, not a comment" {
		t.Fatalf("commit = %q", got)
	}
}

func TestGitAdapter_CommitWithoutIdentityIsReported(t *testing.T) {
	gitfix.RequireGit(t)
	isolateHome(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	gitfix.Git(t, repo, "config", "--unset", "user.name")
	gitfix.Git(t, repo, "config", "--unset", "user.email")
	gitfix.Git(t, repo, "config", "user.useConfigOnly", "true")
	a, ctx := newAdapter(t), context.Background()
	write(t, repo, "README.md", "edited\n")
	gitfix.Git(t, repo, "add", "README.md")

	if ok, err := a.IdentityConfigured(ctx, repo); err != nil || ok {
		t.Fatalf("IdentityConfigured = %v, %v; want false", ok, err)
	}
	res, err := a.Commit(ctx, repo, "msg")
	if err != nil || res.Outcome != ports.CommitIdentityMissing {
		t.Fatalf("Commit = %+v, %v", res, err)
	}
}

func TestGitAdapter_CommitRefusedByAHookReportsItsOutput(t *testing.T) {
	gitfix.RequireGit(t)
	isolateHome(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	hooks := t.TempDir()
	gitfix.Git(t, repo, "config", "core.hooksPath", hooks)
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho 'lint: 2 problems' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, ctx := newAdapter(t), context.Background()
	write(t, repo, "README.md", "edited\n")
	gitfix.Git(t, repo, "add", "README.md")
	before := gitfix.Git(t, repo, "rev-parse", "HEAD")

	res, err := a.Commit(ctx, repo, "msg")
	if err != nil || res.Outcome != ports.CommitHookFailed || !strings.Contains(res.Output, "lint: 2 problems") {
		t.Fatalf("Commit = %+v, %v", res, err)
	}
	if after := gitfix.Git(t, repo, "rev-parse", "HEAD"); after != before {
		t.Fatal("a refused commit moved HEAD")
	}
}

func TestGitAdapter_PushPublishesRejectsAndNeverForces(t *testing.T) {
	gitfix.RequireGit(t)
	isolateHome(t)
	origin := gitfix.NewOrigin(t)
	repo := gitfix.NewClone(t, origin)
	other := gitfix.NewClone(t, origin)
	a, ctx := newAdapter(t), context.Background()

	gitfix.Git(t, repo, "switch", "-q", "-c", "feat")
	gitfix.Commit(t, repo, "feat.txt", "1\n")
	res, err := a.Push(ctx, repo, ports.PushSpec{Remote: "origin", Refspec: "refs/heads/feat:refs/heads/feat", SetUpstream: true})
	if err != nil || res.Outcome != ports.PushDone {
		t.Fatalf("publish = %+v, %v", res, err)
	}
	up, ok, err := a.Upstream(ctx, repo)
	if err != nil || !ok || up.Ref != "origin/feat" || up.RemoteRef != "refs/heads/feat" {
		t.Fatalf("upstream after publish = %+v, %v, %v", up, ok, err)
	}

	gitfix.Commit(t, other, "main.txt", "theirs\n")
	gitfix.Push(t, other, "main")
	gitfix.Git(t, repo, "switch", "-q", "main")
	gitfix.Commit(t, repo, "main.txt", "mine\n")
	res, err = a.Push(ctx, repo, ports.PushSpec{Remote: "origin", Refspec: "refs/heads/main:refs/heads/main"})
	if err != nil || res.Outcome != ports.PushRejected || !strings.Contains(res.Output, "[rejected]") {
		t.Fatalf("non-fast-forward push = %+v, %v", res, err)
	}
	if got := gitfix.Git(t, origin, "log", "-1", "--format=%s", "main"); got != "test commit: main.txt" || gitfix.Git(t, origin, "show", "main:main.txt") != "theirs" {
		t.Fatalf("origin main was overwritten: %q", got)
	}

	res, err = a.Push(ctx, repo, ports.PushSpec{Remote: "no-such-remote", Refspec: "refs/heads/main:refs/heads/main"})
	if err != nil || res.Outcome != ports.PushAuthFailed || !strings.Contains(res.Output, "Could not read from remote repository") {
		t.Fatalf("unreachable remote = %+v, %v", res, err)
	}
	for _, bad := range []ports.PushSpec{{Remote: "--force", Refspec: "refs/heads/main:refs/heads/main"}, {Remote: "origin", Refspec: "+refs/heads/main:refs/heads/main"}, {Remote: "origin", Refspec: "--force"}} {
		if _, err := a.Push(ctx, repo, bad); err == nil {
			t.Fatalf("push %+v accepted", bad)
		}
	}
}

func TestGitAdapter_StashPushWithAndWithoutUntrackedFiles(t *testing.T) {
	gitfix.RequireGit(t)
	isolateHome(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	write(t, repo, "README.md", "edited\n")
	gitfix.Git(t, repo, "add", "README.md")
	write(t, repo, "notes.txt", "untracked\n")

	if err := a.StashPush(ctx, repo, ports.StashPushSpec{Message: "keep index", KeepIndex: true}); err != nil {
		t.Fatalf("StashPush keep-index: %v", err)
	}
	cs := changes(t, a, repo)
	if !slices.Equal(paths(cs.Staged), []string{"README.md"}) || !slices.Equal(paths(cs.Untracked), []string{"notes.txt"}) {
		t.Fatalf("after --keep-index: %+v", cs)
	}
	if err := a.StashPush(ctx, repo, ports.StashPushSpec{Message: "with untracked", IncludeUntracked: true}); err != nil {
		t.Fatalf("StashPush -u: %v", err)
	}
	if cs := changes(t, a, repo); len(cs.Staged)+len(cs.Unstaged)+len(cs.Untracked) != 0 {
		t.Fatalf("after -u: %+v", cs)
	}
	list, _ := a.StashList(ctx, repo)
	if len(list) != 2 || list[0].Message != "with untracked" || list[1].Message != "keep index" {
		t.Fatalf("stash list = %+v", list)
	}
}
