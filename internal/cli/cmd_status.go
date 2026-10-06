// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

func newStatusCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <workspace>",
		Short: "show per-repo branch, ahead/behind and dirty state for a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsRoot, _, err := workspaceRoot(cmd, rt, args[0])
			if err != nil {
				return err
			}
			status, err := app.Status(cmd.Context(), rt.Deps, app.StatusInput{WorkspaceRoot: wsRoot})
			if err != nil {
				return err
			}

			if jsonRequested(cmd) {
				out, err := encodeStatusJSON(status)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			for _, r := range status.Repos {
				dirty := messages.T(messages.CLIStatusClean)
				if r.Dirty {
					dirty = messages.T(messages.CLIStatusDirty)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIStatusRow, r.Alias, string(r.Branch), r.Ahead, r.Behind, dirty))
				if r.BaseMissing {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIStatusBaseMissing, string(r.BaseBranch), r.Alias))
				}
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}
