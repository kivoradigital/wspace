// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

// newProjectCommand groups read-only project subcommands. Registering and
// editing projects stays in the context wizards ("context create",
// "context edit"); this group only lists them, for scripts and agents.
func newProjectCommand(rt *Runtime) *cobra.Command {
	root := &cobra.Command{
		Use:   "project",
		Short: "inspect the projects registered in a context",
	}
	root.AddCommand(newProjectListCommand(rt))
	return root
}

func newProjectListCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list the projects registered in the resolved context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxRec, err := resolveContext(cmd, rt)
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				out, err := encodeProjectListJSON(ctxRec.Projects)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			if len(ctxRec.Projects) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIProjectListEmpty, string(ctxRec.Name)))
				return nil
			}
			for _, p := range ctxRec.Projects {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIProjectListRow, string(p.Key), string(p.SourceDir)))
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}
