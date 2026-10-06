// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"
	"os"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

// newJumpCommand implements shell-integration's "jump never mutates the
// parent shell": stdout carries exactly the resolved path and nothing
// else. An explanatory note goes to stderr, and only when stdout is an
// interactive terminal — a captured/piped stdout (the shell function's own
// use, and every test) gets no extra output at all.
func newJumpCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jump <workspace>",
		Short: "print a workspace's absolute path (does not change any process's cwd)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxRec, err := resolveContext(cmd, rt)
			if err != nil {
				return err
			}
			path, err := app.Jump(cmd.Context(), rt.Deps, app.JumpInput{WorkspacesRoot: ctxRec.WorkspacesRoot, Name: args[0]})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(path))

			if f, ok := cmd.OutOrStdout().(*os.File); ok && isTerminal(f) {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), messages.T(messages.CLIJumpTTYNote))
			}
			return nil
		},
	}
	return cmd
}
