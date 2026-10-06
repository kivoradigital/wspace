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

// TestRemoveContext_DeletesNonActiveContext covers this change's own
// authorized gap-closure: "creating contexts with no way to remove them is
// the same hole" as being unable to edit one.
func TestRemoveContext_DeletesNonActiveContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/tmp/work"})
	store.PutContext(domain.Context{Name: "oss", WorkspacesRoot: "/tmp/oss"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	if err := app.RemoveContext(context.Background(), store, "oss"); err != nil {
		t.Fatalf("RemoveContext: %v", err)
	}

	if _, err := store.LoadContext(context.Background(), "oss"); err == nil {
		t.Fatal("LoadContext(oss) succeeded after RemoveContext, want it gone")
	}
	if _, err := store.LoadContext(context.Background(), "work"); err != nil {
		t.Fatalf("LoadContext(work): %v, want the active context untouched", err)
	}
}

// TestRemoveContext_RefusesToRemoveActiveContext covers the safety guard:
// removing the currently active context would leave the root config
// pointing at a context that no longer exists, so it must be refused with
// a typed, actionable error instead.
func TestRemoveContext_RefusesToRemoveActiveContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/tmp/work"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	err := app.RemoveContext(context.Background(), store, "work")
	if err == nil {
		t.Fatal("RemoveContext(active context) succeeded, want a refusal")
	}
	if domain.Code(err) != domain.CodeContextActive {
		t.Fatalf("Code(err) = %q, want %q", domain.Code(err), domain.CodeContextActive)
	}

	if _, err := store.LoadContext(context.Background(), "work"); err != nil {
		t.Fatalf("LoadContext(work) after refused removal: %v, want it still present", err)
	}
}

// TestRemoveContext_UnknownContextErrors mirrors
// TestSwitchContext_UnknownContextErrors: removing a context that was
// never saved fails rather than silently succeeding.
func TestRemoveContext_UnknownContextErrors(t *testing.T) {
	store := portstest.NewFakeConfigStore()

	if err := app.RemoveContext(context.Background(), store, "ghost"); err == nil {
		t.Fatal("RemoveContext(unknown context) succeeded, want an error")
	}
}

// TestRemoveContextWith_AllowActiveDeletesActiveAndClearsIt covers the
// explicit opt-in that lets a client delete the active context (including
// the last one): the context is deleted and the active pointer is cleared
// instead of left dangling.
func TestRemoveContextWith_AllowActiveDeletesActiveAndClearsIt(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/tmp/work"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	if err := app.RemoveContextWith(context.Background(), store, "work", app.RemoveContextOptions{AllowActive: true}); err != nil {
		t.Fatalf("RemoveContextWith(allowActive): %v", err)
	}

	if _, err := store.LoadContext(context.Background(), "work"); err == nil {
		t.Fatal("LoadContext(work) succeeded after removal, want it gone")
	}
	root, err := store.LoadRoot(context.Background())
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if root.ActiveContext != "" {
		t.Fatalf("ActiveContext = %q, want it cleared", root.ActiveContext)
	}
}

// TestRemoveContextWith_AllowActiveLeavesOtherActiveUntouched: the flag
// only lifts the guard; deleting a non-active context keeps the pointer.
func TestRemoveContextWith_AllowActiveLeavesOtherActiveUntouched(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/tmp/work"})
	store.PutContext(domain.Context{Name: "oss", WorkspacesRoot: "/tmp/oss"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	if err := app.RemoveContextWith(context.Background(), store, "oss", app.RemoveContextOptions{AllowActive: true}); err != nil {
		t.Fatalf("RemoveContextWith: %v", err)
	}
	root, _ := store.LoadRoot(context.Background())
	if root.ActiveContext != "work" {
		t.Fatalf("ActiveContext = %q, want work", root.ActiveContext)
	}
}
