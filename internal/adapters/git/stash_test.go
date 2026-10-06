// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

func stashHash(t *testing.T, repo domain.Path, index string) string {
	t.Helper()
	return gitfix.Git(t, repo, "rev-parse", "stash@{"+index+"}")
}

func TestGitAdapter_StashApplyRestoresIndexAndKeepsTheEntry(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Commit(t, repo, "app.go", "one\n")
	write(t, repo, "app.go", "two\n")
	gitfix.Git(t, repo, "add", "app.go")
	write(t, repo, "new file.txt", "u\n")
	gitfix.Git(t, repo, "stash", "push", "-q", "-u")
	hash := stashHash(t, repo, "0")

	untracked, err := a.StashUntrackedFiles(ctx, repo, hash)
	if err != nil || !slices.Equal(untracked, []string{"new file.txt"}) {
		t.Fatalf("StashUntrackedFiles = %q, %v", untracked, err)
	}
	res, err := a.StashApply(ctx, repo, hash, true)
	if err != nil || res.Outcome != ports.StashApplied {
		t.Fatalf("StashApply = %+v, %v", res, err)
	}
	entries, _ := a.Status(ctx, repo)
	cs := domain.SplitChanges(entries)
	if len(cs.Staged) != 1 || len(cs.Untracked) != 1 {
		t.Fatalf("status = %+v, want the staged change and the untracked file back", cs)
	}
	if list, _ := a.StashList(ctx, repo); len(list) != 1 {
		t.Fatalf("stash list = %+v, want the entry kept by apply", list)
	}
}

func TestGitAdapter_StashUntrackedFilesIsEmptyWithoutAThirdParent(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Commit(t, repo, "app.go", "one\n")
	write(t, repo, "app.go", "two\n")
	gitfix.Git(t, repo, "stash", "push", "-q")

	files, err := a.StashUntrackedFiles(ctx, repo, stashHash(t, repo, "0"))
	if err != nil || len(files) != 0 {
		t.Fatalf("StashUntrackedFiles = %q, %v; want none", files, err)
	}
}

func TestGitAdapter_StashApplyRefusals(t *testing.T) {
	gitfix.RequireGit(t)

	t.Run("local changes git would overwrite change nothing", func(t *testing.T) {
		repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
		a, ctx := newAdapter(t), context.Background()
		gitfix.Commit(t, repo, "app.go", "one\n")
		write(t, repo, "app.go", "stashed\n")
		gitfix.Git(t, repo, "stash", "push", "-q")
		write(t, repo, "app.go", "local\n")

		res, err := a.StashApply(ctx, repo, stashHash(t, repo, "0"), true)
		if err != nil || res.Outcome != ports.StashOverwriteRefused || !slices.Equal(res.Files, []string{"app.go"}) {
			t.Fatalf("StashApply = %+v, %v", res, err)
		}
		if b, _ := os.ReadFile(filepath.Join(string(repo), "app.go")); string(b) != "local\n" {
			t.Fatalf("app.go = %q, want the local edit untouched", b)
		}
	})

	t.Run("a stashed index that no longer applies is refused with nothing changed", func(t *testing.T) {
		repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
		a, ctx := newAdapter(t), context.Background()
		gitfix.Commit(t, repo, "app.go", "1\n2\n3\n")
		write(t, repo, "app.go", "1\nB\n3\n")
		gitfix.Git(t, repo, "add", "app.go")
		gitfix.Git(t, repo, "stash", "push", "-q")
		gitfix.Commit(t, repo, "app.go", "1\nD\n3\n")
		hash := stashHash(t, repo, "0")

		res, err := a.StashApply(ctx, repo, hash, true)
		if err != nil || res.Outcome != ports.StashIndexRefused {
			t.Fatalf("StashApply(--index) = %+v, %v", res, err)
		}
		if entries, _ := a.Status(ctx, repo); len(entries) != 0 {
			t.Fatalf("status = %+v, want a clean worktree after the refusal", entries)
		}
		res, err = a.StashApply(ctx, repo, hash, false)
		if err != nil || res.Outcome != ports.StashStopped {
			t.Fatalf("StashApply(no index) = %+v, %v; want stopped on the conflict", res, err)
		}
		if paths, _ := a.ConflictedPaths(ctx, repo); !slices.Equal(paths, []string{"app.go"}) {
			t.Fatalf("conflicts = %q", paths)
		}
	})
}

func TestGitAdapter_StashDropRemovesOnlyThatEntry(t *testing.T) {
	gitfix.RequireGit(t)
	repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
	a, ctx := newAdapter(t), context.Background()
	gitfix.Commit(t, repo, "app.go", "one\n")
	write(t, repo, "app.go", "first\n")
	gitfix.Git(t, repo, "stash", "push", "-q", "-m", "first")
	write(t, repo, "app.go", "second\n")
	gitfix.Git(t, repo, "stash", "push", "-q", "-m", "second")

	if err := a.StashDrop(ctx, repo, 1); err != nil {
		t.Fatalf("StashDrop: %v", err)
	}
	list, _ := a.StashList(ctx, repo)
	if len(list) != 1 || list[0].Message != "second" {
		t.Fatalf("stash list = %+v, want only the second entry", list)
	}
	if err := a.StashDrop(ctx, repo, -1); err == nil {
		t.Fatal("StashDrop(-1) = nil, want an error")
	}
}

func TestGitAdapter_CleanUntrackedRemovesOnlyTheNamedFiles(t *testing.T) {
	// Each name is also a pathspec glob: only the file literally named is
	// removed. "*" cannot appear in a Windows file name; "[k]" can.
	for _, odd := range []string{"a *.txt", "a [k].txt"} {
		t.Run(odd, func(t *testing.T) {
			gitfix.RequireGit(t)
			gitfix.RequireValidFileName(t, odd)
			repo := gitfix.NewClone(t, gitfix.NewOrigin(t))
			a, ctx := newAdapter(t), context.Background()
			gitfix.Commit(t, repo, ".gitignore", "ignored.log\n")
			write(t, repo, odd, "x\n")
			write(t, repo, "a k.txt", "x\n")
			write(t, repo, "keep.txt", "x\n")
			write(t, repo, "ignored.log", "x\n")

			if err := a.CleanUntracked(ctx, repo, []string{odd, "ignored.log"}); err != nil {
				t.Fatalf("CleanUntracked: %v", err)
			}
			for name, want := range map[string]bool{odd: false, "a k.txt": true, "keep.txt": true, "ignored.log": true} {
				_, err := os.Lstat(filepath.Join(string(repo), name))
				if (err == nil) != want {
					t.Errorf("%s exists = %v, want %v", name, err == nil, want)
				}
			}
			if err := a.CleanUntracked(ctx, repo, []string{"../escape"}); err == nil {
				t.Fatal("CleanUntracked(../escape) = nil, want a refusal")
			}
		})
	}
}
