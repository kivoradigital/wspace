// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"context"
	"sort"

	"github.com/kivoradigital/wspace/internal/domain"
)

// FakeConfigStore is an in-memory ports.ConfigStore. Every load/save error
// is injectable so use-case tests can exercise failure paths without a real
// filesystem.
type FakeConfigStore struct {
	root      domain.RootConfig
	contexts  map[domain.ContextName]domain.Context
	overlays  map[domain.Path]domain.Overlay
	manifests map[domain.Path]domain.Manifest

	LoadRootErr          error
	SaveRootErr          error
	ListContextsErr      error
	LoadContextErr       error
	SaveContextErr       error
	DeleteContextErr     error
	LoadOverlayErr       error
	LoadManifestErr      error
	SaveManifestErr      error
	ArchiveManifestErr   error
	FindWorkspaceRootErr error
	// SaveManifestErrFor fails SaveManifest for one workspace root only,
	// for per-workspace failure tests.
	SaveManifestErrFor map[domain.Path]error
}

// NewFakeConfigStore constructs an empty FakeConfigStore.
func NewFakeConfigStore() *FakeConfigStore {
	return &FakeConfigStore{
		contexts:  map[domain.ContextName]domain.Context{},
		overlays:  map[domain.Path]domain.Overlay{},
		manifests: map[domain.Path]domain.Manifest{},
	}
}

func (c *FakeConfigStore) LoadRoot(_ context.Context) (domain.RootConfig, error) {
	if c.LoadRootErr != nil {
		return domain.RootConfig{}, c.LoadRootErr
	}
	return c.root, nil
}

func (c *FakeConfigStore) SaveRoot(_ context.Context, r domain.RootConfig) error {
	if c.SaveRootErr != nil {
		return c.SaveRootErr
	}
	c.root = r
	return nil
}

func (c *FakeConfigStore) ListContexts(_ context.Context) ([]domain.ContextName, error) {
	if c.ListContextsErr != nil {
		return nil, c.ListContextsErr
	}
	out := make([]domain.ContextName, 0, len(c.contexts))
	for name := range c.contexts {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (c *FakeConfigStore) LoadContext(_ context.Context, name domain.ContextName) (domain.Context, error) {
	if c.LoadContextErr != nil {
		return domain.Context{}, c.LoadContextErr
	}
	ctx, ok := c.contexts[name]
	if !ok {
		return domain.Context{}, domain.NewOpError("config.load_context", domain.CodeContextNotFound, string(name), "", nil)
	}
	return ctx, nil
}

func (c *FakeConfigStore) SaveContext(_ context.Context, ctx domain.Context) error {
	if c.SaveContextErr != nil {
		return c.SaveContextErr
	}
	c.contexts[ctx.Name] = ctx
	return nil
}

func (c *FakeConfigStore) DeleteContext(_ context.Context, name domain.ContextName) error {
	if c.DeleteContextErr != nil {
		return c.DeleteContextErr
	}
	delete(c.contexts, name)
	return nil
}

func (c *FakeConfigStore) LoadOverlay(_ context.Context, from domain.Path) (domain.Overlay, domain.Path, bool, error) {
	if c.LoadOverlayErr != nil {
		return domain.Overlay{}, "", false, c.LoadOverlayErr
	}
	overlay, ok := c.overlays[from]
	if !ok {
		return domain.Overlay{}, "", false, nil
	}
	return overlay, from, true, nil
}

func (c *FakeConfigStore) LoadManifest(_ context.Context, wsRoot domain.Path) (domain.Manifest, error) {
	if c.LoadManifestErr != nil {
		return domain.Manifest{}, c.LoadManifestErr
	}
	m, ok := c.manifests[wsRoot]
	if !ok {
		return domain.Manifest{}, domain.NewOpError("config.load_manifest", domain.CodeWorkspaceNotFound, string(wsRoot), "", nil)
	}
	return m, nil
}

func (c *FakeConfigStore) SaveManifest(_ context.Context, wsRoot domain.Path, m domain.Manifest) error {
	if c.SaveManifestErr != nil {
		return c.SaveManifestErr
	}
	if err := c.SaveManifestErrFor[wsRoot]; err != nil {
		return err
	}
	c.manifests[wsRoot] = m
	return nil
}

func (c *FakeConfigStore) ArchiveManifest(_ context.Context, wsRoot domain.Path) error {
	if c.ArchiveManifestErr != nil {
		return c.ArchiveManifestErr
	}
	delete(c.manifests, wsRoot)
	return nil
}

func (c *FakeConfigStore) FindWorkspaceRoot(_ context.Context, from domain.Path) (domain.Path, bool, error) {
	if c.FindWorkspaceRootErr != nil {
		return "", false, c.FindWorkspaceRootErr
	}
	if _, ok := c.manifests[from]; ok {
		return from, true, nil
	}
	return "", false, nil
}

// PutContext seeds a context directly, bypassing SaveContext, for test setup.
func (c *FakeConfigStore) PutContext(ctx domain.Context) {
	c.contexts[ctx.Name] = ctx
}

// PutManifest seeds a manifest directly, bypassing SaveManifest, for test
// setup.
func (c *FakeConfigStore) PutManifest(wsRoot domain.Path, m domain.Manifest) {
	c.manifests[wsRoot] = m
}

// PutOverlay seeds an overlay directly, bypassing LoadOverlay's discovery
// logic (which belongs to the real configstore adapter), for test setup.
func (c *FakeConfigStore) PutOverlay(at domain.Path, o domain.Overlay) {
	c.overlays[at] = o
}
