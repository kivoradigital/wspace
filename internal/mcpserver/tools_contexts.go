// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
)

// ListContextsOutput wraps the context list in an object.
type ListContextsOutput struct {
	Contexts []engine.ContextSummary `json:"contexts"`
}

// GetContextInput selects a context by name.
type GetContextInput struct {
	Name string `json:"name,omitempty" jsonschema:"Context name; omit for the active context."`
}

// NameInput names one context.
type NameInput struct {
	Name string `json:"name" jsonschema:"Context name (see list_contexts)."`
}

// CreateContextInput are create_context's arguments.
type CreateContextInput struct {
	Name                string          `json:"name" jsonschema:"New context name."`
	WorkspacesRoot      string          `json:"workspacesRoot" jsonschema:"Absolute folder where this context's workspaces are created."`
	ProjectsRoot        string          `json:"projectsRoot,omitempty" jsonschema:"Absolute folder scan_projects searches by default."`
	IgnorePatterns      []string        `json:"ignorePatterns,omitempty" jsonschema:"Folder-name globs scan_projects skips (path.Match syntax)."`
	IncludePatterns     []string        `json:"includePatterns,omitempty" jsonschema:"Globs (folder name or root-relative path) limiting scan_projects; empty offers every repository."`
	ProjectScanMaxDepth int             `json:"projectScanMaxDepth,omitempty" jsonschema:"Default scan depth (default 5)."`
	Defaults            *engine.Options `json:"defaults,omitempty" jsonschema:"Default options for this context's workspaces (baseBranch, branchPrefix, copyEnv, fetchBeforeCreate, envPruneDirs, remote)."`
	Activate            bool            `json:"activate,omitempty" jsonschema:"Make the new context the active one."`
}

// UpdateContextInput are update_context's arguments: every given field
// replaces the stored value.
type UpdateContextInput struct {
	Name                string          `json:"name" jsonschema:"Context to edit."`
	NewName             *string         `json:"newName,omitempty" jsonschema:"Rename the context (refused if another context has that name)."`
	WorkspacesRoot      *string         `json:"workspacesRoot,omitempty" jsonschema:"New absolute workspaces folder. Existing workspaces are not moved."`
	ProjectsRoot        *string         `json:"projectsRoot,omitempty" jsonschema:"New absolute projects folder."`
	IgnorePatterns      *[]string       `json:"ignorePatterns,omitempty" jsonschema:"Replaces the ignore globs."`
	IncludePatterns     *[]string       `json:"includePatterns,omitempty" jsonschema:"Replaces the include globs."`
	ProjectScanMaxDepth *int            `json:"projectScanMaxDepth,omitempty" jsonschema:"Replaces the default scan depth."`
	Defaults            *engine.Options `json:"defaults,omitempty" jsonschema:"Replaces the context's default options."`
}

// DeleteContextInput are delete_context's arguments.
type DeleteContextInput struct {
	Name        string `json:"name" jsonschema:"Context to delete."`
	AllowActive bool   `json:"allowActive,omitempty" jsonschema:"Allow deleting the active context (even the only one); the active context is then cleared. Without it the active context is refused."`
	Confirm     bool   `json:"confirm" jsonschema:"Must be true to proceed. With false the tool changes nothing and returns needs_confirmation previewing every reason; show them to the user first."`
}

// DeleteContextOutput names the deleted context.
type DeleteContextOutput struct {
	Deleted string `json:"deleted"`
}

// ImportLegacyInput are import_legacy_context's arguments.
type ImportLegacyInput struct {
	From    string `json:"from,omitempty" jsonschema:"Absolute path of the legacy ws config file; omit to discover it."`
	Name    string `json:"name,omitempty" jsonschema:"Create a new context with this name (exclusive with into)."`
	Into    string `json:"into,omitempty" jsonschema:"Merge into this existing context (exclusive with name)."`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"Only with into: false (default) is a dry run describing the change; true applies it."`
}

func registerContextTools(s *mcp.Server, eng *engine.Engine) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_contexts",
		Description: "List every wspace context (a named set of workspaces root, projects root and defaults) and which one is active. Start here to learn the user's setup.",
		Annotations: readOnly("List contexts"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoInput) (*mcp.CallToolResult, ListContextsOutput, error) {
		list, err := eng.ListContexts(ctx)
		return done(ListContextsOutput{Contexts: list}, err, func(o ListContextsOutput) string {
			active := "none"
			for _, c := range o.Contexts {
				if c.Active {
					active = c.Name
				}
			}
			return fmt.Sprintf("%s; active: %s.", plural(len(o.Contexts), "context", "contexts"), active)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_context",
		Description: "Show one context in full: roots, scan patterns and depth, default options and every registered project. Omit name for the active context.",
		Annotations: readOnly("Get context"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetContextInput) (*mcp.CallToolResult, engine.Context, error) {
		c, err := eng.GetContext(ctx, engine.ContextRef{Name: in.Name})
		return done(c, err, contextSummary)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "create_context",
		Description: "Create a context from explicit values (no projects yet: find them with scan_projects and add them with register_projects). " +
			"Use when the user wants a separate set of workspaces, e.g. per client or team.",
		Annotations: mutating("Create context", false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateContextInput) (*mcp.CallToolResult, engine.Context, error) {
		c, err := eng.CreateContext(ctx, engine.CreateContextParams{
			Name: in.Name, WorkspacesRoot: in.WorkspacesRoot, ProjectsRoot: in.ProjectsRoot,
			IgnorePatterns: in.IgnorePatterns, IncludePatterns: in.IncludePatterns,
			ProjectScanMaxDepth: in.ProjectScanMaxDepth, Defaults: in.Defaults, Activate: in.Activate,
		})
		return done(c, err, func(c engine.Context) string { return "Created " + contextSummary(c) })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "update_context",
		Description: "Edit or rename a context. Every field you pass replaces the stored value; omitted fields are kept. " +
			"Projects are not changed here (use the project tools) and workspace folders are never moved; a rename moves the old name's workspaces to the new name " +
			"(reassignedWorkspaces lists them; reassignFailures lists any whose manifest could not be updated, which claim_workspaces can retry).",
		Annotations: mutating("Update context", false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateContextInput) (*mcp.CallToolResult, engine.Context, error) {
		c, err := eng.UpdateContext(ctx, engine.UpdateContextParams{
			Name: in.Name, NewName: in.NewName, WorkspacesRoot: in.WorkspacesRoot, ProjectsRoot: in.ProjectsRoot,
			IgnorePatterns: in.IgnorePatterns, IncludePatterns: in.IncludePatterns,
			ProjectScanMaxDepth: in.ProjectScanMaxDepth, Defaults: in.Defaults,
		})
		return done(c, err, func(c engine.Context) string {
			out := "Updated " + contextSummary(c)
			if n := len(c.ReassignedWorkspaces); n > 0 {
				out += fmt.Sprintf(" Reassigned %s to it.", plural(n, "workspace", "workspaces"))
			}
			if n := len(c.ReassignFailures); n > 0 {
				out += fmt.Sprintf(" %s could not be reassigned (retry with claim_workspaces).", plural(n, "workspace", "workspaces"))
			}
			return out
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "switch_context",
		Description: "Make a context the active one, which every tool uses when context is omitted (and the app and CLI show). Ask the user before switching their active context.",
		Annotations: mutating("Switch context", false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in NameInput) (*mcp.CallToolResult, engine.ContextSummary, error) {
		c, err := eng.SwitchContext(ctx, engine.ContextRef{Name: in.Name})
		return done(c, err, func(c engine.ContextSummary) string { return "Active context is now " + c.Name + "." })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "delete_context",
		Description: "Delete a context's configuration (its projects registrations and defaults). Workspaces and repositories on disk are never touched, but the context and its registrations are gone; " +
			"its workspaces then show as orphaned (orphanOf in list_workspaces) to any other context sharing the workspaces root, which can take them over with claim_workspaces. " +
			"Refused for the active context unless allowActive=true, which deletes it and leaves no active context (prefer switch_context to another one first). Requires confirm=true; with confirm=false nothing changes and the tool returns needs_confirmation describing what would be deleted.",
		Annotations: mutating("Delete context", true, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DeleteContextInput) (*mcp.CallToolResult, DeleteContextOutput, error) {
		if !in.Confirm {
			c, err := eng.GetContext(ctx, engine.ContextRef{Name: in.Name})
			if err != nil {
				return fail[DeleteContextOutput](err)
			}
			return fail[DeleteContextOutput](confirmRequired(nil, map[string]any{"context": c.Name, "projectCount": len(c.Projects), "active": c.Active, "allowActive": in.AllowActive}))
		}
		err := eng.DeleteContext(ctx, engine.DeleteContextParams{Name: in.Name, AllowActive: in.AllowActive})
		return done(DeleteContextOutput{Deleted: in.Name}, err, func(o DeleteContextOutput) string {
			return "Deleted context " + o.Deleted + " (workspaces on disk untouched; another context sharing the root can claim them)."
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "import_legacy_context",
		Description: "Convert the legacy bash ws tool's config file into a wspace context. Pass exactly one of name (create a new context) or into (merge into an existing one). " +
			"A merge without confirm=true is a dry run that only describes the change (applied=false); show it to the user, then call again with confirm=true. Once the context is written, orphaned workspaces in its workspaces root (owner context no longer exists) are claimed for it and legacy workspaces are adopted.",
		Annotations: mutating("Import legacy context", false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ImportLegacyInput) (*mcp.CallToolResult, engine.ImportLegacyResult, error) {
		res, err := eng.ImportLegacyContext(ctx, engine.ImportLegacyParams{From: in.From, Name: in.Name, Into: in.Into, Confirm: in.Confirm})
		return done(res, err, func(r engine.ImportLegacyResult) string {
			verb := "Imported"
			if !r.Applied {
				verb = "Dry run (nothing written) for"
			}
			return fmt.Sprintf("%s context %s: %s, %s skipped.", verb, r.Context, plural(len(r.ImportedProjects), "project", "projects"), plural(len(r.SkippedProjects), "project", "projects"))
		})
	})
}

func contextSummary(c engine.Context) string {
	active := ""
	if c.Active {
		active = " (active)"
	}
	return fmt.Sprintf("context %s%s: %s, workspaces in %s.", c.Name, active, plural(len(c.Projects), "project", "projects"), c.WorkspacesRoot)
}
