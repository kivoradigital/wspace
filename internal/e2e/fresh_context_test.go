// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestFreshContext_ListAndDoctorSucceedBeforeFirstWorkspace is CRITICAL-3's
// own regression test (verify-report.md): `ws list` and `ws doctor` must
// both behave sanely on the most ordinary state a brand-new context is
// in — zero workspaces, and workspaces_root not yet created on disk at
// all (it is only created by CreateWorkspace's mutate phase, the first
// time `ws create` succeeds). Every other e2e test in this package runs
// `create` at least once before touching `list`/`doctor`, which always
// makes workspaces_root exist first — this test deliberately never does,
// so it is the one live reproduction of the exact state a new user is in
// immediately after `ws context create`.
func TestFreshContext_ListAndDoctorSucceedBeforeFirstWorkspace(t *testing.T) {
	fx := NewFixture(t)

	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	stdin := strings.Join([]string{
		"fresh-ctx",
		workspacesRoot,
		"",  // projects_root -> skip scanning
		"",  // ignore_patterns -> none
		"",  // base branch -> default main
		"",  // copy env default -> yes
		"",  // fetch before create -> yes
		"n", // decline manual project registration
		"",
	}, "\n")
	fx.MustRun(stdin, "context", "create")

	if PathExists(t, workspacesRoot) {
		t.Fatalf("workspaces_root %s already exists; this test needs it genuinely absent to reproduce CRITICAL-3", workspacesRoot)
	}

	stdout, stderr, code := fx.Run("", "list")
	if code != 0 {
		t.Fatalf("ws list on a fresh context (workspaces_root not yet created) exit=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "no workspaces found") {
		t.Fatalf("ws list stdout = %q, want the empty-list message", stdout)
	}

	stdoutJSON, stderrJSON, codeJSON := fx.Run("", "list", "--json")
	if codeJSON != 0 {
		t.Fatalf("ws list --json on a fresh context exit=%d\nstdout=%s\nstderr=%s", codeJSON, stdoutJSON, stderrJSON)
	}
	if strings.TrimSpace(stdoutJSON) != "[]" {
		t.Fatalf("ws list --json stdout = %q, want an empty JSON array", stdoutJSON)
	}

	stdoutDoctor, stderrDoctor, codeDoctor := fx.Run("", "doctor")
	if codeDoctor != 0 {
		t.Fatalf("ws doctor on a fresh context exit=%d\nstdout=%s\nstderr=%s", codeDoctor, stdoutDoctor, stderrDoctor)
	}
}
