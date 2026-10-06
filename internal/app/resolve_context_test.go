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

// TestResolveContext_ExplicitNameWinsOverActive covers design.md §6 Chain
// A step 1 (--context flag, surfaced here as ExplicitName): it must win
// even when a different context is recorded as active.
func TestResolveContext_ExplicitNameWinsOverActive(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work"})
	store.PutContext(domain.Context{Name: "oss"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	got, err := app.ResolveContext(context.Background(), app.Deps{Store: store}, app.ResolveContextInput{ExplicitName: "oss"})
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if got.Name != "oss" {
		t.Fatalf("Name = %q, want %q", got.Name, "oss")
	}
}

// TestResolveContext_EnvWinsOverActive covers Chain A step 2 (WS_CONTEXT).
func TestResolveContext_EnvWinsOverActive(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work"})
	store.PutContext(domain.Context{Name: "oss"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	got, err := app.ResolveContext(context.Background(), app.Deps{Store: store}, app.ResolveContextInput{EnvName: "oss"})
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if got.Name != "oss" {
		t.Fatalf("Name = %q, want %q", got.Name, "oss")
	}
}

// TestResolveContext_FallsBackToActiveContext covers Chain A step 3.
func TestResolveContext_FallsBackToActiveContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	got, err := app.ResolveContext(context.Background(), app.Deps{Store: store}, app.ResolveContextInput{})
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if got.Name != "work" {
		t.Fatalf("Name = %q, want %q", got.Name, "work")
	}
}

// TestResolveContext_ExactlyOneContextAutoSelectsAndPersists covers Chain A
// step 4: with no flag/env/active-context set, exactly one registered
// context is used and persisted as active.
func TestResolveContext_ExactlyOneContextAutoSelectsAndPersists(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "solo"})

	got, err := app.ResolveContext(context.Background(), app.Deps{Store: store}, app.ResolveContextInput{})
	if err != nil {
		t.Fatalf("ResolveContext: %v", err)
	}
	if got.Name != "solo" {
		t.Fatalf("Name = %q, want %q", got.Name, "solo")
	}

	root, err := store.LoadRoot(context.Background())
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if root.ActiveContext != "solo" {
		t.Fatalf("ActiveContext = %q, want %q (auto-selection must persist)", root.ActiveContext, "solo")
	}
}

// TestResolveContext_NoneConfiguredReturnsCodeNoContext covers Chain A step
// 5: zero contexts and nothing else set is the documented first-run signal.
func TestResolveContext_NoneConfiguredReturnsCodeNoContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()

	_, err := app.ResolveContext(context.Background(), app.Deps{Store: store}, app.ResolveContextInput{})
	if err == nil {
		t.Fatal("ResolveContext: err = nil, want CodeNoContext")
	}
	if domain.Code(err) != domain.CodeNoContext {
		t.Fatalf("Code(err) = %q, want %q", domain.Code(err), domain.CodeNoContext)
	}
}

// TestResolveContext_MultipleContextsWithoutActiveReturnsCodeNoContext
// covers the "ambiguous, no signal" case: more than one context exists, none
// is flagged/enveted/active, so ResolveContext must not guess.
func TestResolveContext_MultipleContextsWithoutActiveReturnsCodeNoContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work"})
	store.PutContext(domain.Context{Name: "oss"})

	_, err := app.ResolveContext(context.Background(), app.Deps{Store: store}, app.ResolveContextInput{})
	if domain.Code(err) != domain.CodeNoContext {
		t.Fatalf("Code(err) = %q, want %q", domain.Code(err), domain.CodeNoContext)
	}
}
