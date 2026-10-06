// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// errServerUnavailable is returned when the composition root did not
// inject a server for a machine-facing command (never the case for the
// real cmd/wspace binary).
var errServerUnavailable = errors.New("cli: server not wired")

// newRPCCommand is `wspace rpc`: the JSON-lines engine protocol over
// stdio used by desktop clients (docs/rpc-contract.md).
// stdout carries protocol lines only.
func newRPCCommand(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "rpc",
		Short: "serve the JSON-lines engine protocol on stdin/stdout (for desktop clients)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(cmd, rt.ServeRPC)
		},
	}
}

// newMCPCommand groups `wspace mcp serve`: a Model Context Protocol server
// over stdio for AI agents (docs/mcp.md).
func newMCPCommand(rt *Runtime) *cobra.Command {
	root := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol server for AI agents",
	}
	root.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "serve MCP over stdin/stdout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(cmd, rt.ServeMCP)
		},
	})
	return root
}

func serve(cmd *cobra.Command, fn ServeFunc) error {
	if fn == nil {
		return errServerUnavailable
	}
	return fn(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
}
