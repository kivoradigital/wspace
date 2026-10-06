// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

func newAddCommand(rt *Runtime) *cobra.Command {
	var copyNodeModules bool
	cmd := &cobra.Command{
		Use:   "add <workspace> <project>",
		Short: "mount one more project's worktree into an existing workspace",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsRoot, ctxRec, err := workspaceRoot(cmd, rt, args[0])
			if err != nil {
				return err
			}
			overlay, err := resolveOverlay(cmd, rt)
			if err != nil {
				return err
			}

			in := app.AddRepoInput{
				WorkspaceRoot: wsRoot,
				Context:       ctxRec,
				Overlay:       overlay,
				ProjectKey:    domain.ProjectKey(args[1]),

				CopyNodeModules: copyNodeModules,
			}
			if _, err := app.AddRepo(cmd.Context(), depsWithHumanReporter(cmd, rt), in); err != nil {
				return &guidanceError{err: err, guidance: messages.T(messages.CLIPartialCreateGuidance, string(wsRoot), args[0])}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&copyNodeModules, "copy-node-modules", false, "copy node_modules from the project's main clone into the new worktree (copy-on-write where supported)")
	return cmd
}
