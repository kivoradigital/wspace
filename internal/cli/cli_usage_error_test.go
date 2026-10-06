// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"strings"
	"testing"
)

// TestCLI_UnknownFlag_SurfacesCobraMessage covers defect B: "ws destroy foo
// --yes" (there is no --yes flag; it is --force) rendered as the generic
// ErrUnknown catalog fallback "an unexpected error occurred", discarding
// cobra's own precise "unknown flag: --yes" message and its usage hint.
func TestCLI_UnknownFlag_SurfacesCobraMessage(t *testing.T) {
	fx := newFixture(t)
	_, stderr, code := run(fx.RT, "", "destroy", "foo", "--yes")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero: --yes is not a real flag")
	}
	if strings.Contains(stderr, "an unexpected error occurred") {
		t.Fatalf("stderr = %q, want cobra's own flag error, not the ErrUnknown fallback", stderr)
	}
	if !strings.Contains(stderr, "unknown flag: --yes") {
		t.Fatalf("stderr = %q, want cobra's own \"unknown flag: --yes\" message", stderr)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Fatalf("stderr = %q, want the command's usage hint appended", stderr)
	}
}

// TestCLI_MissingRequiredArg_SurfacesCobraMessage covers the Args-validator
// half of the same defect: cobra's own arg-count message must reach the
// user too, not the ErrUnknown fallback — it never even reaches RunE, so
// it can't have become a *cliError or a domain-coded error either.
func TestCLI_MissingRequiredArg_SurfacesCobraMessage(t *testing.T) {
	fx := newFixture(t)
	_, stderr, code := run(fx.RT, "", "destroy")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero: destroy requires a workspace name")
	}
	if strings.Contains(stderr, "an unexpected error occurred") {
		t.Fatalf("stderr = %q, want cobra's own arg-count error, not the ErrUnknown fallback", stderr)
	}
	if !strings.Contains(stderr, "arg(s)") {
		t.Fatalf("stderr = %q, want cobra's own arg-count message", stderr)
	}
}

// TestCLI_OperationalError_StillUsesCatalog is the control: an error that
// does reach RunE (a real operational failure) must keep going through the
// existing catalog/ForCode path unchanged — defect B's fix must not turn
// every RunE error into raw cobra-style text.
func TestCLI_OperationalError_StillUsesCatalog(t *testing.T) {
	fx := newFixture(t)
	_, stderr, code := run(fx.RT, "", "jump", "ghost")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}
	if !strings.Contains(stderr, "workspace not found") {
		t.Fatalf("stderr = %q, want the ForCode(CodeWorkspaceNotFound) catalog text unchanged", stderr)
	}
}

// TestCLI_VerboseFlag_DoesNotDuplicateUsageError covers "while you are in
// there": --verbose must not make a usage error worse. Cobra's message and
// the usage hint are already fully shown without --verbose, so --verbose
// on a usage error is a deliberate no-op, not a second, redundant block.
func TestCLI_VerboseFlag_DoesNotDuplicateUsageError(t *testing.T) {
	fx := newFixture(t)
	_, plain, _ := run(fx.RT, "", "destroy", "foo", "--yes")
	_, verbose, _ := run(fx.RT, "", "destroy", "foo", "--verbose", "--yes")
	if plain != verbose {
		t.Fatalf("verbose stderr = %q, want it identical to the non-verbose usage error %q", verbose, plain)
	}
}
