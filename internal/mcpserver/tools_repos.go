// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
)

// RepoDiffInput are repo_diff's arguments.
type RepoDiffInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Repo      string `json:"repo" jsonschema:"Repo alias inside the workspace (see workspace_status)."`
	Path      string `json:"path" jsonschema:"A changed path exactly as repo_changes lists it (relative to the repo). Any other path is refused (not_found)."`
	Staged    bool   `json:"staged,omitempty" jsonschema:"Diff the staged (index) side instead of the worktree side. Untracked files are on the worktree side and shown as all-added."`
}

// RepoCommitsInput are repo_commits' arguments.
type RepoCommitsInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Repo      string `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Range     string `json:"range,omitempty" jsonschema:"base (default): commits since the comparison base, <base>..HEAD. upstream: commits the upstream does not have yet, <upstream>..HEAD (what a push sends); refused with no_upstream when the branch has none."`
	Offset    int    `json:"offset,omitempty" jsonschema:"Commits to skip (paging); default 0."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Page size, 1 to 200; default 50."`
}

// RepoCommitInput are repo_commit's arguments.
type RepoCommitInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Repo      string `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Hash      string `json:"hash" jsonschema:"Commit hash (full or abbreviated, hexadecimal only; revision expressions like HEAD~1 are refused)."`
}

// RepoStashInput are repo_stash's arguments.
type RepoStashInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Repo      string `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Index     int    `json:"index" jsonschema:"Stash index as repo_stashes lists it (0 is the newest, stash@{0})."`
}

// RepoStashActionInput are repo_stash_apply's and repo_stash_pop's arguments.
type RepoStashActionInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Repo      string `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Index     int    `json:"index" jsonschema:"Stash index as repo_stashes lists it (0 is the newest, stash@{0})."`
	Hash      string `json:"hash" jsonschema:"The entry's hash exactly as repo_stashes listed it (full or abbreviated). If the list shifted since, the call is refused with stash_changed: list the stashes again."`
}

// RepoStashDropInput are repo_stash_drop's arguments.
type RepoStashDropInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Repo      string `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Index     int    `json:"index" jsonschema:"Stash index as repo_stashes lists it."`
	Hash      string `json:"hash" jsonschema:"The entry's hash exactly as repo_stashes listed it; a shifted list is refused with stash_changed."`
	Confirm   bool   `json:"confirm" jsonschema:"Must be true to proceed. With false the tool changes nothing and returns needs_confirmation with the entry (data.stash) and its file count (data.files); show them to the user first."`
}

// RepoDeleteUntrackedInput are repo_delete_untracked's arguments.
type RepoDeleteUntrackedInput struct {
	Context   string   `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string   `json:"workspace" jsonschema:"Workspace name."`
	Repo      string   `json:"repo" jsonschema:"Repo alias inside the workspace."`
	Paths     []string `json:"paths" jsonschema:"Untracked files exactly as repo_changes lists them (status untracked). Any other path (tracked, ignored, a directory, outside the repo) refuses the whole call with not_found / path_not_untracked."`
	Confirm   bool     `json:"confirm" jsonschema:"Must be true to proceed. With false the tool changes nothing and returns needs_confirmation listing the validated paths (data.paths); show them to the user first."`
}

// RepoStashesOutput wraps repo_stashes' list.
type RepoStashesOutput struct {
	Stashes []engine.Stash `json:"stashes"`
}

const diffCaps = "Diffs are capped at 1 MiB / 5000 lines (truncated=true when cut); binary files are reported as binary=true without content. "

func registerRepoTools(s *mcp.Server, eng *engine.Engine) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_branch_info",
		Description: "Show one workspace repo's branch: current branch or detached HEAD, its upstream with ahead/behind, its comparison base (the base workspace_status and update_repo use) with commits ahead/behind it, and lastFetch (when the repo was last fetched; counts are against that fetch). " +
			"Read-only; call repo_fetch first for fresh counts.",
		Annotations: readOnly("Repo branch info"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoInput) (*mcp.CallToolResult, engine.BranchInfo, error) {
		b, err := eng.RepoBranch(ctx, engine.RepoRef{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo})
		return done(b, err, func(b engine.BranchInfo) string {
			name := b.Branch
			if b.Detached {
				name = "detached HEAD"
			}
			summary := fmt.Sprintf("%s on %s.", b.Repo, name)
			if b.Upstream != nil {
				summary += fmt.Sprintf(" Upstream %s: %d ahead, %d behind.", b.Upstream.Ref, b.Upstream.Ahead, b.Upstream.Behind)
			} else {
				summary += " No upstream."
			}
			if b.BaseFound && b.BaseAhead != nil && b.BaseBehind != nil {
				summary += fmt.Sprintf(" Base %s: %d ahead, %d behind.", b.BaseRef, *b.BaseAhead, *b.BaseBehind)
			}
			return summary
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "repo_diff",
		Description: "Show the diff of one changed file of a workspace repo (staged or unstaged side), as parsed hunks whose lines keep their ' ', '+', '-' prefix. " + diffCaps + "Read-only.",
		Annotations: readOnly("Repo file diff"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoDiffInput) (*mcp.CallToolResult, engine.FileDiff, error) {
		d, err := eng.RepoDiff(ctx, engine.RepoDiffParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Path: in.Path, Staged: in.Staged})
		return done(d, err, func(d engine.FileDiff) string {
			if d.Binary {
				return fmt.Sprintf("%s: binary file (%s).", d.Path, d.Status)
			}
			return fmt.Sprintf("%s (%s): +%d -%d in %s.", d.Path, d.Status, d.Additions, d.Deletions, plural(len(d.Hunks), "hunk", "hunks"))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_commits",
		Description: "List the commits a workspace repo's branch has on top of its comparison base (range <base>..HEAD, newest first; the whole HEAD history when no base exists), or with range \"upstream\" the commits not pushed yet (<upstream>..HEAD), paged by offset/limit. " +
			"Each commit has hash, shortHash, subject, author (name only) and date. Read-only.",
		Annotations: readOnly("Repo commits"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoCommitsInput) (*mcp.CallToolResult, engine.RepoCommitsResult, error) {
		r, err := eng.RepoCommits(ctx, engine.RepoCommitsParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Range: in.Range, Offset: in.Offset, Limit: in.Limit})
		return done(r, err, func(r engine.RepoCommitsResult) string {
			return fmt.Sprintf("%s: %s in %s (showing %d from %d).", in.Repo, plural(r.Total, "commit", "commits"), r.Range, len(r.Commits), r.Offset)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "repo_commit",
		Description: "Show one commit of a workspace repo: message, author name, date, parents, and its changed files with diffs against its first parent. " + diffCaps + "Read-only.",
		Annotations: readOnly("Repo commit"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoCommitInput) (*mcp.CallToolResult, engine.CommitDetail, error) {
		d, err := eng.RepoCommit(ctx, engine.RepoCommitParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Hash: in.Hash})
		return done(d, err, func(d engine.CommitDetail) string {
			return fmt.Sprintf("%s %s by %s: %s changed.", d.ShortHash, d.Subject, d.Author, plural(len(d.Files), "file", "files"))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "repo_stashes",
		Description: "List a workspace repo's stash entries (index, ref, branch, message, date), newest first. Read-only; see repo_stash for one entry's content.",
		Annotations: readOnly("Repo stashes"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoInput) (*mcp.CallToolResult, RepoStashesOutput, error) {
		list, err := eng.RepoStashes(ctx, engine.RepoRef{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo})
		if list == nil {
			list = []engine.Stash{}
		}
		return done(RepoStashesOutput{Stashes: list}, err, func(o RepoStashesOutput) string {
			return fmt.Sprintf("%s: %s.", in.Repo, plural(len(o.Stashes), "stash entry", "stash entries"))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "repo_stash",
		Description: "Show one stash entry's content: its files and diffs, untracked files included when git supports it (includesUntracked). " + diffCaps + "Read-only: nothing is applied or dropped.",
		Annotations: readOnly("Repo stash content"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoStashInput) (*mcp.CallToolResult, engine.StashDetail, error) {
		d, err := eng.RepoStash(ctx, engine.RepoStashParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Index: in.Index})
		return done(d, err, func(d engine.StashDetail) string {
			return fmt.Sprintf("%s %s: %s.", d.Stash.Ref, d.Stash.Message, plural(len(d.Files), "file", "files"))
		})
	})

	stashApplyDoc := "Refusals change nothing and are reported in the result's error, not as a tool failure: stash_changed (the list shifted; list the stashes again), " +
		"worktree_dirty (local changes or existing files the stash would overwrite, or unresolved conflicts; data.files), integration_in_progress. " +
		"stash_conflict means git applied the stash with conflict markers in the files listed in conflicts: the stash entry is KEPT and the worktree is NOT reset; tell the user to resolve the conflicts. " +
		"warnings may contain index_not_restored: the staged changes could not be restored as staged, so everything came back unstaged."

	mcp.AddTool(s, &mcp.Tool{
		Name:        "repo_stash_apply",
		Description: "Apply one stash entry of a workspace repo to its worktree (git stash apply --index), keeping the entry in the stash list. " + stashApplyDoc,
		Annotations: mutating("Apply stash", false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoStashActionInput) (*mcp.CallToolResult, engine.RepoStashApplyResult, error) {
		r, err := eng.RepoStashApply(ctx, engine.RepoStashActionParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Index: in.Index, Hash: in.Hash})
		return done(r, err, func(r engine.RepoStashApplyResult) string { return stashApplySummary(r, "applied") })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_stash_pop",
		Description: "Apply one stash entry of a workspace repo and remove it from the stash list, but only when the apply was complete (no conflicts, staged changes restored); otherwise the entry is kept (dropped=false). " +
			stashApplyDoc,
		Annotations: mutating("Pop stash", false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoStashActionInput) (*mcp.CallToolResult, engine.RepoStashApplyResult, error) {
		r, err := eng.RepoStashPop(ctx, engine.RepoStashActionParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Index: in.Index, Hash: in.Hash})
		return done(r, err, func(r engine.RepoStashApplyResult) string { return stashApplySummary(r, "popped") })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_stash_drop",
		Description: "Delete one stash entry of a workspace repo; its changes are lost. Requires confirm=true; with confirm=false nothing changes and the tool returns needs_confirmation with the entry and its file count. " +
			"A shifted stash list is refused with stash_changed.",
		Annotations: mutating("Drop stash", true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoStashDropInput) (*mcp.CallToolResult, engine.RepoStashDropResult, error) {
		r, err := eng.RepoStashDrop(ctx, engine.RepoStashActionParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Index: in.Index, Hash: in.Hash, Confirm: in.Confirm})
		return done(r, err, func(r engine.RepoStashDropResult) string {
			return fmt.Sprintf("%s: dropped %s (%s).", r.Repo, r.Stash.Ref, r.Stash.Message)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_delete_untracked",
		Description: "Permanently delete untracked files of a workspace repo (git clean on exactly those files; not recoverable). Only files repo_changes currently lists as untracked are accepted; tracked, ignored and directory paths are refused. " +
			"Requires confirm=true; with confirm=false nothing changes and the tool returns needs_confirmation listing the paths. Never discards tracked changes.",
		Annotations: mutating("Delete untracked files", true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoDeleteUntrackedInput) (*mcp.CallToolResult, engine.DiscardUntrackedResult, error) {
		r, err := eng.RepoDiscardUntracked(ctx, engine.RepoPathsParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo, Paths: in.Paths, Confirm: in.Confirm})
		return done(r, err, func(r engine.DiscardUntrackedResult) string {
			summary := fmt.Sprintf("%s: deleted %s.", r.Repo, plural(len(r.Removed), "untracked file", "untracked files"))
			if len(r.Kept) > 0 {
				summary += fmt.Sprintf(" %s still present.", plural(len(r.Kept), "file", "files"))
			}
			return summary
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_fetch",
		Description: "Fetch a workspace repo's remote (git fetch --prune) and return its branch info afterwards. It only updates remote-tracking refs: " +
			"the worktree, the index and every local branch are left unchanged.",
		Annotations: &mcp.ToolAnnotations{Title: "Fetch repo", DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(true)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RepoInput) (*mcp.CallToolResult, engine.RepoFetchResult, error) {
		r, err := eng.RepoFetch(ctx, engine.RepoRef{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo}, progressFor(ctx, req))
		return done(r, err, func(r engine.RepoFetchResult) string {
			if u := r.Branch.Upstream; u != nil {
				return fmt.Sprintf("Fetched %s for %s: %d ahead, %d behind %s.", r.Remote, r.Repo, u.Ahead, u.Behind, u.Ref)
			}
			return fmt.Sprintf("Fetched %s for %s.", r.Remote, r.Repo)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "repo_pull_ff",
		Description: "Pull a workspace repo's upstream with a fast-forward only: fetch, then move the branch forward to its upstream. It never creates a merge commit and never rebases. " +
			"Refusals change nothing and are reported in the result's error, not as a tool failure: no_upstream, diverged (local commits the upstream lacks; data.ahead/behind; suggest update_repo or manual integration to the user), " +
			"worktree_dirty (local changes the pull would overwrite; data.files), detached_head, integration_in_progress. upToDate=true means there was nothing to pull.",
		Annotations: &mcp.ToolAnnotations{Title: "Pull (fast-forward only)", DestructiveHint: boolPtr(false), IdempotentHint: false, OpenWorldHint: boolPtr(true)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RepoInput) (*mcp.CallToolResult, engine.RepoPullResult, error) {
		r, err := eng.RepoPull(ctx, engine.RepoRef{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo}, progressFor(ctx, req))
		return done(r, err, func(r engine.RepoPullResult) string {
			switch {
			case r.Error != nil:
				return fmt.Sprintf("%s: not pulled (%s).", r.Repo, r.Error.Message)
			case r.UpToDate:
				return fmt.Sprintf("%s: already up to date with %s.", r.Repo, r.Upstream)
			default:
				return fmt.Sprintf("%s: fast-forwarded %s from %s.", r.Repo, plural(r.CommitsPulled, "commit", "commits"), r.Upstream)
			}
		})
	})
}

func stashApplySummary(r engine.RepoStashApplyResult, verb string) string {
	ref := "the stash entry"
	if r.Stash != nil {
		ref = r.Stash.Ref
	}
	switch {
	case r.Error != nil && len(r.Conflicts) > 0:
		return fmt.Sprintf("%s: %s applied with conflicts in %s; the entry was kept.", r.Repo, ref, plural(len(r.Conflicts), "file", "files"))
	case r.Error != nil:
		return fmt.Sprintf("%s: %s not applied (%s).", r.Repo, ref, r.Error.Message)
	case verb == "popped" && !r.Dropped:
		return fmt.Sprintf("%s: %s applied; the entry was kept.", r.Repo, ref)
	}
	return fmt.Sprintf("%s: %s %s.", r.Repo, ref, verb)
}
