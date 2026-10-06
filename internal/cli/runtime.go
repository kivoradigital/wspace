// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"context"
	"io"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
)

// Runtime bundles every already-wired dependency internal/cli's command
// tree needs. cmd/ws (the composition root, R7) constructs every field's
// value with real adapters; internal/cli only ever holds and forwards
// these app-owned types, so it never needs to import internal/ports
// itself (R6: internal/cli may only import app, domain, messages —
// enforced by internal/archtest's TestImportBoundaries). domain.Path and
// domain.RepoCoordinates are both plain, I/O-free value types, so naming
// one here does not reach for an adapter either.
type Runtime struct {
	Deps          app.Deps
	ExecDeps      app.ExecDeps
	ProjectWizard app.ProjectWizardDeps
	ContextWizard app.ContextWizardDeps
	Prompter      app.PrompterDeps
	InstallDeps   app.InstallDeps
	CheckDeps     app.CheckForUpdateDeps
	// AgentsDeps drives `wspace agents` (skill and MCP installation into
	// AI coding agents).
	AgentsDeps app.AgentsDeps

	// Version is the CLI's version string, sourced from
	// internal/buildinfo.Version at cmd/ws's composition root (phase 5's own
	// seam, replacing phase 4b's plain literal — see cmd_version.go).
	Version string

	// RepoCoordinates is buildinfo.Coordinates()'s result, resolved once by
	// cmd/ws (which may import buildinfo, R7) and handed down as plain
	// data — internal/cli never imports internal/buildinfo itself. The zero
	// value means an unbranded build: `version --check` reports the check
	// as unavailable without ever calling CheckDeps.Checker (see
	// app.CheckForUpdate's own handling of a zero-value Coordinates).
	RepoCoordinates domain.RepoCoordinates

	// ServeRPC and ServeMCP run the machine-facing servers (`wspace rpc`,
	// `wspace mcp serve`) over the given streams until EOF or ctx is
	// cancelled. cmd/wspace builds them over internal/engine and injects
	// them as plain functions, so internal/cli never imports the engine,
	// rpc or mcpserver packages (R6). A nil value makes the command fail.
	ServeRPC ServeFunc
	ServeMCP ServeFunc
}

// ServeFunc runs one stdio protocol server.
type ServeFunc func(ctx context.Context, in io.Reader, out io.Writer) error
