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

// The repository inspector's write actions: repos.stage, repos.unstage,
// repos.discard, repos.commitChanges (repos.commit reads one commit),
// repos.push and repos.stashCreate.

// RepoChangePathsParams list changed paths of one repo exactly as
// repos.changes lists them, or All for every path the action applies to.
// Confirm is only read by repos.discard.
type RepoChangePathsParams struct {
	Context   string   `json:"context,omitempty"`
	Workspace string   `json:"workspace"`
	Repo      string   `json:"repo"`
	Paths     []string `json:"paths,omitempty"`
	All       bool     `json:"all,omitempty"`
	Confirm   bool     `json:"confirm,omitempty"`
}

// RepoStageResult is repos.stage's and repos.unstage's result.
type RepoStageResult struct {
	Repo  string   `json:"repo"`
	Paths []string `json:"paths"`
}

// DiscardFileDTO is one file repos.discard would restore.
type DiscardFileDTO struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary"`
}

// RepoDiscardResult is repos.discard's result. Backup is the patch
// holding the discarded changes (absent when git produced none).
type RepoDiscardResult struct {
	Repo      string   `json:"repo"`
	Discarded []string `json:"discarded"`
	Backup    string   `json:"backup,omitempty"`
}

// RepoCommitChangesParams are repos.commitChanges' parameters. Amend is
// reserved: true is refused (invalid_params).
type RepoCommitChangesParams struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	Message   string `json:"message"`
	Amend     bool   `json:"amend,omitempty"`
}

// RepoCommitChangesResult is repos.commitChanges' result. Refusals are in
// Error (data.commands for identity_missing, data.output for hook_failed,
// data.files for unresolved conflicts), not a failed call.
type RepoCommitChangesResult struct {
	Repo     string     `json:"repo"`
	Commit   *Commit    `json:"commit,omitempty"`
	Warnings []string   `json:"warnings"`
	Error    *ErrorBody `json:"error,omitempty"`
}

// RepoPushParams are repos.push's parameters.
type RepoPushParams struct {
	Context     string `json:"context,omitempty"`
	Workspace   string `json:"workspace"`
	Repo        string `json:"repo"`
	SetUpstream bool   `json:"setUpstream,omitempty"`
}

// RepoPushResult is repos.push's result. Refusals are in Error
// (data.remote for no_upstream, data.output for push_rejected,
// auth_failed and git_failed), not a failed call.
type RepoPushResult struct {
	Repo        string     `json:"repo"`
	Branch      string     `json:"branch,omitempty"`
	Remote      string     `json:"remote"`
	Upstream    string     `json:"upstream,omitempty"`
	SetUpstream bool       `json:"setUpstream"`
	UpToDate    bool       `json:"upToDate"`
	Pushed      int        `json:"pushed"`
	Error       *ErrorBody `json:"error,omitempty"`
}

// RepoStashCreateParams are repos.stashCreate's parameters.
type RepoStashCreateParams struct {
	Context          string `json:"context,omitempty"`
	Workspace        string `json:"workspace"`
	Repo             string `json:"repo"`
	Message          string `json:"message,omitempty"`
	IncludeUntracked bool   `json:"includeUntracked,omitempty"`
	KeepIndex        bool   `json:"keepIndex,omitempty"`
}

// RepoStashCreateResult is repos.stashCreate's result: the new entry.
type RepoStashCreateResult struct {
	Repo  string `json:"repo"`
	Stash Stash  `json:"stash"`
}

func (e *Engine) changePathsInput(ctx context.Context, p RepoChangePathsParams, allowAll bool) (app.ChangePathsInput, error) {
	if !p.All && len(p.Paths) == 0 {
		return app.ChangePathsInput{}, invalidParam("paths", errors.New("at least one path is required (or all: true)"))
	}
	if p.All && !allowAll {
		return app.ChangePathsInput{}, invalidParam("all", errors.New("not supported by this method; list the paths"))
	}
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return app.ChangePathsInput{}, err
	}
	return app.ChangePathsInput{RepoRefInput: in, Paths: p.Paths, All: p.All}, nil
}

func (e *Engine) stageOrUnstage(ctx context.Context, p RepoChangePathsParams,
	run func(context.Context, app.Deps, app.ChangePathsInput) (app.StageResult, error)) (RepoStageResult, error) {
	in, err := e.changePathsInput(ctx, p, true)
	if err != nil {
		return RepoStageResult{}, err
	}
	r, err := run(ctx, e.appDeps(nil), in)
	if err != nil {
		return RepoStageResult{}, wrap(err)
	}
	return RepoStageResult{Repo: r.Alias, Paths: append([]string{}, r.Paths...)}, nil
}

// RepoStage stages paths repos.changes lists as unstaged or untracked
// (all: every one). Any other path refuses the call (path_not_changed).
func (e *Engine) RepoStage(ctx context.Context, p RepoChangePathsParams) (RepoStageResult, error) {
	return e.stageOrUnstage(ctx, p, app.StageChanges)
}

// RepoUnstage moves staged paths (all: every one) out of the index; the
// worktree is unchanged.
func (e *Engine) RepoUnstage(ctx context.Context, p RepoChangePathsParams) (RepoStageResult, error) {
	return e.stageOrUnstage(ctx, p, app.UnstageChanges)
}

// RepoDiscard discards the unstaged changes of tracked files after
// writing them to a backup patch. Without Confirm nothing changes and the
// call fails with needs_confirmation and data.files (path, status, line
// counts). Untracked files (path_is_untracked) and staged-only paths
// (staged_only) are refused.
func (e *Engine) RepoDiscard(ctx context.Context, p RepoChangePathsParams) (RepoDiscardResult, error) {
	in, err := e.changePathsInput(ctx, p, false)
	if err != nil {
		return RepoDiscardResult{}, err
	}
	deps := e.appDeps(nil)
	if !p.Confirm {
		prev, err := app.PreviewDiscard(ctx, deps, in)
		if err != nil {
			return RepoDiscardResult{}, wrap(err)
		}
		files := make([]DiscardFileDTO, 0, len(prev.Files))
		for _, f := range prev.Files {
			files = append(files, DiscardFileDTO{Path: f.Path, Status: string(f.Status), Additions: f.Additions, Deletions: f.Deletions, Binary: f.Binary})
		}
		ce := newError(CodeNeedsConfirmation, messages.EngineDiscardNeedsConfirmation, len(files))
		ce.Data = map[string]any{"reasons": []Blocker{}, "files": files}
		return RepoDiscardResult{}, ce
	}
	r, err := app.DiscardChanges(ctx, deps, in)
	if err != nil {
		return RepoDiscardResult{}, wrap(err)
	}
	return RepoDiscardResult{Repo: r.Alias, Discarded: append([]string{}, r.Discarded...), Backup: string(r.Backup)}, nil
}

// errorWithData is err's wire body with extra data merged in.
func errorWithData(err error, extra map[string]any) *ErrorBody {
	body := AsError(err).Body()
	data := map[string]any{}
	for k, v := range body.Data {
		data[k] = v
	}
	for k, v := range extra {
		data[k] = v
	}
	body.Data = data
	return &body
}

// RepoCommitChanges commits the staged changes with the repo's own
// identity and hooks; refusals are in the result.
func (e *Engine) RepoCommitChanges(ctx context.Context, p RepoCommitChangesParams) (RepoCommitChangesResult, error) {
	if p.Amend {
		return RepoCommitChangesResult{}, invalidParam("amend", errors.New("amending is not supported"))
	}
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return RepoCommitChangesResult{}, err
	}
	r, err := app.CommitChanges(ctx, e.appDeps(nil), app.CommitInput{RepoRefInput: in, Message: p.Message})
	if errors.Is(err, app.ErrEmptyCommitMessage) {
		return RepoCommitChangesResult{}, invalidParam("message", err)
	}
	if err != nil {
		return RepoCommitChangesResult{}, wrap(err)
	}
	out := RepoCommitChangesResult{Repo: r.Alias, Warnings: append([]string{}, r.Warnings...)}
	if r.Err != nil {
		extra := map[string]any{}
		if len(r.IdentityCommands) > 0 {
			extra["commands"] = append([]string{}, r.IdentityCommands...)
		}
		if r.Output != "" {
			extra["output"] = r.Output
		}
		if len(r.Files) > 0 {
			extra["files"] = append([]string{}, r.Files...)
		}
		out.Error = errorWithData(r.Err, extra)
		return out, nil
	}
	c := commitToDTO(r.Commit)
	out.Commit = &c
	return out, nil
}

// RepoPush pushes the current branch to its upstream (setUpstream: or
// publishes it to the context's remote); never forced. Refusals are in
// the result.
func (e *Engine) RepoPush(ctx context.Context, p RepoPushParams, emit ProgressFunc) (RepoPushResult, error) {
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return RepoPushResult{}, err
	}
	r, err := app.PushRepo(ctx, e.appDeps(emit), app.PushInput{RepoRefInput: in, SetUpstream: p.SetUpstream})
	if err != nil {
		return RepoPushResult{}, wrap(err)
	}
	out := RepoPushResult{Repo: r.Alias, Branch: r.Branch, Remote: r.Remote, Upstream: r.Upstream, SetUpstream: r.SetUpstream, UpToDate: r.UpToDate, Pushed: r.Pushed}
	if r.Err != nil {
		extra := map[string]any{}
		if domain.Code(r.Err) == domain.CodeNoUpstream {
			extra["remote"] = r.Remote
		}
		if r.Output != "" {
			extra["output"] = r.Output
		}
		out.Error = errorWithData(r.Err, extra)
	}
	return out, nil
}

// RepoStashCreate saves the local changes as a new stash entry
// (nothing_to_stash when there are none).
func (e *Engine) RepoStashCreate(ctx context.Context, p RepoStashCreateParams) (RepoStashCreateResult, error) {
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return RepoStashCreateResult{}, err
	}
	s, err := app.CreateStash(ctx, e.appDeps(nil), app.StashCreateInput{RepoRefInput: in, Message: p.Message, IncludeUntracked: p.IncludeUntracked, KeepIndex: p.KeepIndex})
	if err != nil {
		return RepoStashCreateResult{}, wrap(err)
	}
	return RepoStashCreateResult{Repo: p.Repo, Stash: stashToDTO(s)}, nil
}
