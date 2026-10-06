// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// RemoveContext deletes a registered context after confirming it exists
// (this change's own authorized gap-closure note: "creating contexts with
// no way to remove them is the same hole" as being unable to edit one).
//
// Removing the currently active context is refused rather than silently
// leaving the root config pointing at a context that no longer exists —
// every other command that resolves "the active context" (design.md §6
// Chain A) would otherwise start failing with a confusing
// CodeContextNotFound instead of a clear, actionable error at the moment
// of removal. The caller must switch to a different context first.
func RemoveContext(ctx context.Context, store ports.ConfigStore, name domain.ContextName) error {
	return RemoveContextWith(ctx, store, name, RemoveContextOptions{})
}

// RemoveContextOptions tune RemoveContextWith.
type RemoveContextOptions struct {
	// AllowActive lifts the active-context guard: removing the active
	// context (including the only one) deletes it and clears the root
	// config's active pointer instead of leaving it dangling. Callers that
	// want another context active should switch to it first.
	AllowActive bool
}

// RemoveContextWith is RemoveContext with explicit options.
func RemoveContextWith(ctx context.Context, store ports.ConfigStore, name domain.ContextName, opts RemoveContextOptions) error {
	if _, err := store.LoadContext(ctx, name); err != nil {
		return err
	}

	root, err := store.LoadRoot(ctx)
	initialized := err == nil
	if err != nil && !errors.Is(err, ports.ErrConfigNotInitialized) {
		return err
	}
	isActive := root.ActiveContext == name
	if isActive && !opts.AllowActive {
		return domain.NewOpError("context.remove", domain.CodeContextActive, string(name), "", nil)
	}

	if err := store.DeleteContext(ctx, name); err != nil {
		return err
	}
	if isActive && initialized {
		root.ActiveContext = ""
		return store.SaveRoot(ctx, root)
	}
	return nil
}
