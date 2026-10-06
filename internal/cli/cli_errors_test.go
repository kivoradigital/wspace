// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestCLI_ErrorMessages_RenderThroughCatalog covers tasks.md 4b.15-4b.16:
// every OpError/ErrCode must render via messages.ForCode, never raw
// stderr. jump on an unknown workspace is domain.CodeWorkspaceNotFound.
func TestCLI_ErrorMessages_RenderThroughCatalog(t *testing.T) {
	fx := newFixture(t)

	_, stderr, code := run(fx.RT, "", "jump", "ghost")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}
	if strings.Contains(stderr, "config.load_manifest") || strings.Contains(stderr, "OpError") {
		t.Fatalf("stderr = %q, leaked an internal Op/type name instead of catalog text", stderr)
	}
	if !strings.Contains(stderr, "workspace not found") {
		t.Fatalf("stderr = %q, want the ForCode(CodeWorkspaceNotFound) catalog text", stderr)
	}
}

// TestCLI_ErrorMessages_NoContextHintsWizard covers CodeNoContext's extra
// hint line.
func TestCLI_ErrorMessages_NoContextHintsWizard(t *testing.T) {
	fx := newFixture(t)
	// Replace the seeded store with an empty one: no contexts at all.
	fx.RT.Deps.Store = portstest.NewFakeConfigStore()

	_, stderr, code := run(fx.RT, "", "list")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero (no context configured)")
	}
	if !strings.Contains(stderr, "no context is configured") {
		t.Fatalf("stderr = %q, want the ErrNoContext catalog text", stderr)
	}
	if !strings.Contains(stderr, "context create") {
		t.Fatalf("stderr = %q, want the CLINoContext hint", stderr)
	}
}

// TestCLI_JSONFlag_RejectedOnUnsupportedCommand covers the cli-surface
// spec's "Non-JSON commands remain human-output only" scenario.
func TestCLI_JSONFlag_RejectedOnUnsupportedCommand(t *testing.T) {
	fx := newFixture(t)
	_, stderr, code := run(fx.RT, "", "doctor", "--json")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero: --json is not supported on doctor")
	}
	if !strings.Contains(stderr, "--json") {
		t.Fatalf("stderr = %q, want it to name --json as unsupported", stderr)
	}
}
