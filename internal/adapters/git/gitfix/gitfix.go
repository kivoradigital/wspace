// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package gitfix builds real git repositories in t.TempDir() for the git
// adapter's tests (design.md §12: "Real git against gitfix repos in
// t.TempDir()"). Every fixture is hermetic: no network is touched, and HOME
// plus GIT_CONFIG_GLOBAL/GIT_CONFIG_SYSTEM are redirected into throwaway
// directories so a developer's own gitconfig, credential helpers, or SSH
// setup can never influence a test result.
package gitfix

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// RequireGit skips t when the test binary was invoked with -short (these
// tests spawn a real git subprocess, so they are deliberately excluded from
// the pure-unit `go test -short ./...` run) or when no git binary is on
// PATH.
func RequireGit(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("gitfix: skipping real-git test under -short")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("gitfix: git binary not found on PATH, skipping")
	}
}

// NewOrigin creates a bare repository seeded with one commit on main and
// returns its path. Repositories created by NewClone from it get a
// functioning "origin" remote.
func NewOrigin(t *testing.T) domain.Path {
	t.Helper()
	RequireGit(t)

	bare := t.TempDir()
	runGit(t, bare, "init", "--bare", "--initial-branch=main", "-q")

	seed := t.TempDir()
	runGit(t, seed, "init", "--initial-branch=main", "-q")
	configureIdentity(t, seed)
	writeFile(t, seed, "README.md", "seed\n")
	runGit(t, seed, "add", "--", "README.md")
	runGit(t, seed, "commit", "-q", "-m", "seed")
	runGit(t, seed, "remote", "add", "origin", bare)
	runGit(t, seed, "push", "-q", "origin", "main")

	return toDomainPath(bare)
}

// NewClone clones origin into a fresh directory and configures a local
// commit identity so Commit can be used against it immediately.
func NewClone(t *testing.T, origin domain.Path) domain.Path {
	t.Helper()
	RequireGit(t)

	scratch := t.TempDir()
	target := filepath.Join(t.TempDir(), "clone")
	runGit(t, scratch, "clone", "-q", "--", string(origin), target)
	configureIdentity(t, target)

	return toDomainPath(target)
}

// Commit writes file (creating parent directories as needed) with body,
// stages it and commits it in repo.
func Commit(t *testing.T, repo domain.Path, file, body string) {
	t.Helper()
	dir := string(repo)
	writeFile(t, dir, file, body)
	runGit(t, dir, "add", "--", file)
	runGit(t, dir, "commit", "-q", "-m", "test commit: "+file)
}

// Branch creates a new branch named name in repo, pointing at the branch
// currently checked out there, without checking it out.
func Branch(t *testing.T, repo domain.Path, name string) {
	t.Helper()
	runGit(t, string(repo), "branch", "--", name)
}

// Dirty overwrites an already-tracked file with body, leaving the change
// unstaged so `git status` reports it as a tracked modification.
func Dirty(t *testing.T, repo domain.Path, file, body string) {
	t.Helper()
	writeFile(t, string(repo), file, body)
}

// Untracked writes a new file that is never staged, so `git status` reports
// it as untracked.
func Untracked(t *testing.T, repo domain.Path, file string) {
	t.Helper()
	writeFile(t, string(repo), file, "untracked\n")
}

// Push pushes branch to repo's "origin" remote. It exists alongside the six
// fixtures design.md §7 names because advancing a shared origin (to test
// fast-forward sync) needs it, and it is exercised through the same
// hermetic, no-network-beyond-localhost environment as every other fixture.
func Push(t *testing.T, repo domain.Path, branch string) {
	t.Helper()
	runGit(t, string(repo), "push", "-q", "origin", branch)
}

// Git runs an arbitrary git command in repo with the fixtures' hermetic
// environment and returns its trimmed combined output; it fails t on a
// non-zero exit. Use it for setup steps no named fixture covers.
func Git(t *testing.T, repo domain.Path, args ...string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, string(repo), args...))
}

func configureIdentity(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "config", "user.name", "ws-test")
	runGit(t, dir, "config", "user.email", "ws-test@example.com")
}

func writeFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("gitfix: mkdir for %q: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("gitfix: write %q: %v", rel, err)
	}
}

// isolatedEnv builds a hermetic environment for a fixture-setup git
// invocation. It intentionally mirrors the adapter's own allowlist
// philosophy (design.md ADR D13) but adds GIT_CONFIG_GLOBAL/SYSTEM
// redirection, which only test setup needs.
func isolatedEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"GIT_CONFIG_GLOBAL=" + filepath.Join(home, "no-such-gitconfig"),
		"GIT_CONFIG_SYSTEM=" + filepath.Join(home, "no-such-gitconfig-system"),
		"GIT_CONFIG_NOSYSTEM=1",
		"LC_ALL=C",
		"LANG=C",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ADVICE=0",
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = isolatedEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gitfix: git %s (dir=%s) failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

func toDomainPath(p string) domain.Path {
	return domain.Path(filepath.ToSlash(p))
}
