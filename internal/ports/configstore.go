// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/domain"
)

// ErrConfigNotInitialized is returned by ConfigStore.LoadRoot when no root
// config file exists yet (design.md §5: "no <config>/ws/ at all"). It is
// the store's clean, checkable first-run signal — callers must not
// fabricate defaults on disk, and must not have to distinguish this state
// from an obscure raw filesystem error.
var ErrConfigNotInitialized = errors.New("configstore: not initialized")

// ConfigStore owns every on-disk representation decision (design.md §5).
type ConfigStore interface {
	LoadRoot(ctx context.Context) (domain.RootConfig, error)
	SaveRoot(ctx context.Context, r domain.RootConfig) error

	ListContexts(ctx context.Context) ([]domain.ContextName, error)
	LoadContext(ctx context.Context, name domain.ContextName) (domain.Context, error)
	SaveContext(ctx context.Context, c domain.Context) error
	DeleteContext(ctx context.Context, name domain.ContextName) error

	LoadOverlay(ctx context.Context, from domain.Path) (domain.Overlay, domain.Path, bool, error)

	LoadManifest(ctx context.Context, wsRoot domain.Path) (domain.Manifest, error)
	SaveManifest(ctx context.Context, wsRoot domain.Path, m domain.Manifest) error
	ArchiveManifest(ctx context.Context, wsRoot domain.Path) error
	FindWorkspaceRoot(ctx context.Context, from domain.Path) (domain.Path, bool, error)
}
