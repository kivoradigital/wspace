// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

// newVersionCommand prints rt.Version and, with --check, runs
// app.CheckForUpdate (design.md §11: "ws version --check (CLI) ... calls
// app.CheckForUpdate"). --check never fails the command: an offline
// network, a rate limit, or an unbranded build all render as "update check
// unavailable" and still exit 0 (update-check spec: "Graceful offline
// degradation").
//
// --json is deliberately not registered here: the cli-surface spec's
// --json contract is scoped to info/list/status only ("Non-JSON commands
// remain human-output only"), and version is not one of them.
func newVersionCommand(rt *Runtime) *cobra.Command {
	var check bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "print the ws version and, with --check, look for an update",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIVersionLine, rt.Version))
			if !check {
				return nil
			}

			result, err := app.CheckForUpdate(cmd.Context(), rt.CheckDeps, app.CheckForUpdateInput{
				Coordinates:    rt.RepoCoordinates,
				CurrentVersion: rt.Version,
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderUpdateResult(result))
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "check GitHub releases for a newer version")
	return cmd
}

// renderUpdateResult renders a CheckForUpdateResult's three mutually
// exclusive outcomes through the catalog.
func renderUpdateResult(r app.CheckForUpdateResult) string {
	switch {
	case r.Unavailable:
		return messages.T(messages.CLIUpdateCheckUnavailable)
	case r.Available:
		return messages.T(messages.CLIUpdateAvailable, r.LatestTag, r.CurrentVersion)
	default:
		return messages.T(messages.CLIUpdateUpToDate, r.CurrentVersion)
	}
}
