// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

func newInfoCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "show the resolved context and every Chain B option's winning layer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxRec, err := resolveContext(cmd, rt)
			if err != nil {
				return err
			}
			overlay, err := resolveOverlay(cmd, rt)
			if err != nil {
				return err
			}
			resolver := domain.Resolver{Context: &ctxRec, Overlay: overlay}

			if jsonRequested(cmd) {
				out, err := encodeInfoJSON(ctxRec.Name, rt.Deps.FS.Paths().Config, resolver)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIInfoContext, string(ctxRec.Name)))
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIInfoConfigDir, string(rt.Deps.FS.Paths().Config)))
			printResolvedOptions(cmd, resolver)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

// printResolvedOptions renders every Chain B key's Resolved[T].From layer
// (ADR D6), shared by `info` and `context show`.
func printResolvedOptions(cmd *cobra.Command, r domain.Resolver) {
	base := r.BaseBranch("")
	copyEnv := r.CopyEnv("")
	fetch := r.FetchBeforeCreate()
	prune := r.EnvPruneDirs("")
	remote := r.Remote("")

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIInfoOptionRow, "base_branch", string(base.Value), base.From.String()))
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIInfoOptionRow, "copy_env", copyEnv.Value, copyEnv.From.String()))
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIInfoOptionRow, "fetch_before_create", fetch.Value, fetch.From.String()))
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIInfoOptionRow, "env_prune_dirs", prune.Value, prune.From.String()))
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIInfoOptionRow, "remote", remote.Value, remote.From.String()))
}
