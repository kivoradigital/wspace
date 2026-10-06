// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/kivoradigital/wspace/internal/domain"
	"gopkg.in/yaml.v3"
)

// contextYAML mirrors "<config>/ws/contexts/<name>/config.yaml"
// (design.md §5). The context's Name is its directory name, not a field
// inside the file.
type contextYAML struct {
	SchemaVersion  int         `yaml:"schema_version"`
	WorkspacesRoot string      `yaml:"workspaces_root"`
	ProjectsRoot   string      `yaml:"projects_root,omitempty"`
	Defaults       optionsYAML `yaml:"defaults"`
	IgnorePatterns []string    `yaml:"ignore_patterns,omitempty"`
	// IncludePatterns mirrors domain.Context.IncludePatterns; omitempty
	// keeps files written before it existed byte-identical.
	IncludePatterns []string      `yaml:"include_patterns,omitempty"`
	Projects        []projectYAML `yaml:"projects,omitempty"`
	// ProjectScanMaxDepth mirrors domain.Context.ProjectScanMaxDepth
	// (recursive project-discovery scan's own depth cap). omitempty: a
	// context that never set it round-trips with no key in the file at
	// all, resolved to domain.DefaultProjectScanMaxDepth at scan time,
	// exactly like an ordinary zero value would.
	ProjectScanMaxDepth int `yaml:"project_scan_max_depth,omitempty"`
}

// projectYAML mirrors one entry of a context's "projects" list
// (project-configuration spec: "Explicit per-project record").
type projectYAML struct {
	Key          string      `yaml:"key"`
	SourceDir    string      `yaml:"source_dir"`
	OriginBranch string      `yaml:"origin_branch,omitempty"`
	DestBranch   string      `yaml:"dest_branch,omitempty"`
	WorktreeDir  string      `yaml:"worktree_dir,omitempty"`
	Options      optionsYAML `yaml:"options,omitempty"`
}

func (dto contextYAML) toDomain(name domain.ContextName) domain.Context {
	patterns := make([]domain.Glob, len(dto.IgnorePatterns))
	for i, p := range dto.IgnorePatterns {
		patterns[i] = domain.Glob(p)
	}
	projects := make([]domain.Project, len(dto.Projects))
	for i, p := range dto.Projects {
		projects[i] = p.toDomain()
	}
	var include []domain.Glob
	for _, p := range dto.IncludePatterns {
		include = append(include, domain.Glob(p))
	}
	return domain.Context{
		IncludePatterns:     include,
		Name:                name,
		WorkspacesRoot:      domain.Path(dto.WorkspacesRoot),
		ProjectsRoot:        domain.Path(dto.ProjectsRoot),
		Defaults:            dto.Defaults.toDomain(),
		IgnorePatterns:      patterns,
		Projects:            projects,
		ProjectScanMaxDepth: dto.ProjectScanMaxDepth,
	}
}

func fromDomainContext(c domain.Context) contextYAML {
	patterns := make([]string, len(c.IgnorePatterns))
	for i, p := range c.IgnorePatterns {
		patterns[i] = string(p)
	}
	projects := make([]projectYAML, len(c.Projects))
	for i, p := range c.Projects {
		projects[i] = fromDomainProject(p)
	}
	var include []string
	for _, p := range c.IncludePatterns {
		include = append(include, string(p))
	}
	return contextYAML{
		IncludePatterns:     include,
		SchemaVersion:       currentSchemaVersion,
		WorkspacesRoot:      string(c.WorkspacesRoot),
		ProjectsRoot:        string(c.ProjectsRoot),
		Defaults:            fromDomainOptions(c.Defaults),
		IgnorePatterns:      patterns,
		Projects:            projects,
		ProjectScanMaxDepth: c.ProjectScanMaxDepth,
	}
}

func (dto projectYAML) toDomain() domain.Project {
	var originBranch *domain.BranchName
	if dto.OriginBranch != "" {
		bn := domain.BranchName(dto.OriginBranch)
		originBranch = &bn
	}
	return domain.Project{
		Key:          domain.ProjectKey(dto.Key),
		SourceDir:    domain.Path(dto.SourceDir),
		OriginBranch: originBranch,
		DestBranch:   domain.BranchTemplate(dto.DestBranch),
		WorktreeDir:  domain.PathTemplate(dto.WorktreeDir),
		Options:      dto.Options.toDomain(),
	}
}

func fromDomainProject(p domain.Project) projectYAML {
	origin := ""
	if p.OriginBranch != nil {
		origin = string(*p.OriginBranch)
	}
	return projectYAML{
		Key:          string(p.Key),
		SourceDir:    string(p.SourceDir),
		OriginBranch: origin,
		DestBranch:   string(p.DestBranch),
		WorktreeDir:  string(p.WorktreeDir),
		Options:      fromDomainOptions(p.Options),
	}
}

func (a *Adapter) contextsDir() string {
	return filepath.Join(filepath.FromSlash(string(a.root)), "contexts")
}

func (a *Adapter) contextConfigPath(name domain.ContextName) string {
	return filepath.Join(a.contextsDir(), string(name), "config.yaml")
}

// ListContexts returns every saved context name, sorted. Before any
// context has been saved, it returns an empty slice, not an error
// (design.md §5: "Root config exists, zero contexts").
func (a *Adapter) ListContexts(_ context.Context) ([]domain.ContextName, error) {
	entries, err := os.ReadDir(a.contextsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("config.list_contexts: %w", err)
	}
	var names []domain.ContextName
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, domain.ContextName(e.Name()))
		}
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names, nil
}

// LoadContext reads "<root>/contexts/<name>/config.yaml".
func (a *Adapter) LoadContext(_ context.Context, name domain.ContextName) (domain.Context, error) {
	data, err := os.ReadFile(a.contextConfigPath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return domain.Context{}, domain.NewOpError("config.load_context", domain.CodeContextNotFound, string(name), "", nil)
		}
		return domain.Context{}, fmt.Errorf("config.load_context: %w", err)
	}

	var dto contextYAML
	if err := strictDecode(data, &dto); err != nil {
		return domain.Context{}, fmt.Errorf("config.load_context: %w", err)
	}
	if err := checkSchemaVersion("config.load_context", &dto.SchemaVersion); err != nil {
		return domain.Context{}, err
	}
	return dto.toDomain(name), nil
}

// SaveContext atomically writes c to
// "<root>/contexts/<c.Name>/config.yaml".
func (a *Adapter) SaveContext(_ context.Context, c domain.Context) error {
	dto := fromDomainContext(c)
	data, err := yaml.Marshal(dto)
	if err != nil {
		return fmt.Errorf("config.save_context: %w", err)
	}
	if err := atomicWrite(a.contextConfigPath(c.Name), data); err != nil {
		return fmt.Errorf("config.save_context: %w", err)
	}
	return nil
}

// DeleteContext removes "<root>/contexts/<name>" entirely.
func (a *Adapter) DeleteContext(_ context.Context, name domain.ContextName) error {
	if err := os.RemoveAll(filepath.Join(a.contextsDir(), string(name))); err != nil {
		return fmt.Errorf("config.delete_context: %w", err)
	}
	return nil
}
