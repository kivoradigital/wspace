// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
)

// ListProjectsOutput wraps the project list.
type ListProjectsOutput struct {
	Projects []engine.Project `json:"projects"`
}

// ScanProjectsInput are scan_projects' arguments.
type ScanProjectsInput struct {
	Context string    `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Roots   []string  `json:"roots,omitempty" jsonschema:"Absolute folders to search; default the context's projects root."`
	Depth   int       `json:"depth,omitempty" jsonschema:"Maximum folder depth; default the context's, then 5."`
	Ignore  *[]string `json:"ignore,omitempty" jsonschema:"Folder-name globs to skip; default the context's ignore patterns; [] skips nothing."`
	Include *[]string `json:"include,omitempty" jsonschema:"Globs (folder name or root-relative path) a repository must match; default the context's include patterns; [] includes everything."`
}

// RegisterProjectInput are register_project's arguments.
type RegisterProjectInput struct {
	Context      string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Key          string `json:"key" jsonschema:"Project key: one folder name, not starting with . or - (it names the repo's folder in every workspace)."`
	SourceDir    string `json:"sourceDir" jsonschema:"Absolute path of the git main clone."`
	OriginBranch string `json:"originBranch,omitempty" jsonschema:"Base branch for new workspace branches of this project (overrides the context's baseBranch)."`
	DestBranch   string `json:"destBranch,omitempty" jsonschema:"Branch template ({workspace}, {project}, {branch}, {prefix})."`
	WorktreeDir  string `json:"worktreeDir,omitempty" jsonschema:"Worktree folder template inside the workspace."`
}

// ProjectToRegisterInput is one entry of register_projects.
type ProjectToRegisterInput struct {
	Key       string `json:"key" jsonschema:"Project key (scan_projects suggests one per candidate: suggestedKey)."`
	SourceDir string `json:"sourceDir" jsonschema:"Absolute path of the git main clone (a scan candidate's path)."`
}

// RegisterProjectsInput are register_projects' arguments.
type RegisterProjectsInput struct {
	Context  string                   `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Projects []ProjectToRegisterInput `json:"projects" jsonschema:"Projects to register, all or nothing."`
}

// UpdateProjectInput are update_project's arguments.
type UpdateProjectInput struct {
	Context      string  `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Key          string  `json:"key" jsonschema:"Project to edit."`
	SourceDir    *string `json:"sourceDir,omitempty" jsonschema:"New absolute main clone path."`
	OriginBranch *string `json:"originBranch,omitempty" jsonschema:"New base branch; empty string restores inheritance."`
	DestBranch   *string `json:"destBranch,omitempty" jsonschema:"New branch template; empty string restores inheritance."`
	WorktreeDir  *string `json:"worktreeDir,omitempty" jsonschema:"New worktree folder template; empty string restores inheritance."`
}

// UnregisterProjectInput are unregister_project's arguments.
type UnregisterProjectInput struct {
	Context string `json:"context,omitempty" jsonschema:"Context name; omit to use the active context."`
	Key     string `json:"key" jsonschema:"Project to unregister."`
	Confirm bool   `json:"confirm" jsonschema:"Must be true to proceed. With false the tool changes nothing and returns needs_confirmation previewing every reason; show them to the user first."`
}

// UnregisterProjectOutput names the unregistered project.
type UnregisterProjectOutput struct {
	Unregistered string `json:"unregistered"`
}

func registerProjectTools(s *mcp.Server, eng *engine.Engine) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_projects",
		Description: "List the projects (git main clones) registered in a context. Their keys are what create_workspace and add_project accept.",
		Annotations: readOnly("List projects"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ContextInput) (*mcp.CallToolResult, ListProjectsOutput, error) {
		list, err := eng.ListProjects(ctx, engine.ProjectsRef{Context: in.Context})
		return done(ListProjectsOutput{Projects: list}, err, func(o ListProjectsOutput) string {
			return plural(len(o.Projects), "project", "projects") + " registered."
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "scan_projects",
		Description: "Find git main clones under folders (default the context's projects root) without registering anything. " +
			"Each candidate has a suggestedKey and says whether it is already registered; truncated=true with truncatedDirs means the depth cap stopped the search. " +
			"Then register the ones the user picks with register_projects.",
		Annotations: readOnly("Scan for projects"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ScanProjectsInput) (*mcp.CallToolResult, engine.ScanProjectsResult, error) {
		res, err := eng.ScanProjects(ctx, engine.ScanProjectsParams{Context: in.Context, Roots: in.Roots, Depth: in.Depth, Ignore: in.Ignore, Include: in.Include})
		return done(res, err, func(r engine.ScanProjectsResult) string {
			msg := fmt.Sprintf("Found %s (depth %d).", plural(len(r.Candidates), "repository", "repositories"), r.MaxDepth)
			if r.Truncated {
				msg += fmt.Sprintf(" The depth cap left %s unsearched.", plural(r.TruncatedCount, "folder", "folders"))
			}
			return msg
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "register_project",
		Description: "Register one git main clone as a project of a context, optionally with its own base branch, branch template or worktree folder template. Prefer register_projects for several.",
		Annotations: mutating("Register project", false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RegisterProjectInput) (*mcp.CallToolResult, engine.Project, error) {
		p, err := eng.RegisterProject(ctx, engine.RegisterProjectParams{
			Context: in.Context, Key: in.Key, SourceDir: in.SourceDir,
			OriginBranch: in.OriginBranch, DestBranch: in.DestBranch, WorktreeDir: in.WorktreeDir,
		})
		return done(p, err, func(p engine.Project) string { return fmt.Sprintf("Registered project %s (%s).", p.Key, p.SourceDir) })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "register_projects",
		Description: "Register several projects at once, all or nothing: when any entry has a problem nothing is written and problems lists each one (reason and message). " +
			"Fix them and call again. Typically fed with scan_projects candidates (path and suggestedKey).",
		Annotations: mutating("Register projects", false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RegisterProjectsInput) (*mcp.CallToolResult, engine.RegisterProjectsResult, error) {
		list := make([]engine.ProjectToRegister, 0, len(in.Projects))
		for _, p := range in.Projects {
			list = append(list, engine.ProjectToRegister{Key: p.Key, SourceDir: p.SourceDir})
		}
		res, err := eng.RegisterProjects(ctx, engine.RegisterProjectsParams{Context: in.Context, Projects: list})
		return done(res, err, func(r engine.RegisterProjectsResult) string {
			if len(r.Problems) > 0 {
				return fmt.Sprintf("Nothing registered: %s to fix.", plural(len(r.Problems), "problem", "problems"))
			}
			return fmt.Sprintf("Registered %s.", plural(len(r.Registered), "project", "projects"))
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_project",
		Description: "Edit one registered project: main clone path, base branch, branch template or worktree folder template. Only fields you pass change; an empty string restores inheritance. Existing workspaces are not changed.",
		Annotations: mutating("Update project", false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateProjectInput) (*mcp.CallToolResult, engine.Project, error) {
		p, err := eng.UpdateProject(ctx, engine.UpdateProjectParams{
			Context: in.Context, Key: in.Key, SourceDir: in.SourceDir,
			OriginBranch: in.OriginBranch, DestBranch: in.DestBranch, WorktreeDir: in.WorktreeDir,
		})
		return done(p, err, func(p engine.Project) string { return "Updated project " + p.Key + "." })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "unregister_project",
		Description: "Remove a project's registration from a context. The repository and existing workspaces are never touched, but new workspaces can no longer include it. " +
			"Not to be confused with remove_project, which removes a repo from one workspace. Requires confirm=true; with confirm=false nothing changes and the tool returns needs_confirmation.",
		Annotations: mutating("Unregister project", true, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UnregisterProjectInput) (*mcp.CallToolResult, UnregisterProjectOutput, error) {
		ref := engine.ProjectRef{Context: in.Context, Key: in.Key}
		if !in.Confirm {
			p, err := eng.GetProject(ctx, ref)
			if err != nil {
				return fail[UnregisterProjectOutput](err)
			}
			return fail[UnregisterProjectOutput](confirmRequired(nil, map[string]any{"project": p.Key, "sourceDir": p.SourceDir}))
		}
		err := eng.RemoveProject(ctx, ref)
		return done(UnregisterProjectOutput{Unregistered: in.Key}, err, func(o UnregisterProjectOutput) string {
			return "Unregistered project " + o.Unregistered + " (repository untouched)."
		})
	})
}
