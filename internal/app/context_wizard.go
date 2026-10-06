// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"errors"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// ContextWizardDeps bundles RunContextWizard's two driven ports so
// internal/cli can hold and forward one app-owned value (R6: internal/cli
// may only import app, domain, messages) instead of two bare ports-typed
// values. RunContextWizard's own signature is unchanged from phase 3 — this
// struct is purely additive wiring sugar for the composition root.
type ContextWizardDeps struct {
	Store    ports.ConfigStore
	Prompter ports.Prompter
}

// RunContextWizard drives ports.Prompter to collect a new context's fields
// and persists it via ports.ConfigStore.SaveContext (context-management
// spec: "Create a new context"). This one use case drives both the
// terminal Prompter and the GUI FormPrompter (design.md §4, §8.4) — wizard
// logic is never duplicated per surface.
func RunContextWizard(ctx context.Context, store ports.ConfigStore, prompter ports.Prompter) (domain.Context, error) {
	c, err := promptContext(ctx, prompter, "", domain.Context{})
	if err != nil {
		return domain.Context{}, err
	}
	if err := store.SaveContext(ctx, c); err != nil {
		return domain.Context{}, err
	}
	return c, nil
}

// RunEditContextWizard drives the exact same field sequence as
// RunContextWizard (promptContext, below) against an *existing*
// context's current values as each field's Default, so revisiting an
// option set means answering (or accepting the default for) the same
// questions again, never starting over from blank fields
// (context-management spec's "create, list, switch and edit operations on
// contexts" — edit was the one of the four left unimplemented through
// phase 7; see this change's own authorized gap-closure note).
//
// The context's name is now editable here too (tray-gui gap-closure #2:
// "I must also be able to change the context's name" — the product
// owner's own requirement). It is deliberately collected in *this* flow
// rather than "Initialize context…"'s: initialization is about roots and
// which repositories are mapped, never identity, so folding a rename into
// it would make one menu item do two unrelated things. Edit already
// revisits every other field on the same record, so a rename is simply
// one more field in that same sequence — prefilled with the current name,
// so accepting the default (an unmodified ENTER/submit) is a no-op
// exactly like every other field here.
//
// A changed name is not a simple field update: SaveContext keys purely by
// domain.Context.Name, so saving under a new name while the old record
// still exists would leave two contexts describing the same thing. This
// function instead: saves the record under the new name, re-points the
// root config's active context at the new name IF the renamed context was
// the active one (never touching the root config otherwise), and only
// then deletes the stale record under the old name — in that order, so a
// failure partway through never leaves the store with zero contexts
// resolvable as "active".
//
// This never forks RunContextWizard's own field logic: both call the same
// promptContext, so terminal and GUI callers — and create and edit — can
// never drift from one another.
func RunEditContextWizard(ctx context.Context, store ports.ConfigStore, fs ports.FileSystemPort, prompter ports.Prompter, name domain.ContextName) (domain.Context, WorkspaceReassignment, error) {
	existing, err := store.LoadContext(ctx, name)
	if err != nil {
		return domain.Context{}, WorkspaceReassignment{}, err
	}

	c, err := promptContext(ctx, prompter, name, existing)
	if err != nil {
		return domain.Context{}, WorkspaceReassignment{}, err
	}
	re, err := saveEditedContext(ctx, store, fs, existing, c)
	if err != nil {
		return domain.Context{}, WorkspaceReassignment{}, err
	}
	return c, re, nil
}

// saveEditedContext persists c, an edited version of existing, handling a
// rename (c.Name != existing.Name) in the order RunEditContextWizard's own
// doc comment explains: refuse a rename onto another existing context (it
// would be silently overwritten), save under the new name, re-point the
// active context if it was the renamed one, and only then delete the old
// record.
//
// After a successful rename, every workspace manifest the old name owned
// (under the old and the new WorkspacesRoot) is moved to the new name, so
// the context's workspaces never vanish from its listing. That step never
// undoes the rename: a manifest that cannot be rewritten is reported in the
// returned WorkspaceReassignment and stays claimable (ClaimWorkspaces).
func saveEditedContext(ctx context.Context, store ports.ConfigStore, fs ports.FileSystemPort, existing domain.Context, c domain.Context) (WorkspaceReassignment, error) {
	oldName := existing.Name
	if c.Name != oldName {
		if _, err := store.LoadContext(ctx, c.Name); err == nil {
			return WorkspaceReassignment{}, domain.NewOpError("context.rename", domain.CodeContextExists, string(c.Name), "", nil)
		} else if domain.Code(err) != domain.CodeContextNotFound {
			return WorkspaceReassignment{}, err
		}
	}
	if err := store.SaveContext(ctx, c); err != nil {
		return WorkspaceReassignment{}, err
	}
	if c.Name == oldName {
		return WorkspaceReassignment{}, nil
	}

	root, err := store.LoadRoot(ctx)
	if err != nil && !errors.Is(err, ports.ErrConfigNotInitialized) {
		return WorkspaceReassignment{}, err
	}
	if root.ActiveContext == oldName {
		root.ActiveContext = c.Name
		if err := store.SaveRoot(ctx, root); err != nil {
			return WorkspaceReassignment{}, err
		}
	}
	if err := store.DeleteContext(ctx, oldName); err != nil {
		return WorkspaceReassignment{}, err
	}
	return reassignWorkspaces(ctx, store, fs, []domain.Path{existing.WorkspacesRoot, c.WorkspacesRoot}, oldName, c.Name), nil
}

// promptContext collects a context's name and every context-level option
// field (workspaces_root, projects_root, ignore_patterns, base_branch,
// copy_env_default, fetch_before_create) as a single ports.Step — one GUI
// page, never a sequence of one-field windows (the product layout rule:
// "one page when the fields fit" — every one of these seven fields is
// independent of the others, so nothing here ever depends on an earlier
// answer within this same wizard). defaultName/defaults is "" and the zero
// domain.Context for RunContextWizard's brand-new context, or the existing
// name and its persisted values for RunEditContextWizard, prefilled as
// each field's Default so an unmodified terminal ENTER or GUI submit
// reproduces the existing record exactly (name included — the product
// owner's own requirement to be able to rename a context from the same
// screen that edits everything else about it).
//
// defaults.Projects is carried through untouched: this Step never
// collects a project list (that comes from RunProjectWizard), so editing
// these fields must never silently drop projects an earlier
// RunProjectWizard run already registered.
//
// ignore_patterns is collected right after projects_root, and workspaces_
// root/projects_root/ignore_patterns/base_branch/copy_env_default/
// fetch_before_create are asked in that exact order — unchanged from
// before this Step existed — because cmd_context.go's "context create"
// command calls RunContextWizard (reaching this function) to completion,
// including SaveContext, before it ever calls RunProjectWizard for the
// same context, so whatever ignore_patterns this Step collects is already
// persisted by the time the very first project-discovery scan reads it
// back out of the saved context.
func promptContext(ctx context.Context, prompter ports.Prompter, defaultName domain.ContextName, defaults domain.Context) (domain.Context, error) {
	return promptContextStep(ctx, prompter, defaultName, defaults, ports.Step{})
}

// promptContextStep is promptContext's own implementation, taking the
// caller's own Step template (its Index/Total/Back, Fields left empty) as
// step — the zero ports.Step (no indicator, no Back) for promptContext's
// own callers (RunContextWizard, RunEditContextWizard: each is still a
// single, standalone Step exactly as before), or {Index: 1, Total: 2} when
// app.RunCreateContext drives this same field-collection logic as its own
// wizard's first step, immediately followed by RunProjectWizard's selection
// Step as its second (this change's own back-navigation fix: the two used
// to each independently believe they were "step 1 of 1", so neither a Back
// button nor a step indicator ever rendered for either, and there was no
// way back from project selection to the context fields just filled in).
// Never a forked copy of the field-collection logic itself.
func promptContextStep(ctx context.Context, prompter ports.Prompter, defaultName domain.ContextName, defaults domain.Context, step ports.Step) (domain.Context, error) {
	nameField := ports.TextFieldSpec(ports.TextField{
		Field:    ports.Field{Label: messages.WizardContextName},
		Default:  string(defaultName),
		Validate: validateContextName,
	})
	step.Fields = append([]ports.FieldSpec{nameField}, contextOptionFields(defaults)...)

	answers, err := prompter.Group(ctx, step)
	if err != nil {
		return domain.Context{}, err
	}
	name, err := domain.NewContextName(answers.GetText(messages.WizardContextName))
	if err != nil {
		return domain.Context{}, err
	}
	return contextFromAnswers(answers, name, defaults)
}

// promptContextFieldsOnly collects every context-level option field
// (contextOptionFields) as a single ports.Step, without a name field —
// used by importIntoNewContext, whose own name resolution
// (resolveNewContextName) is not a simple field validator: it must check
// the target name against the store's own existing contexts and re-prompt
// on a collision, so it stays its own loop rather than folding into this
// Step. name is already resolved by the time this is called.
func promptContextFieldsOnly(ctx context.Context, prompter ports.Prompter, name domain.ContextName, defaults domain.Context) (domain.Context, error) {
	answers, err := prompter.Group(ctx, ports.Step{Fields: contextOptionFields(defaults)})
	if err != nil {
		return domain.Context{}, err
	}
	return contextFromAnswers(answers, name, defaults)
}

// contextOptionFields builds the six context-level option fields shared by
// promptContext and promptContextFieldsOnly (workspaces_root, projects_root,
// ignore_patterns, base_branch, copy_env_default, fetch_before_create), in
// that exact order — unchanged from before this Step existed. See
// promptContext's own doc comment for why ignore_patterns is collected
// right after projects_root.
func contextOptionFields(defaults domain.Context) []ports.FieldSpec {
	baseBranchDefault := domain.BuiltinOptions.BaseBranch
	if defaults.Defaults.BaseBranch != nil {
		baseBranchDefault = *defaults.Defaults.BaseBranch
	}
	copyEnvDefault := domain.BuiltinOptions.CopyEnv
	if defaults.Defaults.CopyEnv != nil {
		copyEnvDefault = *defaults.Defaults.CopyEnv
	}
	fetchBeforeCreateDefault := domain.BuiltinOptions.FetchBeforeCreate
	if defaults.Defaults.FetchBeforeCreate != nil {
		fetchBeforeCreateDefault = *defaults.Defaults.FetchBeforeCreate
	}

	return []ports.FieldSpec{
		ports.TextFieldSpec(ports.TextField{
			Field:    ports.Field{Label: messages.WizardWorkspacesRoot, Help: messages.WizardWorkspacesRootHelp},
			Default:  string(defaults.WorkspacesRoot),
			Validate: validateAbsolutePath,
			Kind:     ports.TextKindDirectory,
		}),
		ports.TextFieldSpec(ports.TextField{
			Field:   ports.Field{Label: messages.WizardProjectsRoot, Help: messages.WizardProjectsRootHelp},
			Default: string(defaults.ProjectsRoot),
			Kind:    ports.TextKindDirectory,
		}),
		ports.TextFieldSpec(ports.TextField{
			Field:    ports.Field{Label: messages.WizardIgnorePatterns, Help: messages.WizardIgnorePatternsHelp},
			Default:  joinIgnorePatterns(defaults.IgnorePatterns),
			Validate: validateIgnorePatterns,
		}),
		ports.TextFieldSpec(ports.TextField{
			Field:    ports.Field{Label: messages.WizardBaseBranch},
			Default:  string(baseBranchDefault),
			Validate: validateBranchName,
		}),
		ports.ConfirmFieldSpec(ports.ConfirmField{
			Field:   ports.Field{Label: messages.WizardCopyEnvDefault, Help: messages.WizardCopyEnvDefaultHelp},
			Default: copyEnvDefault,
		}),
		ports.ConfirmFieldSpec(ports.ConfirmField{
			Field:   ports.Field{Label: messages.WizardFetchBeforeCreate, Help: messages.WizardFetchBeforeCreateHelp},
			Default: fetchBeforeCreateDefault,
		}),
	}
}

// contextFromAnswers builds the resulting domain.Context from
// contextOptionFields's own answers, name (resolved by whichever caller —
// promptContext's own Group call, or promptContextFieldsOnly's caller) and
// defaults.Projects, carried through untouched: neither Step ever collects
// a project list (that comes from RunProjectWizard), so this must never
// silently drop projects an earlier RunProjectWizard run already
// registered.
func contextFromAnswers(answers ports.StepAnswers, name domain.ContextName, defaults domain.Context) (domain.Context, error) {
	baseBranch, err := domain.NewBranchName(answers.GetText(messages.WizardBaseBranch))
	if err != nil {
		return domain.Context{}, err
	}
	copyEnv := answers.GetConfirm(messages.WizardCopyEnvDefault)
	fetchBeforeCreate := answers.GetConfirm(messages.WizardFetchBeforeCreate)
	ignorePatterns := parseIgnorePatterns(answers.GetText(messages.WizardIgnorePatterns))

	return domain.Context{
		Name:           name,
		WorkspacesRoot: domain.Path(answers.GetText(messages.WizardWorkspacesRoot)),
		ProjectsRoot:   domain.Path(answers.GetText(messages.WizardProjectsRoot)),
		Defaults: domain.Options{
			BaseBranch:        &baseBranch,
			CopyEnv:           &copyEnv,
			FetchBeforeCreate: &fetchBeforeCreate,
		},
		IgnorePatterns: ignorePatterns,
		Projects:       defaults.Projects,
	}, nil
}

// promptContextName collects and validates a context name on its own,
// prefilled with defaultName — used only by resolveNewContextName's own
// retry loop (import_context.go), which needs a single name prompt
// re-offered after a collision, never as part of a larger Step.
func promptContextName(ctx context.Context, prompter ports.Prompter, defaultName domain.ContextName) (domain.ContextName, error) {
	nameStr, err := prompter.Text(ctx, ports.TextField{
		Field:    ports.Field{Label: messages.WizardContextName},
		Default:  string(defaultName),
		Validate: validateContextName,
	})
	if err != nil {
		return "", err
	}
	return domain.NewContextName(nameStr)
}

// parseIgnorePatterns splits a raw ignore_patterns wizard answer into
// individual globs. Comma is the separator (not whitespace): a directory
// name may legitimately contain a space, but essentially never a comma,
// so splitting on comma is the choice that never mis-splits a real
// pattern. Each entry is trimmed of surrounding whitespace, and an empty
// entry (from a leading/trailing/doubled comma, or the whole answer being
// blank) is dropped rather than turned into a spurious "match everything"
// empty-string pattern. An all-blank answer returns nil, never an empty
// non-nil slice, so "no patterns configured" round-trips identically
// through YAML (context-management spec: "keep it separate from
// env_prune_dirs" — this mirrors env_prune_dirs's own nil-means-unset
// convention).
func parseIgnorePatterns(s string) []domain.Glob {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []domain.Glob
	for _, raw := range strings.Split(s, ",") {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		out = append(out, domain.Glob(p))
	}
	return out
}

// joinIgnorePatterns is parseIgnorePatterns's inverse, used only to
// prefill the ignore_patterns field's Default with a context's
// already-persisted patterns on "context edit" — the same
// prefill-on-edit behavior every other field in this function already
// has, so accepting the default (an empty ENTER) reproduces the existing
// patterns unchanged rather than silently clearing them.
func joinIgnorePatterns(patterns []domain.Glob) string {
	parts := make([]string, len(patterns))
	for i, p := range patterns {
		parts[i] = string(p)
	}
	return strings.Join(parts, ", ")
}
