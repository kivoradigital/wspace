// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/spf13/cobra"
)

// newClaimCommand drives app.ClaimWorkspaces: orphaned workspaces in the
// resolved context's WorkspacesRoot (their manifest names a context that no
// longer exists) become owned by the resolved context. A workspace owned by
// another existing context is never taken.
func newClaimCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claim [names...]",
		Short: "make the resolved context own orphaned workspaces (default: every orphan in its workspaces folder)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxRec, err := resolveContext(cmd, rt)
			if err != nil {
				return err
			}
			res, err := app.ClaimWorkspaces(cmd.Context(), rt.Deps, app.ClaimWorkspacesInput{Context: ctxRec, Workspaces: args})
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				out, err := encodeClaimJSON(res)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			for _, line := range app.RenderClaimSummary(res) {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}
