// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package engine is the non-interactive facade over internal/app that the
// machine-facing surfaces (internal/rpc's JSON-lines server for desktop
// clients, internal/mcpserver's MCP server for AI agents) drive. Every
// capability takes explicit, typed parameters and returns a typed,
// JSON-tagged result; nothing here ever prompts. Progress flows through a
// ProgressFunc as structured ProgressEvents, and every failure is an
// *Error carrying one of a small, stable set of codes.
//
// It imports internal/app, internal/domain, internal/ports and
// internal/messages only — never an adapter, cli, gui, rpc or mcpserver
// package (enforced by internal/archtest).
package engine
