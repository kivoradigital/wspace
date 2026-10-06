// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupSingleProjectWorkspace drives context creation (one manually
// registered project, no scanning) and workspace creation for the failure-
// path tests below, which only need one repo to exercise their scenario.
// Returns the workspace root and the project's real main clone directory.
func setupSingleProjectWorkspace(t *testing.T, fx *Fixture, projectName, wsName string) (wsRoot, projectMain string) {
	t.Helper()

	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatalf("mkdir projects root: %v", err)
	}
	project := NewGitProject(t, projectsRoot, projectName, "develop")

	stdin := strings.Join([]string{
		"solo-ctx",
		workspacesRoot,
		"",           // no projects root: skip scanning, register manually below
		"",           // ignore_patterns -> none
		"",           // base branch -> default main
		"",           // copy env default -> yes
		"",           // fetch before create -> yes
		"y",          // register a project manually
		project.Main, // project source dir
		projectName,  // project key
		"",           // origin_branch -> inherit
		"",           // dest_branch -> inherit
		"",           // worktree_dir -> inherit
		"n",          // stop registering manually
		"",
	}, "\n")
	fx.MustRun(stdin, "context", "create")
	fx.MustRun("", "create", wsName, "--project", projectName)

	return filepath.Join(workspacesRoot, wsName), project.Main
}

// TestFailure_DestroyBlockedByGenuinelyNewUntrackedFile covers the
// workspace-lifecycle spec's teardown safety: a real untracked file this
// tool never created must block `destroy` without --force, and the
// workspace must still be there afterward — not partially torn down.
func TestFailure_DestroyBlockedByGenuinelyNewUntrackedFile(t *testing.T) {
	fx := NewFixture(t)
	wsRoot, _ := setupSingleProjectWorkspace(t, fx, "solo", "blocked-ws")

	stray := filepath.Join(wsRoot, "solo", "stray-file-nobody-asked-for.txt")
	if err := os.WriteFile(stray, []byte("not tracked, not an env copy\n"), 0o644); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	// destroy without --force asks for confirmation first; answer yes so
	// the safety check itself (not the confirmation prompt) is what this
	// test exercises.
	_, stderr, code := fx.Run("y\n", "destroy", "blocked-ws")
	if code == 0 {
		t.Fatal("destroy exit code = 0, want non-zero: a genuinely foreign untracked file must block teardown")
	}
	if !strings.Contains(stderr, "stray-file-nobody-asked-for.txt") {
		t.Fatalf("stderr = %q, want it to name the blocking file", stderr)
	}
	if !PathExists(t, wsRoot) {
		t.Fatal("workspace root no longer exists: destroy must not proceed when blocked")
	}
	if !PathExists(t, stray) {
		t.Fatal("the blocking file itself was removed: destroy must not have touched anything")
	}
}

// TestFailure_DestroyIgnoresItsOwnEnvCopies covers the other half of the
// same safety check: an untracked file destroy itself copied in as part
// of workspace creation (recorded in the manifest's env_copies) must never
// block teardown, unlike an arbitrary foreign untracked file.
func TestFailure_DestroyIgnoresItsOwnEnvCopies(t *testing.T) {
	fx := NewFixture(t)

	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatalf("mkdir projects root: %v", err)
	}
	project := NewGitProject(t, projectsRoot, "envproj", "develop")
	if err := os.WriteFile(filepath.Join(project.Main, ".env"), []byte("SECRET=1\n"), 0o644); err != nil {
		t.Fatalf("seed .env: %v", err)
	}

	stdin := strings.Join([]string{
		"envctx",
		workspacesRoot,
		"",
		"", // ignore_patterns -> none
		"",
		"", // copy env default -> yes (needed: this is exactly what's under test)
		"",
		"y",
		project.Main,
		"envproj",
		"", // origin_branch -> inherit
		"", // dest_branch -> inherit
		"", // worktree_dir -> inherit
		"n",
		"",
	}, "\n")
	fx.MustRun(stdin, "context", "create")
	fx.MustRun("", "create", "env-ws", "--project", "envproj")

	wsRoot := filepath.Join(workspacesRoot, "env-ws")
	copiedEnv := filepath.Join(wsRoot, "envproj", ".env")
	if !PathExists(t, copiedEnv) {
		t.Fatalf(".env was not copied into the worktree at %s", copiedEnv)
	}

	// The copied .env is untracked in the worktree (real git status would
	// show "??"), yet destroy without --force must still succeed, because
	// the manifest records it as this tool's own copy, not a foreign
	// change.
	_, stderr, code := fx.Run("y\n", "destroy", "env-ws")
	if code != 0 {
		t.Fatalf("destroy exit code = %d, want 0: its own env copy must never block teardown; stderr=%s", code, stderr)
	}
	if PathExists(t, wsRoot) {
		t.Fatal("workspace root still exists after a destroy that should have succeeded")
	}
}

// TestFailure_UnknownFlag covers defect B against the real binary: an
// unknown flag must surface cobra's own precise message, not the generic
// ErrUnknown catalog fallback.
func TestFailure_UnknownFlag(t *testing.T) {
	fx := NewFixture(t)
	_, stderr, code := fx.Run("", "destroy", "some-workspace", "--yes")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero: --yes is not a real flag (it is --force)")
	}
	if strings.Contains(stderr, "an unexpected error occurred") {
		t.Fatalf("stderr = %q, want cobra's own flag error, not the generic fallback", stderr)
	}
	if !strings.Contains(stderr, "unknown flag: --yes") {
		t.Fatalf("stderr = %q, want cobra's own \"unknown flag: --yes\" message", stderr)
	}
}

// TestFailure_MissingWorkspace covers a plain operational error end to
// end: looking up a workspace that was never created must render the
// catalog's "workspace not found" text, not a raw Go error or a path leak.
func TestFailure_MissingWorkspace(t *testing.T) {
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")

	stdin := strings.Join([]string{
		"empty-ctx",
		workspacesRoot,
		"",
		"", // ignore_patterns -> none
		"",
		"",
		"",
		"n", // decline manual project registration entirely
		"",
	}, "\n")
	fx.MustRun(stdin, "context", "create")

	_, stderr, code := fx.Run("", "jump", "does-not-exist")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero: the workspace was never created")
	}
	if !strings.Contains(stderr, "workspace not found") {
		t.Fatalf("stderr = %q, want the ForCode(CodeWorkspaceNotFound) catalog text", stderr)
	}
}

// TestFailure_ExecPropagatesNonZeroExitCode covers defect A's exit-code
// contract end to end: a real failing child process's exit code must
// become ws exec's own exit code, with the failing repo named in stderr.
func TestFailure_ExecPropagatesNonZeroExitCode(t *testing.T) {
	fx := NewFixture(t)
	setupSingleProjectWorkspace(t, fx, "execproj", "exec-ws")

	_, stderr, code := fx.Run("", "exec", "exec-ws", "--", "git", "this-is-not-a-real-git-subcommand")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero: the child command fails")
	}
	if !strings.Contains(stderr, "execproj") {
		t.Fatalf("stderr = %q, want it to name the failing repo (execproj)", stderr)
	}
	if !strings.Contains(stderr, "1") {
		t.Fatalf("stderr = %q, want it to mention the exit code", stderr)
	}
}
