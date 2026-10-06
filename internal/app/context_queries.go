// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// ListContexts, LoadContext, LoadRoot, LoadOverlay and Cwd are thin
// read-only forwards to the ports the CLI's `context list`/`context
// show`/`info` commands and Chain B's overlay input need directly. They
// exist so internal/cli never holds a bare ports.ConfigStore/ports.
// FileSystemPort value (R6: internal/cli may only import app, domain,
// messages) — every I/O access still goes through internal/app, matching
// "one use case = one file = one testable unit" for the trivial cases too.

// ListContexts returns every registered context's name.
func ListContexts(ctx context.Context, deps Deps) ([]domain.ContextName, error) {
	return deps.Store.ListContexts(ctx)
}

// LoadContext loads one context record by name.
func LoadContext(ctx context.Context, deps Deps, name domain.ContextName) (domain.Context, error) {
	return deps.Store.LoadContext(ctx, name)
}

// LoadRoot loads the root config (active context, preferences). A store
// that has never been saved (ports.ErrConfigNotInitialized) is not an
// error here: it is the documented first-run state, reported as a
// zero-value RootConfig (no active context yet), exactly like
// ResolveContext's own handling of the same sentinel.
func LoadRoot(ctx context.Context, deps Deps) (domain.RootConfig, error) {
	root, err := deps.Store.LoadRoot(ctx)
	if err != nil && !errors.Is(err, ports.ErrConfigNotInitialized) {
		return domain.RootConfig{}, err
	}
	return root, nil
}

// LoadOverlay walks up from from looking for the nearest .ws.yaml, for
// Chain B's overlay layer (design.md §6).
func LoadOverlay(ctx context.Context, deps Deps, from domain.Path) (domain.Overlay, domain.Path, bool, error) {
	return deps.Store.LoadOverlay(ctx, from)
}

// Cwd returns the process's current working directory.
func Cwd(deps Deps) (domain.Path, error) {
	return deps.FS.Cwd()
}
