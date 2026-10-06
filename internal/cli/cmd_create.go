// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

func newCreateCommand(rt *Runtime) *cobra.Command {
	var branch string
	var projects []string
	var copyNodeModules bool

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "create a new workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			ctxRec, err := resolveContext(cmd, rt)
			if err != nil {
				return err
			}
			overlay, err := resolveOverlay(cmd, rt)
			if err != nil {
				return err
			}

			keys := make([]domain.ProjectKey, 0, len(projects))
			for _, p := range projects {
				keys = append(keys, domain.ProjectKey(p))
			}

			in := app.CreateWorkspaceInput{
				Context:     ctxRec,
				Overlay:     overlay,
				Name:        name,
				Branch:      branch,
				ProjectKeys: keys,

				CopyNodeModules: copyNodeModules,
			}
			if _, err := app.CreateWorkspace(cmd.Context(), depsWithHumanReporter(cmd, rt), in); err != nil {
				wsRoot := ctxRec.WorkspacesRoot.Join(name)
				return &guidanceError{err: err, guidance: messages.T(messages.CLIPartialCreateGuidance, string(wsRoot), name)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&branch, "branch", "", "branch to check out in each new worktree")
	cmd.Flags().StringArrayVar(&projects, "project", nil, "project key to include (repeatable; default: every project in the context)")
	cmd.Flags().BoolVar(&copyNodeModules, "copy-node-modules", false, "copy node_modules from each Node project's main clone into its worktree (copy-on-write where supported)")
	return cmd
}
