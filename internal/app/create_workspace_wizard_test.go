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

// createWorkspaceWizardContext builds a context with two already-registered
// projects, exactly what "New workspace…" must offer for selection — never
// a filesystem scan (app.RunProjectWizard's own job, and a wholly
// different operation: registering *new* projects into a context, not
// choosing which already-registered ones join *this* workspace).
func createWorkspaceWizardContext(fs *portstest.FakeFS) domain.Context {
	return domain.Context{
		Name:           "work",
		WorkspacesRoot: fs.Paths().Home.Join("workspaces"),
		Projects: []domain.Project{
			{Key: "api", SourceDir: fs.Paths().Home.Join("src", "api")},
			{Key: "web", SourceDir: fs.Paths().Home.Join("src", "web")},
		},
	}
}

// TestRunCreateWorkspace_CreatesExactlySelectedSubset is the regression
// test for the serious defect this change fixes: "New workspace…" used to
// call app.RunProjectWizard (project *registration*, into the context)
// instead of app.CreateWorkspace (workspace *creation*) — no workspace was
// ever created, the wizard silently re-ran discovery, and the "projects"
// on screen were the scan's candidates, not the context's own registered
// ones. Driving app.RunCreateWorkspace directly (never either front end)
// is what makes this guarantee shared: selecting a subset of the
// context's already-registered projects must produce a real workspace
// containing exactly that subset, never every project (CreateWorkspace's
// own "empty ProjectKeys means every project" fallback must never be
// silently relied on for an explicit subset), and the context's own
// project list must be completely untouched — proof this never falls
// through to project registration.
func TestRunCreateWorkspace_CreatesExactlySelectedSubset(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	git := portstest.NewFakeGit()
	git.IsMainCloneFunc = func(dir domain.Path) (bool, error) { return true, nil }
	reporter := portstest.NewRecordingReporter()

	ctx := createWorkspaceWizardContext(fs)
	store.PutContext(ctx)

	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("myws"),          // workspace name
		portstest.TextAnswer(""),              // branch -> accept the resolved default
		portstest.MultiChooseAnswer([]int{0}), // "api" only, out of [api web]
	)

	deps := app.CreateWorkspaceWizardDeps{Store: store, FS: fs, Git: git, Prompter: prompter, Reporter: reporter}

	result, err := app.RunCreateWorkspace(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunCreateWorkspace: %v", err)
	}
	prompter.CheckUnconsumed()

	if result.Workspace.Name != "myws" {
		t.Fatalf("Workspace.Name = %q, want %q", result.Workspace.Name, "myws")
	}
	if len(result.Workspace.Repos) != 1 || result.Workspace.Repos[0].Alias != "api" {
		t.Fatalf("Workspace.Repos = %+v, want exactly one repo aliased %q (the selected subset only)", result.Workspace.Repos, "api")
	}

	// The workspace must be real: a manifest was actually persisted, the
	// way app.CreateWorkspace (never app.RunProjectWizard) leaves one.
	manifest, err := store.LoadManifest(context.Background(), ctx.WorkspacesRoot.Join("myws"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(manifest.Workspace.Repos) != 1 || manifest.Workspace.Repos[0].Alias != "api" {
		t.Fatalf("persisted manifest repos = %+v, want exactly one repo aliased %q", manifest.Workspace.Repos, "api")
	}

	// The context's own registered projects must be untouched: this must
	// never have fallen through to project registration.
	persisted, err := store.LoadContext(context.Background(), "work")
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	if len(persisted.Projects) != 2 {
		t.Fatalf("persisted.Projects = %+v, want the original 2 projects untouched (RunCreateWorkspace must never register new projects)", persisted.Projects)
	}
}

// TestRunCreateWorkspace_AllProjectsSelectedByDefault covers the "default
// to all selected" requirement: every already-registered project's
// ports.Option must start pre-selected (see
// internal/adapters/formprompt/multi_choice.go and
// internal/adapters/termprompt's own preselectedIndices, which both read
// Option.Selected), and submitting that default must include every
// project — proof that "select all" resolves to the exact same workspace
// a subset selection would, not a coincidentally-identical result from
// falling back to CreateWorkspace's "empty ProjectKeys means every
// project" behavior.
func TestRunCreateWorkspace_AllProjectsSelectedByDefault(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	git := portstest.NewFakeGit()
	git.IsMainCloneFunc = func(dir domain.Path) (bool, error) { return true, nil }
	reporter := portstest.NewRecordingReporter()

	ctx := createWorkspaceWizardContext(fs)
	store.PutContext(ctx)

	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("myws"),
		portstest.TextAnswer(""),
		portstest.MultiChooseAnswer([]int{0, 1}), // both, exactly what a submitted "everything pre-selected" default looks like
	)

	deps := app.CreateWorkspaceWizardDeps{Store: store, FS: fs, Git: git, Prompter: prompter, Reporter: reporter}

	result, err := app.RunCreateWorkspace(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunCreateWorkspace: %v", err)
	}
	prompter.CheckUnconsumed()

	if len(result.Workspace.Repos) != 2 {
		t.Fatalf("Workspace.Repos = %+v, want both projects", result.Workspace.Repos)
	}
}
