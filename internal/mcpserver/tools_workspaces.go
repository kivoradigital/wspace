// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
)

// ListWorkspacesOutput wraps the workspace list.
type ListWorkspacesOutput struct {
	Workspaces []engine.WorkspaceStatus `json:"workspaces"`
}

// RepoInput names one repo (by alias) of one workspace.
type RepoInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name (a directory name under the context's workspaces root, see list_workspaces)."`
	Repo      string `json:"repo" jsonschema:"Repo alias inside the workspace (see workspace_status)."`
}

// RepoChangesOutput wraps repo_changes' list.
type RepoChangesOutput struct {
	Changes []engine.FileChange `json:"changes"`
}

// TeardownCheckOutput is teardown_check's result.
type TeardownCheckOutput struct {
	// Safe is true when an unforced destroy would proceed.
	Safe    bool             `json:"safe"`
	Reasons []engine.Blocker `json:"reasons"`
}

// CreateWorkspaceInput are create_workspace's arguments.
type CreateWorkspaceInput struct {
	Context         string          `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Name            string          `json:"name" jsonschema:"New workspace name: one directory name, no slashes."`
	Branch          string          `json:"branch,omitempty" jsonschema:"Branch to create or check out in every worktree; defaults to the branch prefix plus the workspace name."`
	Projects        []string        `json:"projects,omitempty" jsonschema:"Project keys to include (see list_projects); omit to include every project in the context."`
	Options         *engine.Options `json:"options,omitempty" jsonschema:"Option overrides for this workspace, e.g. {\"branchPrefix\":\"fix\"} for a fix/<name> branch or {\"baseBranch\":\"main\"}."`
	CopyNodeModules bool            `json:"copyNodeModules,omitempty" jsonschema:"Copy node_modules from each Node project's main clone into its new worktree (list_projects reports hasNodeModules), so dependencies need no reinstall."`
}

// AddProjectInput are add_project's arguments.
type AddProjectInput struct {
	Context         string          `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace       string          `json:"workspace" jsonschema:"Existing workspace name."`
	Project         string          `json:"project" jsonschema:"Project key to add (see list_projects)."`
	Options         *engine.Options `json:"options,omitempty" jsonschema:"Option overrides for this repo (e.g. baseBranch)."`
	CopyNodeModules bool            `json:"copyNodeModules,omitempty" jsonschema:"Copy node_modules from the project's main clone into the new worktree."`
}

// RemoveProjectInput are remove_project's arguments.
type RemoveProjectInput struct {
	Context      string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace    string `json:"workspace" jsonschema:"Workspace name."`
	Project      string `json:"project" jsonschema:"Alias of the repo to remove from the workspace (the project key it was added with)."`
	Confirm      bool   `json:"confirm" jsonschema:"Must be true to proceed. With false the tool changes nothing and returns needs_confirmation previewing every reason; show them to the user first."`
	Force        bool   `json:"force,omitempty" jsonschema:"Discard uncommitted changes and unpushed commits. Only set after the user explicitly agreed to lose them; never on your own."`
	DeleteBranch bool   `json:"deleteBranch,omitempty" jsonschema:"Also delete the repo's workspace branch (kept by default)."`
}

// DestroyWorkspaceInput are destroy_workspace's arguments.
type DestroyWorkspaceInput struct {
	Context        string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace      string `json:"workspace" jsonschema:"Workspace name."`
	Confirm        bool   `json:"confirm" jsonschema:"Must be true to proceed. With false the tool changes nothing and returns needs_confirmation previewing every reason; show them to the user first."`
	Force          bool   `json:"force,omitempty" jsonschema:"Discard uncommitted changes and unpushed commits. Only set after the user explicitly agreed to lose them; never on your own."`
	DeleteBranches bool   `json:"deleteBranches,omitempty" jsonschema:"Also delete the workspace branches (kept by default)."`
}

// SyncEnvInput are sync_env's arguments.
type SyncEnvInput struct {
	Context   string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Workspace string `json:"workspace" jsonschema:"Workspace name."`
	Confirm   bool   `json:"confirm" jsonschema:"Must be true to proceed. With false the tool changes nothing and returns needs_confirmation previewing every reason; show them to the user first."`
}

func registerWorkspaceTools(s *mcp.Server, eng *engine.Engine) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_workspaces",
		Description: "List the workspaces of a context with live per-repo status (branch, ahead/behind, dirty). A damaged workspace is listed with an error instead of hiding the others; legacy=true marks one only the legacy ws tool manages.",
		Annotations: readOnly("List workspaces"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ContextInput) (*mcp.CallToolResult, ListWorkspacesOutput, error) {
		list, err := eng.ListWorkspaces(ctx, engine.WorkspacesRef{Context: in.Context})
		return done(ListWorkspacesOutput{Workspaces: list}, err, func(o ListWorkspacesOutput) string {
			dirty := 0
			for _, w := range o.Workspaces {
				if w.Dirty {
					dirty++
				}
			}
			return fmt.Sprintf("%s, %d with uncommitted changes.", plural(len(o.Workspaces), "workspace", "workspaces"), dirty)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "workspace_status",
		Description: "Show one workspace's per-repo status: branch, detached, ahead/behind its upstream, dirty, and each uncommitted path classified as tracked, foreign (untracked, not created by wspace) or env_copy. " +
			"Without an upstream, ahead is counted against the repo's base (baseBranch); baseMissing=true means that base does not exist in the repo, so ahead/behind are unknown. " +
			"Fix it by setting the project's originBranch with update_project.",
		Annotations: readOnly("Workspace status"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in WorkspaceInput) (*mcp.CallToolResult, engine.WorkspaceStatus, error) {
		st, err := eng.WorkspaceStatus(ctx, engine.WorkspaceRef{Context: in.Context, Workspace: in.Workspace})
		return done(st, err, func(st engine.WorkspaceStatus) string {
			state := "clean"
			if st.Dirty {
				state = "has uncommitted changes"
			}
			summary := fmt.Sprintf("Workspace %s: %s, %s.", st.Name, plural(st.RepoCount, "repo", "repos"), state)
			for _, r := range st.Repos {
				if r.BaseMissing {
					summary += fmt.Sprintf(" %s: base branch %s not found, ahead/behind unknown.", r.Alias, r.BaseBranch)
				}
			}
			return summary
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "repo_changes",
		Description: "List one repo's changed files in a workspace (modified, added, deleted, renamed, copied, typechange, untracked, conflicted; staged and unstaged), in git's order. Use it to show the user what a teardown would lose.",
		Annotations: readOnly("Repo changes"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RepoInput) (*mcp.CallToolResult, RepoChangesOutput, error) {
		list, err := eng.RepoChanges(ctx, engine.RepoRef{Context: in.Context, Workspace: in.Workspace, Repo: in.Repo})
		return done(RepoChangesOutput{Changes: list}, err, func(o RepoChangesOutput) string {
			return fmt.Sprintf("%s: %s.", in.Repo, plural(len(o.Changes), "changed file", "changed files"))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "teardown_check",
		Description: "Preview, without changing anything, every reason destroy_workspace (without force) would refuse: tracked changes, foreign untracked files and unpushed commits per repo. " +
			"safe=true means a confirmed, unforced destroy would proceed. Call it before destroying and show the reasons to the user.",
		Annotations: readOnly("Teardown check"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in WorkspaceInput) (*mcp.CallToolResult, TeardownCheckOutput, error) {
		reasons, err := eng.TeardownBlockers(ctx, engine.WorkspaceRef{Context: in.Context, Workspace: in.Workspace})
		if reasons == nil {
			reasons = []engine.Blocker{}
		}
		return done(TeardownCheckOutput{Safe: len(reasons) == 0, Reasons: reasons}, err, func(o TeardownCheckOutput) string {
			if o.Safe {
				return "Safe to destroy: nothing would be lost."
			}
			return fmt.Sprintf("Destroying would lose work: %s.", plural(len(o.Reasons), "reason", "reasons"))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "create_workspace",
		Description: "Create a workspace: one git worktree per selected project, all on one branch, with env files copied in. Every project is validated before anything is created, and a failure part-way is rolled back. " +
			"It can take a while (it fetches each base branch): pass a progress token to receive per-repo progress notifications; the result lists every repo.",
		Annotations: mutating("Create workspace", false, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CreateWorkspaceInput) (*mcp.CallToolResult, engine.Workspace, error) {
		ws, err := eng.CreateWorkspace(ctx, engine.CreateWorkspaceParams{Context: in.Context, Name: in.Name, Branch: in.Branch, Projects: in.Projects, Options: in.Options, CopyNodeModules: in.CopyNodeModules}, progressFor(ctx, req))
		return done(ws, err, func(ws engine.Workspace) string {
			return fmt.Sprintf("Created workspace %s on branch %s with %s at %s.", ws.Name, ws.Branch, plural(len(ws.Repos), "repo", "repos"), ws.Path)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_addable_projects",
		Description: "List the context's projects that add_project accepts for a workspace: every registered project not already in it (matched by project key, main clone directory or repo alias). Read-only; an empty list means every project is already in the workspace.",
		Annotations: readOnly("Addable projects"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in WorkspaceInput) (*mcp.CallToolResult, ListProjectsOutput, error) {
		list, err := eng.AddableProjects(ctx, engine.WorkspaceRef{Context: in.Context, Workspace: in.Workspace})
		if list == nil {
			list = []engine.Project{}
		}
		return done(ListProjectsOutput{Projects: list}, err, func(o ListProjectsOutput) string {
			return fmt.Sprintf("%s can be added to %s.", plural(len(o.Projects), "project", "projects"), in.Workspace)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_project",
		Description: "Add one more project's worktree to an existing workspace, on the workspace's branch (the base branch is fetched first). A failure is rolled back, leaving the workspace as it was.",
		Annotations: mutating("Add project to workspace", false, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in AddProjectInput) (*mcp.CallToolResult, engine.RepoEntry, error) {
		entry, err := eng.AddRepo(ctx, engine.AddRepoParams{Context: in.Context, Workspace: in.Workspace, Project: in.Project, Options: in.Options, CopyNodeModules: in.CopyNodeModules}, progressFor(ctx, req))
		return done(entry, err, func(e engine.RepoEntry) string {
			return fmt.Sprintf("Added %s to %s on branch %s.", e.Alias, in.Workspace, e.Branch)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "remove_project",
		Description: "Remove one repo's worktree from a workspace (the project stays registered; see unregister_project for that). Requires confirm=true; with confirm=false nothing changes and the tool previews what would be lost. " +
			"Without force it refuses when the repo has uncommitted tracked changes, foreign untracked files or unpushed commits, returning needs_confirmation with every reason.",
		Annotations: mutating("Remove project from workspace", true, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RemoveProjectInput) (*mcp.CallToolResult, engine.AliasResult, error) {
		ref := engine.WorkspaceRef{Context: in.Context, Workspace: in.Workspace}
		if !in.Confirm {
			// RepoChanges validates the alias (not_found for an unknown one).
			if _, err := eng.RepoChanges(ctx, engine.RepoRef{Context: in.Context, Workspace: in.Workspace, Repo: in.Project}); err != nil {
				return fail[engine.AliasResult](err)
			}
			all, err := eng.TeardownBlockers(ctx, ref)
			if err != nil {
				return fail[engine.AliasResult](err)
			}
			reasons := []engine.Blocker{}
			for _, b := range all {
				if b.Repo == in.Project {
					reasons = append(reasons, b)
				}
			}
			return fail[engine.AliasResult](confirmRequired(reasons, nil))
		}
		res, err := eng.RemoveRepo(ctx, engine.RemoveRepoParams{Context: in.Context, Workspace: in.Workspace, Repo: in.Project, Force: in.Force, DeleteBranch: in.DeleteBranch}, progressFor(ctx, req))
		return done(res, err, func(r engine.AliasResult) string { return fmt.Sprintf("Removed %s from %s.", r.Alias, in.Workspace) })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "destroy_workspace",
		Description: "Destroy a workspace: remove every worktree and the workspace directory (branches are kept unless deleteBranches). Requires confirm=true; with confirm=false nothing changes and the tool previews what would be lost (same as teardown_check). " +
			"Without force it refuses when any repo has uncommitted tracked changes, foreign untracked files or unpushed commits, returning needs_confirmation with every reason.",
		Annotations: mutating("Destroy workspace", true, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in DestroyWorkspaceInput) (*mcp.CallToolResult, engine.PathResult, error) {
		ref := engine.WorkspaceRef{Context: in.Context, Workspace: in.Workspace}
		if !in.Confirm {
			reasons, err := eng.TeardownBlockers(ctx, ref)
			if err != nil {
				return fail[engine.PathResult](err)
			}
			return fail[engine.PathResult](confirmRequired(reasons, nil))
		}
		res, err := eng.DestroyWorkspace(ctx, engine.DestroyWorkspaceParams{Context: in.Context, Workspace: in.Workspace, Force: in.Force, DeleteBranches: in.DeleteBranches}, progressFor(ctx, req))
		return done(res, err, func(r engine.PathResult) string { return "Destroyed workspace at " + r.Path + "." })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "repair_workspace",
		Description: "Recreate every worktree a workspace's manifest declares but that is missing from disk (e.g. a repo folder deleted by hand). Existing worktrees are left alone.",
		Annotations: mutating("Repair workspace", false, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceInput) (*mcp.CallToolResult, engine.RepairResult, error) {
		res, err := eng.Repair(ctx, engine.WorkspaceRef{Context: in.Context, Workspace: in.Workspace}, progressFor(ctx, req))
		return done(res, err, func(r engine.RepairResult) string {
			return fmt.Sprintf("Repaired %s: %s recreated.", in.Workspace, plural(len(r.Recreated), "worktree", "worktrees"))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "sync_env",
		Description: "Re-copy env files (.env and similar) from each project's main clone into the workspace's repos, overwriting the copies there, including any local edits to them. " +
			"Requires confirm=true; with confirm=false nothing changes and the tool returns needs_confirmation.",
		Annotations: mutating("Sync env files", true, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SyncEnvInput) (*mcp.CallToolResult, engine.SyncEnvResult, error) {
		ref := engine.WorkspaceRef{Context: in.Context, Workspace: in.Workspace}
		if !in.Confirm {
			st, err := eng.WorkspaceStatus(ctx, ref)
			if err != nil {
				return fail[engine.SyncEnvResult](err)
			}
			return fail[engine.SyncEnvResult](confirmRequired(nil, map[string]any{"workspace": st.Name, "effect": "env file copies in every repo are overwritten from the main clones"}))
		}
		res, err := eng.SyncEnv(ctx, ref, progressFor(ctx, req))
		return done(res, err, func(r engine.SyncEnvResult) string {
			return fmt.Sprintf("Synced %s in %s.", plural(len(r.Copied), "env file", "env files"), in.Workspace)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "adopt_legacy_workspaces",
		Description: "Give every workspace the legacy bash ws tool created in a context (legacy=true in list_workspaces) a wspace manifest, so wspace can manage it. " +
			"Legacy files are never changed. Idempotent; problems are reported per workspace in skipped, never as a failure.",
		Annotations: mutating("Adopt legacy workspaces", false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ContextInput) (*mcp.CallToolResult, engine.AdoptLegacyResult, error) {
		res, err := eng.AdoptLegacyWorkspaces(ctx, engine.WorkspacesRef{Context: in.Context})
		return done(res, err, func(r engine.AdoptLegacyResult) string {
			return fmt.Sprintf("Adopted %s, skipped %d.", plural(len(r.Adopted), "workspace", "workspaces"), len(r.Skipped))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "claim_workspaces",
		Description: "Make a context the owner of orphaned workspaces: those whose manifest names a context that no longer exists (orphanOf in list_workspaces), " +
			"e.g. after the context was deleted or renamed by an older version. Omit workspaces to claim every orphan in the context's workspaces root. " +
			"Only the manifest's owner field changes; a workspace owned by another existing context is never taken (it is reported in skipped). Idempotent.",
		Annotations: mutating("Claim orphaned workspaces", false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ClaimInput) (*mcp.CallToolResult, engine.ClaimWorkspacesResult, error) {
		res, err := eng.ClaimWorkspaces(ctx, engine.ClaimWorkspacesParams{Context: in.Context, Workspaces: in.Workspaces})
		return done(res, err, func(r engine.ClaimWorkspacesResult) string {
			return fmt.Sprintf("Claimed %s, skipped %d.", plural(len(r.Claimed), "workspace", "workspaces"), len(r.Skipped))
		})
	})
}
