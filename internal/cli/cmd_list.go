// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

func newListCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list every workspace in the resolved context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxRec, err := resolveContext(cmd, rt)
			if err != nil {
				return err
			}
			statuses, err := app.List(cmd.Context(), rt.Deps, app.ListInput{WorkspacesRoot: ctxRec.WorkspacesRoot, Context: ctxRec.Name})
			if err != nil {
				return err
			}

			if jsonRequested(cmd) {
				out, err := encodeListJSON(statuses)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			if len(statuses) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIListEmpty))
				return nil
			}
			for _, s := range statuses {
				if s.Legacy {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIListRowLegacy, s.Name, string(s.Root)))
					continue
				}
				if s.Err != nil {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIListRowDamaged, s.Name, string(s.Root), s.Err.Error()))
					continue
				}
				if s.OrphanOf != "" {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIListRowOrphan, s.Name, string(s.Root), len(s.Repos), string(s.OrphanOf)))
					continue
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIListRow, s.Name, string(s.Root), len(s.Repos)))
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}
