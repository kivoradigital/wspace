// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
)

// validateContextName adapts domain.NewContextName into a ports.TextField
// validator, shared by every wizard that asks for a context name — the
// exact same pure domain validator drives both the terminal Prompter and
// the GUI FormPrompter (design.md §4: "a pure domain validator, reused by
// both surfaces").
func validateContextName(s string) error {
	_, err := domain.NewContextName(s)
	return err
}

// validateAbsolutePath adapts domain.NewPath, shared by every wizard field
// that collects an absolute filesystem path (workspaces_root,
// projects_root, source_dir).
func validateAbsolutePath(s string) error {
	_, err := domain.NewPath(s)
	return err
}

// validateBranchName adapts domain.NewBranchName, shared by every wizard
// field that collects a branch name (base_branch and friends).
func validateBranchName(s string) error {
	_, err := domain.NewBranchName(s)
	return err
}

// validateWorkspaceName rejects a blank workspace name — every other
// value is accepted, exactly matching internal/cli's own "ws create
// <name>" positional argument, which places no further restriction on a
// workspace name's content beyond requiring one to be given.
func validateWorkspaceName(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("workspace name must not be empty")
	}
	return nil
}

// ErrInvalidIgnorePattern is the sentinel wrapped by every malformed
// ignore-pattern rejection; callers may match it with errors.Is.
var ErrInvalidIgnorePattern = errors.New("invalid ignore pattern")

// validateIgnorePatterns validates a raw ignore_patterns wizard answer
// before it is ever split into individual globs (parseIgnorePatterns, in
// context_wizard.go, does that split with identical comma/whitespace
// handling). Every comma-separated, trimmed entry must be a valid
// path.Match pattern — checked here with path.Match(pattern, ""), which
// surfaces path.ErrBadPattern for a malformed glob exactly the same way
// domain.MatchesIgnorePattern will later apply it during discovery — so a
// typo is rejected immediately at wizard entry instead of silently
// accepted and only failing deep inside a future scan. An empty answer
// ("no patterns") is always valid.
func validateIgnorePatterns(s string) error {
	for _, raw := range strings.Split(s, ",") {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("%w %q: %w", ErrInvalidIgnorePattern, p, err)
		}
	}
	return nil
}

// templateValidationVars are safe, non-empty placeholder values used only
// to check that a user-supplied dest_branch/worktree_dir template's
// variables are all known and that the substituted result still satisfies
// the underlying domain rule (a valid branch name / path component). The
// real values (the workspace's actual name, the project's actual key)
// only exist once a workspace is actually created, long after this wizard
// runs, so validation here can only ever prove the template's shape is
// sound — never that a specific future substitution will succeed.
var templateValidationVars = domain.Vars{
	Workspace: "workspace",
	Project:   "project",
	Branch:    "branch",
	Prefix:    "prefix",
}

// templateValidationRoot is an arbitrary absolute path used only to
// exercise domain.PathTemplate.Resolve during validation; the resolved
// path itself is discarded, since only the template's own well-formedness
// is being checked here.
const templateValidationRoot = domain.Path("/placeholder")

// validateOptionalBranchName allows an empty answer (meaning "inherit
// the resolved base branch") and otherwise defers to validateBranchName —
// shared by every wizard field that collects an OPTIONAL branch name
// (project-configuration spec's origin_branch, whose zero value means
// "unset => resolved BaseBranch").
func validateOptionalBranchName(s string) error {
	if s == "" {
		return nil
	}
	return validateBranchName(s)
}

// validateDestBranchTemplate allows an empty answer (meaning "inherit the
// workspace's branch") and otherwise checks that s parses as a
// domain.BranchTemplate: every "{name}" token is a known template
// variable, and substituting placeholder values still yields a valid
// branch name.
func validateDestBranchTemplate(s string) error {
	if s == "" {
		return nil
	}
	_, err := domain.BranchTemplate(s).Resolve(templateValidationVars)
	return err
}

// validateWorktreeDirTemplate allows an empty answer (meaning "inherit
// the built-in {project} layout") and otherwise checks that s parses as a
// domain.PathTemplate the same way validateDestBranchTemplate checks a
// domain.BranchTemplate.
func validateWorktreeDirTemplate(s string) error {
	if s == "" {
		return nil
	}
	_, err := domain.PathTemplate(s).Resolve(templateValidationVars, templateValidationRoot)
	return err
}
