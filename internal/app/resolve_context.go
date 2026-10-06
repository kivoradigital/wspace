// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// ResolveContextInput carries the two explicit-selection signals design.md
// §6 Chain A ranks above every persisted default: an explicit --context
// flag value and the WSPACE_CONTEXT environment variable. Reading the flag and
// the environment is internal/cli's job (R6); this use case stays pure
// I/O-through-ports so it is testable with FakeConfigStore alone.
type ResolveContextInput struct {
	ExplicitName string
	EnvName      string
}

// ResolveContext implements design.md §6 Chain A ("which context, once per
// invocation"): an explicit flag, then WSPACE_CONTEXT, then the root config's
// active context, then — when exactly one context is registered and none of
// the above applies — that context, auto-selected and persisted as active.
// Anything else (zero or multiple contexts with no explicit signal) is the
// documented first-run/ambiguous case, reported as CodeNoContext.
//
// A local .ws.yaml overlay deliberately never participates in this chain
// (design.md ADR D7: an overlay can never select a context, only override
// option values within whatever context Chain A already resolved).
func ResolveContext(ctx context.Context, deps Deps, in ResolveContextInput) (domain.Context, error) {
	if name := in.ExplicitName; name != "" {
		cn, err := domain.NewContextName(name)
		if err != nil {
			return domain.Context{}, err
		}
		return deps.Store.LoadContext(ctx, cn)
	}
	if name := in.EnvName; name != "" {
		cn, err := domain.NewContextName(name)
		if err != nil {
			return domain.Context{}, err
		}
		return deps.Store.LoadContext(ctx, cn)
	}

	root, err := deps.Store.LoadRoot(ctx)
	if err != nil && !errors.Is(err, ports.ErrConfigNotInitialized) {
		return domain.Context{}, err
	}
	if root.ActiveContext != "" {
		return deps.Store.LoadContext(ctx, root.ActiveContext)
	}

	names, err := deps.Store.ListContexts(ctx)
	if err != nil {
		return domain.Context{}, err
	}
	if len(names) == 1 {
		c, err := deps.Store.LoadContext(ctx, names[0])
		if err != nil {
			return domain.Context{}, err
		}
		root.ActiveContext = names[0]
		if err := deps.Store.SaveRoot(ctx, root); err != nil {
			return domain.Context{}, err
		}
		return c, nil
	}

	return domain.Context{}, domain.NewOpError("context.resolve", domain.CodeNoContext, "", "", nil)
}
