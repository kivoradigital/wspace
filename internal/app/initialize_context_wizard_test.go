// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestRunInitializeContextWizard_CollectsRootsAndMapsProjects covers
// tray-gui gap-closure #2's own ordering requirement: "projects folder ->
// scan that folder and offer the git repositories found -> let the user
// choose which of them I want to map -> workspaces folder". Before this
// use case existed, there was no way to drive that sequence at all: context
// creation asked for workspaces_root before projects_root
// (promptContextFields's fixed order), and project registration was a
// wholly separate call the caller had to remember to make.
func TestRunInitializeContextWizard_CollectsRootsAndMapsProjects(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	for _, d := range []string{"api", "web"} {
		if err := fakeFS.MkdirAll(projectsRoot.Join(d, ".git")); err != nil {
			t.Fatalf("mkdir %q/.git: %v", d, err)
		}
	}
	workspacesRoot := fakeFS.Paths().Home.Join("workspaces")

	store.PutContext(domain.Context{Name: "work"})

	fakeGit := portstest.NewFakeGit()
	fakeGit.IsMainCloneFunc = func(dir domain.Path) (bool, error) { return true, nil }

	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer(string(projectsRoot)),
		portstest.TextAnswer(string(workspacesRoot)),
		portstest.MultiChooseAnswer([]int{0}), // "api" only
		portstest.TextAnswer("api"),
		portstest.TextAnswer(""), // origin_branch -> inherit
		portstest.TextAnswer(""), // dest_branch -> inherit
		portstest.TextAnswer(""), // worktree_dir -> inherit
	)

	deps := app.ProjectWizardDeps{
		Store:    store,
		FS:       fakeFS,
		Git:      fakeGit,
		Prompter: prompter,
	}

	got, err := app.RunInitializeContextWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunInitializeContextWizard: %v", err)
	}
	prompter.CheckUnconsumed()

	if got.ProjectsRoot != projectsRoot {
		t.Fatalf("ProjectsRoot = %q, want %q", got.ProjectsRoot, projectsRoot)
	}
	if got.WorkspacesRoot != workspacesRoot {
		t.Fatalf("WorkspacesRoot = %q, want %q", got.WorkspacesRoot, workspacesRoot)
	}
	if len(got.Projects) != 1 || got.Projects[0].Key != "api" {
		t.Fatalf("Projects = %+v, want exactly one mapped project (\"api\")", got.Projects)
	}

	persisted, err := store.LoadContext(context.Background(), "work")
	if err != nil {
		t.Fatalf("LoadContext(work) after initialize: %v", err)
	}
	if persisted.ProjectsRoot != projectsRoot || persisted.WorkspacesRoot != workspacesRoot || len(persisted.Projects) != 1 {
		t.Fatalf("persisted context = %+v, want roots and the mapped project all saved", persisted)
	}
}

// TestRunInitializeContextWizard_PersistsProjectsRootEvenIfScanFindsNothing
// covers the "save as you go" behavior: ProjectsRoot must already be
// persisted by the time the scan runs (RunProjectWizard reloads the
// context by name), so a scan that maps nothing still leaves the folder
// choice itself in place rather than silently discarding it.
func TestRunInitializeContextWizard_PersistsProjectsRootEvenIfScanFindsNothing(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("empty-src")
	if err := fakeFS.MkdirAll(projectsRoot); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	workspacesRoot := fakeFS.Paths().Home.Join("workspaces")

	store.PutContext(domain.Context{Name: "work"})

	fakeGit := portstest.NewFakeGit()

	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer(string(projectsRoot)),
		portstest.TextAnswer(string(workspacesRoot)),
		portstest.ConfirmAnswer(false), // decline manual project registration
	)

	deps := app.ProjectWizardDeps{
		Store:    store,
		FS:       fakeFS,
		Git:      fakeGit,
		Prompter: prompter,
	}

	got, err := app.RunInitializeContextWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunInitializeContextWizard: %v", err)
	}
	prompter.CheckUnconsumed()

	if got.ProjectsRoot != projectsRoot {
		t.Fatalf("ProjectsRoot = %q, want %q even with no projects mapped", got.ProjectsRoot, projectsRoot)
	}
	if len(got.Projects) != 0 {
		t.Fatalf("Projects = %+v, want none", got.Projects)
	}
}

// TestRunInitializeContextWizard_StepsCarryIndexAndBack covers the
// wizard's own genuine two-step structure (design's own concrete example:
// "Project selection after the scan is a genuine second step"): the
// projects-folder Step is asked as Step 1 of 2 with no Back, and the
// scanned-candidate selection is asked as Step 2 of 2 with Back true — the
// only two Steps in this whole wizard, since workspaces_root depends on
// neither and is asked as a plain single-field prompt exactly as before,
// never wrapped in Step chrome of its own.
func TestRunInitializeContextWizard_StepsCarryIndexAndBack(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("api", ".git")); err != nil {
		t.Fatalf("mkdir api/.git: %v", err)
	}
	workspacesRoot := fakeFS.Paths().Home.Join("workspaces")
	store.PutContext(domain.Context{Name: "work"})

	fakeGit := portstest.NewFakeGit()
	fakeGit.IsMainCloneFunc = func(dir domain.Path) (bool, error) { return true, nil }

	var seenSteps []ports.Step
	prompter := &stepRecordingPrompter{
		t:                 t,
		onGroup:           func(step ports.Step) { seenSteps = append(seenSteps, step) },
		textAnswers:       []string{string(projectsRoot), string(workspacesRoot), "api", "", "", ""},
		multiChoiceAnswer: []int{0},
	}

	deps := app.ProjectWizardDeps{Store: store, FS: fakeFS, Git: fakeGit, Prompter: prompter}

	if _, err := app.RunInitializeContextWizard(context.Background(), deps, "work"); err != nil {
		t.Fatalf("RunInitializeContextWizard: %v", err)
	}

	if len(seenSteps) != 2 {
		t.Fatalf("Group was called %d time(s), want exactly 2 (the projects-folder Step and the selection Step)", len(seenSteps))
	}
	if seenSteps[0].Index != 1 || seenSteps[0].Total != 2 || seenSteps[0].Back {
		t.Fatalf("Step 1 = %+v, want Index=1 Total=2 Back=false", seenSteps[0])
	}
	if seenSteps[1].Index != 2 || seenSteps[1].Total != 2 || !seenSteps[1].Back {
		t.Fatalf("Step 2 = %+v, want Index=2 Total=2 Back=true", seenSteps[1])
	}
}

// TestRunInitializeContextWizard_BackReturnsToProjectsFolderStep covers
// the other half of the same rule: when the selection Step resolves with
// ports.ErrStepBack, the wizard must return to the projects-folder Step
// and offer it again — never abort, never skip straight to workspaces_root
// as if nothing happened.
func TestRunInitializeContextWizard_BackReturnsToProjectsFolderStep(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("api", ".git")); err != nil {
		t.Fatalf("mkdir api/.git: %v", err)
	}
	workspacesRoot := fakeFS.Paths().Home.Join("workspaces")
	store.PutContext(domain.Context{Name: "work"})

	fakeGit := portstest.NewFakeGit()
	fakeGit.IsMainCloneFunc = func(dir domain.Path) (bool, error) { return true, nil }

	stepOneCalls := 0
	prompter := &stepRecordingPrompter{
		t: t,
		onGroup: func(step ports.Step) {
			if step.Index == 1 {
				stepOneCalls++
			}
		},
		backOnFirstSelection: true,
		// Step 1 (projects_root + workspaces_root together) is answered
		// once before the Back, and again after — both fields, both times.
		textAnswers:       []string{string(projectsRoot), string(workspacesRoot), string(projectsRoot), string(workspacesRoot), "api", "", "", ""},
		multiChoiceAnswer: []int{0},
	}

	deps := app.ProjectWizardDeps{Store: store, FS: fakeFS, Git: fakeGit, Prompter: prompter}

	got, err := app.RunInitializeContextWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunInitializeContextWizard: %v", err)
	}
	if stepOneCalls != 2 {
		t.Fatalf("projects-folder Step was asked %d time(s), want 2 (once, then again after Back)", stepOneCalls)
	}
	if got.WorkspacesRoot != workspacesRoot {
		t.Fatalf("WorkspacesRoot = %q, want %q (the wizard must still finish normally after a Back)", got.WorkspacesRoot, workspacesRoot)
	}
	if len(got.Projects) != 1 || got.Projects[0].Key != "api" {
		t.Fatalf("Projects = %+v, want exactly one mapped project (\"api\")", got.Projects)
	}
}

// stepRecordingPrompter is a minimal ports.Prompter used only by the two
// tests above: it answers every Step 1 (a lone text field: projects_root)
// with the next textAnswers entry, and every Step 2 (a lone multi-choice
// field: project selection) with multiChoiceAnswer — returning
// ports.ErrStepBack instead the very first time Step 2 is asked, when
// backOnFirstSelection is set. onGroup, when set, is called with every
// Step this prompter is asked, letting a test record what it saw without
// this fake having to expose its own internal state.
type stepRecordingPrompter struct {
	t                    *testing.T
	onGroup              func(ports.Step)
	backOnFirstSelection bool
	selectionAsked       int
	textAnswers          []string
	textI                int
	multiChoiceAnswer    []int
}

func (p *stepRecordingPrompter) Text(_ context.Context, f ports.TextField) (string, error) {
	if p.textI >= len(p.textAnswers) {
		p.t.Fatalf("Text called more times than scripted (label %q)", f.Label)
	}
	v := p.textAnswers[p.textI]
	p.textI++
	return v, nil
}

func (p *stepRecordingPrompter) Confirm(_ context.Context, f ports.ConfirmField) (bool, error) {
	p.t.Fatalf("Confirm unexpectedly called (label %q)", f.Label)
	return false, nil
}

func (p *stepRecordingPrompter) Choose(_ context.Context, f ports.ChoiceField) (int, error) {
	p.t.Fatalf("Choose unexpectedly called (label %q)", f.Label)
	return 0, nil
}

func (p *stepRecordingPrompter) MultiChoose(_ context.Context, f ports.ChoiceField) ([]int, error) {
	p.t.Fatalf("MultiChoose unexpectedly called (label %q)", f.Label)
	return nil, nil
}

func (p *stepRecordingPrompter) Group(ctx context.Context, step ports.Step) (ports.StepAnswers, error) {
	if p.onGroup != nil {
		p.onGroup(step)
	}
	if len(step.Fields) == 1 && step.Fields[0].MultiChoice != nil {
		p.selectionAsked++
		if p.backOnFirstSelection && p.selectionAsked == 1 {
			return ports.StepAnswers{}, ports.ErrStepBack
		}
		answers := ports.NewStepAnswers()
		answers.MultiChoice[messages.WizardPickProjects] = p.multiChoiceAnswer
		return answers, nil
	}
	return portstest.DispatchGroup(ctx, p, step)
}

// TestRunInitializeContextWizard_BackNeverSavesANamelessContext covers a
// defect in the Back path, and the earlier Back test could not see it:
// that fixture's answers were identical either side of the Back, so
// nothing it asserted on differed.
//
// The loop used to assign the selection step's return value before
// checking its error. On the Back path that value is the zero Context, so
// the next iteration set the two roots on an empty struct and saved it.
// Both stores key a context by its own Name, so an empty name does not
// overwrite the real context — it writes a second, nameless one, which in
// the real store lands as a stray config file directly in the contexts
// directory.
//
// Going back is a navigation, never a write.
func TestRunInitializeContextWizard_BackNeverSavesANamelessContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("api", ".git")); err != nil {
		t.Fatalf("mkdir api/.git: %v", err)
	}
	workspacesRoot := fakeFS.Paths().Home.Join("workspaces")
	store.PutContext(domain.Context{Name: "work"})

	fakeGit := portstest.NewFakeGit()
	fakeGit.IsMainCloneFunc = func(domain.Path) (bool, error) { return true, nil }

	prompter := &stepRecordingPrompter{
		t:                    t,
		backOnFirstSelection: true,
		textAnswers:          []string{string(projectsRoot), string(workspacesRoot), string(projectsRoot), string(workspacesRoot), "api", "", "", ""},
		multiChoiceAnswer:    []int{0},
	}

	deps := app.ProjectWizardDeps{Store: store, FS: fakeFS, Git: fakeGit, Prompter: prompter, Reporter: portstest.NewRecordingReporter()}
	if _, err := app.RunInitializeContextWizard(context.Background(), deps, "work"); err != nil {
		t.Fatalf("RunInitializeContextWizard: %v", err)
	}

	names, err := store.ListContexts(context.Background())
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	for _, n := range names {
		if n == "" {
			t.Fatalf("a context was saved under an empty name; stored names = %q", names)
		}
	}
	if len(names) != 1 || names[0] != "work" {
		t.Errorf("stored contexts = %q, want exactly the one the wizard was given", names)
	}
}
