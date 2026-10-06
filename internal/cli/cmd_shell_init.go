// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

func newShellInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell-init {bash|zsh|sh|fish}",
		Short: "print a shell function that wraps ws so jump can cd in the live shell",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result := app.ShellInit()
			switch args[0] {
			case "bash", "zsh", "sh":
				_, _ = fmt.Fprint(cmd.OutOrStdout(), result.POSIX)
			case "fish":
				_, _ = fmt.Fprint(cmd.OutOrStdout(), result.Fish)
			default:
				return &cliError{text: messages.T(messages.CLIShellInitUsage)}
			}
			return nil
		},
	}
	return cmd
}
