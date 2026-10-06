// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/spf13/cobra"
)

func newDoctorCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "check git version and prune stale worktree registrations for every workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxRec, err := resolveContext(cmd, rt)
			if err != nil {
				if domain.Code(err) != domain.CodeNoContext {
					return err
				}
				// No context yet: still check git (the first thing a new
				// user needs), then report the missing context.
				if _, derr := app.Doctor(cmd.Context(), depsWithHumanReporter(cmd, rt), app.DoctorInput{}); derr != nil {
					return derr
				}
				return err
			}
			_, err = app.Doctor(cmd.Context(), depsWithHumanReporter(cmd, rt), app.DoctorInput{WorkspacesRoot: ctxRec.WorkspacesRoot})
			return err
		},
	}
	return cmd
}
