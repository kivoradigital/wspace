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

// TestRunCreateContext_DrivesProjectRegistrationAfterContextCreation is
// the shared-use-case regression test for this change's own fix: creating
// a context from the tray never used to reach project registration,
// because the two-wizard sequence (RunContextWizard, then
// RunProjectWizard) was composed independently inside internal/cli's own
// "context create" command, and internal/gui/wizard's ContextWindow.Open
// only ever ran the first half. Testing this against RunCreateContext
// itself — never against either front end — is what makes both front
// ends inherit the guarantee: neither internal/cli/cmd_context.go nor
// internal/gui/wizard/context_window.go composes these two wizard calls
// itself any more.
func TestRunCreateContext_DrivesProjectRegistrationAfterContextCreation(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("api", ".git")); err != nil {
		t.Fatalf("mkdir api/.git: %v", err)
	}
	fakeGit := portstest.NewFakeGit()
	fakeGit.IsMainCloneFunc = func(dir domain.Path) (bool, error) { return true, nil }

	prompter := portstest.NewScriptedPrompter(t,
		// RunContextWizard's own Step: name + the six option fields.
		portstest.TextAnswer("work"),
		portstest.TextAnswer(string(fakeFS.Paths().Home.Join("workspaces"))),
		portstest.TextAnswer(string(projectsRoot)),
		portstest.TextAnswer(""), // ignore_patterns -> none
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
		// RunProjectWizard's own scan-and-select Step, continuing in the
		// same session immediately afterward — the actual fix.
		portstest.MultiChooseAnswer([]int{0}), // "api"
		portstest.TextAnswer("api"),
		portstest.TextAnswer(""), // origin_branch -> inherit
		portstest.TextAnswer(""), // dest_branch -> inherit
		portstest.TextAnswer(""), // worktree_dir -> inherit
	)

	got, err := app.RunCreateContext(context.Background(), app.CreateContextDeps{
		Store:    store,
		FS:       fakeFS,
		Git:      fakeGit,
		Prompter: prompter,
	})
	if err != nil {
		t.Fatalf("RunCreateContext: %v", err)
	}
	prompter.CheckUnconsumed()

	if got.Name != "work" {
		t.Fatalf("Name = %q, want %q", got.Name, "work")
	}
	if len(got.Projects) != 1 || got.Projects[0].Key != "api" {
		t.Fatalf("Projects = %+v, want exactly one mapped project (\"api\") — project registration must run right after context creation", got.Projects)
	}

	persisted, err := store.LoadContext(context.Background(), "work")
	if err != nil {
		t.Fatalf("LoadContext(work) after RunCreateContext: %v", err)
	}
	if len(persisted.Projects) != 1 {
		t.Fatalf("persisted Projects = %+v, want the mapped project saved", persisted.Projects)
	}
}

// TestRunCreateContext_StillSucceedsWithNoProjectsMapped covers the
// existing "decline everything" outcome RunProjectWizard already supports
// (its own fallback to manual registration, declined): a freshly created,
// project-less context is still a valid, successful outcome — never an
// error just because nothing was mapped.
func TestRunCreateContext_StillSucceedsWithNoProjectsMapped(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)

	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("work"),
		portstest.TextAnswer(string(fakeFS.Paths().Home.Join("workspaces"))),
		portstest.TextAnswer(""), // projects_root -> skip scanning
		portstest.TextAnswer(""), // ignore_patterns -> none
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
		portstest.ConfirmAnswer(false), // decline manual project registration
	)

	got, err := app.RunCreateContext(context.Background(), app.CreateContextDeps{
		Store:    store,
		FS:       fakeFS,
		Git:      portstest.NewFakeGit(),
		Prompter: prompter,
	})
	if err != nil {
		t.Fatalf("RunCreateContext: %v", err)
	}
	prompter.CheckUnconsumed()
	if len(got.Projects) != 0 {
		t.Fatalf("Projects = %+v, want none", got.Projects)
	}
}

// createContextStepPrompter wraps a *portstest.ScriptedPrompter to record
// every ports.Step a Group call is asked with (seenSteps), and to resolve
// the scanned-candidate selection Step with ports.ErrStepBack the first
// time it is asked, when backOnFirstSelection is set — the two things
// TestRunCreateContext_StepsCarryIndexAndBack and
// TestRunCreateContext_BackReturnsToContextFieldsStepWithAnswersPreserved
// need that a plain ScriptedPrompter cannot express. Every field answer
// still comes from inner, in FIFO order, exactly like ScriptedPrompter
// alone — this wrapper only ever observes or short-circuits Group itself,
// never Text/Confirm/Choose/MultiChoose.
type createContextStepPrompter struct {
	inner                *portstest.ScriptedPrompter
	backOnFirstSelection bool
	selectionAsked       int
	seenSteps            []ports.Step
}

func (p *createContextStepPrompter) Text(ctx context.Context, f ports.TextField) (string, error) {
	return p.inner.Text(ctx, f)
}

func (p *createContextStepPrompter) Confirm(ctx context.Context, f ports.ConfirmField) (bool, error) {
	return p.inner.Confirm(ctx, f)
}

func (p *createContextStepPrompter) Choose(ctx context.Context, f ports.ChoiceField) (int, error) {
	return p.inner.Choose(ctx, f)
}

func (p *createContextStepPrompter) MultiChoose(ctx context.Context, f ports.ChoiceField) ([]int, error) {
	return p.inner.MultiChoose(ctx, f)
}

func (p *createContextStepPrompter) Group(ctx context.Context, step ports.Step) (ports.StepAnswers, error) {
	p.seenSteps = append(p.seenSteps, step)
	isSelectionStep := len(step.Fields) == 1 && step.Fields[0].MultiChoice != nil
	if isSelectionStep {
		p.selectionAsked++
		if p.backOnFirstSelection && p.selectionAsked == 1 {
			return ports.StepAnswers{}, ports.ErrStepBack
		}
	}
	return portstest.DispatchGroup(ctx, p, step)
}

// TestRunCreateContext_StepsCarryIndexAndBack covers this change's own
// back-navigation fix: RunContextWizard's field Step and RunProjectWizard's
// selection Step used to each believe they were "step 1 of 1" — neither a
// Back button nor a step indicator ever rendered, and there was no way back
// from project selection to the context fields just filled in. Sequenced
// through RunCreateContext, they must instead present as one two-step flow:
// the context-fields Step as Step 1 of 2 with no Back (nothing before it to
// return to), and the scanned-candidate selection Step as Step 2 of 2 with
// Back true.
func TestRunCreateContext_StepsCarryIndexAndBack(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("api", ".git")); err != nil {
		t.Fatalf("mkdir api/.git: %v", err)
	}
	workspacesRoot := fakeFS.Paths().Home.Join("workspaces")
	fakeGit := portstest.NewFakeGit()
	fakeGit.IsMainCloneFunc = func(dir domain.Path) (bool, error) { return true, nil }

	inner := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("work"),
		portstest.TextAnswer(string(workspacesRoot)),
		portstest.TextAnswer(string(projectsRoot)),
		portstest.TextAnswer(""), // ignore_patterns -> none
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
		portstest.MultiChooseAnswer([]int{0}),
		portstest.TextAnswer("api"),
		portstest.TextAnswer(""), portstest.TextAnswer(""), portstest.TextAnswer(""),
	)
	prompter := &createContextStepPrompter{inner: inner}

	deps := app.CreateContextDeps{Store: store, FS: fakeFS, Git: fakeGit, Prompter: prompter}

	if _, err := app.RunCreateContext(context.Background(), deps); err != nil {
		t.Fatalf("RunCreateContext: %v", err)
	}
	inner.CheckUnconsumed()

	if len(prompter.seenSteps) != 2 {
		t.Fatalf("Group was called %d time(s), want exactly 2 (the context-fields Step and the selection Step)", len(prompter.seenSteps))
	}
	if s := prompter.seenSteps[0]; s.Index != 1 || s.Total != 2 || s.Back {
		t.Fatalf("Step 1 = %+v, want Index=1 Total=2 Back=false", s)
	}
	if s := prompter.seenSteps[1]; s.Index != 2 || s.Total != 2 || !s.Back {
		t.Fatalf("Step 2 = %+v, want Index=2 Total=2 Back=true", s)
	}
}

// TestRunCreateContext_BackReturnsToContextFieldsStepWithAnswersPreserved
// covers the other half of the same fix: when the selection Step resolves
// with ports.ErrStepBack, RunCreateContext must return to the
// context-fields Step and offer it again, prefilled with exactly what was
// submitted before the Back — never abort, never silently discard what the
// user already typed (a Back that discards answers is worse than no Back
// at all), and still finish the wizard normally afterward.
func TestRunCreateContext_BackReturnsToContextFieldsStepWithAnswersPreserved(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("api", ".git")); err != nil {
		t.Fatalf("mkdir api/.git: %v", err)
	}
	workspacesRoot := fakeFS.Paths().Home.Join("workspaces")
	fakeGit := portstest.NewFakeGit()
	fakeGit.IsMainCloneFunc = func(dir domain.Path) (bool, error) { return true, nil }

	inner := portstest.NewScriptedPrompter(t,
		// Step 1 of 2, answered once...
		portstest.TextAnswer("work"),
		portstest.TextAnswer(string(workspacesRoot)),
		portstest.TextAnswer(string(projectsRoot)),
		portstest.TextAnswer(""),
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
		// Step 2 of 2 resolves with ErrStepBack the first time (scripted
		// via backOnFirstSelection below) — no answer is consumed for it.
		// Step 1 of 2, asked again after the Back — the same answers
		// re-submitted.
		portstest.TextAnswer("work"),
		portstest.TextAnswer(string(workspacesRoot)),
		portstest.TextAnswer(string(projectsRoot)),
		portstest.TextAnswer(""),
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
		// Step 2 of 2, answered for real this time.
		portstest.MultiChooseAnswer([]int{0}),
		portstest.TextAnswer("api"),
		portstest.TextAnswer(""), portstest.TextAnswer(""), portstest.TextAnswer(""),
	)
	prompter := &createContextStepPrompter{inner: inner, backOnFirstSelection: true}

	deps := app.CreateContextDeps{Store: store, FS: fakeFS, Git: fakeGit, Prompter: prompter}

	got, err := app.RunCreateContext(context.Background(), deps)
	if err != nil {
		t.Fatalf("RunCreateContext: %v", err)
	}
	inner.CheckUnconsumed()

	if len(prompter.seenSteps) != 4 {
		t.Fatalf("Group was called %d time(s), want exactly 4 (context step, selection step returning Back, context step again, selection step answered for real)", len(prompter.seenSteps))
	}
	first, second, third, fourth := prompter.seenSteps[0], prompter.seenSteps[1], prompter.seenSteps[2], prompter.seenSteps[3]
	if first.Index != 1 || first.Total != 2 || first.Back {
		t.Fatalf("first Step = %+v, want Index=1 Total=2 Back=false", first)
	}
	if second.Index != 2 || second.Total != 2 || !second.Back {
		t.Fatalf("second Step = %+v, want Index=2 Total=2 Back=true", second)
	}
	if third.Index != 1 || third.Total != 2 || third.Back {
		t.Fatalf("third Step (after Back) = %+v, want Index=1 Total=2 Back=false", third)
	}
	if fourth.Index != 2 || fourth.Total != 2 || !fourth.Back {
		t.Fatalf("fourth Step = %+v, want Index=2 Total=2 Back=true", fourth)
	}

	if got := third.Fields[0].Text.Default; got != "work" {
		t.Fatalf("re-asked name Default = %q, want %q (the value submitted before Back)", got, "work")
	}
	if got := third.Fields[1].Text.Default; got != string(workspacesRoot) {
		t.Fatalf("re-asked workspaces_root Default = %q, want %q", got, workspacesRoot)
	}
	if got := third.Fields[2].Text.Default; got != string(projectsRoot) {
		t.Fatalf("re-asked projects_root Default = %q, want %q", got, projectsRoot)
	}
	if got := third.Fields[4].Text.Default; got != "main" {
		t.Fatalf("re-asked base_branch Default = %q, want %q", got, "main")
	}
	if got := third.Fields[5].Confirm.Default; got != true {
		t.Fatalf("re-asked copy_env_default Default = %v, want true", got)
	}

	if got.Name != "work" {
		t.Fatalf("Name = %q, want %q", got.Name, "work")
	}
	if len(got.Projects) != 1 || got.Projects[0].Key != "api" {
		t.Fatalf("Projects = %+v, want exactly one mapped project (\"api\") — the wizard must still finish normally after a Back", got.Projects)
	}
}
