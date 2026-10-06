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

// TestContextLifecycle_EditAndRemove drives the real ws binary through
// this change's own authorized gap-closure: "ws context edit" and "ws
// context remove", against a real (albeit git-repo-free) config directory.
// Every other e2e test in this package proves workspace-lifecycle
// commands against real git repositories; this one proves the
// context-management commands the rest of them all depend on being able
// to reach in the first place, closing the one gap phase 7's own
// apply-progress record flagged: a context could be created and switched
// to, but never edited or removed.
func TestContextLifecycle_EditAndRemove(t *testing.T) {
	fx := NewFixture(t)

	teamAWorkspaces := filepath.Join(fx.Root, "team-a-workspaces")
	teamBWorkspaces := filepath.Join(fx.Root, "team-b-workspaces")

	// Every stdin script below carries one extra trailing "" element beyond
	// its last real answer: TerminalPrompter's readLine only returns a
	// blank-default answer when it can actually see the line's trailing
	// '\n' (see termprompt's own ReadString('\n') use) — without this
	// sentinel the last blank line and EOF arrive together and the
	// prompter reports a raw I/O error instead of "accept the default".
	// happy_path_test.go's own stdin scripts use the identical pattern.

	t.Run("create the first context", func(t *testing.T) {
		stdin := strings.Join([]string{
			"team-a", // context name
			teamAWorkspaces,
			"", // projects root -> skip scanning
			"", // ignore patterns -> none
			"", // base branch -> default "develop"
			"", // copy env files by default -> default yes
			"", // fetch before create by default -> default yes
			"", // decline manual project registration
			"",
		}, "\n")
		stdout, _ := fx.MustRun(stdin, "context", "create")
		if !strings.Contains(stdout, "team-a") {
			t.Fatalf("stdout = %q, want it to confirm team-a was created", stdout)
		}
	})

	t.Run("create a second context to exercise remove on a non-active one", func(t *testing.T) {
		stdin := strings.Join([]string{
			"team-b",
			teamBWorkspaces,
			"",
			"",
			"",
			"",
			"",
			"",
			"",
		}, "\n")
		fx.MustRun(stdin, "context", "create")
	})

	t.Run("switch makes team-a the active context", func(t *testing.T) {
		// "context create" never auto-activates the context it creates
		// (see cli_context_test.go's own fixture, which seeds an unrelated
		// active context and switches explicitly); "context edit" with no
		// argument resolves the active context exactly like "context
		// show" does, so this test switches first.
		fx.MustRun("", "context", "switch", "team-a")
	})

	t.Run("edit prefills the existing values and persists a changed one", func(t *testing.T) {
		// "context edit" with no argument must resolve the now-active
		// team-a, per newContextEditCommand's own fallback (mirroring
		// "context show"'s).
		stdin := strings.Join([]string{
			"",        // context name -> accept the existing value (unrenamed)
			"",        // workspaces_root -> accept the existing value
			"",        // projects_root -> accept the existing (empty) value
			"",        // ignore_patterns -> accept the existing (empty) value
			"release", // base_branch -> the one field this test changes
			"",        // copy_env_default -> accept the existing value
			"",        // fetch_before_create -> accept the existing value
			"",
		}, "\n")
		stdout, stderr := fx.MustRun(stdin, "context", "edit")
		if !strings.Contains(stdout, "team-a") {
			t.Fatalf("context edit stdout = %q, stderr = %q, want it to confirm team-a was updated", stdout, stderr)
		}

		showStdout, _ := fx.MustRun("", "context", "show")
		if !strings.Contains(showStdout, "release") {
			t.Fatalf("context show stdout = %q, want the edited base_branch (release) to be reflected", showStdout)
		}
		// workspaces_root itself is not printed by "context show"
		// (printResolvedOptions only covers Chain B option keys, not the
		// context's own roots) — nothing further to assert here beyond the
		// base_branch change above, which is this test's one substantive
		// edit.
	})

	t.Run("remove deletes the non-active context", func(t *testing.T) {
		stdout, _ := fx.MustRun("", "context", "remove", "team-b")
		if !strings.Contains(stdout, "team-b") {
			t.Fatalf("context remove stdout = %q, want it to confirm team-b was removed", stdout)
		}

		listStdout, _ := fx.MustRun("", "context", "list")
		if strings.Contains(listStdout, "team-b") {
			t.Fatalf("context list stdout = %q, want team-b gone after remove", listStdout)
		}
		if !strings.Contains(listStdout, "team-a") {
			t.Fatalf("context list stdout = %q, want team-a still present", listStdout)
		}
	})

	t.Run("remove refuses the active context", func(t *testing.T) {
		_, stderr, code := fx.Run("", "context", "remove", "team-a")
		if code == 0 {
			t.Fatalf("context remove team-a (active): exit code = 0, want non-zero; stderr=%q", stderr)
		}
	})
}

// TestContextCreate_IgnorePatternsExcludeMatchingDirectoryFromDiscovery
// covers this change's own authorized gap-closure for
// context-management spec's "The system MUST accept a glob list of
// ignore_patterns at context initialization, applied during project
// discovery": the wizard must actually collect ignore_patterns, and
// project discovery, run as part of the very same "context create"
// invocation, must actually honor them by never even offering a matching
// directory as a scan candidate. This is asserted against the real
// context config.yaml the real binary wrote to disk (never printed
// output): the ignored directory's project key must not appear in the
// persisted project list at all, while its non-ignored sibling's must.
func TestContextCreate_IgnorePatternsExcludeMatchingDirectoryFromDiscovery(t *testing.T) {
	fx := NewFixture(t)

	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatalf("mkdir projects root: %v", err)
	}

	NewGitProject(t, projectsRoot, "alpha", "develop")
	NewGitProject(t, projectsRoot, "vendor-mirror", "develop")

	stdin := strings.Join([]string{
		"ignoretest", // context name
		workspacesRoot,
		projectsRoot,
		"vendor-*", // ignore_patterns: excludes "vendor-mirror" from the scan
		"",         // base branch -> default "develop"
		"",         // copy env files by default -> default yes
		"",         // fetch before create by default -> default yes
		"1",        // the only candidate offered is "alpha" (vendor-mirror was filtered before the pick prompt)
		"",         // project key for alpha -> default "alpha"
		"",         // origin_branch -> inherit
		"",         // dest_branch -> inherit
		"",         // worktree_dir -> inherit
		"",
	}, "\n")

	stdout, stderr := fx.MustRun(stdin, "context", "create")
	if !strings.Contains(stdout, "ignoretest") {
		t.Fatalf("context create stdout = %q, stderr = %q, want it to confirm ignoretest was created", stdout, stderr)
	}

	cfg := ReadContextConfig(t, fx.XDG, "ignoretest")
	if len(cfg.IgnorePatterns) != 1 || cfg.IgnorePatterns[0] != "vendor-*" {
		t.Fatalf("persisted ignore_patterns = %+v, want [\"vendor-*\"]", cfg.IgnorePatterns)
	}

	var keys []string
	for _, p := range cfg.Projects {
		keys = append(keys, p.Key)
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0].Key != "alpha" {
		t.Fatalf("persisted projects = %+v, want exactly one project (alpha); vendor-mirror must have been excluded from discovery by ignore_patterns", keys)
	}
}

// TestProjectWizard_CustomTemplatesResolveToRealWorktreeAndBranch covers
// this change's own authorized gap-closure for project-configuration
// spec's four-field wizard: a project registered through the wizard with
// a custom dest_branch and worktree_dir template must actually drive a
// real "ws create" — domain.Resolver.DestBranch/WorktreePath (already
// built and tested against whatever ended up on domain.Project, but never
// reachable through the wizard before this change) resolves those
// templates using the real workspace name, and the real worktree that
// lands on disk must be at the resolved path, checked out on the
// resolved branch — asserted here against real filesystem and git state,
// never printed output.
func TestProjectWizard_CustomTemplatesResolveToRealWorktreeAndBranch(t *testing.T) {
	fx := NewFixture(t)

	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatalf("mkdir projects root: %v", err)
	}
	project := NewGitProject(t, projectsRoot, "svc", "develop")

	stdin := strings.Join([]string{
		"tmplctx", // context name
		workspacesRoot,
		"",                   // projects root -> skip scanning, register manually below
		"",                   // ignore_patterns -> none
		"",                   // base branch -> default "develop"
		"",                   // copy env files by default -> default yes
		"",                   // fetch before create by default -> default yes
		"y",                  // register a project manually
		project.Main,         // project source dir
		"svc",                // project key
		"",                   // origin_branch -> inherit
		"custom-{workspace}", // dest_branch template
		"{project}-checkout", // worktree_dir template
		"n",                  // stop registering manually
		"",
	}, "\n")

	stdout, stderr := fx.MustRun(stdin, "context", "create")
	if !strings.Contains(stdout, "tmplctx") {
		t.Fatalf("context create stdout = %q, stderr = %q, want it to confirm tmplctx was created", stdout, stderr)
	}

	cfg := ReadContextConfig(t, fx.XDG, "tmplctx")
	if len(cfg.Projects) != 1 {
		t.Fatalf("persisted projects = %+v, want exactly one", cfg.Projects)
	}
	if cfg.Projects[0].DestBranch != "custom-{workspace}" || cfg.Projects[0].WorktreeDir != "{project}-checkout" {
		t.Fatalf("persisted project = %+v, want the custom templates round-tripped verbatim", cfg.Projects[0])
	}

	fx.MustRun("", "create", "demo-tmpl", "--project", "svc")

	wsRoot := filepath.Join(workspacesRoot, "demo-tmpl")
	worktree := filepath.Join(wsRoot, "svc-checkout")
	if !PathExists(t, worktree) {
		t.Fatalf("worktree does not exist on disk at the resolved custom path %s", worktree)
	}
	wantBranch := "custom-demo-tmpl"
	if got := CurrentBranch(t, worktree); got != wantBranch {
		t.Fatalf("worktree branch = %q, want %q (resolved from the custom dest_branch template)", got, wantBranch)
	}
}
