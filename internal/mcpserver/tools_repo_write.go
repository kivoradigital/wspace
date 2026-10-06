// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
)

// RepoChangePathsInput are repo_stage's and repo_unstage's arguments.
type RepoChangePathsInput struct {
	Context   string   `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string   `json:"workspace" jsonschema:"Workspace name."`
	Repo      string   `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Paths     []string `json:"paths,omitempty" jsonschema:"Changed paths exactly as repo_changes lists them. Any other path refuses the whole call (not_found / path_not_changed) and changes nothing."`
	All       bool     `json:"all,omitempty" jsonschema:"Act on every path the action applies to instead of paths."`
}

// RepoDiscardInput are repo_discard's arguments.
type RepoDiscardInput struct {
	Context   string   `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string   `json:"workspace" jsonschema:"Workspace name."`
	Repo      string   `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Paths     []string `json:"paths" jsonschema:"Tracked files with unstaged changes, exactly as repo_changes lists them. Untracked files (path_is_untracked) and files whose changes are all staged (staged_only) are refused."`
	Confirm   bool     `json:"confirm" jsonschema:"Must be true to proceed. With false the tool changes nothing and returns needs_confirmation listing the files with their line counts (data.files); show them to the user first."`
}

// RepoCommitChangesInput are repo_commit_changes' arguments.
type RepoCommitChangesInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Repo      string `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Message   string `json:"message" jsonschema:"The commit message the user approved: a subject line (72 characters or fewer is recommended), optionally a blank line and a body."`
}

// RepoPushInput are repo_push's arguments.
type RepoPushInput struct {
	Context     string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace   string `json:"workspace" jsonschema:"Workspace name."`
	Repo        string `json:"repo" jsonschema:"Repo alias inside the workspace."`
	SetUpstream bool   `json:"setUpstream,omitempty" jsonschema:"Publish a branch that has no upstream yet to the context's remote (data.remote of the no_upstream refusal) and set it as the upstream. Only when the user agreed to publish the branch."`
}

// RepoStashCreateInput are repo_stash_create's arguments.
type RepoStashCreateInput struct {
	Context          string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace        string `json:"workspace" jsonschema:"Workspace name."`
	Repo             string `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Message          string `json:"message,omitempty" jsonschema:"The stash entry's message."`
	IncludeUntracked bool   `json:"includeUntracked,omitempty" jsonschema:"Also stash (and remove from the worktree) untracked files."`
	KeepIndex        bool   `json:"keepIndex,omitempty" jsonschema:"Keep the staged changes in the index (they are stashed too)."`
}

func registerRepoWriteTools(s *mcp.Server, eng *engine.Engine) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_stage",
		Description: "Stage changed files of a workspace repo (git add on exactly those paths): modified, deleted and untracked files as repo_changes lists them, or all of them with all=true. " +
			"Staged-only, conflicted or unknown paths refuse the whole call. It never commits.",
		Annotations: mutating("Stage files", false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoChangePathsInput) (*mcp.CallToolResult, engine.RepoStageResult, error) {
		r, err := eng.RepoStage(ctx, engine.RepoChangePathsParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Paths: in.Paths, All: in.All})
		return done(r, err, func(r engine.RepoStageResult) string {
			return fmt.Sprintf("%s: staged %s.", r.Repo, plural(len(r.Paths), "path", "paths"))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_unstage",
		Description: "Unstage staged files of a workspace repo (git restore --staged; in a repo without commits, git rm --cached): the changes stay in the worktree, only the index changes. " +
			"Paths as repo_changes lists them on the staged side, or all=true.",
		Annotations: mutating("Unstage files", false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoChangePathsInput) (*mcp.CallToolResult, engine.RepoStageResult, error) {
		r, err := eng.RepoUnstage(ctx, engine.RepoChangePathsParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Paths: in.Paths, All: in.All})
		return done(r, err, func(r engine.RepoStageResult) string {
			return fmt.Sprintf("%s: unstaged %s.", r.Repo, plural(len(r.Paths), "path", "paths"))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_discard",
		Description: "Discard the unstaged changes of tracked files of a workspace repo (git restore --worktree from the index; staged changes are kept). Only call it when the user explicitly asked to discard those changes. " +
			"Requires confirm=true; with confirm=false nothing changes and the tool returns needs_confirmation with the files and their line counts. " +
			"The discarded changes are first saved as a patch (backup; restore with git apply). Untracked files are refused (path_is_untracked; see repo_delete_untracked), and so are files with only staged changes (staged_only).",
		Annotations: mutating("Discard unstaged changes", true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoDiscardInput) (*mcp.CallToolResult, engine.RepoDiscardResult, error) {
		r, err := eng.RepoDiscard(ctx, engine.RepoChangePathsParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Paths: in.Paths, Confirm: in.Confirm})
		return done(r, err, func(r engine.RepoDiscardResult) string {
			summary := fmt.Sprintf("%s: discarded the unstaged changes of %s.", r.Repo, plural(len(r.Discarded), "file", "files"))
			if r.Backup != "" {
				summary += " Backup patch: " + r.Backup + "."
			}
			return summary
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_commit_changes",
		Description: "Commit the staged changes of a workspace repo with the given message. Only call it when the user asked you to commit, with a message the user approved. " +
			"It uses the repository's configured identity (it never sets one), runs the repository's hooks (never skips them) and never amends. " +
			"Refusals change nothing and are in the result's error: nothing_staged (stage first with repo_stage), identity_missing (data.commands: the git config commands the user must run themselves; never run them for the user), " +
			"hook_failed (data.output: what the hook printed; report it, do not bypass it), detached_head, integration_in_progress, worktree_dirty (unresolved conflicts). warnings may hold subject_too_long (over 72 characters).",
		Annotations: mutating("Commit staged changes", false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoCommitChangesInput) (*mcp.CallToolResult, engine.RepoCommitChangesResult, error) {
		r, err := eng.RepoCommitChanges(ctx, engine.RepoCommitChangesParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Message: in.Message})
		return done(r, err, func(r engine.RepoCommitChangesResult) string {
			if r.Error != nil || r.Commit == nil {
				msg := "unknown error"
				if r.Error != nil {
					msg = r.Error.Message
				}
				return fmt.Sprintf("%s: not committed (%s).", r.Repo, msg)
			}
			return fmt.Sprintf("%s: committed %s %s.", r.Repo, r.Commit.ShortHash, r.Commit.Subject)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_push",
		Description: "Push a workspace repo's current branch to its upstream. Only call it when the user asked you to push. It never force-pushes. " +
			"Refusals are in the result's error: no_upstream (the branch was never published, or it tracks a remote branch of another name such as its base; data.remote; ask the user, then retry with setUpstream=true), " +
			"push_rejected (the remote has commits the branch lacks; suggest repo_pull_ff or update_repo, never a force push), auth_failed (data.output: git's message; the user must fix credentials), detached_head. " +
			"upToDate=true means there was nothing to push.",
		Annotations: &mcp.ToolAnnotations{Title: "Push branch", DestructiveHint: boolPtr(false), IdempotentHint: false, OpenWorldHint: boolPtr(true)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RepoPushInput) (*mcp.CallToolResult, engine.RepoPushResult, error) {
		r, err := eng.RepoPush(ctx, engine.RepoPushParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, SetUpstream: in.SetUpstream}, progressFor(ctx, req))
		return done(r, err, func(r engine.RepoPushResult) string {
			switch {
			case r.Error != nil:
				return fmt.Sprintf("%s: not pushed (%s).", r.Repo, r.Error.Message)
			case r.UpToDate:
				return fmt.Sprintf("%s: nothing to push; %s is up to date.", r.Repo, r.Upstream)
			case r.SetUpstream:
				return fmt.Sprintf("%s: published %s; upstream is %s.", r.Repo, r.Branch, r.Upstream)
			}
			return fmt.Sprintf("%s: pushed %s to %s.", r.Repo, plural(r.Pushed, "commit", "commits"), r.Upstream)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_stash_create",
		Description: "Save a workspace repo's local changes as a new stash entry (git stash push) and return it; the worktree is then clean (staged changes stay with keepIndex=true, untracked files are only stashed with includeUntracked=true). " +
			"Nothing to save is refused with nothing_to_stash; unresolved conflicts or a merge/rebase in progress are refused. Restore with repo_stash_apply or repo_stash_pop.",
		Annotations: mutating("Create stash", false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoStashCreateInput) (*mcp.CallToolResult, engine.RepoStashCreateResult, error) {
		r, err := eng.RepoStashCreate(ctx, engine.RepoStashCreateParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Message: in.Message, IncludeUntracked: in.IncludeUntracked, KeepIndex: in.KeepIndex})
		return done(r, err, func(r engine.RepoStashCreateResult) string {
			return fmt.Sprintf("%s: saved %s (%s).", r.Repo, r.Stash.Ref, r.Stash.Message)
		})
	})
}
