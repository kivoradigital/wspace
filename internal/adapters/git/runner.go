// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
)

// classifyExecErr maps a failure to start or complete the subprocess itself
// (as opposed to the subprocess running and exiting non-zero) to an ErrCode:
// a cancelled/expired context is a timeout, a missing git binary stays
// git_missing, anything else is a generic failure to invoke git at all.
func classifyExecErr(ctx context.Context, err error) domain.ErrCode {
	if ctx.Err() != nil {
		return domain.CodeTimeout
	}
	if domain.Code(err) == domain.CodeGitMissing {
		return domain.CodeGitMissing
	}
	return domain.CodeGitFailed
}

// run executes a git subcommand against repo and requires a clean (exit 0)
// result; any other outcome is classified into a *domain.OpError via the
// exit-code table in errors.go. This is the shared "runGit" helper referred
// to by tasks.md 2.25: the fixed invocation prefix (argv), the allowlisted
// environment (buildEnv, used by exec), and the error-table lookup
// (classifyError) all funnel through this one call for every command whose
// exit code is a plain success/failure signal.
func (a *Adapter) run(ctx context.Context, op string, repo domain.Path, args ...string) (string, error) {
	if err := requireAbsRepo(op, repo); err != nil {
		return "", err
	}

	inv, err := a.exec(ctx, argv(repo, args...))
	if err != nil {
		return "", domain.NewOpError(op, classifyExecErr(ctx, err), string(repo), err.Error(), err)
	}
	if inv.exitCode != 0 {
		return "", domain.NewOpError(op, classifyError(args, inv.exitCode, inv.stderr), string(repo), inv.stderr, nil)
	}
	return inv.stdout, nil
}

// runExpecting executes a git subcommand whose exit code carries meaning
// beyond plain success/failure (e.g. show-ref's 0/1, check-ignore's 0/1).
// Any exitCode in okExitCodes is reported without error; anything else is
// classified exactly like run.
func (a *Adapter) runExpecting(ctx context.Context, op string, repo domain.Path, okExitCodes []int, args ...string) (exitCode int, stdout, stderr string, err error) {
	if verr := requireAbsRepo(op, repo); verr != nil {
		return 0, "", "", verr
	}

	inv, execErr := a.exec(ctx, argv(repo, args...))
	if execErr != nil {
		return 0, "", "", domain.NewOpError(op, classifyExecErr(ctx, execErr), string(repo), execErr.Error(), execErr)
	}
	for _, ok := range okExitCodes {
		if inv.exitCode == ok {
			return inv.exitCode, inv.stdout, inv.stderr, nil
		}
	}
	return inv.exitCode, inv.stdout, inv.stderr,
		domain.NewOpError(op, classifyError(args, inv.exitCode, inv.stderr), string(repo), inv.stderr, nil)
}
