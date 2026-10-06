// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// commandRunner abstracts one repo's command execution for Exec, letting
// tests substitute a deterministic function instead of a real subprocess.
// It is not one of the six canonical ports (design.md §4): unlike GitPort,
// running an arbitrary user command has no alternative implementation to
// swap, so a dedicated port would be ceremony without benefit.
//
// stdout/stderr are the writers the child's own output streams go to.
// Exec runs one repo's command to completion (defect A: previously there
// was nowhere for that output to go at all — the real binary printed
// nothing) before starting the next repo's, so writing straight through
// live as the child produces it is both simpler and more useful than
// capturing into a buffer and replaying it afterwards: there is nothing to
// interleave between repos (they never run concurrently), and a
// long-running child (an installer, a test suite) shows its progress in
// real time instead of only once the whole exec finishes.
type commandRunner func(ctx context.Context, dir domain.Path, argv []string, stdout, stderr io.Writer) (exitCode int, err error)

// ExecDeps bundles Exec's dependencies. Run defaults to a real
// exec.CommandContext-backed runner when nil. Stdout/Stderr default to
// io.Discard when nil, which keeps every test that never sets them
// working unchanged; the real CLI entry point (cmd_exec.go) always
// supplies the invocation's actual stdout/stderr so the child's output
// reaches the user.
type ExecDeps struct {
	Store    ports.ConfigStore
	Reporter ports.Reporter
	Run      commandRunner
	Stdout   io.Writer
	Stderr   io.Writer
}

// ExecInput parameterizes Exec. Argv[0] is the command; the rest are its
// arguments, passed through unmodified (design.md §13: "Arbitrary command
// execution").
type ExecInput struct {
	WorkspaceRoot domain.Path
	Argv          []string
}

// ExecResult carries the last non-zero exit code observed across every
// repo, or 0 if every repo's command exited cleanly.
type ExecResult struct {
	ExitCode int
}

// Exec runs Argv in every repo of the target workspace, in order, with no
// shell involved anywhere: argv is passed directly to exec.CommandContext,
// so a command or argument starting with "-" or containing shell
// metacharacters is never reinterpreted (design.md §13: "Arbitrary command
// execution (ws exec)"). Each repo invocation shares the caller's context,
// so cancellation stops Exec at the in-flight repo without starting the
// next one, and the last non-zero exit code observed is returned.
//
// Before each repo's command runs, Reporter.Step announces which repo is
// running (defect A: interleaved multi-repo output is unattributable
// without this). A non-zero exit is reported per repo via Reporter.Warn,
// naming the repo and the code, instead of only a single opaque summary
// at the end. A repo whose command cannot even be started (the binary
// named after "--" does not exist, permissions deny it, etc.) is
// distinguished from a normal non-zero exit: it becomes a
// domain.OpError coded CodeExecFailed naming the repo as its Subject, so
// it renders through the catalog instead of falling back to the generic
// "an unexpected error occurred" a bare *exec.Error produced before.
func Exec(ctx context.Context, deps ExecDeps, in ExecInput) (ExecResult, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return ExecResult{}, err
	}

	run := deps.Run
	if run == nil {
		run = runRealCommand
	}
	stdout := deps.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := deps.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	lastNonZero := 0
	for _, repo := range manifest.Workspace.Repos {
		if ctx.Err() != nil {
			return ExecResult{}, ctx.Err()
		}
		deps.Reporter.Step(messages.CLIExecRepoHeader, repo.Alias)

		worktree := in.WorkspaceRoot.Join(repo.Alias)
		code, err := run(ctx, worktree, in.Argv, stdout, stderr)
		if err != nil {
			if ctx.Err() != nil {
				return ExecResult{}, ctx.Err()
			}
			return ExecResult{}, domain.NewOpError("exec.run", domain.CodeExecFailed, repo.Alias, err.Error(), err)
		}
		if code != 0 {
			lastNonZero = code
			deps.Reporter.Warn(messages.CLIExecExitNonZero, repo.Alias, code)
		}
	}

	return ExecResult{ExitCode: lastNonZero}, nil
}

// runRealCommand is the default commandRunner: a direct
// exec.CommandContext invocation, never a shell.
func runRealCommand(ctx context.Context, dir domain.Path, argv []string, stdout, stderr io.Writer) (int, error) {
	if len(argv) == 0 {
		return 0, fmt.Errorf("exec: empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = string(dir)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	runErr := cmd.Run()
	if runErr == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return 0, runErr
}
