// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// SwitchContext sets name as the active context in the root config, after
// confirming it exists (context-management spec: "Switch active context").
// A missing root config file (first ever switch) is treated as a fresh
// zero-value root, not an error — SaveRoot creates it.
func SwitchContext(ctx context.Context, store ports.ConfigStore, name domain.ContextName) error {
	if _, err := store.LoadContext(ctx, name); err != nil {
		return err
	}

	root, err := store.LoadRoot(ctx)
	if err != nil && !errors.Is(err, ports.ErrConfigNotInitialized) {
		return err
	}

	root.ActiveContext = name
	return store.SaveRoot(ctx, root)
}
