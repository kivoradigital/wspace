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

// TestRunContextWizard_CollectsIgnorePatterns is the failing-test-first
// proof for this change's own authorized gap-closure: context-management
// spec requires "The system MUST accept a glob list of ignore_patterns at
// context initialization", but promptContextFields never prompted for it
// (the domain matcher, YAML round-trip and discovery filtering were all
// already built and tested against whatever ended up in
// domain.Context.IgnorePatterns — nothing ever populated it from the
// wizard). Before the fix, promptContextFields asks for workspaces_root,
// projects_root and base_branch, in that order, with nothing in between:
// this script's 4th scripted text answer (meant for ignore_patterns) is
// consumed instead as the base_branch answer, and the 5th scripted answer
// (meant for base_branch) is then offered to Confirm(copy_env_default),
// which is a kind mismatch ScriptedPrompter fails the test over — proving
// no distinct ignore_patterns prompt exists yet.
func TestRunContextWizard_CollectsIgnorePatterns(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("staging"),
		portstest.TextAnswer("/Users/me/workspaces"),
		portstest.TextAnswer("/Users/me/src"),
		portstest.TextAnswer(" vendor , archive-* "),
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
	)

	got, err := app.RunContextWizard(context.Background(), store, prompter)
	if err != nil {
		t.Fatalf("RunContextWizard: %v", err)
	}
	prompter.CheckUnconsumed()

	want := []domain.Glob{"vendor", "archive-*"}
	if len(got.IgnorePatterns) != len(want) {
		t.Fatalf("IgnorePatterns = %+v, want %+v", got.IgnorePatterns, want)
	}
	for i := range want {
		if got.IgnorePatterns[i] != want[i] {
			t.Fatalf("IgnorePatterns = %+v, want %+v", got.IgnorePatterns, want)
		}
	}
	if got.Defaults.BaseBranch == nil || *got.Defaults.BaseBranch != domain.BranchName("main") {
		t.Fatalf("Defaults.BaseBranch = %v, want pointer to \"main\" (must not have been overwritten by the ignore-patterns answer)", got.Defaults.BaseBranch)
	}

	persisted, err := store.LoadContext(context.Background(), "staging")
	if err != nil {
		t.Fatalf("LoadContext(staging) after wizard: %v", err)
	}
	if len(persisted.IgnorePatterns) != len(want) {
		t.Fatalf("persisted IgnorePatterns = %+v, want %+v (SaveContext must have been called)", persisted.IgnorePatterns, want)
	}
}

// TestRunContextWizard_EmptyIgnorePatternsIsNotAnError covers requirement
// 5 of this change's own gap-closure note: an empty ignore_patterns
// answer means "no patterns" and must not be treated as an error.
func TestRunContextWizard_EmptyIgnorePatternsIsNotAnError(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("staging"),
		portstest.TextAnswer("/Users/me/workspaces"),
		portstest.TextAnswer(""),
		portstest.TextAnswer(""),
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
	)

	got, err := app.RunContextWizard(context.Background(), store, prompter)
	if err != nil {
		t.Fatalf("RunContextWizard: %v", err)
	}
	prompter.CheckUnconsumed()
	if len(got.IgnorePatterns) != 0 {
		t.Fatalf("IgnorePatterns = %+v, want empty (nil) for an empty answer", got.IgnorePatterns)
	}
}

// TestRunContextWizard_ScriptedPrompter_CreatesContext covers tasks.md
// 3.16 (context-management spec: "Create a new context"): a scripted
// answer sequence drives one SaveContext call with the answers reflected
// in the persisted domain.Context.
func TestRunContextWizard_ScriptedPrompter_CreatesContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("staging"),
		portstest.TextAnswer("/Users/me/workspaces"),
		portstest.TextAnswer("/Users/me/src"),
		portstest.TextAnswer(""), // ignore_patterns -> none
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
	)

	got, err := app.RunContextWizard(context.Background(), store, prompter)
	if err != nil {
		t.Fatalf("RunContextWizard: %v", err)
	}
	prompter.CheckUnconsumed()

	if got.Name != "staging" {
		t.Fatalf("Name = %q, want %q", got.Name, "staging")
	}
	if got.WorkspacesRoot != "/Users/me/workspaces" {
		t.Fatalf("WorkspacesRoot = %q, want %q", got.WorkspacesRoot, "/Users/me/workspaces")
	}
	if got.ProjectsRoot != "/Users/me/src" {
		t.Fatalf("ProjectsRoot = %q, want %q", got.ProjectsRoot, "/Users/me/src")
	}
	if got.Defaults.BaseBranch == nil || *got.Defaults.BaseBranch != domain.BranchName("main") {
		t.Fatalf("Defaults.BaseBranch = %v, want pointer to \"main\"", got.Defaults.BaseBranch)
	}
	if got.Defaults.CopyEnv == nil || *got.Defaults.CopyEnv != true {
		t.Fatalf("Defaults.CopyEnv = %v, want pointer to true", got.Defaults.CopyEnv)
	}
	if got.Defaults.FetchBeforeCreate == nil || *got.Defaults.FetchBeforeCreate != false {
		t.Fatalf("Defaults.FetchBeforeCreate = %v, want pointer to false", got.Defaults.FetchBeforeCreate)
	}

	persisted, err := store.LoadContext(context.Background(), "staging")
	if err != nil {
		t.Fatalf("LoadContext(staging) after wizard: %v", err)
	}
	if persisted.Name != got.Name {
		t.Fatalf("persisted context Name = %q, want %q (SaveContext must have been called)", persisted.Name, got.Name)
	}
}

// TestRunEditContextWizard_PrefillsExistingValuesAsDefaults covers this
// change's own authorized gap-closure: RunEditContextWizard must prompt
// through the exact same field sequence as RunContextWizard (never a
// forked flow), with each field's current persisted value offered as the
// Default — proven here by accepting every default answer (empty text,
// zero-value bools aren't applicable since Confirm's Default IS the
// answer for an accepted default) and asserting the round-tripped context
// still carries the original values, plus one changed field (WorkspacesRoot)
// to prove edits really do take effect.
func TestRunEditContextWizard_PrefillsExistingValuesAsDefaults(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	baseBranch := domain.BranchName("develop")
	copyEnv := true
	fetchBeforeCreate := false
	store.PutContext(domain.Context{
		Name:           "staging",
		WorkspacesRoot: "/Users/me/old-workspaces",
		ProjectsRoot:   "/Users/me/src",
		Defaults: domain.Options{
			BaseBranch:        &baseBranch,
			CopyEnv:           &copyEnv,
			FetchBeforeCreate: &fetchBeforeCreate,
		},
		IgnorePatterns: []domain.Glob{"vendor", "dist"},
		Projects:       []domain.Project{{Key: "svc", SourceDir: "/Users/me/src/svc"}},
	})

	captured := map[string]ports.TextField{}
	prompter := &defaultCapturingPrompter{
		t: t,
		onText: func(f ports.TextField) string {
			captured[string(f.Label)] = f
			if f.Label == messages.WizardWorkspacesRoot {
				return "/Users/me/new-workspaces" // the one changed field
			}
			return f.Default
		},
		confirms: []bool{true, false},
	}

	got, _, err := app.RunEditContextWizard(context.Background(), store, portstest.NewFakeFS(t), prompter, "staging")
	if err != nil {
		t.Fatalf("RunEditContextWizard: %v", err)
	}

	if got.Name != "staging" {
		t.Fatalf("Name = %q, want unchanged %q", got.Name, "staging")
	}
	if got.WorkspacesRoot != "/Users/me/new-workspaces" {
		t.Fatalf("WorkspacesRoot = %q, want the edited value", got.WorkspacesRoot)
	}
	if got.ProjectsRoot != "/Users/me/src" {
		t.Fatalf("ProjectsRoot = %q, want the untouched default preserved", got.ProjectsRoot)
	}
	if got.Defaults.BaseBranch == nil || *got.Defaults.BaseBranch != domain.BranchName("develop") {
		t.Fatalf("Defaults.BaseBranch = %v, want the pre-existing value carried through as the accepted default", got.Defaults.BaseBranch)
	}
	if len(got.Projects) != 1 || got.Projects[0].Key != "svc" {
		t.Fatalf("Projects = %#v, want the pre-existing project list preserved untouched", got.Projects)
	}
	if len(got.IgnorePatterns) != 2 || got.IgnorePatterns[0] != "vendor" || got.IgnorePatterns[1] != "dist" {
		t.Fatalf("IgnorePatterns = %+v, want the pre-existing patterns preserved when the default is accepted", got.IgnorePatterns)
	}

	if f, ok := captured[string(messages.WizardWorkspacesRoot)]; !ok || f.Default != "/Users/me/old-workspaces" {
		t.Fatalf("workspaces_root field Default = %q, want the persisted value prefilled", f.Default)
	}
	if f, ok := captured[string(messages.WizardBaseBranch)]; !ok || f.Default != "develop" {
		t.Fatalf("base_branch field Default = %q, want the persisted value prefilled", f.Default)
	}
	if f, ok := captured[string(messages.WizardIgnorePatterns)]; !ok || f.Default != "vendor, dist" {
		t.Fatalf("ignore_patterns field Default = %q, want the persisted patterns joined and prefilled", f.Default)
	}

	persisted, err := store.LoadContext(context.Background(), "staging")
	if err != nil {
		t.Fatalf("LoadContext(staging) after edit: %v", err)
	}
	if persisted.WorkspacesRoot != "/Users/me/new-workspaces" {
		t.Fatalf("persisted WorkspacesRoot = %q, want the edited value (SaveContext must have been called)", persisted.WorkspacesRoot)
	}
}

// TestRunEditContextWizard_CanRenameContext covers the product owner's own
// requirement ("I must also be able to change the context's name" —
// tray-gui gap-closure #2). RunEditContextWizard now shares
// promptContextName with RunContextWizard (never a forked name prompt),
// prefilled with the existing name so an unmodified ENTER keeps it
// unchanged; answering a different name renames the record: the new name
// is saved, the old one is deleted, and — since "staging" was the active
// context here — the root config is re-pointed at the new name so no
// command ever resolves "the active context" to a name that no longer
// exists.
func TestRunEditContextWizard_CanRenameContext(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "staging", WorkspacesRoot: "/Users/me/workspaces"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "staging"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("production"), // renamed
		portstest.TextAnswer("/Users/me/workspaces"),
		portstest.TextAnswer(""),
		portstest.TextAnswer(""), // ignore_patterns -> none
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
	)

	got, _, err := app.RunEditContextWizard(context.Background(), store, portstest.NewFakeFS(t), prompter, "staging")
	if err != nil {
		t.Fatalf("RunEditContextWizard: %v", err)
	}
	prompter.CheckUnconsumed()

	if got.Name != "production" {
		t.Fatalf("Name = %q, want the renamed value %q", got.Name, "production")
	}
	if _, err := store.LoadContext(context.Background(), "staging"); err == nil {
		t.Fatal("LoadContext(staging) succeeded after rename, want the old name gone")
	}
	renamed, err := store.LoadContext(context.Background(), "production")
	if err != nil {
		t.Fatalf("LoadContext(production) after rename: %v", err)
	}
	if renamed.WorkspacesRoot != "/Users/me/workspaces" {
		t.Fatalf("WorkspacesRoot = %q, want preserved across the rename", renamed.WorkspacesRoot)
	}

	root, err := store.LoadRoot(context.Background())
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if root.ActiveContext != "production" {
		t.Fatalf("root.ActiveContext = %q, want it re-pointed to the renamed context", root.ActiveContext)
	}
}

// TestRunEditContextWizard_RenamingInactiveContextLeavesActivePointerAlone
// covers the other half of the rename guard: renaming a context that is
// NOT currently active must never touch the root config at all.
func TestRunEditContextWizard_RenamingInactiveContextLeavesActivePointerAlone(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "staging", WorkspacesRoot: "/Users/me/workspaces"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("production"),
		portstest.TextAnswer("/Users/me/workspaces"),
		portstest.TextAnswer(""),
		portstest.TextAnswer(""),
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false),
	)

	if _, _, err := app.RunEditContextWizard(context.Background(), store, portstest.NewFakeFS(t), prompter, "staging"); err != nil {
		t.Fatalf("RunEditContextWizard: %v", err)
	}

	root, err := store.LoadRoot(context.Background())
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if root.ActiveContext != "work" {
		t.Fatalf("root.ActiveContext = %q, want it untouched by an inactive context's rename", root.ActiveContext)
	}
}

// defaultCapturingPrompter is a minimal ports.Prompter used only by
// TestRunEditContextWizard_PrefillsExistingValuesAsDefaults, which needs to
// both inspect every field's Default (portstest.ScriptedPrompter discards
// the field it was asked, only ever returning its scripted answer) and
// answer "accept the default" for text fields — something a fixed FIFO
// script cannot express generically since the default varies per context.
type defaultCapturingPrompter struct {
	t        *testing.T
	onText   func(ports.TextField) string
	confirms []bool
	confirmI int
}

func (p *defaultCapturingPrompter) Text(_ context.Context, f ports.TextField) (string, error) {
	return p.onText(f), nil
}

func (p *defaultCapturingPrompter) Confirm(_ context.Context, f ports.ConfirmField) (bool, error) {
	if p.confirmI >= len(p.confirms) {
		p.t.Fatalf("Confirm called more times than scripted (label %q)", f.Label)
	}
	v := p.confirms[p.confirmI]
	p.confirmI++
	return v, nil
}

func (p *defaultCapturingPrompter) Choose(_ context.Context, f ports.ChoiceField) (int, error) {
	p.t.Fatalf("Choose unexpectedly called (label %q)", f.Label)
	return 0, nil
}

func (p *defaultCapturingPrompter) MultiChoose(_ context.Context, f ports.ChoiceField) ([]int, error) {
	p.t.Fatalf("MultiChoose unexpectedly called (label %q)", f.Label)
	return nil, nil
}

// Group dispatches every field in step to this fake's own Text/Confirm
// (portstest.DispatchGroup), exactly like TerminalPrompter.Group and
// portstest.ScriptedPrompter.Group do — RunEditContextWizard now asks its
// whole field set as one Step, so this fake must still answer it the same
// way it always answered each field individually.
func (p *defaultCapturingPrompter) Group(ctx context.Context, step ports.Step) (ports.StepAnswers, error) {
	return portstest.DispatchGroup(ctx, p, step)
}
