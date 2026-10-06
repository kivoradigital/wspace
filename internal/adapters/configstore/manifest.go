// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"gopkg.in/yaml.v3"
)

// manifestYAML mirrors "<workspace>/.wspace/workspace.yaml" (design.md §5).
type manifestYAML struct {
	SchemaVersion int           `yaml:"schema_version"`
	Workspace     workspaceYAML `yaml:"workspace"`
	EnvCopies     []string      `yaml:"env_copies,omitempty"`
}

type workspaceYAML struct {
	Name    string      `yaml:"name"`
	Context string      `yaml:"context"`
	Branch  string      `yaml:"branch"`
	Created time.Time   `yaml:"created"`
	Options optionsYAML `yaml:"options,omitempty"`
	Repos   []repoYAML  `yaml:"repos,omitempty"`
}

type repoYAML struct {
	Alias     string `yaml:"alias"`
	Project   string `yaml:"project"`
	SourceDir string `yaml:"source_dir"`
	Branch    string `yaml:"branch"`
	// BaseBranch is the repo's own comparison base; written only when
	// recorded (domain.RepoEntry.BaseBranch).
	BaseBranch string `yaml:"base_branch,omitempty"`
}

func fromDomainManifest(m domain.Manifest) manifestYAML {
	repos := make([]repoYAML, len(m.Workspace.Repos))
	for i, r := range m.Workspace.Repos {
		repos[i] = repoYAML{
			Alias:      r.Alias,
			Project:    string(r.Project),
			SourceDir:  string(r.SourceDir),
			Branch:     string(r.Branch),
			BaseBranch: string(r.BaseBranch),
		}
	}
	return manifestYAML{
		SchemaVersion: currentSchemaVersion,
		Workspace: workspaceYAML{
			Name:    m.Workspace.Name,
			Context: string(m.Workspace.Context),
			Branch:  string(m.Workspace.Branch),
			Created: m.Workspace.Created,
			Options: fromDomainOptions(m.Workspace.Options),
			Repos:   repos,
		},
		EnvCopies: m.EnvCopies,
	}
}

// toDomain converts dto into a domain.Manifest. root becomes
// Workspace.Root: it is the caller-supplied workspace directory, never a
// field persisted in the YAML itself.
func (dto manifestYAML) toDomain(root domain.Path) domain.Manifest {
	repos := make([]domain.RepoEntry, len(dto.Workspace.Repos))
	for i, r := range dto.Workspace.Repos {
		repos[i] = domain.RepoEntry{
			Alias:      r.Alias,
			Project:    domain.ProjectKey(r.Project),
			SourceDir:  domain.Path(r.SourceDir),
			Branch:     domain.BranchName(r.Branch),
			BaseBranch: domain.BranchName(r.BaseBranch),
		}
	}
	return domain.Manifest{
		SchemaVersion: dto.SchemaVersion,
		Workspace: domain.Workspace{
			Name:    dto.Workspace.Name,
			Root:    root,
			Context: domain.ContextName(dto.Workspace.Context),
			Branch:  domain.BranchName(dto.Workspace.Branch),
			Created: dto.Workspace.Created,
			Options: dto.Workspace.Options.toDomain(),
			Repos:   repos,
		},
		EnvCopies: dto.EnvCopies,
	}
}

func (a *Adapter) manifestPath(wsRoot domain.Path) string {
	return filepath.Join(filepath.FromSlash(string(wsRoot)), ".wspace", "workspace.yaml")
}

// LoadManifest reads "<wsRoot>/.wspace/workspace.yaml".
func (a *Adapter) LoadManifest(_ context.Context, wsRoot domain.Path) (domain.Manifest, error) {
	data, err := os.ReadFile(a.manifestPath(wsRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return domain.Manifest{}, domain.NewOpError("config.load_manifest", domain.CodeWorkspaceNotFound, string(wsRoot), "", nil)
		}
		return domain.Manifest{}, fmt.Errorf("config.load_manifest: %w", err)
	}

	var dto manifestYAML
	if err := strictDecode(data, &dto); err != nil {
		return domain.Manifest{}, fmt.Errorf("config.load_manifest: %w", err)
	}
	if err := checkSchemaVersion("config.load_manifest", &dto.SchemaVersion); err != nil {
		return domain.Manifest{}, err
	}
	return dto.toDomain(wsRoot), nil
}

// SaveManifest atomically writes m to "<wsRoot>/.wspace/workspace.yaml".
func (a *Adapter) SaveManifest(_ context.Context, wsRoot domain.Path, m domain.Manifest) error {
	dto := fromDomainManifest(m)
	data, err := yaml.Marshal(dto)
	if err != nil {
		return fmt.Errorf("config.save_manifest: %w", err)
	}
	if err := atomicWrite(a.manifestPath(wsRoot), data); err != nil {
		return fmt.Errorf("config.save_manifest: %w", err)
	}
	return nil
}

// ArchiveManifest copies the manifest currently at wsRoot into
// "<parent-of-wsRoot>/.ws-archive/<name>-<timestamp>.yaml" (design.md §5
// tree). It does not remove the live manifest or the workspace directory:
// that is the caller's responsibility (destroy), once archiving succeeds.
func (a *Adapter) ArchiveManifest(ctx context.Context, wsRoot domain.Path) error {
	m, err := a.LoadManifest(ctx, wsRoot)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(fromDomainManifest(m))
	if err != nil {
		return fmt.Errorf("config.archive_manifest: %w", err)
	}

	name := m.Workspace.Name
	if name == "" {
		name = filepath.Base(filepath.FromSlash(string(wsRoot)))
	}
	parent := filepath.Dir(filepath.FromSlash(string(wsRoot)))
	archivePath := filepath.Join(parent, ".ws-archive", fmt.Sprintf("%s-%s.yaml", name, a.timestamp()))

	if err := atomicWrite(archivePath, data); err != nil {
		return fmt.Errorf("config.archive_manifest: %w", err)
	}
	return nil
}

// timestamp formats a's clock as "20060102T150405Z" UTC (design.md §5
// sample: "payments-fix-20260921T120000Z.yaml").
func (a *Adapter) timestamp() string {
	return a.now().UTC().Format("20060102T150405") + "Z"
}

// FindWorkspaceRoot walks up from "from" looking for a directory
// containing ".wspace/workspace.yaml".
func (a *Adapter) FindWorkspaceRoot(_ context.Context, from domain.Path) (domain.Path, bool, error) {
	cur := filepath.Clean(filepath.FromSlash(string(from)))
	for {
		candidate := filepath.Join(cur, ".wspace", "workspace.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return domain.Path(filepath.ToSlash(cur)), true, nil
		} else if !os.IsNotExist(err) {
			return "", false, fmt.Errorf("config.find_workspace_root: %w", err)
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false, nil
		}
		cur = parent
	}
}
