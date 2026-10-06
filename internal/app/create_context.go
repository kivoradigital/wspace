// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// CreateContextDeps bundles every driven port RunCreateContext needs:
// ContextWizardDeps's own two fields (Store, Prompter) plus the
// FileSystemPort/GitPort/Reporter RunProjectWizard needs for the project
// registration step that immediately follows context creation. Kept as
// its own struct rather than nesting ContextWizardDeps/ProjectWizardDeps
// (which would need a duplicated Store and Prompter reconciled at every
// call site) — every field here is used exactly once, forwarded straight
// into RunContextWizard's and ProjectWizardDeps's own signatures.
type CreateContextDeps struct {
	Store    ports.ConfigStore
	FS       ports.FileSystemPort
	Git      ports.GitPort
	Prompter ports.Prompter
	Reporter ports.Reporter
}

// RunCreateContext drives the context-fields step (promptContextStep, the
// same field collection RunContextWizard uses), then immediately continues
// into RunProjectWizard's own scan-and-select step for the context it just
// created — the two steps a context-creation flow always performs
// together, since a new context is not very useful without at least a
// chance to map its projects.
//
// This composition used to live independently in each front end:
// internal/cli's own "context create" command called RunContextWizard and
// then RunProjectWizard itself, while internal/gui/wizard's
// ContextWindow.Open only ever called RunContextWizard — silently giving
// the tray's "New context…" a shorter flow than the CLI's equivalent
// command (product-owner report: creating a context from the tray never
// reaches project selection). That was the same kind of drift the
// shared-use-case rule exists to prevent, just arriving through
// orchestration (which wizard calls follow which) rather than through
// forked wizard logic. Hoisting the sequence here, so both front ends call
// this one entry point instead of composing the two wizard calls
// themselves, is what keeps them from drifting apart again.
//
// The two steps used to each independently believe they were "step 1 of
// 1" (promptContext's own zero-value Step, RunProjectWizard's own
// standalone zero-value selectionStep), so neither a Back button nor a
// step indicator ever rendered for either, and there was no way back from
// project selection to the context fields just filled in (this change's
// own back-navigation fix). They are now driven as one genuine two-step
// flow: the context-fields step as Step 1 of 2 (Back false — nothing
// before it in this flow to return to) and RunProjectWizard's selection
// step as Step 2 of 2 (Back true). A Back from the selection step
// (ports.ErrStepBack, exactly the mechanism RunInitializeContextWizard
// already uses for its own two-step sequence, never a second mechanism)
// returns to the context-fields step and offers it again, prefilled with
// whatever was just entered (c, carried across loop iterations as both the
// name and the defaults promptContextStep prefills from) — never
// discarding it, never restarting the wizard from blank fields.
//
// RunProjectWizard itself already falls back to "register manually? y/n"
// when no ProjectsRoot scan finds anything, so a user who declines every
// prompt still ends up with a valid, if project-less, context — this
// function never treats that as an error.
func RunCreateContext(ctx context.Context, deps CreateContextDeps) (domain.Context, error) {
	// c holds this loop's own prefill state across a Back: only ever
	// overwritten by a *successful* promptContextStep answer, never by
	// runProjectWizard's return value (which is the zero domain.Context on
	// its own ErrStepBack path) — an accidental overwrite there would wipe
	// out the very answers a Back is supposed to preserve.
	var c domain.Context
	for {
		fields, err := promptContextStep(ctx, deps.Prompter, c.Name, c, ports.Step{Index: 1, Total: 2})
		if err != nil {
			return domain.Context{}, err
		}
		c = fields
		if err := deps.Store.SaveContext(ctx, c); err != nil {
			return domain.Context{}, err
		}

		// CreateContextDeps and ProjectWizardDeps share the exact same
		// field set, in the same order, on purpose (this struct's own doc
		// comment) — a direct conversion rather than a field-by-field
		// literal.
		updated, err := runProjectWizard(ctx, ProjectWizardDeps(deps), c.Name, ports.Step{Index: 2, Total: 2, Back: true})
		if err != nil {
			if errors.Is(err, ports.ErrStepBack) {
				continue
			}
			return domain.Context{}, err
		}
		return updated, nil
	}
}
