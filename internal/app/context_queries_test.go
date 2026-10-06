// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestContextQueries_ForwardToConfigStore covers the thin read-only
// wrappers internal/cli needs for `context list`/`context show`/`info`
// (tasks.md 4b) without ever holding a bare ports.ConfigStore itself
// (R6: internal/cli may only import app, domain, messages).
func TestContextQueries_ForwardToConfigStore(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/tmp/work"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}
	deps := app.Deps{Store: store}

	names, err := app.ListContexts(context.Background(), deps)
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	if len(names) != 1 || names[0] != "work" {
		t.Fatalf("ListContexts = %v, want [work]", names)
	}

	c, err := app.LoadContext(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	if c.Name != "work" {
		t.Fatalf("LoadContext.Name = %q, want %q", c.Name, "work")
	}

	root, err := app.LoadRoot(context.Background(), deps)
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if root.ActiveContext != "work" {
		t.Fatalf("LoadRoot.ActiveContext = %q, want %q", root.ActiveContext, "work")
	}

	overlay, _, found, err := app.LoadOverlay(context.Background(), deps, "/tmp/work/repo")
	if err != nil {
		t.Fatalf("LoadOverlay: %v", err)
	}
	if found {
		t.Fatalf("LoadOverlay found = true, want false (none seeded)")
	}
	_ = overlay
}

// TestLoadRoot_NotInitializedReturnsZeroValueNotError covers the real
// end-to-end gap this phase's own manual smoke test caught: a context can
// exist (SaveContext) before any root config file does (SaveRoot only
// happens on SwitchContext or ResolveContext's auto-select path), so
// `context list`/`context show` must not treat "not initialized yet" as a
// fatal error — there is simply no active context recorded yet.
func TestLoadRoot_NotInitializedReturnsZeroValueNotError(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.LoadRootErr = ports.ErrConfigNotInitialized

	root, err := app.LoadRoot(context.Background(), app.Deps{Store: store})
	if err != nil {
		t.Fatalf("LoadRoot: %v, want nil (not-initialized is not an error to this caller)", err)
	}
	if root.ActiveContext != "" {
		t.Fatalf("root.ActiveContext = %q, want empty", root.ActiveContext)
	}
}
