// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestSwitchContext_PersistsActiveContext covers tasks.md 3.14
// (context-management spec: "Switch active context") against
// portstest.FakeConfigStore.
func TestSwitchContext_PersistsActiveContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/tmp/work"})
	store.PutContext(domain.Context{Name: "oss", WorkspacesRoot: "/tmp/oss"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	if err := app.SwitchContext(context.Background(), store, "oss"); err != nil {
		t.Fatalf("SwitchContext: %v", err)
	}

	root, err := store.LoadRoot(context.Background())
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if root.ActiveContext != "oss" {
		t.Fatalf("ActiveContext = %q, want %q", root.ActiveContext, "oss")
	}
}

// TestSwitchContext_UnknownContextErrors covers the "must exist first"
// guard: switching to a context that was never saved fails rather than
// silently recording a dangling active_context.
func TestSwitchContext_UnknownContextErrors(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	if err := app.SwitchContext(context.Background(), store, "ghost"); err == nil {
		t.Fatal("SwitchContext(ghost): err = nil, want an error")
	}
}
