// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"strings"
	"testing"
)

// TestExecuteBindsReporterForReporterDrivenCommands guards the composition
// gap that shipped undetected through phase 4b: Runtime is built before the
// invocation's writers exist, so its Reporter fields arrive nil, and a use
// case that reports its result dereferences that nil.
//
// Every other test in this package injects fakes and asserts on rendered
// text, which cannot observe the gap — the nil only matters on the path
// where a use case succeeds far enough to report. This test drives a
// successful create through the public Execute entry point with a Runtime
// whose Reporter fields are left nil, exactly as the composition root
// leaves them.
func TestExecuteBindsReporterForReporterDrivenCommands(t *testing.T) {
	f := newFixture(t)
	if f.RT.Deps.Reporter != nil || f.RT.ExecDeps.Reporter != nil {
		t.Fatal("fixture must leave Reporter nil to reproduce the composition root's shape")
	}

	stdout, stderr, exitCode := run(f.RT, "", "create", "feature-x")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Error("stdout is empty: a successful create must report its result through the bound reporter")
	}
	if f.RT.Deps.Reporter == nil {
		t.Error("Deps.Reporter is still nil after Execute")
	}
	if f.RT.ExecDeps.Reporter == nil {
		t.Error("ExecDeps.Reporter is still nil after Execute")
	}
}

// TestExecuteKeepsAnAlreadyBoundReporter proves the binding never overwrites
// a Reporter a caller supplied deliberately.
func TestExecuteKeepsAnAlreadyBoundReporter(t *testing.T) {
	f := newFixture(t)
	own := &countingReporter{}
	f.RT.Deps.Reporter = own

	if _, _, exitCode := run(f.RT, "", "create", "feature-y"); exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if f.RT.Deps.Reporter != own {
		t.Error("Execute replaced a caller-supplied Reporter")
	}
	if own.results == 0 {
		t.Error("the caller-supplied Reporter received no Result call")
	}
}
