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

// TestRunDeleteContextWizard_RemovesThePickedContext covers tray-gui
// gap-closure #2's "Delete context…" requirement: the tray has no
// argument list to name a context with (unlike the CLI's "context remove
// <name>"), so this shared use case prompts (Choose) for which registered
// context to delete, then delegates to the existing app.RemoveContext —
// never reimplementing its active-context guard.
func TestRunDeleteContextWizard_RemovesThePickedContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "oss", WorkspacesRoot: "/tmp/oss"})
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/tmp/work"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	prompter := portstest.NewScriptedPrompter(t, portstest.ChooseAnswer(0)) // "oss" (sorted before "work")

	removed, err := app.RunDeleteContextWizard(context.Background(), store, prompter)
	if err != nil {
		t.Fatalf("RunDeleteContextWizard: %v", err)
	}
	prompter.CheckUnconsumed()

	if removed != "oss" {
		t.Fatalf("removed = %q, want %q", removed, "oss")
	}
	if _, err := store.LoadContext(context.Background(), "oss"); err == nil {
		t.Fatal("LoadContext(oss) succeeded after RunDeleteContextWizard, want it gone")
	}
	if _, err := store.LoadContext(context.Background(), "work"); err != nil {
		t.Fatalf("LoadContext(work): %v, want the active context untouched", err)
	}
}

// TestRunDeleteContextWizard_RefusesActiveContext covers the same refusal
// app.RemoveContext already enforces, surfaced here (never re-implemented)
// so the tray's "Delete context…" reports it clearly instead of silently
// doing nothing.
func TestRunDeleteContextWizard_RefusesActiveContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/tmp/work"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	prompter := portstest.NewScriptedPrompter(t, portstest.ChooseAnswer(0))

	_, err := app.RunDeleteContextWizard(context.Background(), store, prompter)
	if err == nil {
		t.Fatal("RunDeleteContextWizard(active context) succeeded, want a refusal")
	}
	if domain.Code(err) != domain.CodeContextActive {
		t.Fatalf("Code(err) = %q, want %q", domain.Code(err), domain.CodeContextActive)
	}
	if _, err := store.LoadContext(context.Background(), "work"); err != nil {
		t.Fatalf("LoadContext(work) after refused delete: %v, want it still present", err)
	}
}

// TestRunDeleteContextWizard_NoContextsErrors covers the degenerate case:
// nothing to choose from must fail explicitly rather than calling Choose
// with zero options.
func TestRunDeleteContextWizard_NoContextsErrors(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	prompter := portstest.NewScriptedPrompter(t)

	if _, err := app.RunDeleteContextWizard(context.Background(), store, prompter); err == nil {
		t.Fatal("RunDeleteContextWizard with no contexts succeeded, want an error")
	}
	prompter.CheckUnconsumed()
}
