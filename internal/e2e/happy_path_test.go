// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// statusRow mirrors internal/cli's statusItemJSON contract (alias, branch,
// ahead, behind, dirty) — see cli-surface spec's --json schemas.
type statusRow struct {
	Alias  string `json:"alias"`
	Branch string `json:"branch"`
	Ahead  int    `json:"ahead"`
	Behind int    `json:"behind"`
	Dirty  bool   `json:"dirty"`
}

// listRow mirrors internal/cli's listItemJSON contract (name, path,
// project_count).
type listRow struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	ProjectCount int    `json:"project_count"`
}

// infoPayload mirrors internal/cli's infoJSON contract (context_name plus
// every resolved Chain B option and its winning layer).
type infoPayload struct {
	ContextName string `json:"context_name"`
	Options     map[string]struct {
		Value any    `json:"value"`
		From  string `json:"from"`
	} `json:"options"`
}

// TestHappyPath_FullLifecycle drives the real ws binary through context
// creation, project registration, workspace creation without --branch,
// every read command's --json contract, exec, sync-env, add, rm, repair,
// doctor and destroy — asserting on real observable effects (worktrees on
// disk, branches in the real source repos, env files actually copied, and
// the manifest's own on-disk contents) rather than only on printed text.
//
// This is exactly the class of test the rest of this module has none of:
// every other test injects a fake port and asserts on a rendered string,
// which cannot see a nil composition-root dependency, swallowed subprocess
// output, or a cobra error discarded before it ever became a domain error.
func TestHappyPath_FullLifecycle(t *testing.T) {
	fx := NewFixture(t)

	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatalf("mkdir projects root: %v", err)
	}

	alpha := NewGitProject(t, projectsRoot, "alpha", "develop")
	_ = NewGitProject(t, projectsRoot, "beta", "develop")

	// Seed alpha with a real, never-committed env file before it is ever
	// registered — exactly how a real developer's .env looks — so
	// workspace creation's own copyEnv step has something real to copy.
	// This is the "env files copied" effect the happy path must observe,
	// not just accept on faith.
	if err := os.WriteFile(filepath.Join(alpha.Main, ".env"), []byte("SECRET=from-alpha-local\n"), 0o644); err != nil {
		t.Fatalf("seed alpha .env: %v", err)
	}

	t.Run("context create registers both projects", func(t *testing.T) {
		stdin := strings.Join([]string{
			"e2e", // context name
			workspacesRoot,
			projectsRoot,
			"",    // ignore_patterns -> none
			"",    // base branch -> default "develop"
			"",    // copy env files by default -> default yes
			"",    // fetch before create by default -> default yes
			"1,2", // pick both scanned candidates (sorted: alpha, beta)
			"",    // project key for alpha -> default "alpha"
			"",    // alpha origin_branch -> inherit
			"",    // alpha dest_branch -> inherit
			"",    // alpha worktree_dir -> inherit
			"",    // project key for beta -> default "beta"
			"",    // beta origin_branch -> inherit
			"",    // beta dest_branch -> inherit
			"",    // beta worktree_dir -> inherit
			"",
		}, "\n")

		stdout, _ := fx.MustRun(stdin, "context", "create")
		if !strings.Contains(stdout, "e2e") {
			t.Fatalf("stdout = %q, want it to confirm context e2e was created", stdout)
		}
	})

	t.Run("create builds a workspace without --branch", func(t *testing.T) {
		fx.MustRun("", "create", "demo", "--project", "alpha")

		wsRoot := filepath.Join(workspacesRoot, "demo")
		alphaWorktree := filepath.Join(wsRoot, "alpha")
		if !PathExists(t, alphaWorktree) {
			t.Fatalf("worktree %s does not exist on disk", alphaWorktree)
		}
		// The context has two registered projects (alpha, beta); selecting
		// only alpha must produce a workspace containing exactly that
		// subset on disk — this is the real-binary, real-git-repository
		// regression proof for the shared app.CreateWorkspace mechanism
		// both the CLI's own "--project" flag and the tray's "New
		// workspace…" wizard (app.RunCreateWorkspace) funnel through: an
		// unselected project's worktree must never appear, exactly as if
		// CreateWorkspace's own "empty ProjectKeys means every project"
		// fallback had silently been relied on instead of the explicit
		// selection.
		if PathExists(t, filepath.Join(wsRoot, "beta")) {
			t.Fatal("beta worktree exists on disk, want only the selected subset (alpha) to have been created")
		}
		if got := CurrentBranch(t, alphaWorktree); got != "demo" {
			t.Fatalf("alpha worktree branch = %q, want %q (branch_prefix \"\" + workspace name, no --branch given)", got, "demo")
		}
		if !PathExists(t, filepath.Join(alphaWorktree, ".env")) {
			t.Fatalf("alpha/.env was not copied into the new worktree")
		}
		data, err := os.ReadFile(filepath.Join(alphaWorktree, ".env"))
		if err != nil {
			t.Fatalf("read copied .env: %v", err)
		}
		if !strings.Contains(string(data), "SECRET=from-alpha-local") {
			t.Fatalf("copied .env = %q, want the real local content", data)
		}

		m := ReadManifest(t, wsRoot)
		if m.Workspace.Branch != "demo" || len(m.Workspace.Repos) != 1 || m.Workspace.Repos[0].Alias != "alpha" {
			t.Fatalf("manifest = %+v, want branch demo with exactly one repo aliased alpha", m)
		}
		if len(m.EnvCopies) != 1 || m.EnvCopies[0] != "alpha/.env" {
			t.Fatalf("manifest.EnvCopies = %+v, want [\"alpha/.env\"]", m.EnvCopies)
		}
	})

	t.Run("add mounts a second project", func(t *testing.T) {
		fx.MustRun("", "add", "demo", "beta")

		wsRoot := filepath.Join(workspacesRoot, "demo")
		betaWorktree := filepath.Join(wsRoot, "beta")
		if !PathExists(t, betaWorktree) {
			t.Fatalf("worktree %s does not exist on disk", betaWorktree)
		}
		if got := CurrentBranch(t, betaWorktree); got != "demo" {
			t.Fatalf("beta worktree branch = %q, want %q", got, "demo")
		}

		m := ReadManifest(t, wsRoot)
		if len(m.Workspace.Repos) != 2 {
			t.Fatalf("manifest repos = %+v, want 2 after add", m.Workspace.Repos)
		}
	})

	t.Run("list shows the workspace with both projects", func(t *testing.T) {
		stdout, _ := fx.MustRun("", "list", "--json")
		var rows []listRow
		if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
			t.Fatalf("parse list --json: %v\noutput: %s", err, stdout)
		}
		if len(rows) != 1 || rows[0].Name != "demo" || rows[0].ProjectCount != 2 {
			t.Fatalf("list --json = %+v, want one row for demo with project_count 2", rows)
		}
	})

	t.Run("status --json reports both repos clean on their new branch", func(t *testing.T) {
		stdout, _ := fx.MustRun("", "status", "demo", "--json")
		var rows []statusRow
		if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
			t.Fatalf("parse status --json: %v\noutput: %s", err, stdout)
		}
		if len(rows) != 2 {
			t.Fatalf("status --json = %+v, want 2 rows", rows)
		}
		byAlias := map[string]statusRow{}
		for _, r := range rows {
			byAlias[r.Alias] = r
		}
		for _, alias := range []string{"alpha", "beta"} {
			r, ok := byAlias[alias]
			if !ok {
				t.Fatalf("status --json = %+v, missing alias %q", rows, alias)
			}
			if r.Branch != "demo" {
				t.Fatalf("status[%s].Branch = %q, want %q", alias, r.Branch, "demo")
			}
		}
	})

	t.Run("info --json reports the resolved context and base branch", func(t *testing.T) {
		stdout, _ := fx.MustRun("", "info", "--json")
		var info infoPayload
		if err := json.Unmarshal([]byte(stdout), &info); err != nil {
			t.Fatalf("parse info --json: %v\noutput: %s", err, stdout)
		}
		if info.ContextName != "e2e" {
			t.Fatalf("info.ContextName = %q, want %q", info.ContextName, "e2e")
		}
		if opt, ok := info.Options["base_branch"]; !ok || fmt.Sprint(opt.Value) != "develop" {
			t.Fatalf("info.Options[base_branch] = %+v, want value \"main\"", info.Options["base_branch"])
		}
	})

	t.Run("jump prints the workspace's absolute path", func(t *testing.T) {
		stdout, _ := fx.MustRun("", "jump", "demo")
		want := filepath.Join(workspacesRoot, "demo")
		if strings.TrimSpace(stdout) != want {
			t.Fatalf("jump stdout = %q, want %q", strings.TrimSpace(stdout), want)
		}
	})

	t.Run("exec runs in every repo, with a header and forwarded output", func(t *testing.T) {
		stdout, stderr, code := fx.Run("", "exec", "demo", "--", "git", "rev-parse", "--abbrev-ref", "HEAD")
		if code != 0 {
			t.Fatalf("exec exit code = %d, want 0; stderr=%s", code, stderr)
		}
		for _, alias := range []string{"alpha", "beta"} {
			if !strings.Contains(stdout, alias) {
				t.Fatalf("exec stdout = %q, want a header naming %q", stdout, alias)
			}
		}
		if strings.Count(stdout, "demo") != 2 {
			t.Fatalf("exec stdout = %q, want the real branch name \"demo\" forwarded once per repo", stdout)
		}
	})

	t.Run("sync-env re-discovers and copies a newly added env file", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(alpha.Main, "config"), 0o755); err != nil {
			t.Fatalf("mkdir alpha/config: %v", err)
		}
		if err := os.WriteFile(filepath.Join(alpha.Main, "config", ".env.local"), []byte("EXTRA=1\n"), 0o644); err != nil {
			t.Fatalf("write alpha/config/.env.local: %v", err)
		}

		fx.MustRun("", "sync-env", "demo")

		copiedPath := filepath.Join(workspacesRoot, "demo", "alpha", "config", ".env.local")
		if !PathExists(t, copiedPath) {
			t.Fatalf("sync-env did not copy the newly added config/.env.local into the worktree")
		}

		m := ReadManifest(t, filepath.Join(workspacesRoot, "demo"))
		found := false
		for _, e := range m.EnvCopies {
			if e == "alpha/config/.env.local" {
				found = true
			}
		}
		if !found {
			t.Fatalf("manifest.EnvCopies = %+v, want it to include alpha/config/.env.local", m.EnvCopies)
		}
	})

	t.Run("rm removes beta's worktree", func(t *testing.T) {
		fx.MustRun("", "rm", "demo", "beta", "--force")

		betaWorktree := filepath.Join(workspacesRoot, "demo", "beta")
		if PathExists(t, betaWorktree) {
			t.Fatalf("worktree %s still exists on disk after rm", betaWorktree)
		}
		m := ReadManifest(t, filepath.Join(workspacesRoot, "demo"))
		if len(m.Workspace.Repos) != 1 || m.Workspace.Repos[0].Alias != "alpha" {
			t.Fatalf("manifest repos = %+v, want only alpha left after rm", m.Workspace.Repos)
		}
	})

	t.Run("repair recreates a worktree deleted manually", func(t *testing.T) {
		alphaWorktree := filepath.Join(workspacesRoot, "demo", "alpha")

		// Simulate the spec's own literal scenario: "the worktree
		// directory was deleted manually" (workspace-lifecycle spec,
		// "Repair recreates a missing worktree") — a plain rm -rf, exactly
		// what a user's Finder, shell, or a build script that wipes a
		// directory would do. This deliberately does NOT deregister the
		// worktree from the main clone's ".git/worktrees/" the way `git
		// worktree remove` would: real git still believes this worktree is
		// registered, which is exactly the state that used to make Repair
		// fail with "is a missing but already registered worktree" (a
		// previous version of this test substituted `git worktree remove
		// --force` here, which cleanly deregisters the worktree and so
		// never exercised that failure at all).
		if err := os.RemoveAll(alphaWorktree); err != nil {
			t.Fatalf("rm -rf alpha worktree: %v", err)
		}
		if PathExists(t, alphaWorktree) {
			t.Fatalf("alpha worktree still present after rm -rf")
		}

		fx.MustRun("", "repair", "demo")

		if !PathExists(t, alphaWorktree) {
			t.Fatalf("repair did not recreate the alpha worktree on disk")
		}
		if got := CurrentBranch(t, alphaWorktree); got != "demo" {
			t.Fatalf("repaired alpha worktree branch = %q, want %q", got, "demo")
		}
	})

	t.Run("doctor completes cleanly", func(t *testing.T) {
		_, stderr, code := fx.Run("", "doctor")
		if code != 0 {
			t.Fatalf("doctor exit code = %d, want 0; stderr=%s", code, stderr)
		}
	})

	t.Run("destroy tears the workspace down", func(t *testing.T) {
		fx.MustRun("", "destroy", "demo", "--force")

		wsRoot := filepath.Join(workspacesRoot, "demo")
		if PathExists(t, wsRoot) {
			t.Fatalf("workspace root %s still exists on disk after destroy", wsRoot)
		}

		stdout, _ := fx.MustRun("", "list", "--json")
		var rows []listRow
		if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
			t.Fatalf("parse list --json: %v\noutput: %s", err, stdout)
		}
		if len(rows) != 0 {
			t.Fatalf("list --json after destroy = %+v, want none left", rows)
		}
	})
}
