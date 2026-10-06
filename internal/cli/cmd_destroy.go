// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

func newDestroyCommand(rt *Runtime) *cobra.Command {
	var force bool
	var deleteBranches bool

	cmd := &cobra.Command{
		Use:   "destroy <workspace>",
		Short: "tear down a workspace's worktrees (blocked by unsafe changes unless --force)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsRoot, _, err := workspaceRoot(cmd, rt, args[0])
			if err != nil {
				return err
			}

			if !force {
				ok, err := app.ConfirmWithHelp(cmd.Context(), rt.Prompter, messages.CLIConfirmDestroy, messages.CLIConfirmDestroyTarget, []any{args[0]}, false)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}

			in := app.DestroyWorkspaceInput{WorkspaceRoot: wsRoot, Force: force, DeleteBranches: deleteBranches}
			return app.DestroyWorkspace(cmd.Context(), depsWithHumanReporter(cmd, rt), in)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override blocking changes and skip the confirmation prompt")
	cmd.Flags().BoolVar(&deleteBranches, "delete-branches", false, "also delete every repo's branch")
	return cmd
}
