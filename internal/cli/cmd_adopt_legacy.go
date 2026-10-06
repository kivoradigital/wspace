// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/spf13/cobra"
)

// newAdoptLegacyCommand drives app.AdoptLegacyWorkspaces: every workspace
// the legacy bash tool created in the resolved context's WorkspacesRoot gets
// a wspace manifest. The legacy ".ws/" files are never changed.
func newAdoptLegacyCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt-legacy",
		Short: "give every workspace created by the legacy ws tool a wspace manifest (its .ws/ files are left untouched)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxRec, err := resolveContext(cmd, rt)
			if err != nil {
				return err
			}
			res, err := app.AdoptLegacyWorkspaces(cmd.Context(), rt.Deps, app.AdoptLegacyWorkspacesInput{Context: ctxRec})
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				out, err := encodeAdoptJSON(res)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			for _, line := range app.RenderAdoptSummary(res) {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}
