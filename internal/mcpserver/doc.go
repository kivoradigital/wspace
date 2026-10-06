// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package mcpserver is `wspace mcp serve`: a Model Context Protocol server
// (stdio transport, official Go SDK github.com/modelcontextprotocol/go-sdk)
// exposing every engine capability desktop clients use as thin tools over
// internal/engine (docs/mcp.md). Read-only tools carry
// readOnlyHint; destructive tools carry destructiveHint, require
// confirm=true (confirm=false previews the reasons as needs_confirmation)
// and never force on their own. Long operations forward engine progress as
// MCP progress notifications when the caller sends a progress token.
//
// While a client is connected the server keeps a presence record for its
// process (engine.StartPresence), so `mcp.sessions` can list live agents.
//
// It imports internal/engine and internal/messages only (plus the SDK) —
// never app, ports or an adapter (enforced by internal/archtest).
package mcpserver
