// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
)

// Tool arguments mirror the `wspace rpc` parameters (camelCase) of the
// engine method each tool wraps (docs/rpc-contract.md §4),
// with three MCP-only additions: confirm on destructive tools, project
// (not repo) for a workspace's repo alias, and wrapped list results.

// NoInput takes no arguments.
type NoInput struct{}

// ContextInput selects a context.
type ContextInput struct {
	Context string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
}

// ClaimInput selects the orphaned workspaces claim_workspaces claims.
type ClaimInput struct {
	Context    string   `json:"context,omitempty" jsonschema:"Context that claims the workspaces; omit to use the active context."`
	Workspaces []string `json:"workspaces,omitempty" jsonschema:"Workspace folder names to claim; omit to claim every orphaned workspace in the context's workspaces root."`
}

// WorkspaceInput names one workspace.
type WorkspaceInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name (a directory name under the context's workspaces root, see list_workspaces)."`
}

func registerTools(s *mcp.Server, eng *engine.Engine) {
	registerEngineTools(s, eng)
	registerContextTools(s, eng)
	registerProjectTools(s, eng)
	registerWorkspaceTools(s, eng)
	registerUpdateTools(s, eng)
	registerRepoTools(s, eng)
	registerRepoWriteTools(s, eng)
}
