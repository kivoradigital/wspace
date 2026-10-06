// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// TestCLI_VerboseFlag_SurfacesOpErrorDiagnostics covers defect 3: without
// --verbose, an *OpError renders only its catalog text (ADR D4: raw
// subprocess output never reaches a user by default); with --verbose, the
// diagnostic layer OpError.Details() exists for ("used by `doctor` and
// `--verbose`", internal/domain/errors.go) must additionally surface the
// Op, the Code, Details(), and the unwrapped error chain.
func TestCLI_VerboseFlag_SurfacesOpErrorDiagnostics(t *testing.T) {
	fx := newFixture(t)
	wrapped := errors.New("exit status 128")
	fx.Git.IsMainCloneFunc = func(dir domain.Path) (bool, error) {
		return false, domain.NewOpError(
			"git.is_main_clone", domain.CodeRefNotFound, "main",
			"fatal: ambiguous argument 'origin/main': unknown revision", wrapped,
		)
	}

	_, stderr, code := run(fx.RT, "", "create", "ws1")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}
	if !strings.Contains(stderr, "git ref not found") {
		t.Fatalf("stderr = %q, want the ForCode(CodeRefNotFound) catalog text", stderr)
	}
	for _, leak := range []string{"git.is_main_clone", "ref_not_found", "fatal: ambiguous argument", "exit status 128"} {
		if strings.Contains(stderr, leak) {
			t.Fatalf("stderr = %q, leaked diagnostic detail %q without --verbose", stderr, leak)
		}
	}

	_, verboseStderr, code := run(fx.RT, "", "create", "ws1", "--verbose")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}
	for _, want := range []string{"git.is_main_clone", "ref_not_found", "fatal: ambiguous argument 'origin/main': unknown revision", "exit status 128"} {
		if !strings.Contains(verboseStderr, want) {
			t.Fatalf("verbose stderr = %q, want it to contain %q", verboseStderr, want)
		}
	}
}

// TestCLI_VerboseFlag_ShowsRawErrorForNonOpError covers the other half of
// defect 3: an error that never became an *OpError renders as the bare
// ErrUnknown catalog text ("an unexpected error occurred") today, with
// nothing else to go on. --verbose must show the actual Go error text
// instead, because the catalog has nothing useful to say about it.
func TestCLI_VerboseFlag_ShowsRawErrorForNonOpError(t *testing.T) {
	fx := newFixture(t)

	// "foo bar" is not flag-like (cobra parses it as one positional arg)
	// but fails domain.NewBranchName (embedded space), which is a plain
	// wrapped error, never an *OpError.
	_, stderr, code := run(fx.RT, "", "create", "foo bar")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero for an invalid workspace name")
	}
	if !strings.Contains(stderr, "an unexpected error occurred") {
		t.Fatalf("stderr = %q, want the ErrUnknown catalog text", stderr)
	}
	if strings.Contains(stderr, "invalid branch name") {
		t.Fatalf("stderr = %q, leaked the raw Go error without --verbose", stderr)
	}

	_, verboseStderr, code := run(fx.RT, "", "create", "foo bar", "--verbose")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}
	if !strings.Contains(verboseStderr, "invalid branch name") {
		t.Fatalf("verbose stderr = %q, want the raw Go error text surfaced", verboseStderr)
	}
}
