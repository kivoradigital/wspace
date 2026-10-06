// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/spf13/cobra"
)

func newSyncEnvCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync-env <workspace>",
		Short: "re-run env file discovery and copy for every repo in a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsRoot, _, err := workspaceRoot(cmd, rt, args[0])
			if err != nil {
				return err
			}
			_, err = app.SyncEnv(cmd.Context(), depsWithHumanReporter(cmd, rt), app.SyncEnvInput{WorkspaceRoot: wsRoot})
			return err
		},
	}
	return cmd
}
