// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"
	"os"
	"time"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Deps are the driven ports and build data the engine needs. cmd/wspace
// (the composition root) fills them with real adapters; tests use
// internal/ports/portstest fakes.
type Deps struct {
	Store       ports.ConfigStore
	Git         ports.GitPort
	FS          ports.FileSystemPort
	Checker     ports.ReleaseChecker
	Version     string
	Coordinates domain.RepoCoordinates
	// BundledBy names the desktop app that embeds this CLI (empty for a
	// standalone build); a bundled engine never checks for updates.
	BundledBy string
	// Presence records live MCP server sessions (nil: presence is off and
	// MCPSessions reports none).
	Presence ports.PresenceStore
	// Agents drives agents.* (skill and MCP installation into AI coding
	// agents); nil makes those methods fail with internal.
	Agents *AgentsDeps
	// Trees copies node_modules for copyNodeModules (nil: skipped with a
	// warning).
	Trees ports.TreeCloner
	// Now and PID default to time.Now and os.Getpid (tests inject them).
	Now func() time.Time
	PID int
}

// Engine exposes every wspace capability non-interactively. It holds no
// mutable state of its own (the YAML store is the only source of truth),
// so one Engine may serve any number of sequential calls.
type Engine struct{ deps Deps }

// New returns an Engine over deps.
func New(deps Deps) *Engine {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.PID == 0 {
		deps.PID = os.Getpid()
	}
	return &Engine{deps: deps}
}

// appDeps returns the app.Deps bundle with progress routed to emit.
func (e *Engine) appDeps(emit ProgressFunc) app.Deps {
	return app.Deps{Store: e.deps.Store, Git: e.deps.Git, FS: e.deps.FS, Reporter: progressReporter{emit: emit}, Trees: e.deps.Trees, Now: e.deps.Now}
}

// projectWithNodeInfo is projectToDTO plus the Node detection fields.
func (e *Engine) projectWithNodeInfo(p domain.Project) Project {
	out := projectToDTO(p)
	out.IsNode, out.HasNodeModules = app.NodeProjectInfo(e.deps.FS, p.SourceDir)
	return out
}

// resolveContext loads the context named name, or — when name is empty —
// the active context (or the only registered one), exactly like the CLI's
// Chain A minus the --context flag and WSPACE_CONTEXT, which belong to a
// terminal session, not to a long-lived server.
func (e *Engine) resolveContext(ctx context.Context, name string) (domain.Context, error) {
	if name != "" {
		cn, err := domain.NewContextName(name)
		if err != nil {
			return domain.Context{}, invalidParam("context", err)
		}
		c, err := app.LoadContext(ctx, e.appDeps(nil), cn)
		return c, wrap(err)
	}
	c, err := app.ResolveContext(ctx, e.appDeps(nil), app.ResolveContextInput{})
	return c, wrap(err)
}

// workspaceRoot resolves the context and validates workspace as a single
// directory name under its WorkspacesRoot (never "../x").
func (e *Engine) workspaceRoot(ctx context.Context, contextName, workspace string) (domain.Path, domain.Context, error) {
	c, err := e.resolveContext(ctx, contextName)
	if err != nil {
		return "", domain.Context{}, err
	}
	name, err := domain.NewWorkspaceName(workspace)
	if err != nil {
		return "", domain.Context{}, invalidParam("workspace", err)
	}
	return c.WorkspacesRoot.Join(name), c, nil
}

// activeContext returns the active context's name ("" when none).
func (e *Engine) activeContext(ctx context.Context) (domain.ContextName, error) {
	root, err := app.LoadRoot(ctx, e.appDeps(nil))
	if err != nil {
		return "", wrap(err)
	}
	return root.ActiveContext, nil
}
