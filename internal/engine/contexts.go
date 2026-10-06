// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// ListContexts returns every registered context with its active flag.
func (e *Engine) ListContexts(ctx context.Context) ([]ContextSummary, error) {
	deps := e.appDeps(nil)
	names, err := app.ListContexts(ctx, deps)
	if err != nil {
		return nil, wrap(err)
	}
	active, err := e.activeContext(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ContextSummary, 0, len(names))
	for _, n := range names {
		c, err := app.LoadContext(ctx, deps, n)
		if err != nil {
			return nil, wrap(err)
		}
		out = append(out, ContextSummary{
			Name:           string(c.Name),
			Active:         c.Name == active,
			WorkspacesRoot: string(c.WorkspacesRoot),
			ProjectsRoot:   string(c.ProjectsRoot),
			ProjectCount:   len(c.Projects),
		})
	}
	return out, nil
}

// ContextRef names one context; an empty Name means the active context.
type ContextRef struct {
	Name string `json:"name,omitempty"`
}

// GetContext returns one full context record.
func (e *Engine) GetContext(ctx context.Context, p ContextRef) (Context, error) {
	c, err := e.resolveContext(ctx, p.Name)
	if err != nil {
		return Context{}, err
	}
	active, err := e.activeContext(ctx)
	if err != nil {
		return Context{}, err
	}
	out := contextToDTO(c, active)
	for i, pr := range c.Projects {
		out.Projects[i].IsNode, out.Projects[i].HasNodeModules = app.NodeProjectInfo(e.deps.FS, pr.SourceDir)
	}
	return out, nil
}

// CreateContextParams are contexts.create's parameters. Activate makes
// the new context the active one.
type CreateContextParams struct {
	Name                string   `json:"name"`
	WorkspacesRoot      string   `json:"workspacesRoot"`
	ProjectsRoot        string   `json:"projectsRoot,omitempty"`
	IgnorePatterns      []string `json:"ignorePatterns,omitempty"`
	IncludePatterns     []string `json:"includePatterns,omitempty"`
	ProjectScanMaxDepth int      `json:"projectScanMaxDepth,omitempty"`
	Defaults            *Options `json:"defaults,omitempty"`
	Activate            bool     `json:"activate,omitempty"`
}

// CreateContext creates a new context from explicit values (no projects;
// register them with RegisterProject).
func (e *Engine) CreateContext(ctx context.Context, p CreateContextParams) (Context, error) {
	c, err := contextFromParams(p.Name, p.WorkspacesRoot, p.ProjectsRoot, p.IgnorePatterns, p.ProjectScanMaxDepth, p.Defaults)
	if err != nil {
		return Context{}, err
	}
	if c.IncludePatterns, err = includeFromParams(p.IncludePatterns); err != nil {
		return Context{}, err
	}
	created, err := app.CreateContext(ctx, e.deps.Store, c)
	if err != nil {
		return Context{}, wrap(err)
	}
	if p.Activate {
		if err := app.SwitchContext(ctx, e.deps.Store, created.Name); err != nil {
			return Context{}, wrap(err)
		}
	}
	active, err := e.activeContext(ctx)
	if err != nil {
		return Context{}, err
	}
	return contextToDTO(created, active), nil
}

// UpdateContextParams are contexts.update's parameters: Name selects the
// context; every other non-nil field replaces the stored value (NewName
// renames it, re-pointing the active context when needed).
type UpdateContextParams struct {
	Name                string    `json:"name"`
	NewName             *string   `json:"newName,omitempty"`
	WorkspacesRoot      *string   `json:"workspacesRoot,omitempty"`
	ProjectsRoot        *string   `json:"projectsRoot,omitempty"`
	IgnorePatterns      *[]string `json:"ignorePatterns,omitempty"`
	IncludePatterns     *[]string `json:"includePatterns,omitempty"`
	ProjectScanMaxDepth *int      `json:"projectScanMaxDepth,omitempty"`
	Defaults            *Options  `json:"defaults,omitempty"`
}

// UpdateContext edits (and optionally renames) a context.
func (e *Engine) UpdateContext(ctx context.Context, p UpdateContextParams) (Context, error) {
	name, err := domain.NewContextName(p.Name)
	if err != nil {
		return Context{}, invalidParam("name", err)
	}
	existing, err := app.LoadContext(ctx, e.appDeps(nil), name)
	if err != nil {
		return Context{}, wrap(err)
	}

	newName := string(existing.Name)
	if p.NewName != nil {
		newName = *p.NewName
	}
	workspacesRoot := string(existing.WorkspacesRoot)
	if p.WorkspacesRoot != nil {
		workspacesRoot = *p.WorkspacesRoot
	}
	projectsRoot := string(existing.ProjectsRoot)
	if p.ProjectsRoot != nil {
		projectsRoot = *p.ProjectsRoot
	}
	ignore := make([]string, 0, len(existing.IgnorePatterns))
	for _, g := range existing.IgnorePatterns {
		ignore = append(ignore, string(g))
	}
	if p.IgnorePatterns != nil {
		ignore = *p.IgnorePatterns
	}
	depth := existing.ProjectScanMaxDepth
	if p.ProjectScanMaxDepth != nil {
		depth = *p.ProjectScanMaxDepth
	}

	c, err := contextFromParams(newName, workspacesRoot, projectsRoot, ignore, depth, nil)
	if err != nil {
		return Context{}, err
	}
	c.Defaults = existing.Defaults
	c.IncludePatterns = existing.IncludePatterns
	if p.IncludePatterns != nil {
		if c.IncludePatterns, err = includeFromParams(*p.IncludePatterns); err != nil {
			return Context{}, err
		}
	}
	if p.Defaults != nil {
		if c.Defaults, err = p.Defaults.toDomain("defaults"); err != nil {
			return Context{}, err
		}
	}

	updated, re, err := app.UpdateContext(ctx, e.deps.Store, e.deps.FS, name, c)
	if err != nil {
		return Context{}, wrap(err)
	}
	active, err := e.activeContext(ctx)
	if err != nil {
		return Context{}, err
	}
	out := contextToDTO(updated, active)
	for _, w := range re.Reassigned {
		out.ReassignedWorkspaces = append(out.ReassignedWorkspaces, w.Name)
	}
	for _, f := range re.Failures {
		out.ReassignFailures = append(out.ReassignFailures, ReassignFailure{Name: f.Name, Root: string(f.Root), Message: AsError(f.Err).Message})
	}
	return out, nil
}

// DeleteContextParams are contexts.delete's parameters. AllowActive lifts
// the active-context guard: the active context (even the only one) is
// deleted and the active context is cleared.
type DeleteContextParams struct {
	Name        string `json:"name"`
	AllowActive bool   `json:"allowActive,omitempty"`
}

// DeleteContext removes a context (refused, as conflict, for the active
// one unless AllowActive). It never touches any workspace or repository on
// disk.
func (e *Engine) DeleteContext(ctx context.Context, p DeleteContextParams) error {
	name, err := domain.NewContextName(p.Name)
	if err != nil {
		return invalidParam("name", err)
	}
	return wrap(app.RemoveContextWith(ctx, e.deps.Store, name, app.RemoveContextOptions{AllowActive: p.AllowActive}))
}

// SwitchContext makes a context the active one.
func (e *Engine) SwitchContext(ctx context.Context, p ContextRef) (ContextSummary, error) {
	name, err := domain.NewContextName(p.Name)
	if err != nil {
		return ContextSummary{}, invalidParam("name", err)
	}
	if err := app.SwitchContext(ctx, e.deps.Store, name); err != nil {
		return ContextSummary{}, wrap(err)
	}
	c, err := app.LoadContext(ctx, e.appDeps(nil), name)
	if err != nil {
		return ContextSummary{}, wrap(err)
	}
	return ContextSummary{Name: string(c.Name), Active: true, WorkspacesRoot: string(c.WorkspacesRoot), ProjectsRoot: string(c.ProjectsRoot), ProjectCount: len(c.Projects)}, nil
}

// ImportLegacyParams are contexts.importLegacy's parameters. From forces a
// source file (otherwise the global and folder-level legacy locations are
// discovered; finding both is reported as invalid_params). Exactly one of
// Name (create a new context) or Into (merge into an existing one) must be
// set. When merging, Confirm=false is a dry run: the result describes the
// change and nothing is written.
type ImportLegacyParams struct {
	From    string `json:"from,omitempty"`
	Name    string `json:"name,omitempty"`
	Into    string `json:"into,omitempty"`
	Confirm bool   `json:"confirm,omitempty"`
}

// ImportSkipped is one legacy project that could not be registered.
type ImportSkipped struct {
	Name    string `json:"name"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// ImportIssue is one legacy key that needs attention.
type ImportIssue struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// ImportFieldChange is one option a merge would change.
type ImportFieldChange struct {
	Field    string `json:"field"`
	Current  string `json:"current"`
	Incoming string `json:"incoming"`
}

// ImportLegacyResult is contexts.importLegacy's result. Applied is false
// for a merge dry run (Confirm=false).
type ImportLegacyResult struct {
	SourcePath       string              `json:"sourcePath"`
	Context          string              `json:"context"`
	Created          bool                `json:"created"`
	Applied          bool                `json:"applied"`
	ImportedProjects []string            `json:"importedProjects"`
	SkippedProjects  []ImportSkipped     `json:"skippedProjects"`
	UnmappedKeys     []ImportIssue       `json:"unmappedKeys"`
	FieldIssues      []ImportIssue       `json:"fieldIssues"`
	FieldChanges     []ImportFieldChange `json:"fieldChanges"`
	ProjectsKept     []string            `json:"projectsKept"`
	Summary          []string            `json:"summary"`
	// AdoptedWorkspaces and SkippedWorkspaces report the legacy workspaces
	// adopted right after the context was written (both empty when nothing
	// was written).
	AdoptedWorkspaces []AdoptedWorkspace `json:"adoptedWorkspaces"`
	SkippedWorkspaces []SkippedWorkspace `json:"skippedWorkspaces"`
	// ClaimedWorkspaces reports the orphaned workspaces (owner context no
	// longer exists) under the written context's root that were claimed
	// for it (additive, v1; empty when nothing was written).
	ClaimedWorkspaces []ClaimedWorkspace `json:"claimedWorkspaces"`
}

// ImportLegacyContext converts a legacy flat configuration file into a
// context without prompting.
func (e *Engine) ImportLegacyContext(ctx context.Context, p ImportLegacyParams) (ImportLegacyResult, error) {
	if (p.Name == "") == (p.Into == "") {
		return ImportLegacyResult{}, newError(CodeInvalidParams, messages.EngineInvalidParam, "name|into", "exactly one of name or into is required")
	}
	in := app.ImportLegacyContextInput{}
	if p.From != "" {
		from, err := domain.NewPath(p.From)
		if err != nil {
			return ImportLegacyResult{}, invalidParam("from", err)
		}
		in.From = from
	}
	confirm := true
	if p.Into != "" {
		into, err := domain.NewContextName(p.Into)
		if err != nil {
			return ImportLegacyResult{}, invalidParam("into", err)
		}
		in.TargetName = into
		confirm = p.Confirm
	} else {
		name, err := domain.NewContextName(p.Name)
		if err != nil {
			return ImportLegacyResult{}, invalidParam("name", err)
		}
		if _, err := app.LoadContext(ctx, e.appDeps(nil), name); err == nil {
			return ImportLegacyResult{}, AsError(domain.NewOpError("context.import", domain.CodeContextExists, string(name), "", nil))
		}
		in.NewName = string(name)
	}

	if in.From != "" {
		if ok, _ := e.deps.FS.Exists(in.From); !ok {
			return ImportLegacyResult{}, AsError(domain.NewOpError("context.import", domain.CodeLegacyConfigNotFound, string(in.From), "", nil))
		}
	}

	res, err := app.ImportLegacyContext(ctx, app.ImportLegacyContextDeps{
		Store:    e.deps.Store,
		FS:       e.deps.FS,
		Git:      e.deps.Git,
		Prompter: nonInteractivePrompter{confirm: confirm},
		Reporter: progressReporter{},
	}, in)
	if err != nil {
		return ImportLegacyResult{}, wrap(err)
	}
	return importResultToDTO(res), nil
}

func importResultToDTO(r app.ImportLegacyContextResult) ImportLegacyResult {
	out := ImportLegacyResult{
		SourcePath:       string(r.SourcePath),
		Context:          string(r.ContextName),
		Created:          r.Created,
		Applied:          !r.Cancelled,
		ImportedProjects: append([]string{}, r.ImportedProjects...),
		SkippedProjects:  []ImportSkipped{},
		UnmappedKeys:     issuesToDTO(r.UnmappedKeys),
		FieldIssues:      issuesToDTO(r.FieldIssues),
		FieldChanges:     []ImportFieldChange{},
		ProjectsKept:     append([]string{}, r.ProjectsKept...),
		Summary:          app.RenderImportSummary(r),
	}
	adoption := adoptionToDTO(r.Adoption)
	out.AdoptedWorkspaces = adoption.Adopted
	out.SkippedWorkspaces = adoption.Skipped
	out.ClaimedWorkspaces = claimToDTO(r.Claim).Claimed
	for _, s := range r.SkippedProjects {
		msg := messages.T(s.Reason)
		if s.Reason == messages.ImportProjectGitCheckFailed {
			msg = messages.T(s.Reason, s.Detail)
		}
		out.SkippedProjects = append(out.SkippedProjects, ImportSkipped{Name: s.Name, Reason: string(s.Reason), Message: msg})
	}
	for _, c := range r.FieldChanges {
		out.FieldChanges = append(out.FieldChanges, ImportFieldChange{Field: c.Field, Current: c.Current, Incoming: c.Incoming})
	}
	return out
}

func issuesToDTO(in []domain.LegacyIssue) []ImportIssue {
	out := make([]ImportIssue, 0, len(in))
	for _, i := range in {
		out = append(out, ImportIssue{Key: i.Key, Reason: string(i.Reason), Detail: i.Detail})
	}
	return out
}

// contextFromParams builds and validates a domain.Context from wire values.
func contextFromParams(name, workspacesRoot, projectsRoot string, ignore []string, depth int, defaults *Options) (domain.Context, error) {
	cn, err := domain.NewContextName(name)
	if err != nil {
		return domain.Context{}, invalidParam("name", err)
	}
	wr, err := domain.NewPath(workspacesRoot)
	if err != nil {
		return domain.Context{}, invalidParam("workspacesRoot", err)
	}
	var pr domain.Path
	if projectsRoot != "" {
		if pr, err = domain.NewPath(projectsRoot); err != nil {
			return domain.Context{}, invalidParam("projectsRoot", err)
		}
	}
	if depth < 0 {
		return domain.Context{}, newError(CodeInvalidParams, messages.EngineInvalidParam, "projectScanMaxDepth", "must not be negative")
	}
	opts, err := defaults.toDomain("defaults")
	if err != nil {
		return domain.Context{}, err
	}
	c := domain.Context{Name: cn, WorkspacesRoot: wr, ProjectsRoot: pr, ProjectScanMaxDepth: depth, Defaults: opts}
	for _, g := range ignore {
		c.IgnorePatterns = append(c.IgnorePatterns, domain.Glob(g))
	}
	return c, nil
}

// includeFromParams validates include patterns (path.Match syntax, like
// ignore patterns) as the includePatterns parameter.
func includeFromParams(patterns []string) ([]domain.Glob, error) {
	var out []domain.Glob
	for _, g := range patterns {
		out = append(out, domain.Glob(g))
	}
	if _, err := domain.MatchesIncludePattern(out, ""); err != nil {
		return nil, invalidParam("includePatterns", err)
	}
	return out, nil
}
