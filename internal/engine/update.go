// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
)

// UpdateRepoParams are workspaces.updateRepo's parameters. Strategy is
// "merge" (default) or "rebase"; Autostash stashes uncommitted tracked
// changes around the update instead of refusing it.
type UpdateRepoParams struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	Strategy  string `json:"strategy,omitempty"`
	Autostash bool   `json:"autostash,omitempty"`
}

// UpdateWorkspaceParams are workspaces.update's parameters.
type UpdateWorkspaceParams struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
	Strategy  string `json:"strategy,omitempty"`
	Autostash bool   `json:"autostash,omitempty"`
}

// RepoUpdateResult is one repository's update outcome. Error is absent on
// success. On a refusal or failure it is an error object (§3): a dirty
// worktree carries data.files, a conflict carries data.paths and
// data.restored (also listed in Conflicts).
type RepoUpdateResult struct {
	Repo              string     `json:"repo"`
	Strategy          string     `json:"strategy"`
	Base              string     `json:"base"`
	BeforeHead        string     `json:"beforeHead"`
	AfterHead         string     `json:"afterHead"`
	UpToDate          bool       `json:"upToDate"`
	CommitsIntegrated int        `json:"commitsIntegrated"`
	Conflicts         []string   `json:"conflicts"`
	Error             *ErrorBody `json:"error,omitempty"`
}

// UpdateWorkspaceResult lists every repository's outcome, in manifest
// order.
type UpdateWorkspaceResult struct {
	Repos []RepoUpdateResult `json:"repos"`
}

// UpdateRepo integrates one repository's comparison base into its
// current branch, streaming progress to emit. Per-repository refusals and
// conflicts are reported in the result, not as an error.
func (e *Engine) UpdateRepo(ctx context.Context, p UpdateRepoParams, emit ProgressFunc) (RepoUpdateResult, error) {
	strategy, err := domain.ParseUpdateStrategy(p.Strategy)
	if err != nil {
		return RepoUpdateResult{}, invalidParam("strategy", err)
	}
	root, _, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return RepoUpdateResult{}, err
	}
	res, err := app.UpdateRepo(ctx, e.appDeps(emit), app.UpdateRepoInput{WorkspaceRoot: root, Alias: p.Repo, Strategy: strategy, Autostash: p.Autostash})
	if err != nil {
		return RepoUpdateResult{}, wrap(err)
	}
	return repoUpdateToDTO(res), nil
}

// UpdateWorkspace updates every repository of a workspace sequentially;
// one repository's refusal or failure never stops the others.
func (e *Engine) UpdateWorkspace(ctx context.Context, p UpdateWorkspaceParams, emit ProgressFunc) (UpdateWorkspaceResult, error) {
	strategy, err := domain.ParseUpdateStrategy(p.Strategy)
	if err != nil {
		return UpdateWorkspaceResult{}, invalidParam("strategy", err)
	}
	root, _, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return UpdateWorkspaceResult{}, err
	}
	list, err := app.UpdateWorkspace(ctx, e.appDeps(emit), app.UpdateWorkspaceInput{WorkspaceRoot: root, Strategy: strategy, Autostash: p.Autostash})
	if err != nil {
		return UpdateWorkspaceResult{}, wrap(err)
	}
	out := UpdateWorkspaceResult{Repos: make([]RepoUpdateResult, 0, len(list))}
	for _, r := range list {
		out.Repos = append(out.Repos, repoUpdateToDTO(r))
	}
	return out, nil
}

func repoUpdateToDTO(r app.RepoUpdate) RepoUpdateResult {
	out := RepoUpdateResult{
		Repo:              r.Alias,
		Strategy:          string(r.Strategy),
		Base:              r.Base,
		BeforeHead:        r.BeforeHead,
		AfterHead:         r.AfterHead,
		UpToDate:          r.UpToDate,
		CommitsIntegrated: r.CommitsIntegrated,
		Conflicts:         append([]string{}, r.Conflicts...),
	}
	if r.Err == nil {
		return out
	}
	body := AsError(r.Err).Body()
	data := map[string]any{}
	for k, v := range body.Data {
		data[k] = v
	}
	var opErr *domain.OpError
	if errors.As(r.Err, &opErr) {
		switch opErr.Code {
		case domain.CodeWorktreeDirty:
			data["files"] = append([]string{}, r.DirtyFiles...)
		case domain.CodeUpdateConflict:
			data["paths"] = append([]string{}, r.Conflicts...)
			data["restored"] = r.Restored
		}
	}
	body.Data = data
	out.Error = &body
	return out
}
