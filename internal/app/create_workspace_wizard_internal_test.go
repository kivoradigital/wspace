// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestPickWorkspaceProjects_EveryOptionPreSelected covers the "New
// workspace…" project picker's own contract: every already-registered
// project must be offered with Option.Selected true, the field both
// prompter adapters (formprompt.buildMultiChoiceWidget,
// termprompt.preselectedIndices) already read as "start this option
// checked" — never a filesystem scan's candidates (app.RunProjectWizard's
// pickCandidates builds its own Options with Selected always false, since
// there is no sensible "already selected" default for a fresh scan).
func TestPickWorkspaceProjects_EveryOptionPreSelected(t *testing.T) {
	c := domain.Context{
		Name: "work",
		Projects: []domain.Project{
			{Key: "api"},
			{Key: "web"},
		},
	}
	prompter := portstest.NewScriptedPrompter(t, portstest.MultiChooseAnswer([]int{0, 1}))

	keys, err := pickWorkspaceProjects(context.Background(), prompter, c)
	if err != nil {
		t.Fatalf("pickWorkspaceProjects: %v", err)
	}
	prompter.CheckUnconsumed()

	// Submitting "everything" must return an explicit, non-nil key list
	// naming every project — never nil/empty, which CreateWorkspace reads
	// as its own "select every project" fallback. A caller further up
	// (RunCreateWorkspace) must always pass this exact list through to
	// CreateWorkspace's ProjectKeys, rather than letting a full selection
	// collapse into that fallback by omission.
	if keys == nil {
		t.Fatal("keys = nil, want an explicit non-nil list naming every project")
	}
	if len(keys) != 2 || keys[0] != "api" || keys[1] != "web" {
		t.Fatalf("keys = %+v, want [api web]", keys)
	}
}

// TestPickWorkspaceProjects_NoProjectsRegistered covers the edge case a
// brand-new context (or one whose projects were all removed) presents:
// there is nothing to offer, so no prompt should even be shown, and the
// result must be an explicit empty (not nil, to distinguish "chose
// nothing to offer" from "user deselected everything") slice — either
// way, CreateWorkspace's preflight already rejects an empty selection
// against zero projects; this just proves pickWorkspaceProjects itself
// never blocks on a prompt no context this bare could ever answer.
func TestPickWorkspaceProjects_NoProjectsRegistered(t *testing.T) {
	c := domain.Context{Name: "empty"}
	prompter := portstest.NewScriptedPrompter(t)

	keys, err := pickWorkspaceProjects(context.Background(), prompter, c)
	if err != nil {
		t.Fatalf("pickWorkspaceProjects: %v", err)
	}
	prompter.CheckUnconsumed()
	if len(keys) != 0 {
		t.Fatalf("keys = %+v, want empty (nothing was ever offered)", keys)
	}
}
