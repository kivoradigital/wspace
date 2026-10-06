// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
)

// RegisterProject is the non-interactive counterpart of RunProjectWizard's
// registration step: it validates p (key, absolute source directory, a
// genuine main clone — the same "Main-clone detection" rule the wizard's
// manual path applies — and the optional branch/template fields), refuses
// a duplicate key or source directory, and appends it to the context.
func RegisterProject(ctx context.Context, deps Deps, contextName domain.ContextName, p domain.Project) (domain.Context, error) {
	c, err := deps.Store.LoadContext(ctx, contextName)
	if err != nil {
		return domain.Context{}, err
	}
	if err := validateProjectRecord(ctx, deps, p); err != nil {
		return domain.Context{}, err
	}
	for _, existing := range c.Projects {
		if existing.Key == p.Key {
			return domain.Context{}, domain.NewOpError("project.register", domain.CodeProjectExists, string(p.Key), "", nil)
		}
		if existing.SourceDir == p.SourceDir {
			return domain.Context{}, domain.NewOpError("project.register", domain.CodeProjectExists, string(p.SourceDir), "", nil)
		}
	}
	c.Projects = append(c.Projects, p)
	if err := deps.Store.SaveContext(ctx, c); err != nil {
		return domain.Context{}, err
	}
	return c, nil
}

// ProjectPatch carries the fields UpdateProject changes; a nil field is
// left untouched. An empty OriginBranch clears the override (inherit the
// resolved base branch); an empty DestBranch/WorktreeDir likewise restores
// inheritance, matching the wizard's "empty answer means inherit" rule.
type ProjectPatch struct {
	SourceDir    *domain.Path
	OriginBranch *string
	DestBranch   *string
	WorktreeDir  *string
}

// UpdateProject applies patch to the project registered under key and
// re-validates the result exactly like RegisterProject.
func UpdateProject(ctx context.Context, deps Deps, contextName domain.ContextName, key domain.ProjectKey, patch ProjectPatch) (domain.Context, error) {
	c, err := deps.Store.LoadContext(ctx, contextName)
	if err != nil {
		return domain.Context{}, err
	}
	p := findProject(&c, key)
	if p == nil {
		return domain.Context{}, domain.NewOpError("project.update", domain.CodeProjectNotFound, string(key), "", nil)
	}
	updated := *p
	if patch.SourceDir != nil {
		updated.SourceDir = *patch.SourceDir
	}
	if patch.OriginBranch != nil {
		if *patch.OriginBranch == "" {
			updated.OriginBranch = nil
		} else {
			b := domain.BranchName(*patch.OriginBranch)
			updated.OriginBranch = &b
		}
	}
	if patch.DestBranch != nil {
		updated.DestBranch = domain.BranchTemplate(*patch.DestBranch)
	}
	if patch.WorktreeDir != nil {
		updated.WorktreeDir = domain.PathTemplate(*patch.WorktreeDir)
	}
	if err := validateProjectRecord(ctx, deps, updated); err != nil {
		return domain.Context{}, err
	}
	*p = updated
	if err := deps.Store.SaveContext(ctx, c); err != nil {
		return domain.Context{}, err
	}
	return c, nil
}

// RemoveProject unregisters the project stored under key. It never touches
// the project's repository or any workspace that already mounts it.
func RemoveProject(ctx context.Context, deps Deps, contextName domain.ContextName, key domain.ProjectKey) (domain.Context, error) {
	c, err := deps.Store.LoadContext(ctx, contextName)
	if err != nil {
		return domain.Context{}, err
	}
	for i := range c.Projects {
		if c.Projects[i].Key == key {
			c.Projects = append(c.Projects[:i], c.Projects[i+1:]...)
			if err := deps.Store.SaveContext(ctx, c); err != nil {
				return domain.Context{}, err
			}
			return c, nil
		}
	}
	return domain.Context{}, domain.NewOpError("project.remove", domain.CodeProjectNotFound, string(key), "", nil)
}

// validateProjectRecord checks a project record against the same rules the
// project wizard applies field by field.
func validateProjectRecord(ctx context.Context, deps Deps, p domain.Project) error {
	if _, err := domain.NewProjectKey(string(p.Key)); err != nil {
		return err
	}
	if err := validateAbsolutePath(string(p.SourceDir)); err != nil {
		return err
	}
	if p.OriginBranch != nil {
		if err := validateBranchName(string(*p.OriginBranch)); err != nil {
			return err
		}
	}
	if err := validateDestBranchTemplate(string(p.DestBranch)); err != nil {
		return err
	}
	if err := validateWorktreeDirTemplate(string(p.WorktreeDir)); err != nil {
		return err
	}
	ok, err := deps.Git.IsMainClone(ctx, p.SourceDir)
	if err != nil {
		return err
	}
	if !ok {
		return domain.NewOpError("project.register", domain.CodeNotAMainClone, string(p.SourceDir), "", nil)
	}
	return nil
}
