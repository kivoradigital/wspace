// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// ScanProjectsParams are projects.scan's parameters. Every field is
// optional: Roots defaults to the context's projectsRoot, Ignore to its
// ignorePatterns (an explicit empty list ignores nothing), Depth to its
// projectScanMaxDepth (then the built-in default).
type ScanProjectsParams struct {
	Context string    `json:"context,omitempty"`
	Roots   []string  `json:"roots,omitempty"`
	Depth   int       `json:"depth,omitempty"`
	Ignore  *[]string `json:"ignore,omitempty"`
	Include *[]string `json:"include,omitempty"`
}

// ScanCandidate is one main clone found by a scan.
type ScanCandidate struct {
	Path          string `json:"path"`
	Root          string `json:"root"`
	RelativePath  string `json:"relativePath"`
	SuggestedKey  string `json:"suggestedKey"`
	Registered    bool   `json:"registered"`
	RegisteredKey string `json:"registeredKey,omitempty"`
}

// ScanProjectsResult is projects.scan's result.
type ScanProjectsResult struct {
	Candidates      []ScanCandidate `json:"candidates"`
	LinkedWorktrees []string        `json:"linkedWorktrees"`
	Truncated       bool            `json:"truncated"`
	// TruncatedDirs names the folders at the depth cap whose subfolders
	// were not searched (at most maxTruncatedDirs; TruncatedCount is the
	// full number). MaxDepth is the cap the scan used.
	TruncatedDirs  []string `json:"truncatedDirs"`
	TruncatedCount int      `json:"truncatedCount"`
	MaxDepth       int      `json:"maxDepth"`
}

// maxTruncatedDirs bounds TruncatedDirs on the wire.
const maxTruncatedDirs = 20

// ScanProjects discovers candidate repositories without registering any.
func (e *Engine) ScanProjects(ctx context.Context, p ScanProjectsParams) (ScanProjectsResult, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return ScanProjectsResult{}, err
	}
	in := app.ScanProjectsInput{Ignore: c.IgnorePatterns, Include: c.IncludePatterns, MaxDepth: c.ProjectScanMaxDepth, Registered: c.Projects}
	if p.Depth < 0 {
		return ScanProjectsResult{}, invalidParam("depth", errNegative)
	}
	if p.Depth > 0 {
		in.MaxDepth = p.Depth
	}
	if p.Ignore != nil {
		in.Ignore = nil
		for _, g := range *p.Ignore {
			in.Ignore = append(in.Ignore, domain.Glob(g))
		}
	}
	if p.Include != nil {
		in.Include = nil
		for _, g := range *p.Include {
			in.Include = append(in.Include, domain.Glob(g))
		}
		if _, err := domain.MatchesIncludePattern(in.Include, ""); err != nil {
			return ScanProjectsResult{}, invalidParam("include", err)
		}
	}
	for _, r := range p.Roots {
		root, err := domain.NewPath(r)
		if err != nil {
			return ScanProjectsResult{}, invalidParam("roots", err)
		}
		in.Roots = append(in.Roots, root)
	}
	if len(in.Roots) == 0 && c.ProjectsRoot != "" {
		in.Roots = []domain.Path{c.ProjectsRoot}
	}

	res, err := app.ScanProjects(ctx, e.appDeps(nil), in)
	if err != nil {
		return ScanProjectsResult{}, wrap(err)
	}
	out := ScanProjectsResult{
		Candidates: []ScanCandidate{}, LinkedWorktrees: []string{}, Truncated: res.Truncated,
		TruncatedDirs: []string{}, TruncatedCount: len(res.TruncatedDirs), MaxDepth: res.MaxDepth,
	}
	for i, d := range res.TruncatedDirs {
		if i == maxTruncatedDirs {
			break
		}
		out.TruncatedDirs = append(out.TruncatedDirs, string(d))
	}
	for _, cand := range res.Candidates {
		out.Candidates = append(out.Candidates, ScanCandidate{
			Path:          string(cand.Path),
			Root:          string(cand.Root),
			RelativePath:  cand.RelativePath,
			SuggestedKey:  cand.SuggestedKey,
			Registered:    cand.Registered,
			RegisteredKey: string(cand.RegisteredKey),
		})
	}
	for _, l := range res.LinkedWorktrees {
		out.LinkedWorktrees = append(out.LinkedWorktrees, string(l))
	}
	return out, nil
}

// ProjectsRef selects a context for projects.list.
type ProjectsRef struct {
	Context string `json:"context,omitempty"`
}

// ListProjects returns a context's registered projects.
func (e *Engine) ListProjects(ctx context.Context, p ProjectsRef) ([]Project, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(c.Projects))
	for _, pr := range c.Projects {
		out = append(out, e.projectWithNodeInfo(pr))
	}
	return out, nil
}

// RegisterProjectParams are projects.register's parameters.
type RegisterProjectParams struct {
	Context      string `json:"context,omitempty"`
	Key          string `json:"key"`
	SourceDir    string `json:"sourceDir"`
	OriginBranch string `json:"originBranch,omitempty"`
	DestBranch   string `json:"destBranch,omitempty"`
	WorktreeDir  string `json:"worktreeDir,omitempty"`
}

// RegisterProject adds one project (a main clone) to a context.
func (e *Engine) RegisterProject(ctx context.Context, p RegisterProjectParams) (Project, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return Project{}, err
	}
	key, err := domain.NewProjectKey(p.Key)
	if err != nil {
		return Project{}, invalidParam("key", err)
	}
	src, err := domain.NewPath(p.SourceDir)
	if err != nil {
		return Project{}, invalidParam("sourceDir", err)
	}
	rec := domain.Project{Key: key, SourceDir: src, DestBranch: domain.BranchTemplate(p.DestBranch), WorktreeDir: domain.PathTemplate(p.WorktreeDir)}
	if p.OriginBranch != "" {
		b, err := domain.NewBranchName(p.OriginBranch)
		if err != nil {
			return Project{}, invalidParam("originBranch", err)
		}
		rec.OriginBranch = &b
	}
	if _, err := app.RegisterProject(ctx, e.appDeps(nil), c.Name, rec); err != nil {
		return Project{}, wrap(err)
	}
	return projectToDTO(rec), nil
}

// UpdateProjectParams are projects.update's parameters; nil fields are
// kept, an empty string restores inheritance.
type UpdateProjectParams struct {
	Context      string  `json:"context,omitempty"`
	Key          string  `json:"key"`
	SourceDir    *string `json:"sourceDir,omitempty"`
	OriginBranch *string `json:"originBranch,omitempty"`
	DestBranch   *string `json:"destBranch,omitempty"`
	WorktreeDir  *string `json:"worktreeDir,omitempty"`
}

// UpdateProject edits one registered project.
func (e *Engine) UpdateProject(ctx context.Context, p UpdateProjectParams) (Project, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return Project{}, err
	}
	patch := app.ProjectPatch{OriginBranch: p.OriginBranch, DestBranch: p.DestBranch, WorktreeDir: p.WorktreeDir}
	if p.SourceDir != nil {
		src, err := domain.NewPath(*p.SourceDir)
		if err != nil {
			return Project{}, invalidParam("sourceDir", err)
		}
		patch.SourceDir = &src
	}
	updated, err := app.UpdateProject(ctx, e.appDeps(nil), c.Name, domain.ProjectKey(p.Key), patch)
	if err != nil {
		return Project{}, wrap(err)
	}
	for _, pr := range updated.Projects {
		if string(pr.Key) == p.Key {
			return projectToDTO(pr), nil
		}
	}
	return Project{}, nil
}

// ProjectRef names one project in a context.
type ProjectRef struct {
	Context string `json:"context,omitempty"`
	Key     string `json:"key"`
}

// GetProject returns one registered project (not_found when the context
// has no project with that key).
func (e *Engine) GetProject(ctx context.Context, p ProjectRef) (Project, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return Project{}, err
	}
	for _, pr := range c.Projects {
		if string(pr.Key) == p.Key {
			return projectToDTO(pr), nil
		}
	}
	return Project{}, AsError(domain.NewOpError("project.get", domain.CodeProjectNotFound, p.Key, "", nil))
}

// RemoveProject unregisters one project; repositories and existing
// workspaces are never touched.
func (e *Engine) RemoveProject(ctx context.Context, p ProjectRef) error {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return err
	}
	_, err = app.RemoveProject(ctx, e.appDeps(nil), c.Name, domain.ProjectKey(p.Key))
	return wrap(err)
}

// ProjectToRegister is one entry of projects.registerMany.
type ProjectToRegister struct {
	Key       string `json:"key"`
	SourceDir string `json:"sourceDir"`
}

// RegisterProjectsParams are projects.registerMany's parameters.
type RegisterProjectsParams struct {
	Context  string              `json:"context,omitempty"`
	Projects []ProjectToRegister `json:"projects"`
}

// RegisterProblem is one reason projects[index] cannot be registered.
type RegisterProblem struct {
	Index     int    `json:"index"`
	Key       string `json:"key"`
	SourceDir string `json:"sourceDir"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
}

// RegisterProjectsResult is projects.registerMany's result: either every
// project registered (Problems empty) or none (Registered empty).
type RegisterProjectsResult struct {
	Registered []Project         `json:"registered"`
	Problems   []RegisterProblem `json:"problems"`
}

// RegisterProjects registers a batch all-or-nothing (app.RegisterProjects):
// when any project has a problem nothing is written and every problem is
// reported with a stable reason and a message naming the project.
func (e *Engine) RegisterProjects(ctx context.Context, p RegisterProjectsParams) (RegisterProjectsResult, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return RegisterProjectsResult{}, err
	}
	recs := make([]domain.Project, len(p.Projects))
	for i, pr := range p.Projects {
		recs[i] = domain.Project{Key: domain.ProjectKey(pr.Key), SourceDir: domain.Path(pr.SourceDir)}
	}
	problems, err := app.RegisterProjects(ctx, e.appDeps(nil), c.Name, recs)
	if err != nil {
		return RegisterProjectsResult{}, wrap(err)
	}
	out := RegisterProjectsResult{Registered: []Project{}, Problems: []RegisterProblem{}}
	for _, pr := range problems {
		src := p.Projects[pr.Index]
		out.Problems = append(out.Problems, RegisterProblem{
			Index: pr.Index, Key: src.Key, SourceDir: src.SourceDir, Reason: string(pr.Reason),
			Message: registerProblemMessage(pr, src),
		})
	}
	if len(problems) == 0 {
		for _, r := range recs {
			out.Registered = append(out.Registered, projectToDTO(r))
		}
	}
	return out, nil
}

func registerProblemMessage(pr app.RegisterProblem, src ProjectToRegister) string {
	switch pr.Reason {
	case app.ProblemInvalidKey:
		return messages.T(messages.EngineRegisterInvalidKey, src.Key, pr.Detail)
	case app.ProblemKeyTaken:
		return messages.T(messages.EngineRegisterKeyTaken, src.Key)
	case app.ProblemKeyDuplicate:
		return messages.T(messages.EngineRegisterKeyDuplicate, src.Key, pr.Other)
	case app.ProblemSourceInvalid:
		return messages.T(messages.EngineRegisterSourceInvalid, src.SourceDir)
	case app.ProblemSourceRegistered:
		return messages.T(messages.EngineRegisterSourceRegistered, src.SourceDir, pr.Other)
	case app.ProblemSourceDuplicate:
		return messages.T(messages.EngineRegisterSourceDuplicate, src.SourceDir)
	case app.ProblemNotAMainClone:
		return messages.T(messages.EngineRegisterNotAMainClone, src.SourceDir)
	default:
		return messages.T(messages.EngineRegisterGitFailed, src.SourceDir)
	}
}
