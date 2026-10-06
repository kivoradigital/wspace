// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

// Package e2e is the one place in this module that builds cmd/wspace and runs
// it as a real subprocess. Every other test in this repository injects a
// fake into internal/app or internal/cli and asserts on rendered strings;
// none of them ever compiles the actual binary or observes what it prints
// on a real stdout/stderr, or what it actually does to a real git
// repository on disk. That gap is exactly how a whole class of defect —
// nil dependencies the composition root never wired, subprocess output
// that never reached the terminal, a cobra flag error rendered as a
// generic sentence — shipped behind a fully green `go test ./...`.
//
// Every test here drives the real ws binary, built fresh into a temporary
// directory, against real git repositories (a bare "remote" plus a real
// main clone, both under t.TempDir()) with an isolated HOME and
// XDG_CONFIG_HOME, so a developer's own ~/.config/wspace (or global git
// config) is never read or written by these tests. Run with
// `go test -tags e2e ./internal/e2e/...` (wired as `make test-e2e`).
package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// runTimeout bounds every subprocess invocation (wizard included) so a
// hung prompt or a runaway git command fails the test instead of hanging
// the suite forever.
const runTimeout = 30 * time.Second

var (
	buildOnce sync.Once
	buildErr  error
	binPath   string
)

// requireGit skips the calling test cleanly when git is not on PATH: every
// fixture in this package is a real git repository, so without a real git
// binary there is nothing for these tests to exercise.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH; skipping e2e suite")
	}
}

// wsBinary builds cmd/wspace exactly once for the whole test binary run (via
// sync.Once, so every test in this package reuses one compiled binary
// instead of paying a fresh `go build` per test) and returns its path.
func wsBinary(t *testing.T) string {
	t.Helper()
	requireGit(t)

	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ws-e2e-bin-")
		if err != nil {
			buildErr = fmt.Errorf("create build tempdir: %w", err)
			return
		}

		out := filepath.Join(dir, "wspace")
		if runtime.GOOS == "windows" {
			out += ".exe"
		}

		root, err := moduleRoot()
		if err != nil {
			buildErr = err
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/wspace")
		cmd.Dir = root
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			buildErr = fmt.Errorf("go build ./cmd/wspace: %w\n%s", err, stderr.String())
			return
		}
		binPath = out
	})
	if buildErr != nil {
		t.Fatalf("build ws binary: %v", buildErr)
	}
	return binPath
}

// moduleRoot locates the repository root by walking up from this file's
// own directory until go.mod is found — the same technique
// internal/archtest's moduleRoot uses, needed here for the same reason:
// `go test` sets the working directory to the package under test, not the
// module root `go build ./cmd/wspace` needs.
func moduleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("e2e: cannot determine caller for module root discovery")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("e2e: go.mod not found above internal/e2e")
		}
		dir = parent
	}
}

// Fixture is one hermetic invocation environment for the real ws binary:
// an isolated HOME and XDG_CONFIG_HOME, both fresh under t.TempDir(), so a
// developer's real configuration is never read or written by a test.
type Fixture struct {
	t    *testing.T
	Bin  string
	Home string
	XDG  string
	Root string // the fixture's own temp root, for building further paths
}

// NewFixture builds (or reuses) the ws binary and prepares one hermetic
// HOME/XDG_CONFIG_HOME pair for the calling test.
func NewFixture(t *testing.T) *Fixture {
	t.Helper()
	bin := wsBinary(t)

	root := t.TempDir()
	home := filepath.Join(root, "home")
	xdg := filepath.Join(root, "xdg-config")
	for _, d := range []string{home, xdg} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	return &Fixture{t: t, Bin: bin, Home: home, XDG: xdg, Root: root}
}

// Run executes the real ws binary as a subprocess with args exactly as
// given, feeding stdin (for wizard prompts) and capturing stdout/stderr
// separately, exactly as a real terminal session would see them. The
// child's environment is reduced to PATH (so it can still find git) plus
// this fixture's own HOME and XDG_CONFIG_HOME — no other variable from the
// test process survives, so a real developer's WSPACE_CONTEXT or git identity
// can never leak into what is being tested.
func (f *Fixture) Run(stdin string, args ...string) (stdout, stderr string, exitCode int) {
	f.t.Helper()
	return f.runEnv(nil, stdin, args...)
}

// RunEnv is Run with extraEnv appended on top of the fixture's own
// baseline (PATH/HOME/XDG_CONFIG_HOME) — needed by the install e2e tests,
// which must control PATH and SHELL precisely to exercise a specific
// candidate directory, and by extension every other test in this package
// stays on Run's fixed baseline untouched.
func (f *Fixture) RunEnv(extraEnv []string, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	f.t.Helper()
	return f.runEnv(extraEnv, stdin, args...)
}

func (f *Fixture) runEnv(extraEnv []string, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, f.Bin, args...)
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + f.Home,
		"XDG_CONFIG_HOME=" + f.XDG,
	}, extraEnv...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return out.String(), errBuf.String(), exitErr.ExitCode()
		}
		f.t.Fatalf("run ws %v: %v\nstdout: %s\nstderr: %s", args, err, out.String(), errBuf.String())
	}
	return out.String(), errBuf.String(), 0
}

// MustRun is Run but fails the test immediately on a non-zero exit code —
// convenience for the many happy-path steps that must simply succeed.
func (f *Fixture) MustRun(stdin string, args ...string) (stdout, stderr string) {
	f.t.Helper()
	stdout, stderr, code := f.Run(stdin, args...)
	if code != 0 {
		f.t.Fatalf("ws %v: exit code = %d, want 0\nstdout: %s\nstderr: %s", args, code, stdout, stderr)
	}
	return stdout, stderr
}

// GitProject is one project fixture: a bare "remote" and a real main
// clone pushed to it, with one commit on its default branch.
type GitProject struct {
	Name   string
	Remote string // bare repo path
	Main   string // main clone's working directory (this is what gets registered)
}

// NewGitProject creates a real bare remote plus a real main clone with one
// commit on branch, pushed to the remote, under root. main is placed
// directly at <root>/<projectsDirName>/<name> so a caller can point a
// context's ProjectsRoot at <root>/<projectsDirName> and have the project
// wizard's scan discover it by directory name alone.
func NewGitProject(t *testing.T, projectsRoot, name, branch string) GitProject {
	t.Helper()
	remotesDir := filepath.Join(filepath.Dir(projectsRoot), "remotes")
	if err := os.MkdirAll(remotesDir, 0o755); err != nil {
		t.Fatalf("mkdir remotes dir: %v", err)
	}
	remote := filepath.Join(remotesDir, name+".git")
	main := filepath.Join(projectsRoot, name)

	runGit(t, filepath.Dir(remote), "init", "--bare", "-q", remote)
	runGit(t, filepath.Dir(main), "init", "-q", "-b", branch, main)
	runGit(t, main, "config", "user.email", "e2e@example.invalid")
	runGit(t, main, "config", "user.name", "wspace e2e")

	if err := os.WriteFile(filepath.Join(main, "README.md"), []byte("hello from "+name+"\n"), 0o644); err != nil {
		t.Fatalf("write %s/README.md: %v", name, err)
	}
	runGit(t, main, "add", "README.md")
	runGit(t, main, "commit", "-q", "-m", "init")
	runGit(t, main, "remote", "add", "origin", remote)
	runGit(t, main, "push", "-q", "origin", branch)

	return GitProject{Name: name, Remote: remote, Main: main}
}

// CurrentBranch returns the branch checked out in dir (a worktree or main
// clone), via the real git binary — used to assert on the real observable
// state of a source repo, never on this codebase's own idea of it.
func CurrentBranch(t *testing.T, dir string) string {
	t.Helper()
	return runGit(t, dir, "branch", "--show-current")
}

// runGit runs a real git command for fixture setup/assertions (not the ws
// binary under test) and fails the test immediately on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (dir=%s): %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}
