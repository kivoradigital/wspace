// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// TestCLI_Exec_StreamsChildOutputWithPerRepoHeader covers defect A at the
// full cobra-command level: "ws exec <workspace> -- <command>" reached the
// real binary's stdout/stderr with nothing on them at all, because
// commandRunner had no writers to write to. This proves cmd_exec.go wires
// ExecDeps.Stdout/Stderr to the invocation's actual writers, not only that
// internal/app.Exec accepts them (covered separately in internal/app).
func TestCLI_Exec_StreamsChildOutputWithPerRepoHeader(t *testing.T) {
	fx := newFixture(t)
	wsRoot := domain.Path("/fixture/workspaces/ws1")
	fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{
		Name: "ws1", Root: wsRoot,
		Repos: []domain.RepoEntry{{Alias: "svc"}},
	}})

	fx.RT.ExecDeps.Run = func(_ context.Context, _ domain.Path, _ []string, stdout, stderr io.Writer) (int, error) {
		_, _ = io.WriteString(stdout, "hello-from-child\n")
		_, _ = io.WriteString(stderr, "warn-from-child\n")
		return 0, nil
	}

	stdout, stderr, code := run(fx.RT, "", "exec", "ws1", "--", "irrelevant")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "hello-from-child") {
		t.Fatalf("stdout = %q, want the child's stdout forwarded", stdout)
	}
	if !strings.Contains(stdout, "svc") {
		t.Fatalf("stdout = %q, want a per-repo header naming svc", stdout)
	}
	if !strings.Contains(stderr, "warn-from-child") {
		t.Fatalf("stderr = %q, want the child's stderr forwarded", stderr)
	}
}

// TestCLI_Exec_NonZeroExitNamesRepoAndCode covers the other half of defect
// A: a failing repo's exit code must be attributed to that repo by name,
// not printed as a bare "command exited with code N".
func TestCLI_Exec_NonZeroExitNamesRepoAndCode(t *testing.T) {
	fx := newFixture(t)
	wsRoot := domain.Path("/fixture/workspaces/ws1")
	fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{
		Name: "ws1", Root: wsRoot,
		Repos: []domain.RepoEntry{{Alias: "svc"}},
	}})
	fx.RT.ExecDeps.Run = func(_ context.Context, _ domain.Path, _ []string, _, _ io.Writer) (int, error) {
		return 9, nil
	}

	_, stderr, code := run(fx.RT, "", "exec", "ws1", "--", "irrelevant")
	if code != 9 {
		t.Fatalf("exit code = %d, want 9", code)
	}
	if !strings.Contains(stderr, "svc") || !strings.Contains(stderr, "9") {
		t.Fatalf("stderr = %q, want it to name the repo (svc) and the exit code (9)", stderr)
	}
}

// TestCLI_Exec_LaunchFailureNamesRepoInsteadOfGenericError covers the
// "ws exec ... -- <nonexistent binary>" repro: a launch failure used to
// surface as the bare ErrUnknown catalog fallback ("an unexpected error
// occurred"), naming neither the repo nor the reason.
func TestCLI_Exec_LaunchFailureNamesRepoInsteadOfGenericError(t *testing.T) {
	fx := newFixture(t)
	wsRoot := domain.Path("/fixture/workspaces/ws1")
	fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{
		Name: "ws1", Root: wsRoot,
		Repos: []domain.RepoEntry{{Alias: "svc"}},
	}})

	_, stderr, code := run(fx.RT, "", "exec", "ws1", "--", "/no/such/binary-really-does-not-exist")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero: the binary does not exist")
	}
	if strings.Contains(stderr, "an unexpected error occurred") {
		t.Fatalf("stderr = %q, want the ErrExecFailed catalog text, not the generic ErrUnknown fallback", stderr)
	}
	if !strings.Contains(stderr, "svc") {
		t.Fatalf("stderr = %q, want it to name the failing repo (svc)", stderr)
	}
}
