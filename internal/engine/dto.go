// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"github.com/kivoradigital/wspace/internal/domain"
)

// Options mirrors domain.Options on the wire: a nil field means "unset at
// this layer". EnvPruneDirs is a pointer so an explicit empty list
// ("prune nothing") survives a round trip distinctly from "unset".
type Options struct {
	BaseBranch        *string   `json:"baseBranch,omitempty"`
	BranchPrefix      *string   `json:"branchPrefix,omitempty"`
	CopyEnv           *bool     `json:"copyEnv,omitempty"`
	FetchBeforeCreate *bool     `json:"fetchBeforeCreate,omitempty"`
	EnvPruneDirs      *[]string `json:"envPruneDirs,omitempty"`
	Remote            *string   `json:"remote,omitempty"`
}

// ContextSummary is one row of contexts.list.
type ContextSummary struct {
	Name           string `json:"name"`
	Active         bool   `json:"active"`
	WorkspacesRoot string `json:"workspacesRoot"`
	ProjectsRoot   string `json:"projectsRoot,omitempty"`
	ProjectCount   int    `json:"projectCount"`
}

// Context is a full context record.
type Context struct {
	Name                string    `json:"name"`
	Active              bool      `json:"active"`
	WorkspacesRoot      string    `json:"workspacesRoot"`
	ProjectsRoot        string    `json:"projectsRoot,omitempty"`
	IgnorePatterns      []string  `json:"ignorePatterns"`
	IncludePatterns     []string  `json:"includePatterns"`
	ProjectScanMaxDepth int       `json:"projectScanMaxDepth,omitempty"`
	Defaults            Options   `json:"defaults"`
	Projects            []Project `json:"projects"`
	// ReassignedWorkspaces names the workspaces a rename moved from the old
	// context name to the new one, and ReassignFailures each one whose
	// manifest could not be rewritten (it stays claimable with
	// workspaces.claim). Only contexts.update sets them (additive, v1;
	// omitted when empty).
	ReassignedWorkspaces []string          `json:"reassignedWorkspaces,omitempty"`
	ReassignFailures     []ReassignFailure `json:"reassignFailures,omitempty"`
}

// ReassignFailure is one workspace a context rename could not move to the
// new name.
type ReassignFailure struct {
	Name    string `json:"name,omitempty"`
	Root    string `json:"root"`
	Message string `json:"message"`
}

// Project is one registered project record.
type Project struct {
	Key          string  `json:"key"`
	SourceDir    string  `json:"sourceDir"`
	OriginBranch *string `json:"originBranch,omitempty"`
	DestBranch   string  `json:"destBranch,omitempty"`
	WorktreeDir  string  `json:"worktreeDir,omitempty"`
	Options      Options `json:"options"`
	// IsNode: a package.json at the main clone's root. HasNodeModules: it
	// also has node_modules, so create/add can offer copyNodeModules. Set
	// by projects.list and workspaces.addableProjects only.
	IsNode         bool `json:"isNode,omitempty"`
	HasNodeModules bool `json:"hasNodeModules,omitempty"`
}

// RepoEntry is one worktree recorded in a workspace manifest.
type RepoEntry struct {
	Alias     string `json:"alias"`
	Project   string `json:"project"`
	SourceDir string `json:"sourceDir"`
	Branch    string `json:"branch"`
}

// Workspace is a created workspace.
type Workspace struct {
	Name    string      `json:"name"`
	Path    string      `json:"path"`
	Context string      `json:"context"`
	Branch  string      `json:"branch"`
	Created string      `json:"created"` // RFC 3339, UTC
	Repos   []RepoEntry `json:"repos"`
}

// Change is one uncommitted path and its teardown-safety class:
// "tracked", "foreign" (untracked, not created by wspace) or "env_copy".
type Change struct {
	Path  string `json:"path"`
	Class string `json:"class"`
}

// RepoStatus is one repo's live status inside a workspace.
type RepoStatus struct {
	Alias string `json:"alias"`
	// Project is the context project key the repo was created from
	// (additive, v1).
	Project  string   `json:"project,omitempty"`
	Branch   string   `json:"branch"`
	Detached bool     `json:"detached"`
	Ahead    int      `json:"ahead"`
	Behind   int      `json:"behind"`
	Dirty    bool     `json:"dirty"`
	Changes  []Change `json:"changes"`
	// BaseBranch is the branch ahead was counted against when the branch
	// has no upstream; absent when the upstream was used (additive, v1).
	BaseBranch string `json:"baseBranch,omitempty"`
	// BaseMissing is true when no comparison base exists in this repo:
	// ahead/behind are then unknown and reported as 0, and BaseBranch names
	// the base that was expected (additive, v1; absent means false).
	BaseMissing bool `json:"baseMissing,omitempty"`
	// BaseBehind counts the comparison base's commits not yet in the
	// branch, against the last fetch (additive, v1); absent when unknown:
	// the branch has an upstream, the base is missing or the count failed.
	BaseBehind *int `json:"baseBehind,omitempty"`
}

// WorkspaceStatus is one workspace's live status. Error is set (and Repos
// empty) when the workspace could not be read (a damaged workspace in a
// listing never hides the healthy ones).
type WorkspaceStatus struct {
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	RepoCount int          `json:"repoCount"`
	Dirty     bool         `json:"dirty"`
	Repos     []RepoStatus `json:"repos"`
	Error     *ErrorBody   `json:"error,omitempty"`
	// Legacy is true for a workspace only the legacy bash tool manages
	// (no wspace manifest yet): it carries no live status until it is
	// adopted with workspaces.adoptLegacy. Omitted when false.
	Legacy bool `json:"legacy,omitempty"`
	// OrphanOf names the context the workspace's manifest says owns it
	// when that context no longer exists (deleted, or renamed by an older
	// version). Such a workspace is listed for every context sharing the
	// root and can be claimed with workspaces.claim (additive, v1; omitted
	// when empty).
	OrphanOf string `json:"orphanOf,omitempty"`
}

// ErrorBody is the wire shape of an *Error, shared by every surface.
type ErrorBody struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

// Body returns e's wire shape.
func (e *Error) Body() ErrorBody {
	return ErrorBody{Code: e.Code, Message: e.Message, Data: e.Data}
}

// Blocker is one reason an unforced teardown needs confirmation. Kind is
// "tracked_change", "foreign_file" (Path set) or "unpushed_commits" (Count
// set).
type Blocker struct {
	Repo    string `json:"repo"`
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Count   int    `json:"count,omitempty"`
	Message string `json:"message"`
}

func optionsToDTO(o domain.Options) Options {
	var out Options
	if o.BaseBranch != nil {
		s := string(*o.BaseBranch)
		out.BaseBranch = &s
	}
	out.BranchPrefix = o.BranchPrefix
	out.CopyEnv = o.CopyEnv
	out.FetchBeforeCreate = o.FetchBeforeCreate
	if o.EnvPruneDirs != nil {
		dirs := append([]string{}, o.EnvPruneDirs...)
		out.EnvPruneDirs = &dirs
	}
	out.Remote = o.Remote
	return out
}

// toDomain converts wire options, validating the base branch.
func (o *Options) toDomain(param string) (domain.Options, error) {
	var out domain.Options
	if o == nil {
		return out, nil
	}
	if o.BaseBranch != nil {
		b, err := domain.NewBranchName(*o.BaseBranch)
		if err != nil {
			return domain.Options{}, invalidParam(param+".baseBranch", err)
		}
		out.BaseBranch = &b
	}
	out.BranchPrefix = o.BranchPrefix
	out.CopyEnv = o.CopyEnv
	out.FetchBeforeCreate = o.FetchBeforeCreate
	if o.EnvPruneDirs != nil {
		out.EnvPruneDirs = append([]string{}, *o.EnvPruneDirs...)
	}
	out.Remote = o.Remote
	return out, nil
}

func projectToDTO(p domain.Project) Project {
	out := Project{
		Key:         string(p.Key),
		SourceDir:   string(p.SourceDir),
		DestBranch:  string(p.DestBranch),
		WorktreeDir: string(p.WorktreeDir),
		Options:     optionsToDTO(p.Options),
	}
	if p.OriginBranch != nil {
		s := string(*p.OriginBranch)
		out.OriginBranch = &s
	}
	return out
}

func contextToDTO(c domain.Context, active domain.ContextName) Context {
	out := Context{
		Name:                string(c.Name),
		Active:              c.Name == active,
		WorkspacesRoot:      string(c.WorkspacesRoot),
		ProjectsRoot:        string(c.ProjectsRoot),
		IgnorePatterns:      []string{},
		IncludePatterns:     []string{},
		ProjectScanMaxDepth: c.ProjectScanMaxDepth,
		Defaults:            optionsToDTO(c.Defaults),
		Projects:            []Project{},
	}
	for _, g := range c.IgnorePatterns {
		out.IgnorePatterns = append(out.IgnorePatterns, string(g))
	}
	for _, g := range c.IncludePatterns {
		out.IncludePatterns = append(out.IncludePatterns, string(g))
	}
	for _, p := range c.Projects {
		out.Projects = append(out.Projects, projectToDTO(p))
	}
	return out
}

func repoEntryToDTO(r domain.RepoEntry) RepoEntry {
	return RepoEntry{Alias: r.Alias, Project: string(r.Project), SourceDir: string(r.SourceDir), Branch: string(r.Branch)}
}

func changeClass(c domain.ChangeClass) string {
	switch c {
	case domain.ChangeTracked:
		return "tracked"
	case domain.ChangeForeign:
		return "foreign"
	default:
		return "env_copy"
	}
}

func repoStatusToDTO(r domain.RepoStatus) RepoStatus {
	out := RepoStatus{
		Alias:       r.Alias,
		Project:     string(r.Project),
		BaseBranch:  string(r.BaseBranch),
		BaseMissing: r.BaseMissing,
		BaseBehind:  r.BaseBehind,
		Branch:      string(r.Branch),
		Detached:    r.Detached,
		Ahead:       r.Ahead,
		Behind:      r.Behind,
		Dirty:       r.Dirty,
		Changes:     []Change{},
	}
	for _, ch := range r.Changes.Changes {
		out.Changes = append(out.Changes, Change{Path: ch.RelPath, Class: changeClass(ch.Class)})
	}
	return out
}
