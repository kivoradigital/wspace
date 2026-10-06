// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// TestCLI_Create_HumanOutput covers tasks.md 4b.5: `create`'s
// human-readable output, golden-compared.
func TestCLI_Create_HumanOutput(t *testing.T) {
	fx := newFixture(t)

	stdout, stderr, code := run(fx.RT, "", "create", "ws1", "--branch", "feature-x")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	compareGolden(t, "create.golden", stdout)
}

// TestCLI_Create_PartialFailureRendersGuidance covers phase 4a's
// no-automatic-rollback risk (its own "What Phase 4b needs to know"): a
// mutate-phase failure must tell the user what was possibly created and
// what to run next, never fail silently.
func TestCLI_Create_PartialFailureRendersGuidance(t *testing.T) {
	fx := newFixture(t)
	fx.Git.FetchFunc = func(repo domain.Path, remote string) error {
		return domain.NewOpError("git.fetch", domain.CodeGitFailed, string(repo), "", nil)
	}

	_, stderr, code := run(fx.RT, "", "create", "ws1", "--branch", "feature-x")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero on a mutate-phase failure")
	}
	if !strings.Contains(stderr, "/fixture/workspaces/ws1") {
		t.Fatalf("stderr = %q, want it to mention the partially-created workspace path", stderr)
	}
	if !strings.Contains(stderr, "repair") || !strings.Contains(stderr, "destroy") {
		t.Fatalf("stderr = %q, want it to suggest 'repair' and 'destroy --force'", stderr)
	}
}

// TestCLI_Create_WithoutBranchDefaultsToWorkspaceName is the end-to-end
// regression test for defect 2: "ws create <name>" without "--branch" used
// to fail opaquely (domain.NewBranchName("") on an empty flag value). It
// now produces a real, usable branch — the resolved branch_prefix (empty
// here, since the fixture's context sets none) concatenated with the
// workspace name — verified through the same Execute entry point the real
// binary uses, all the way down to the persisted manifest.
func TestCLI_Create_WithoutBranchDefaultsToWorkspaceName(t *testing.T) {
	fx := newFixture(t)

	stdout, stderr, code := run(fx.RT, "", "create", "payments-fix")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("stdout is empty: create must still report its result")
	}

	m, err := fx.Store.LoadManifest(context.Background(), domain.Path("/fixture/workspaces/payments-fix"))
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}
	if string(m.Workspace.Branch) != "payments-fix" {
		t.Fatalf("manifest workspace branch = %q, want %q", m.Workspace.Branch, "payments-fix")
	}
	if len(m.Workspace.Repos) != 1 || string(m.Workspace.Repos[0].Branch) != "payments-fix" {
		t.Fatalf("manifest repos = %+v, want the one repo checked out on %q", m.Workspace.Repos, "payments-fix")
	}
}

// TestCLI_Create_WithoutBranchAppliesContextBranchPrefix covers the other
// half of defect 2: branch_prefix was declared, stored and round-tripped
// through every config layer but never read by anything. This proves a
// context-level branch_prefix is now honored when "--branch" is omitted.
func TestCLI_Create_WithoutBranchAppliesContextBranchPrefix(t *testing.T) {
	fx := newFixture(t)
	prefix := "feature/"
	fx.Store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: "/fixture/workspaces",
		Defaults:       domain.Options{BranchPrefix: &prefix},
		Projects: []domain.Project{
			{Key: "svc", SourceDir: "/fixture/src/svc"},
		},
	})

	_, stderr, code := run(fx.RT, "", "create", "payments-fix")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}

	m, err := fx.Store.LoadManifest(context.Background(), domain.Path("/fixture/workspaces/payments-fix"))
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}
	if string(m.Workspace.Branch) != "feature/payments-fix" {
		t.Fatalf("manifest workspace branch = %q, want %q", m.Workspace.Branch, "feature/payments-fix")
	}
}
