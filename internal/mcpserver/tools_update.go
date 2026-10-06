// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
)

// UpdateRepoInput are update_repo's arguments.
type UpdateRepoInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Repo      string `json:"repo" jsonschema:"Repo alias inside the workspace (see workspace_status)."`
	Strategy  string `json:"strategy,omitempty" jsonschema:"\"merge\" (default) or \"rebase\". Rebase rewrites the branch's local commits: a branch that was already pushed then needs a force push, so only use it when the user asked for it."`
	Autostash bool   `json:"autostash,omitempty" jsonschema:"Stash uncommitted tracked changes before updating and re-apply them after. Without it a repo with uncommitted changes is refused (error data.files lists them)."`
}

// UpdateWorkspaceInput are update_workspace's arguments.
type UpdateWorkspaceInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Strategy  string `json:"strategy,omitempty" jsonschema:"\"merge\" (default) or \"rebase\" (rewrites local commits; pushed branches then need a force push)."`
	Autostash bool   `json:"autostash,omitempty" jsonschema:"Stash uncommitted tracked changes around each repo's update instead of refusing that repo."`
}

const updateSemantics = "Fetches the remote, then merges (default) or rebases the repo's base branch (the same base workspace_status compares against, e.g. origin/develop) into the workspace branch. " +
	"Never leaves a repo mid-merge or mid-rebase: on a conflict the operation is aborted, the repo is restored to its previous HEAD and status (error data.restored confirms it), and the conflicting paths are listed in conflicts. " +
	"Refusals (uncommitted changes without autostash, detached HEAD, a merge or rebase already in progress, missing base) change nothing and are reported in the result's error, not as a tool failure. " +
	"Rebase rewrites the branch's local commits. "

func registerUpdateTools(s *mcp.Server, eng *engine.Engine) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_repo",
		Description: "Update one workspace repo from its base branch. " + updateSemantics + "upToDate=true means there was nothing to integrate.",
		Annotations: mutating("Update repo from base", false, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in UpdateRepoInput) (*mcp.CallToolResult, engine.RepoUpdateResult, error) {
		res, err := eng.UpdateRepo(ctx, engine.UpdateRepoParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Strategy: in.Strategy, Autostash: in.Autostash}, progressFor(ctx, req))
		return done(res, err, func(r engine.RepoUpdateResult) string { return updateSummary(r) })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_workspace",
		Description: "Update every repo of a workspace from its base branch, one repo at a time; one repo's refusal or conflict never stops the others (each repo reports its own outcome). " + updateSemantics + "Pass a progress token for per-repo progress.",
		Annotations: mutating("Update workspace from base", false, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in UpdateWorkspaceInput) (*mcp.CallToolResult, engine.UpdateWorkspaceResult, error) {
		res, err := eng.UpdateWorkspace(ctx, engine.UpdateWorkspaceParams{Context: in.Context, Workspace: in.Workspace, Strategy: in.Strategy, Autostash: in.Autostash}, progressFor(ctx, req))
		return done(res, err, func(r engine.UpdateWorkspaceResult) string {
			parts := make([]string, 0, len(r.Repos))
			for _, repo := range r.Repos {
				parts = append(parts, updateSummary(repo))
			}
			if len(parts) == 0 {
				return "No repos to update in " + in.Workspace + "."
			}
			return strings.Join(parts, " ")
		})
	})
}

func updateSummary(r engine.RepoUpdateResult) string {
	switch {
	case r.Error != nil && len(r.Conflicts) > 0:
		return fmt.Sprintf("%s: %s conflicted on %s and was aborted.", r.Repo, r.Strategy, plural(len(r.Conflicts), "file", "files"))
	case r.Error != nil:
		return fmt.Sprintf("%s: not updated (%s).", r.Repo, r.Error.Message)
	case r.UpToDate:
		return fmt.Sprintf("%s: already up to date with %s.", r.Repo, r.Base)
	default:
		return fmt.Sprintf("%s: integrated %s from %s (%s).", r.Repo, plural(r.CommitsIntegrated, "commit", "commits"), r.Base, r.Strategy)
	}
}
