// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/spf13/cobra"
)

func newRmCommand(rt *Runtime) *cobra.Command {
	var force bool
	var deleteBranch bool

	cmd := &cobra.Command{
		Use:   "rm <workspace> <alias>",
		Short: "remove one repo's worktree from an existing workspace",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsRoot, _, err := workspaceRoot(cmd, rt, args[0])
			if err != nil {
				return err
			}
			in := app.RemoveRepoInput{
				WorkspaceRoot: wsRoot,
				Alias:         args[1],
				Force:         force,
				DeleteBranch:  deleteBranch,
			}
			return app.RemoveRepo(cmd.Context(), depsWithHumanReporter(cmd, rt), in)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "remove even if the worktree has blocking changes")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "also delete the repo's branch")
	return cmd
}
