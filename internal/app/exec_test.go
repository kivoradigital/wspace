// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func execManifest(t *testing.T, fs *portstest.FakeFS, store *portstest.FakeConfigStore, aliases ...string) domain.Path {
	t.Helper()
	wsRoot := fs.Paths().Home.Join("workspaces", "ws1")
	var repos []domain.RepoEntry
	for _, alias := range aliases {
		repos = append(repos, domain.RepoEntry{Alias: alias, Project: domain.ProjectKey(alias)})
	}
	store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{Name: "ws1", Root: wsRoot, Repos: repos}})
	return wsRoot
}

func TestExec_NoShellInterpretation(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot := execManifest(t, fs, store, "api")

	// A single argv element containing shell metacharacters must be looked
	// up as one literal (nonexistent) executable name, never split and
	// interpreted by a shell (design.md §13: "Arbitrary command execution").
	deps := app.ExecDeps{Store: store, Reporter: reporter}
	_, err := app.Exec(context.Background(), deps, app.ExecInput{
		WorkspaceRoot: wsRoot,
		Argv:          []string{"echo hello; exit 7"},
	})
	if err == nil {
		t.Fatal("Exec() error = nil, want a \"command not found\"-style error proving no shell interpretation ran the embedded exit")
	}
}

func TestExec_CommandStartingWithDash(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot := execManifest(t, fs, store, "api")

	var gotArgv []string
	run := func(_ context.Context, _ domain.Path, argv []string, _, _ io.Writer) (int, error) {
		gotArgv = argv
		return 0, nil
	}

	deps := app.ExecDeps{Store: store, Reporter: reporter, Run: run}
	in := app.ExecInput{WorkspaceRoot: wsRoot, Argv: []string{"--upload-pack=evil", "arg"}}
	if _, err := app.Exec(context.Background(), deps, in); err != nil {
		t.Fatalf("Exec() unexpected error: %v", err)
	}
	if len(gotArgv) != 2 || gotArgv[0] != "--upload-pack=evil" {
		t.Fatalf("runner received argv = %+v, want the dash-leading command passed through unchanged", gotArgv)
	}
}

func TestExec_ReturnsLastNonZeroExit(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot := execManifest(t, fs, store, "api", "web", "worker")

	codes := []int{2, 0, 5}
	i := 0
	run := func(_ context.Context, _ domain.Path, _ []string, _, _ io.Writer) (int, error) {
		c := codes[i]
		i++
		return c, nil
	}

	deps := app.ExecDeps{Store: store, Reporter: reporter, Run: run}
	result, err := app.Exec(context.Background(), deps, app.ExecInput{WorkspaceRoot: wsRoot, Argv: []string{"true"}})
	if err != nil {
		t.Fatalf("Exec() unexpected error: %v", err)
	}
	if result.ExitCode != 5 {
		t.Fatalf("result.ExitCode = %d, want 5 (the last non-zero exit across all repos)", result.ExitCode)
	}
}

func TestExec_CancelsOnContext(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot := execManifest(t, fs, store, "api", "web")

	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	run := func(c context.Context, _ domain.Path, _ []string, _, _ io.Writer) (int, error) {
		calls++
		cancel()
		<-c.Done()
		return 0, c.Err()
	}

	deps := app.ExecDeps{Store: store, Reporter: reporter, Run: run}
	if _, err := app.Exec(ctx, deps, app.ExecInput{WorkspaceRoot: wsRoot, Argv: []string{"sleep"}}); err == nil {
		t.Fatal("Exec() error = nil, want the propagated context cancellation error")
	}
	if calls != 1 {
		t.Fatalf("runner called %d times, want exactly 1 (must stop at the cancelled repo, not continue to the next)", calls)
	}
}

// TestExec_StreamsChildOutputAndPrintsPerRepoHeader covers defect A: the
// real binary printed nothing at all for "ws exec <workspace> -- /bin/echo
// hello" because commandRunner had nowhere to put the child's output and
// Exec never told the reporter which repo was running. Both must now
// happen: the child's stdout/stderr reach the writers the caller supplied,
// and a header names each repo before its command runs.
func TestExec_StreamsChildOutputAndPrintsPerRepoHeader(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot := execManifest(t, fs, store, "api", "web")

	var stdout, stderr strings.Builder
	run := func(_ context.Context, dir domain.Path, _ []string, out, errW io.Writer) (int, error) {
		_, _ = io.WriteString(out, "stdout from "+dir.Base()+"\n")
		_, _ = io.WriteString(errW, "stderr from "+dir.Base()+"\n")
		return 0, nil
	}

	deps := app.ExecDeps{Store: store, Reporter: reporter, Run: run, Stdout: &stdout, Stderr: &stderr}
	if _, err := app.Exec(context.Background(), deps, app.ExecInput{WorkspaceRoot: wsRoot, Argv: []string{"true"}}); err != nil {
		t.Fatalf("Exec() unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), "stdout from api") || !strings.Contains(stdout.String(), "stdout from web") {
		t.Fatalf("stdout = %q, want each repo's child stdout forwarded to the caller-provided writer", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stderr from api") || !strings.Contains(stderr.String(), "stderr from web") {
		t.Fatalf("stderr = %q, want each repo's child stderr forwarded to the caller-provided writer", stderr.String())
	}
	if len(reporter.Steps) != 2 {
		t.Fatalf("Steps = %+v, want one CLIExecRepoHeader step per repo", reporter.Steps)
	}
	for i, alias := range []string{"api", "web"} {
		s := reporter.Steps[i]
		if s.Key != messages.CLIExecRepoHeader || len(s.Args) != 1 || s.Args[0] != alias {
			t.Fatalf("Steps[%d] = %+v, want a CLIExecRepoHeader step naming %q", i, s, alias)
		}
	}
}

// TestExec_NonZeroExitReportsWhichRepoAndCode covers the other half of
// defect A: "ws exec ... -- /bin/false" reported a bare "command exited
// with code 1" naming no repository at all. Every repo whose command
// exits non-zero must be individually warned about, by alias and code.
func TestExec_NonZeroExitReportsWhichRepoAndCode(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot := execManifest(t, fs, store, "api", "web")

	run := func(_ context.Context, _ domain.Path, _ []string, _, _ io.Writer) (int, error) {
		return 3, nil
	}
	deps := app.ExecDeps{Store: store, Reporter: reporter, Run: run}
	result, err := app.Exec(context.Background(), deps, app.ExecInput{WorkspaceRoot: wsRoot, Argv: []string{"x"}})
	if err != nil {
		t.Fatalf("Exec() unexpected error: %v", err)
	}
	if result.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", result.ExitCode)
	}
	if len(reporter.Warnings) != 2 {
		t.Fatalf("Warnings = %+v, want one CLIExecExitNonZero warning per failing repo", reporter.Warnings)
	}
	for i, alias := range []string{"api", "web"} {
		w := reporter.Warnings[i]
		if w.Key != messages.CLIExecExitNonZero || len(w.Args) != 2 || w.Args[0] != alias || w.Args[1] != 3 {
			t.Fatalf("Warnings[%d] = %+v, want alias %q and code 3", i, w, alias)
		}
	}
}

// TestExec_LaunchFailureNamesTheRepo covers the case where the child never
// even starts (e.g. the binary named after "--" does not exist): today
// that raw *exec.Error reaches renderError uncoded and falls back to
// "an unexpected error occurred" with no indication of which repo or why.
// It must instead become a domain.OpError coded CodeExecFailed, naming the
// failing repo as its Subject.
func TestExec_LaunchFailureNamesTheRepo(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot := execManifest(t, fs, store, "api")

	launchErr := errors.New("fork/exec /no/such/binary: no such file or directory")
	run := func(_ context.Context, _ domain.Path, _ []string, _, _ io.Writer) (int, error) {
		return 0, launchErr
	}
	deps := app.ExecDeps{Store: store, Reporter: reporter, Run: run}
	_, err := app.Exec(context.Background(), deps, app.ExecInput{WorkspaceRoot: wsRoot, Argv: []string{"/no/such/binary"}})
	if err == nil {
		t.Fatal("Exec() error = nil, want the wrapped launch failure")
	}
	var opErr *domain.OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("err = %v (%T), want an *domain.OpError naming the failing repo", err, err)
	}
	if opErr.Subject != "api" {
		t.Fatalf("OpError.Subject = %q, want the failing repo's alias %q", opErr.Subject, "api")
	}
	if opErr.Code != domain.CodeExecFailed {
		t.Fatalf("OpError.Code = %q, want %q", opErr.Code, domain.CodeExecFailed)
	}
}
