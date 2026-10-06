// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/adapters/git"
	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

func newAdapter(t *testing.T) *git.Adapter {
	t.Helper()
	a, err := git.New()
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func write(t *testing.T, repo domain.Path, rel, body string) {
	t.Helper()
	full := filepath.Join(string(repo), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitAdapter_DiffStagedUnstagedUntrackedAndBinary(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Commit(t, repo, "app.go", "one\ntwo\n")
	write(t, repo, "app.go", "one\nTWO\n")
	gitfix.Git(t, repo, "add", "app.go")
	write(t, repo, "app.go", "one\nTWO\nthree\n")
	write(t, repo, "new file.txt", "hello\n")
	write(t, repo, "blob.bin", "\x00\x01\x02binary")

	staged, err := a.Diff(ctx, repo, ports.DiffSpec{Path: "app.go", Staged: true, Limits: domain.DefaultDiffLimits})
	if err != nil || len(staged.Files) != 1 || staged.Files[0].Additions != 1 || staged.Files[0].Deletions != 1 {
		t.Fatalf("staged = %+v, %v", staged, err)
	}
	unstaged, err := a.Diff(ctx, repo, ports.DiffSpec{Path: "app.go", Limits: domain.DefaultDiffLimits})
	if err != nil || len(unstaged.Files) != 1 || unstaged.Files[0].Additions != 1 || unstaged.Files[0].Deletions != 0 {
		t.Fatalf("unstaged = %+v, %v", unstaged, err)
	}
	untracked, err := a.Diff(ctx, repo, ports.DiffSpec{Path: "new file.txt", Untracked: true, Limits: domain.DefaultDiffLimits})
	if err != nil || len(untracked.Files) != 1 || untracked.Files[0].Path != "new file.txt" ||
		untracked.Files[0].Status != domain.FileAdded || untracked.Files[0].Hunks[0].Lines[0] != "+hello" {
		t.Fatalf("untracked = %+v, %v", untracked, err)
	}
	bin, err := a.Diff(ctx, repo, ports.DiffSpec{Path: "blob.bin", Untracked: true, Limits: domain.DefaultDiffLimits})
	if err != nil || len(bin.Files) != 1 || !bin.Files[0].Binary {
		t.Fatalf("binary = %+v, %v", bin, err)
	}
}

func TestGitAdapter_DiffStagedRenameUsesBothPaths(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Commit(t, repo, "old.txt", strings.Repeat("line\n", 20))
	gitfix.Git(t, repo, "mv", "old.txt", "new.txt")

	p, err := a.Diff(ctx, repo, ports.DiffSpec{Path: "new.txt", OrigPath: "old.txt", Staged: true, Limits: domain.DefaultDiffLimits})
	if err != nil || len(p.Files) != 1 || p.Files[0].Status != domain.FileRenamed || p.Files[0].OrigPath != "old.txt" {
		t.Fatalf("rename = %+v, %v", p, err)
	}
}

func TestGitAdapter_DiffByteCapTruncates(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a := newAdapter(t)
	write(t, repo, "big.txt", strings.Repeat("0123456789\n", 50000))

	p, err := a.Diff(context.Background(), repo, ports.DiffSpec{Path: "big.txt", Untracked: true, Limits: domain.DiffLimits{MaxBytes: 4096, MaxLines: 100000}})
	if err != nil || !p.Truncated || len(p.Files) != 1 || !p.Files[0].Truncated {
		t.Fatalf("patch truncated=%v files=%d err=%v", p.Truncated, len(p.Files), err)
	}
	if n := len(p.Files[0].Hunks[0].Lines); n == 0 || n > 400 {
		t.Fatalf("kept %d lines", n)
	}
}

func TestGitAdapter_CommitLogCountAndDetailNeverExposeEmails(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Git(t, repo, "checkout", "-q", "-b", "feat")
	gitfix.Commit(t, repo, "a.txt", "a\n")
	gitfix.Commit(t, repo, "b.txt", "b\n")

	n, err := a.CountCommits(ctx, repo, "origin/main..HEAD")
	if err != nil || n != 2 {
		t.Fatalf("count = %d, %v", n, err)
	}
	log, err := a.CommitLog(ctx, repo, "origin/main..HEAD", 0, 10)
	if err != nil || len(log) != 2 || log[0].Subject != "test commit: b.txt" || log[0].AuthorName != "ws-test" || log[0].AuthorDate.IsZero() {
		t.Fatalf("log = %+v, %v", log, err)
	}
	page, err := a.CommitLog(ctx, repo, "origin/main..HEAD", 1, 10)
	if err != nil || len(page) != 1 || page[0].Hash != log[1].Hash {
		t.Fatalf("page = %+v, %v", page, err)
	}

	d, err := a.CommitDetail(ctx, repo, log[0].ShortHash, domain.DefaultDiffLimits)
	if err != nil || d.Hash != log[0].Hash || len(d.Parents) != 1 || len(d.Patch.Files) != 1 || d.Patch.Files[0].Path != "b.txt" {
		t.Fatalf("detail = %+v, %v", d, err)
	}
	if strings.Contains(d.Body, "@") || strings.Contains(d.AuthorName, "@") {
		t.Fatalf("detail exposes an e-mail: %+v", d)
	}
	if _, err := a.CommitDetail(ctx, repo, "deadbeef", domain.DefaultDiffLimits); domain.Code(err) != domain.CodeRefNotFound {
		t.Fatalf("unknown hash err = %v", err)
	}
	if _, err := a.CommitDetail(ctx, repo, "--all", domain.DefaultDiffLimits); domain.Code(err) != domain.CodeRefNotFound {
		t.Fatalf("option-like hash err = %v", err)
	}
}

func TestGitAdapter_CommitDetailOfARootCommit(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a := newAdapter(t)
	root := gitfix.Git(t, repo, "rev-list", "--max-parents=0", "HEAD")

	d, err := a.CommitDetail(context.Background(), repo, root, domain.DefaultDiffLimits)
	if err != nil || len(d.Parents) != 0 || len(d.Patch.Files) != 1 || d.Patch.Files[0].Status != domain.FileAdded {
		t.Fatalf("root detail = %+v, %v", d, err)
	}
}

func TestGitAdapter_StashListAndShowIncludeUntracked(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Dirty(t, repo, "README.md", "changed\n")
	write(t, repo, "scratch.txt", "wip\n")
	gitfix.Git(t, repo, "stash", "push", "-q", "-u", "-m", "half done")

	list, err := a.StashList(ctx, repo)
	if err != nil || len(list) != 1 || list[0].Index != 0 || list[0].Branch != "main" || list[0].Message != "half done" || list[0].Date.IsZero() {
		t.Fatalf("stashes = %+v, %v", list, err)
	}
	p, err := a.StashShow(ctx, repo, 0, true, domain.DefaultDiffLimits)
	if err != nil || len(p.Files) != 2 {
		t.Fatalf("stash show = %+v, %v", p, err)
	}
	paths := []string{p.Files[0].Path, p.Files[1].Path}
	if strings.Join(paths, ",") != "README.md,scratch.txt" {
		t.Fatalf("paths = %v", paths)
	}
	tracked, err := a.StashShow(ctx, repo, 0, false, domain.DefaultDiffLimits)
	if err != nil || len(tracked.Files) != 1 {
		t.Fatalf("tracked only = %+v, %v", tracked, err)
	}
}

func TestGitAdapter_UpstreamAndLastFetch(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()

	u, ok, err := a.Upstream(ctx, repo)
	if err != nil || !ok || u.Ref != "origin/main" || u.Remote != "origin" || u.Gone {
		t.Fatalf("upstream = %+v %v %v", u, ok, err)
	}
	gitfix.Git(t, repo, "checkout", "-q", "-b", "local-only")
	if _, ok, err := a.Upstream(ctx, repo); ok || err != nil {
		t.Fatalf("local-only branch upstream ok=%v err=%v", ok, err)
	}

	before := time.Now().Add(-time.Minute)
	if err := a.Fetch(ctx, repo, "origin"); err != nil {
		t.Fatal(err)
	}
	at, ok, err := a.LastFetch(ctx, repo)
	if err != nil || !ok || at.Before(before) {
		t.Fatalf("last fetch = %v %v %v", at, ok, err)
	}
}

func TestGitAdapter_MergeFastForwardRefusals(t *testing.T) {
	u := newUpdateRepo(t) // repo on feat (one local commit), origin/main advanced
	ctx := context.Background()
	gitfix.Git(t, u.repo, "checkout", "-q", "main")

	// a tracked change the fast-forward would overwrite
	gitfix.Commit(t, u.other, "README.md", "remote\n")
	gitfix.Push(t, u.other, "main")
	if err := u.a.Fetch(ctx, u.repo, "origin"); err != nil {
		t.Fatal(err)
	}
	gitfix.Dirty(t, u.repo, "README.md", "local\n")
	head := gitfix.Git(t, u.repo, "rev-parse", "HEAD")
	files, err := u.a.MergeFastForward(ctx, u.repo, "origin/main")
	if domain.Code(err) != domain.CodeWorktreeDirty || len(files) != 1 || files[0] != "README.md" {
		t.Fatalf("dirty ff = %v, %v", files, err)
	}
	gitfix.Git(t, u.repo, "checkout", "--", "README.md")

	// diverged
	gitfix.Commit(t, u.repo, "local.txt", "x\n")
	if _, err := u.a.MergeFastForward(ctx, u.repo, "origin/main"); domain.Code(err) != domain.CodeDiverged {
		t.Fatalf("diverged err = %v", err)
	}
	if got := gitfix.Git(t, u.repo, "rev-list", "--parents", "-n", "1", "HEAD"); len(strings.Fields(got)) != 2 {
		t.Fatal("a merge commit was created")
	}
	gitfix.Git(t, u.repo, "reset", "-q", "--hard", head)

	// success
	if _, err := u.a.MergeFastForward(ctx, u.repo, "origin/main"); err != nil {
		t.Fatal(err)
	}
	if gitfix.Git(t, u.repo, "rev-parse", "HEAD") != gitfix.Git(t, u.repo, "rev-parse", "origin/main") {
		t.Fatal("not fast-forwarded")
	}
}
