// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/spf13/cobra"
)

// workspaceRoot resolves the active context (Chain A) and joins name onto
// its WorkspacesRoot — the same path every command that targets one
// existing workspace by name needs before calling its app use case.
func workspaceRoot(cmd *cobra.Command, rt *Runtime, name string) (domain.Path, domain.Context, error) {
	ctxRec, err := resolveContext(cmd, rt)
	if err != nil {
		return "", domain.Context{}, err
	}
	return ctxRec.WorkspacesRoot.Join(name), ctxRec, nil
}

// depsWithHumanReporter returns a copy of rt.Deps with Reporter bound to
// cmd's own output streams, so every use-case call renders through
// whichever writer cobra is using for this invocation (real stdio in
// production, an in-memory buffer in tests).
//
// It never overwrites a Reporter that is already set — bindReporter (called
// once from Execute, before the command tree even runs) already leaves one
// in place on the real cmd/ws path, and a caller that injects its own fake
// (a test, or a future GUI-hosted invocation) must keep it, exactly like
// bindReporter's own contract. This function used to assign
// unconditionally, which silently discarded any Reporter Execute or a test
// had already bound — the gap TestExecuteKeepsAnAlreadyBoundReporter now
// guards.
func depsWithHumanReporter(cmd *cobra.Command, rt *Runtime) app.Deps {
	deps := rt.Deps
	if deps.Reporter == nil {
		deps.Reporter = &HumanReporter{Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()}
	}
	return deps
}

// installDepsWithHumanReporter is depsWithHumanReporter's counterpart for
// app.InstallDeps (install has no ports.ConfigStore/GitPort/FileSystemPort
// bundle in common with the rest of the command tree, so it keeps its own
// small deps type — see app.InstallDeps's own doc comment).
func installDepsWithHumanReporter(cmd *cobra.Command, rt *Runtime) app.InstallDeps {
	deps := rt.InstallDeps
	if deps.Reporter == nil {
		deps.Reporter = &HumanReporter{Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()}
	}
	return deps
}
