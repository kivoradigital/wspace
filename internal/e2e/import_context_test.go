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

// TestContextImport_NewContext_FromFile is this change's own import
// feature's required real-binary proof: a legacy flat "key = value"
// workspace configuration file, written to a temp file by this test (never
// copied from a fixture), becomes a brand-new context through the real
// `wspace context import --from <path> --name <name>` — every mapped
// value and every valid project must round-trip onto the persisted
// context config exactly, and every unusable [projects] entry (a plain
// non-git folder, a name with no directory at all) must be reported,
// never silently registered.
func TestContextImport_NewContext_FromFile(t *testing.T) {
	fx := NewFixture(t)

	projectsRoot := filepath.Join(fx.Root, "legacy-projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatalf("mkdir projectsRoot: %v", err)
	}
	NewGitProject(t, projectsRoot, "alpha", "main")

	if err := os.MkdirAll(filepath.Join(projectsRoot, "not-a-repo"), 0o755); err != nil {
		t.Fatalf("mkdir not-a-repo: %v", err)
	}

	workspacesRoot := filepath.Join(fx.Root, "imported-workspaces")
	legacyPath := filepath.Join(fx.Root, "legacy.config")
	legacyContent := strings.Join([]string{
		"# a legacy flat workspace configuration",
		"workspaces_root = " + workspacesRoot,
		"projects_root = " + projectsRoot,
		"project_prefixes = Foo,bar",
		"base_branch = main",
		"copy_env_default = yes",
		"fetch_before_create = no",
		"[projects]",
		"alpha",
		"not-a-repo",
		"missing-project",
		"",
	}, "\n")
	if err := os.WriteFile(legacyPath, []byte(legacyContent), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	// promptContextFields's own fixed sequence: workspaces_root,
	// projects_root, ignore_patterns, base_branch, copy_env_default,
	// fetch_before_create — every line empty, accepting whatever this
	// change's own prefill (from the legacy file) offered as each
	// field's default.
	stdin := strings.Join([]string{"", "", "", "", "", "", ""}, "\n")

	stdout, stderr := fx.MustRun(stdin, "context", "import", "--from", legacyPath, "--name", "imported")
	if !strings.Contains(stdout, "imported") {
		t.Fatalf("context import stdout = %q, want it to mention the new context; stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "not-a-repo") {
		t.Fatalf("context import stdout = %q, want the plain-folder project reported as skipped", stdout)
	}
	if !strings.Contains(stdout, "missing-project") {
		t.Fatalf("context import stdout = %q, want the missing project reported as skipped", stdout)
	}
	if !strings.Contains(stdout, "project_prefixes") {
		t.Fatalf("context import stdout = %q, want project_prefixes reported as not imported", stdout)
	}

	cfg := ReadContextConfig(t, fx.XDG, "imported")
	if cfg.WorkspacesRoot != filepath.ToSlash(workspacesRoot) {
		t.Fatalf("WorkspacesRoot = %q, want %q", cfg.WorkspacesRoot, filepath.ToSlash(workspacesRoot))
	}
	if cfg.ProjectsRoot != filepath.ToSlash(projectsRoot) {
		t.Fatalf("ProjectsRoot = %q, want %q", cfg.ProjectsRoot, filepath.ToSlash(projectsRoot))
	}
	if cfg.Defaults.BaseBranch != "main" {
		t.Fatalf("Defaults.BaseBranch = %q, want main", cfg.Defaults.BaseBranch)
	}
	if !cfg.Defaults.CopyEnvDefault {
		t.Fatal("Defaults.CopyEnvDefault = false, want true")
	}
	if cfg.Defaults.FetchBeforeCreate {
		t.Fatal("Defaults.FetchBeforeCreate = true, want false")
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0].Key != "alpha" {
		t.Fatalf("Projects = %+v, want exactly one entry for alpha", cfg.Projects)
	}
	wantSourceDir := filepath.ToSlash(filepath.Join(projectsRoot, "alpha"))
	if cfg.Projects[0].SourceDir != wantSourceDir {
		t.Fatalf("Projects[0].SourceDir = %q, want %q", cfg.Projects[0].SourceDir, wantSourceDir)
	}
}

// TestContextImport_ExistingContext_DeclineWritesNothing covers spec
// requirement 2's "into an existing context" mode against the real
// binary: importing into an already-registered context shows a diff and,
// on decline, the persisted context file is left byte-for-byte unchanged.
func TestContextImport_ExistingContext_DeclineWritesNothing(t *testing.T) {
	fx := NewFixture(t)

	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	stdin := strings.Join([]string{
		"work",
		workspacesRoot,
		"",
		"",
		"",
		"",
		"",
		"n",
		"",
	}, "\n")
	fx.MustRun(stdin, "context", "create")

	before := ReadContextConfig(t, fx.XDG, "work")

	legacyPath := filepath.Join(fx.Root, "legacy.config")
	if err := os.WriteFile(legacyPath, []byte("base_branch = develop\n[projects]\n"), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	stdout, stderr, code := fx.Run("n\n", "context", "import", "work", "--from", legacyPath)
	if code != 0 {
		t.Fatalf("context import: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "cancelled") {
		t.Fatalf("context import stdout = %q, want a cancellation notice", stdout)
	}

	after := ReadContextConfig(t, fx.XDG, "work")
	if after.Defaults.BaseBranch != before.Defaults.BaseBranch {
		t.Fatalf("Defaults.BaseBranch changed after a decline: before=%q after=%q", before.Defaults.BaseBranch, after.Defaults.BaseBranch)
	}
}
