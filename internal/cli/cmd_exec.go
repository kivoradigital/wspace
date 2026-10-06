// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

func newExecCommand(rt *Runtime, exitCode *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "exec <workspace> -- <command> [args...]",
		Short:              "run a command in every repo of a workspace (no shell involved)",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			dash := cmd.ArgsLenAtDash()
			if dash < 1 || dash >= len(args) {
				return &cliError{text: messages.T(messages.CLIExecNoCommand)}
			}
			name := args[0]
			argv := args[dash:]

			wsRoot, _, err := workspaceRoot(cmd, rt, name)
			if err != nil {
				return err
			}

			deps := rt.ExecDeps
			deps.Reporter = &HumanReporter{Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()}
			deps.Stdout = cmd.OutOrStdout()
			deps.Stderr = cmd.ErrOrStderr()

			result, err := app.Exec(cmd.Context(), deps, app.ExecInput{WorkspaceRoot: wsRoot, Argv: argv})
			if err != nil {
				return err
			}
			*exitCode = result.ExitCode
			return nil
		},
	}
	return cmd
}
