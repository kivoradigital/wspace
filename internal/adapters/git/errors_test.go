// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/git"
	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TestGitAdapter_IsIgnored_ExitCodeMapping covers check-ignore's three exit
// codes: 0 = ignored, 1 = not ignored, >= 2 = a genuine error (design.md
// §7, tasks.md 2.20).
func TestGitAdapter_IsIgnored_ExitCodeMapping(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)
	gitfix.Commit(t, clone, ".gitignore", "*.log\n")
	gitfix.Untracked(t, clone, "not-ignored.txt")

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}
	ctx := context.Background()

	ignored, err := a.IsIgnored(ctx, clone, "debug.log")
	if err != nil {
		t.Fatalf("IsIgnored(debug.log): %v", err)
	}
	if !ignored {
		t.Fatal("expected debug.log to be reported as ignored (exit 0)")
	}

	ignored, err = a.IsIgnored(ctx, clone, "not-ignored.txt")
	if err != nil {
		t.Fatalf("IsIgnored(not-ignored.txt): %v", err)
	}
	if ignored {
		t.Fatal("expected not-ignored.txt to be reported as not ignored (exit 1)")
	}

	// check-ignore exits >= 2 on invalid usage, e.g. an absolute path
	// outside the repository. Use "--" plus a path escaping the repo root
	// to provoke exit 128.
	_, err = a.IsIgnored(ctx, clone, "../outside-repo")
	if err == nil {
		t.Fatal("expected an error for check-ignore's exit >= 2 case")
	}
	if domain.Code(err) != domain.CodeGitFailed {
		t.Fatalf("expected CodeGitFailed for an unexpected exit code, got %q", domain.Code(err))
	}
}

// TestGitErrors_MapsEachSignal is the named §13 test with one case per
// exit-code/stderr-marker row in the error table: asserts domain.Code(err)
// and that Error() never contains raw stderr (tasks.md 2.22).
func TestGitErrors_MapsEachSignal(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}
	ctx := context.Background()

	t.Run("branch_checked_out", func(t *testing.T) {
		// main is checked out in clone's own working directory; creating
		// another worktree that also attaches to main must fail with
		// CodeBranchCheckedOut.
		target := domain.Path(string(clone) + "-dup")
		err := a.WorktreeAdd(ctx, clone, ports.WorktreeSpec{Target: target, Branch: "main"})
		assertOpError(t, err, domain.CodeBranchCheckedOut)
	})

	t.Run("worktree_exists", func(t *testing.T) {
		target := domain.Path(string(clone) + "-exists")
		if err := a.WorktreeAdd(ctx, clone, ports.WorktreeSpec{
			Target: target, Branch: "feature-exists", StartPoint: "main",
		}); err != nil {
			t.Fatalf("initial WorktreeAdd: %v", err)
		}
		err := a.WorktreeAdd(ctx, clone, ports.WorktreeSpec{
			Target: target, Branch: "feature-exists-2", StartPoint: "main",
		})
		assertOpError(t, err, domain.CodeWorktreeExists)
	})

	t.Run("worktree_dirty", func(t *testing.T) {
		target := domain.Path(string(clone) + "-dirty")
		if err := a.WorktreeAdd(ctx, clone, ports.WorktreeSpec{
			Target: target, Branch: "feature-dirty", StartPoint: "main",
		}); err != nil {
			t.Fatalf("WorktreeAdd: %v", err)
		}
		gitfix.Untracked(t, target, "uncommitted.txt")
		err := a.WorktreeRemove(ctx, clone, target, false)
		assertOpError(t, err, domain.CodeWorktreeDirty)
	})

	t.Run("worktree_missing", func(t *testing.T) {
		err := a.WorktreeRemove(ctx, clone, domain.Path(string(clone)+"-never-existed"), false)
		assertOpError(t, err, domain.CodeWorktreeMissing)
	})

	t.Run("ref_not_found", func(t *testing.T) {
		_, _, err := a.AheadBehind(ctx, clone, "refs/heads/does-not-exist")
		assertOpError(t, err, domain.CodeRefNotFound)
	})

	t.Run("git_failed_fallback", func(t *testing.T) {
		// Deleting a branch that does not exist is a plain non-zero exit
		// with none of the specific markers above.
		err := a.DeleteBranch(ctx, clone, "never-existed", false)
		assertOpError(t, err, domain.CodeGitFailed)
	})

	t.Run("timeout", func(t *testing.T) {
		expired, cancel := context.WithTimeout(ctx, 0)
		defer cancel()
		<-expired.Done()
		err := a.Fetch(expired, clone, "origin")
		assertOpError(t, err, domain.CodeTimeout)
	})
}

func assertOpError(t *testing.T, err error, want domain.ErrCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error mapping to %q, got nil", want)
	}
	if got := domain.Code(err); got != want {
		t.Fatalf("expected ErrCode %q, got %q (err: %v)", want, got, err)
	}
	if containsRawStderrMarker(err.Error()) {
		t.Fatalf("Error() must never contain raw stderr, got: %q", err.Error())
	}
}

// containsRawStderrMarker is a narrow smoke check: git's own stderr prose
// ("fatal:", "error:") must never leak into OpError.Error()'s rendered
// text, which is only "<op>: <code> (<subject>)".
func containsRawStderrMarker(s string) bool {
	for _, marker := range []string{"fatal:", "error: "} {
		if len(s) >= len(marker) {
			for i := 0; i+len(marker) <= len(s); i++ {
				if s[i:i+len(marker)] == marker {
					return true
				}
			}
		}
	}
	return false
}

// TestGitAdapter_CommitAndPushOnlyInTheWriteActions is the §13
// threat-matrix guard, narrowed when the repository inspector gained its
// explicit, user-driven write actions: the quoted git subcommands
// "commit", "push" and "stage" may appear only in write.go, and no
// write action may ever pass a flag that forces a push, skips hooks or
// rewrites a commit (worktree.go's `worktree remove --force` is a
// separate, confirmed teardown path). It scans this package's production source (excluding
// gitfix, which is test infrastructure), so a future change is caught
// before it can run.
func TestGitAdapter_CommitAndPushOnlyInTheWriteActions(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine this file's own path")
	}
	dir := filepath.Dir(thisFile)

	writeOnly := []string{`"commit"`, `"push"`, `"stage"`}
	never := []string{`"--force"`, `"-f", "push"`, `"--force-with-lease"`, `"--force-if-includes"`, `"--no-verify"`, `"-n", "commit"`, `"--amend"`, `"--mirror"`, `"--delete"`}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("ReadFile %q: %v", name, err)
		}
		src := string(data)
		if name != "write.go" {
			for _, marker := range writeOnly {
				if strings.Contains(src, marker) {
					t.Errorf("%s contains git subcommand literal %s, allowed only in write.go", name, marker)
				}
			}
		}
		for _, marker := range never {
			if name == "write.go" && strings.Contains(src, marker) {
				t.Errorf("%s contains forbidden flag literal %s", name, marker)
			}
		}
	}
}
