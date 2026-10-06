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

// TestRecursiveScan_FindsNestedProjectAndSkipsLinkedWorktree drives the
// real ws binary through this change's own recursive-discovery fix: a
// repository nested two levels below the projects root ("team/api") used
// to be invisible to "context create"'s scan, which only ever looked one
// level deep (deps.FS.ListDirs(c.ProjectsRoot)). This test proves the fix
// against real git repositories on real disk, not fakes: a main clone
// directly under the root, a second one nested under a plain container
// directory, and a real linked worktree (via a genuine `git worktree add`)
// that must be reported and skipped, never registered as a project of its
// own.
func TestRecursiveScan_FindsNestedProjectAndSkipsLinkedWorktree(t *testing.T) {
	fx := NewFixture(t)

	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatalf("mkdir projects root: %v", err)
	}

	// "solo" sits directly under the root (depth 1) — visible even to the
	// old single-level scan.
	solo := NewGitProject(t, projectsRoot, "solo", "develop")

	// "team/api" sits two levels below the root, under a plain container
	// directory ("team") that is not itself a git repository — exactly
	// the shape the old ListDirs-only scan could never see.
	teamDir := filepath.Join(projectsRoot, "team")
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatalf("mkdir team dir: %v", err)
	}
	NewGitProject(t, teamDir, "api", "develop")

	// A real linked worktree off of "solo", placed directly under the
	// root: it must be found by the walk (it is structurally a directory
	// whose ".git" is a file), reported, and never registered — and,
	// since it is itself a git repository, never descended into either.
	linkedWorktree := filepath.Join(projectsRoot, "solo-wt")
	runGit(t, solo.Main, "worktree", "add", "-q", "-b", "solo-wt-branch", linkedWorktree)

	stdin := strings.Join([]string{
		"e2e-recursive", // context name
		workspacesRoot,
		projectsRoot,
		"",    // ignore_patterns -> none
		"",    // base branch -> default "develop"
		"",    // copy env files by default -> default yes
		"",    // fetch before create by default -> default yes
		"all", // pick every offered candidate: "solo" and "team/api" (the linked worktree is never offered)
		"",    // project key for "solo" -> default "solo"
		"",    // solo origin_branch -> inherit
		"",    // solo dest_branch -> inherit
		"",    // solo worktree_dir -> inherit
		"",    // project key for "team/api" -> default "api" (unique among the two offered candidates)
		"",    // api origin_branch -> inherit
		"",    // api dest_branch -> inherit
		"",    // api worktree_dir -> inherit
		"",
	}, "\n")

	stdout, stderr := fx.MustRun(stdin, "context", "create")
	if !strings.Contains(stdout, "e2e-recursive") {
		t.Fatalf("context create stdout = %q, want it to confirm the context was created", stdout)
	}
	if !strings.Contains(stderr, "solo-wt") {
		t.Fatalf("context create stderr = %q, want a notice naming the skipped linked worktree %q", stderr, linkedWorktree)
	}

	// Real, observable proof both scanned candidates were genuinely
	// registered as usable projects: creating a workspace naming both by
	// key actually materializes a worktree for each, including the one
	// nested two levels below the projects root.
	fx.MustRun("", "create", "demo", "--project", "solo", "--project", "api")

	wsRoot := filepath.Join(workspacesRoot, "demo")
	soloWorktree := filepath.Join(wsRoot, "solo")
	apiWorktree := filepath.Join(wsRoot, "api")
	if !PathExists(t, soloWorktree) {
		t.Fatalf("worktree %s does not exist on disk", soloWorktree)
	}
	if !PathExists(t, apiWorktree) {
		t.Fatalf("worktree %s does not exist on disk (the nested \"team/api\" project was not registered)", apiWorktree)
	}

	m := ReadManifest(t, wsRoot)
	if len(m.Workspace.Repos) != 2 {
		t.Fatalf("manifest repos = %+v, want exactly 2 (solo and api)", m.Workspace.Repos)
	}
}
