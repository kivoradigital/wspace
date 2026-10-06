// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/spf13/cobra"
)

func newRepairCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repair <workspace>",
		Short: "recreate any worktree the manifest declares but that is missing from disk",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsRoot, _, err := workspaceRoot(cmd, rt, args[0])
			if err != nil {
				return err
			}
			_, err = app.Repair(cmd.Context(), depsWithHumanReporter(cmd, rt), app.RepairInput{WorkspaceRoot: wsRoot})
			return err
		},
	}
	return cmd
}
