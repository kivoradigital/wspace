// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// CreateContext is the non-interactive counterpart of RunContextWizard:
// it validates c (validateContextRecord) and saves it, refusing to
// overwrite an already-registered context of the same name. It never
// prompts, so the rpc/mcp surfaces can create a context from explicit
// parameters alone.
func CreateContext(ctx context.Context, store ports.ConfigStore, c domain.Context) (domain.Context, error) {
	if err := validateContextRecord(c); err != nil {
		return domain.Context{}, err
	}
	if _, err := store.LoadContext(ctx, c.Name); err == nil {
		return domain.Context{}, domain.NewOpError("context.create", domain.CodeContextExists, string(c.Name), "", nil)
	} else if domain.Code(err) != domain.CodeContextNotFound {
		return domain.Context{}, err
	}
	if err := store.SaveContext(ctx, c); err != nil {
		return domain.Context{}, err
	}
	return c, nil
}

// UpdateContext is the non-interactive counterpart of
// RunEditContextWizard: it replaces the context stored as name with c
// (c.Projects is ignored and the stored projects are carried over, exactly
// like the wizard, which never edits projects), handling a rename through
// the same saveEditedContext sequence the wizard uses — including moving
// the old name's workspaces to the new name (see WorkspaceReassignment).
func UpdateContext(ctx context.Context, store ports.ConfigStore, fs ports.FileSystemPort, name domain.ContextName, c domain.Context) (domain.Context, WorkspaceReassignment, error) {
	existing, err := store.LoadContext(ctx, name)
	if err != nil {
		return domain.Context{}, WorkspaceReassignment{}, err
	}
	c.Projects = existing.Projects
	if err := validateContextRecord(c); err != nil {
		return domain.Context{}, WorkspaceReassignment{}, err
	}
	re, err := saveEditedContext(ctx, store, fs, existing, c)
	if err != nil {
		return domain.Context{}, WorkspaceReassignment{}, err
	}
	return c, re, nil
}

// validateContextRecord applies the same pure validators the context
// wizard attaches to its fields (validators.go), so a context created or
// edited without a prompt can never hold a value the wizard would have
// rejected.
func validateContextRecord(c domain.Context) error {
	if err := validateContextName(string(c.Name)); err != nil {
		return err
	}
	if err := validateAbsolutePath(string(c.WorkspacesRoot)); err != nil {
		return err
	}
	if c.ProjectsRoot != "" {
		if err := validateAbsolutePath(string(c.ProjectsRoot)); err != nil {
			return err
		}
	}
	patterns := make([]string, len(c.IgnorePatterns))
	for i, g := range c.IgnorePatterns {
		patterns[i] = string(g)
	}
	if err := validateIgnorePatterns(strings.Join(patterns, ",")); err != nil {
		return err
	}
	include := make([]string, len(c.IncludePatterns))
	for i, g := range c.IncludePatterns {
		include[i] = string(g)
	}
	if err := validateIgnorePatterns(strings.Join(include, ",")); err != nil {
		return err
	}
	return validateOptions(c.Defaults)
}

// validateOptions checks every set option value that has a domain rule.
func validateOptions(o domain.Options) error {
	if o.BaseBranch != nil {
		if err := validateBranchName(string(*o.BaseBranch)); err != nil {
			return err
		}
	}
	return nil
}
