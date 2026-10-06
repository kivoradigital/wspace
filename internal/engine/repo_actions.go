// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// The repository inspector's local actions (repos.stashApply/stashPop/
// stashDrop, repos.validateUntracked/discardUntracked) and the add-project
// candidates query (workspaces.addableProjects).

// RepoStashActionParams name the stash entry an action targets: index and
// the hash the caller saw for it (full or abbreviated; required). Confirm
// is only read by repos.stashDrop.
type RepoStashActionParams struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	Index     int    `json:"index"`
	Hash      string `json:"hash"`
	Confirm   bool   `json:"confirm,omitempty"`
}

// RepoStashApplyResult is repos.stashApply's and repos.stashPop's result.
// Error is absent on success; a refusal carries data.files
// (worktree_dirty) and a conflict data.conflicts (stash_conflict).
type RepoStashApplyResult struct {
	Repo          string     `json:"repo"`
	Stash         *Stash     `json:"stash,omitempty"`
	IndexRestored bool       `json:"indexRestored"`
	Dropped       bool       `json:"dropped"`
	Conflicts     []string   `json:"conflicts"`
	Warnings      []string   `json:"warnings"`
	Error         *ErrorBody `json:"error,omitempty"`
}

// RepoStashDropResult is repos.stashDrop's result: the entry dropped.
type RepoStashDropResult struct {
	Repo  string `json:"repo"`
	Stash Stash  `json:"stash"`
}

// RepoPathsParams list untracked files of one repo, exactly as
// repos.changes lists them. Confirm is only read by
// repos.discardUntracked.
type RepoPathsParams struct {
	Context   string   `json:"context,omitempty"`
	Workspace string   `json:"workspace"`
	Repo      string   `json:"repo"`
	Paths     []string `json:"paths"`
	Confirm   bool     `json:"confirm,omitempty"`
}

// UntrackedPathDTO is one validated untracked file.
type UntrackedPathDTO struct {
	Path         string `json:"path"`
	AbsolutePath string `json:"absolutePath"`
}

// UntrackedTargetsResult is repos.validateUntracked's result.
type UntrackedTargetsResult struct {
	Repo     string             `json:"repo"`
	Worktree string             `json:"worktree"`
	Paths    []UntrackedPathDTO `json:"paths"`
}

// DiscardUntrackedResult is repos.discardUntracked's result.
type DiscardUntrackedResult struct {
	Repo    string   `json:"repo"`
	Removed []string `json:"removed"`
	Kept    []string `json:"kept"`
}

func (e *Engine) stashActionInput(ctx context.Context, p RepoStashActionParams) (app.RepoStashActionInput, error) {
	if p.Index < 0 {
		return app.RepoStashActionInput{}, invalidParam("index", errors.New("must not be negative"))
	}
	if !domain.ValidCommitHash(p.Hash) {
		return app.RepoStashActionInput{}, invalidParam("hash", errors.New("required: the entry's hash as repos.stashes lists it (4 to 64 hexadecimal digits)"))
	}
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return app.RepoStashActionInput{}, err
	}
	return app.RepoStashActionInput{RepoRefInput: in, Index: p.Index, Hash: p.Hash}, nil
}

// RepoStashApply applies one stash entry, keeping it; refusals and
// conflicts are in the result, not an error.
func (e *Engine) RepoStashApply(ctx context.Context, p RepoStashActionParams) (RepoStashApplyResult, error) {
	return e.repoStashApply(ctx, p, app.ApplyStash)
}

// RepoStashPop applies one stash entry and drops it only after a complete
// apply (no conflicts, index restored).
func (e *Engine) RepoStashPop(ctx context.Context, p RepoStashActionParams) (RepoStashApplyResult, error) {
	return e.repoStashApply(ctx, p, app.PopStash)
}

func (e *Engine) repoStashApply(ctx context.Context, p RepoStashActionParams,
	run func(context.Context, app.Deps, app.RepoStashActionInput) (app.StashApplyReport, error)) (RepoStashApplyResult, error) {
	in, err := e.stashActionInput(ctx, p)
	if err != nil {
		return RepoStashApplyResult{}, err
	}
	r, err := run(ctx, e.appDeps(nil), in)
	if err != nil {
		return RepoStashApplyResult{}, wrap(err)
	}
	out := RepoStashApplyResult{Repo: r.Alias, IndexRestored: r.IndexRestored, Dropped: r.Dropped,
		Conflicts: append([]string{}, r.Conflicts...), Warnings: append([]string{}, r.Warnings...)}
	if r.Entry.Hash != "" {
		s := stashToDTO(r.Entry)
		out.Stash = &s
	}
	if r.Err == nil {
		return out, nil
	}
	body := AsError(r.Err).Body()
	data := map[string]any{}
	for k, v := range body.Data {
		data[k] = v
	}
	if len(r.Files) > 0 {
		data["files"] = append([]string{}, r.Files...)
	}
	if len(r.Conflicts) > 0 {
		data["conflicts"] = append([]string{}, r.Conflicts...)
	}
	body.Data = data
	out.Error = &body
	return out, nil
}

// RepoStashDrop drops one stash entry. Without Confirm nothing changes and
// the call fails with needs_confirmation, data.stash and data.files (the
// number of files the entry holds).
func (e *Engine) RepoStashDrop(ctx context.Context, p RepoStashActionParams) (RepoStashDropResult, error) {
	in, err := e.stashActionInput(ctx, p)
	if err != nil {
		return RepoStashDropResult{}, err
	}
	deps := e.appDeps(nil)
	if !p.Confirm {
		d, err := app.VerifiedStash(ctx, deps, in)
		if err != nil {
			return RepoStashDropResult{}, wrap(err)
		}
		ce := newError(CodeNeedsConfirmation, messages.EngineStashDropNeedsConfirmation, d.Entry.Ref, d.Entry.Message)
		ce.Data = map[string]any{"reasons": []Blocker{}, "stash": stashToDTO(d.Entry), "files": len(d.Patch.Files)}
		return RepoStashDropResult{}, ce
	}
	entry, err := app.DropStash(ctx, deps, in)
	if err != nil {
		return RepoStashDropResult{}, wrap(err)
	}
	return RepoStashDropResult{Repo: p.Repo, Stash: stashToDTO(entry)}, nil
}

func (e *Engine) pathsInput(ctx context.Context, p RepoPathsParams) (app.UntrackedPathsInput, error) {
	if len(p.Paths) == 0 {
		return app.UntrackedPathsInput{}, invalidParam("paths", errors.New("at least one path is required"))
	}
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return app.UntrackedPathsInput{}, err
	}
	return app.UntrackedPathsInput{RepoRefInput: in, Paths: p.Paths}, nil
}

// RepoValidateUntracked checks that every path is currently an untracked
// file of the repo (not_found, path_not_untracked otherwise) and resolves
// it to an absolute path inside the worktree. It changes nothing; the app
// uses it before moving the files to the Trash.
func (e *Engine) RepoValidateUntracked(ctx context.Context, p RepoPathsParams) (UntrackedTargetsResult, error) {
	in, err := e.pathsInput(ctx, p)
	if err != nil {
		return UntrackedTargetsResult{}, err
	}
	t, err := app.ValidateUntracked(ctx, e.appDeps(nil), in)
	if err != nil {
		return UntrackedTargetsResult{}, wrap(err)
	}
	return untrackedToDTO(p.Repo, t), nil
}

func untrackedToDTO(repo string, t app.UntrackedTargets) UntrackedTargetsResult {
	out := UntrackedTargetsResult{Repo: repo, Worktree: string(t.Worktree), Paths: make([]UntrackedPathDTO, 0, len(t.Paths))}
	for _, u := range t.Paths {
		out.Paths = append(out.Paths, UntrackedPathDTO{Path: u.Path, AbsolutePath: string(u.Absolute)})
	}
	return out
}

// RepoDiscardUntracked permanently deletes untracked files (validated like
// RepoValidateUntracked; one bad path refuses the whole call). Without
// Confirm nothing changes and the call fails with needs_confirmation and
// data.paths.
func (e *Engine) RepoDiscardUntracked(ctx context.Context, p RepoPathsParams) (DiscardUntrackedResult, error) {
	in, err := e.pathsInput(ctx, p)
	if err != nil {
		return DiscardUntrackedResult{}, err
	}
	deps := e.appDeps(nil)
	if !p.Confirm {
		t, err := app.ValidateUntracked(ctx, deps, in)
		if err != nil {
			return DiscardUntrackedResult{}, wrap(err)
		}
		paths := make([]string, 0, len(t.Paths))
		for _, u := range t.Paths {
			paths = append(paths, u.Path)
		}
		ce := newError(CodeNeedsConfirmation, messages.EngineCleanNeedsConfirmation, len(paths))
		ce.Data = map[string]any{"reasons": []Blocker{}, "paths": paths}
		return DiscardUntrackedResult{}, ce
	}
	r, err := app.DiscardUntracked(ctx, deps, in)
	if err != nil {
		return DiscardUntrackedResult{}, wrap(err)
	}
	return DiscardUntrackedResult{Repo: r.Alias, Removed: append([]string{}, r.Removed...), Kept: append([]string{}, r.Kept...)}, nil
}

// AddableProjects lists the context's projects workspaces.addRepo accepts
// for the workspace: those not already in it (by project key, main clone
// or alias).
func (e *Engine) AddableProjects(ctx context.Context, p WorkspaceRef) ([]Project, error) {
	root, c, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return nil, err
	}
	list, err := app.AddableProjects(ctx, e.appDeps(nil), app.AddableProjectsInput{WorkspaceRoot: root, Context: c})
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]Project, 0, len(list))
	for _, pr := range list {
		out = append(out, e.projectWithNodeInfo(pr))
	}
	return out, nil
}
