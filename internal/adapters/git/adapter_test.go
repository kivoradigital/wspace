// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/git"
	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
)

// TestGitAdapter_RejectsRelativeRepoPath is a named §13 threat-matrix test:
// "Git repository selection" — a relative repo path must never reach a `git
// -C` invocation.
func TestGitAdapter_RejectsRelativeRepoPath(t *testing.T) {
	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	_, err = a.IsMainClone(context.Background(), domain.Path("relative/path"))
	if err == nil {
		t.Fatal("expected an error for a relative repo path, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidPath) {
		t.Fatalf("expected errors.Is(err, domain.ErrInvalidPath), got %v", err)
	}
}

// TestGitAdapter_DropsInheritedGitEnv is a named §13 threat-matrix test:
// inherited GIT_DIR/GIT_WORK_TREE must never retarget a `-C` invocation
// (ADR D13).
func TestGitAdapter_DropsInheritedGitEnv(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	decoyDir := t.TempDir()
	t.Setenv("GIT_DIR", filepath.Join(decoyDir, ".git"))
	t.Setenv("GIT_WORK_TREE", decoyDir)

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	ok, err := a.IsMainClone(context.Background(), clone)
	if err != nil {
		t.Fatalf("IsMainClone: %v (inherited GIT_DIR/GIT_WORK_TREE must be dropped)", err)
	}
	if !ok {
		t.Fatal("expected clone to be recognized as a main clone")
	}
}

// TestGitAdapter_RejectsLinkedWorktreeAsSource is a named §13 threat-matrix
// test: a SourceDir whose ".git" is a file (a linked worktree), not a
// directory, must be rejected.
func TestGitAdapter_RejectsLinkedWorktreeAsSource(t *testing.T) {
	gitfix.RequireGit(t)

	dir := t.TempDir()
	gitFile := filepath.Join(dir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /nonexistent/.git/worktrees/x\n"), 0o644); err != nil {
		t.Fatalf("write fake .git file: %v", err)
	}

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	ok, err := a.IsMainClone(context.Background(), domain.Path(filepath.ToSlash(dir)))
	if err == nil {
		t.Fatal("expected an error for a linked-worktree source, got nil")
	}
	if ok {
		t.Fatal("expected IsMainClone to report false for a linked worktree")
	}
	if domain.Code(err) != domain.CodeNotAMainClone {
		t.Fatalf("expected CodeNotAMainClone, got %q", domain.Code(err))
	}
}

// TestGitAdapter_OrdinaryNonRepoDirectoryIsNotAnError is CRITICAL-1's own
// regression test (verify-report.md): a directory with no ".git" entry at
// all — an ordinary folder, or a bare repository — is not a git
// repository, but that is not an operational failure either. IsMainClone
// must report (false, nil), never an error, so a caller scanning many such
// directories (project_wizard.go's scanCandidates) can skip this one
// candidate without aborting the whole scan.
func TestGitAdapter_OrdinaryNonRepoDirectoryIsNotAnError(t *testing.T) {
	dir := t.TempDir()

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	ok, err := a.IsMainClone(context.Background(), domain.Path(filepath.ToSlash(dir)))
	if err != nil {
		t.Fatalf("IsMainClone(ordinary non-repo dir) = %v, want nil error", err)
	}
	if ok {
		t.Fatal("expected IsMainClone to report false for an ordinary non-repo directory")
	}
}

// TestGitAdapter_BareRepoIsNotAnError covers the same "no .git entry"
// shape as TestGitAdapter_OrdinaryNonRepoDirectoryIsNotAnError but for a
// bare repository specifically, since it is the other real-world case
// CRITICAL-1's reproduction named (a bare mirror repo in projects_root).
func TestGitAdapter_BareRepoIsNotAnError(t *testing.T) {
	gitfix.RequireGit(t)

	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	ok, err := a.IsMainClone(context.Background(), domain.Path(filepath.ToSlash(dir)))
	if err != nil {
		t.Fatalf("IsMainClone(bare repo) = %v, want nil error", err)
	}
	if ok {
		t.Fatal("expected IsMainClone to report false for a bare repo (no \".git\" entry)")
	}
}
