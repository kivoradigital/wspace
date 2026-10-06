// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// CreateWorkspaceWizardDeps bundles the driven ports RunCreateWorkspace
// needs: the same shape CreateContextDeps/ProjectWizardDeps already carry
// (Store, FS, Git, Prompter, Reporter, in that order, on purpose — see
// CreateContextDeps's own doc comment for why this is a direct
// conversion, not a field-by-field literal, wherever this file forwards
// into Deps/CreateWorkspaceInput below).
type CreateWorkspaceWizardDeps struct {
	Store    ports.ConfigStore
	FS       ports.FileSystemPort
	Git      ports.GitPort
	Prompter ports.Prompter
	Reporter ports.Reporter
}

// RunCreateWorkspace is "New workspace…"'s own shared use case, the
// workspace-creation counterpart to RunCreateContext: it asks for a
// workspace name and an optional branch override, offers the active
// context's own already-registered projects for selection — never a
// filesystem scan, which is RunProjectWizard's own, entirely different
// job — and then actually calls CreateWorkspace with exactly the
// selected project keys.
//
// This exists because internal/gui/wizard's ProjectWindow.Open used to
// call RunProjectWizard instead of CreateWorkspace: "New workspace…"
// silently re-ran project registration into the active context (offering
// freshly scanned filesystem candidates, not the context's own
// registered projects) and never created a workspace at all — the
// product owner's three separate reports ("it shows me all the
// projects, not the ones I chose", "there is no loading, no progress",
// "afterwards I have no way to see the workspace I just created") were
// all the same one defect. Putting the sequence here, exactly as
// RunCreateContext already does for context creation, is what keeps the
// CLI and the tray from drifting apart on this again — internal/cli's
// own "create" command takes its name/branch/projects straight from
// flags/args (no prompting to share), so it calls CreateWorkspace
// directly rather than through this wizard-shaped entry point, but both
// paths still funnel through the exact same CreateWorkspace underneath.
func RunCreateWorkspace(ctx context.Context, deps CreateWorkspaceWizardDeps, contextName domain.ContextName) (CreateWorkspaceResult, error) {
	c, err := deps.Store.LoadContext(ctx, contextName)
	if err != nil {
		return CreateWorkspaceResult{}, err
	}

	name, branch, keys, err := promptCreateWorkspaceFields(ctx, deps.Prompter, c)
	if err != nil {
		return CreateWorkspaceResult{}, err
	}

	return CreateWorkspace(ctx, Deps{Store: deps.Store, Git: deps.Git, FS: deps.FS, Reporter: deps.Reporter}, CreateWorkspaceInput{
		Context:     c,
		Name:        name,
		Branch:      branch,
		ProjectKeys: keys,
	})
}

// promptCreateWorkspaceFields collects the workspace name, then the
// optional branch override — shown against a live preview of what an
// empty answer resolves to, exactly the same live-preview pattern
// promptProjectDetails already uses for origin_branch/dest_branch/
// worktree_dir — and finally the project selection (pickWorkspaceProjects
// below). The name must be known before the branch preview can be
// computed (DestBranch's own built-in fallback folds the workspace name
// in), the same genuine sequential dependency promptProjectDetails's own
// doc comment already justifies for its three fields.
func promptCreateWorkspaceFields(ctx context.Context, prompter ports.Prompter, c domain.Context) (name, branch string, keys []domain.ProjectKey, err error) {
	name, err = prompter.Text(ctx, ports.TextField{
		Field:    ports.Field{Label: messages.WizardWorkspaceName},
		Validate: validateWorkspaceName,
	})
	if err != nil {
		return "", "", nil, err
	}

	r := domain.Resolver{Context: &c}
	preview := name
	if resolved, derr := r.DestBranch("", domain.Vars{Workspace: name}); derr == nil {
		preview = string(resolved.Value)
	}
	branch, err = prompter.Text(ctx, ports.TextField{
		Field:    ports.Field{Label: messages.WizardWorkspaceBranch, Help: messages.WizardWorkspaceBranchHelp, Args: []any{preview}},
		Validate: validateOptionalBranchName,
	})
	if err != nil {
		return "", "", nil, err
	}

	keys, err = pickWorkspaceProjects(ctx, prompter, c)
	if err != nil {
		return "", "", nil, err
	}

	return name, branch, keys, nil
}

// pickWorkspaceProjects offers c's own already-registered projects for
// selection — every option pre-selected by default (Option.Selected
// true), reusing the exact same select-all/deselect-all affordance every
// other multi-choice field already carries, rather than a second
// "default" mechanism invented for this one field. A context with no
// projects registered yet offers nothing to choose and returns
// immediately, exactly like pickCandidates does with zero scanned
// candidates — there is nothing a prompt could usefully ask here.
//
// The returned slice is always explicit: even when every project ends up
// selected, this returns every one of their keys by name, never nil —
// CreateWorkspace's own empty-ProjectKeys-means-every-project fallback
// must never be relied on here by omission, since a context that later
// grows a new project must not silently pull that project into a
// workspace whose selection step already ran and already meant "these
// two, specifically".
func pickWorkspaceProjects(ctx context.Context, prompter ports.Prompter, c domain.Context) ([]domain.ProjectKey, error) {
	if len(c.Projects) == 0 {
		return nil, nil
	}

	opts := make([]ports.Option, len(c.Projects))
	for i, p := range c.Projects {
		opts[i] = ports.Option{Raw: string(p.Key), Selected: true}
	}
	step := ports.Step{Fields: []ports.FieldSpec{ports.MultiChoiceFieldSpec(ports.ChoiceField{
		Field:   ports.Field{Label: messages.WizardWorkspaceProjects},
		Options: opts,
	})}}
	answers, err := prompter.Group(ctx, step)
	if err != nil {
		return nil, err
	}
	picks := answers.GetMultiChoice(messages.WizardWorkspaceProjects)

	keys := make([]domain.ProjectKey, 0, len(picks))
	for _, i := range picks {
		keys = append(keys, c.Projects[i].Key)
	}
	return keys, nil
}
