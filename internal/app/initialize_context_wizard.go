// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// RunInitializeContextWizard is the tray's "Initialize context…" menu
// item's shared use case (tray-gui gap-closure #2, the product owner's own
// words: "I want... 'initialize context', which lets me decide the
// projects folder... then it filters all the repositories inside that
// folder and lets me choose which of them I want to map... then it lets
// me decide the folder where the workspaces of that context are
// mounted").
//
// The two steps run in exactly that order — roots, then scan/select
// (delegated to RunProjectWizard, never reimplemented here) — which is why
// this is its own use case rather than a reordering of promptContextFields:
// that function is shared by RunContextWizard/RunEditContextWizard, which
// collect a much larger field set (base branch, copy-env default, etc.)
// that "Initialize context…" deliberately leaves untouched. Renaming the
// context is likewise out of scope here — see RunEditContextWizard's own
// doc comment for why that lives in "Edit context…" instead.
//
// ProjectsRoot and WorkspacesRoot are saved together, as soon as the first
// Step is answered (rather than held in memory until the very end), so a
// context left partially initialized — the user closes the window
// mid-flow — still keeps whatever it was told so far, instead of losing it
// all to an in-memory value that was never persisted.
//
// This wizard has exactly two genuine Steps (design's own layout rule:
// "one page when the fields fit... use a step only when it genuinely
// depends on an earlier answer"): the roots Step (projects_root and
// workspaces_root together — Step 1 of 2, no Back, since there is nothing
// before it to return to) and the scanned-candidate selection Step
// RunProjectWizard's own pickCandidates renders (Step 2 of 2, Back true —
// the concrete example: "Project selection after the scan is a genuine
// second step", since it cannot exist before the folder it scans is
// known). workspaces_root depends on neither root nor the scan, so it
// belongs on the same page as projects_root rather than asked as its own
// plain single-field prompt afterward — this change's own fix for a real
// defect (a screenshot of a window containing only "Workspaces root"):
// wrapping one lone, independent field in Step chrome of its own would
// still be "a step for its own sake", but leaving it to render as its own
// single-field window was the actual, reported bug — grouping it with
// projects_root (equally independent, already on this Step) fixes it
// without inventing a third Step.
//
// A Back on the selection Step (ports.ErrStepBack) returns to the roots
// Step and offers it again, prefilled with whatever was just entered,
// rather than aborting the wizard or silently skipping ahead as if nothing
// had happened.
func RunInitializeContextWizard(ctx context.Context, deps ProjectWizardDeps, name domain.ContextName) (domain.Context, error) {
	c, err := deps.Store.LoadContext(ctx, name)
	if err != nil {
		return domain.Context{}, err
	}

	projectsRootDefault := string(c.ProjectsRoot)
	workspacesRootDefault := string(c.WorkspacesRoot)
	for {
		answers, err := deps.Prompter.Group(ctx, ports.Step{
			Fields: []ports.FieldSpec{
				ports.TextFieldSpec(ports.TextField{
					Field:   ports.Field{Label: messages.WizardProjectsRoot, Help: messages.WizardProjectsRootHelp},
					Default: projectsRootDefault,
					Kind:    ports.TextKindDirectory,
				}),
				ports.TextFieldSpec(ports.TextField{
					Field:    ports.Field{Label: messages.WizardWorkspacesRoot, Help: messages.WizardWorkspacesRootHelp},
					Default:  workspacesRootDefault,
					Validate: validateAbsolutePath,
					Kind:     ports.TextKindDirectory,
				}),
			},
			Index: 1,
			Total: 2,
		})
		if err != nil {
			return domain.Context{}, err
		}
		projectsRootDefault = answers.GetText(messages.WizardProjectsRoot)
		workspacesRootDefault = answers.GetText(messages.WizardWorkspacesRoot)
		c.ProjectsRoot = domain.Path(projectsRootDefault)
		c.WorkspacesRoot = domain.Path(workspacesRootDefault)
		if err := deps.Store.SaveContext(ctx, c); err != nil {
			return domain.Context{}, err
		}

		// Scan c.ProjectsRoot, offer the git repositories found, and let
		// the user choose which to map — runProjectWizard already
		// implements exactly this (scanCandidates -> pickCandidates ->
		// per-project prompts, with the manual-registration fallback when
		// nothing is selected), so it is reused verbatim rather than
		// forked, driven as this wizard's own Step 2 of 2 with Back
		// enabled.
		// Assigned only once the step has genuinely succeeded. Taking the
		// return value before checking the error would overwrite the
		// context being carried through the loop with the zero value the
		// Back path returns, and the next iteration writes that back to
		// the store — losing the context's name, defaults, ignore
		// patterns and already-registered projects. Going back is a
		// navigation, never an edit.
		next, err := runProjectWizard(ctx, deps, name, ports.Step{Index: 2, Total: 2, Back: true})
		if err != nil {
			if errors.Is(err, ports.ErrStepBack) {
				continue
			}
			return domain.Context{}, err
		}
		c = next
		break
	}

	return c, nil
}
