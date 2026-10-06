// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidProjectKey is the sentinel wrapped by every NewProjectKey
// rejection; callers may match it with errors.Is.
var ErrInvalidProjectKey = errors.New("invalid project key")

// ErrInvalidWorkspaceName is the sentinel wrapped by every
// NewWorkspaceName rejection; callers may match it with errors.Is.
var ErrInvalidWorkspaceName = errors.New("invalid workspace name")

// NewProjectKey validates s as a project key. A key becomes a worktree
// directory name ("{project}") and a template variable, so it must be a
// single, non-hidden, non-flag-like path component: not blank, no "/" or
// "\", not "." or "..", and not starting with "." or "-".
func NewProjectKey(s string) (ProjectKey, error) {
	if reason := invalidComponentReason(s); reason != "" {
		return "", fmt.Errorf("%w: %q %s", ErrInvalidProjectKey, s, reason)
	}
	return ProjectKey(s), nil
}

// NewWorkspaceName validates s as a workspace name — the directory created
// under a context's WorkspacesRoot — with the same single-component rules
// as NewProjectKey, so a name can never escape WorkspacesRoot ("../x").
func NewWorkspaceName(s string) (string, error) {
	if reason := invalidComponentReason(s); reason != "" {
		return "", fmt.Errorf("%w: %q %s", ErrInvalidWorkspaceName, s, reason)
	}
	return s, nil
}

func invalidComponentReason(s string) string {
	switch {
	case strings.TrimSpace(s) == "":
		return "must not be empty"
	case strings.ContainsAny(s, `/\`):
		return "must not contain a path separator"
	case strings.HasPrefix(s, "."):
		return "must not start with \".\""
	case strings.HasPrefix(s, "-"):
		return "must not start with \"-\""
	default:
		return ""
	}
}

// SuggestProjectKeys proposes one project key per relative path (a
// repository's path below its scan root, "/"-separated) so that every
// suggestion can be registered together with the keys already taken in
// the context:
//
//   - the leaf folder name while it is unique among relPaths and taken;
//   - otherwise the relative path with "/" replaced by "-";
//   - otherwise that with "-2", "-3", ... appended.
//
// Uniqueness is case-insensitive: a key is a worktree folder name, and on
// a case-insensitive file system (the macOS default) "App" and "app" are
// the same folder. Leading "." and "-" are dropped so every suggestion is
// a valid key; a name with nothing left becomes "project".
func SuggestProjectKeys(relPaths []string, taken []ProjectKey) []string {
	fold := func(s string) string { return strings.ToLower(s) }
	used := make(map[string]bool, len(taken)+len(relPaths))
	for _, k := range taken {
		used[fold(string(k))] = true
	}
	leafUses := make(map[string]int, len(relPaths))
	for _, rel := range relPaths {
		leafUses[fold(suggestLeaf(rel))]++
	}

	out := make([]string, len(relPaths))
	for i, rel := range relPaths {
		leaf := suggestLeaf(rel)
		candidate := leaf
		if leafUses[fold(leaf)] > 1 || used[fold(leaf)] {
			candidate = sanitizeKey(strings.ReplaceAll(strings.Trim(rel, "/"), "/", "-"))
		}
		base, n := candidate, 2
		for used[fold(candidate)] {
			candidate = fmt.Sprintf("%s-%d", base, n)
			n++
		}
		used[fold(candidate)] = true
		out[i] = candidate
	}
	return out
}

func suggestLeaf(rel string) string {
	rel = strings.Trim(rel, "/")
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		rel = rel[i+1:]
	}
	return sanitizeKey(rel)
}

func sanitizeKey(s string) string {
	s = strings.TrimLeft(strings.ReplaceAll(s, `\`, "-"), ".-")
	if strings.TrimSpace(s) == "" {
		return "project"
	}
	return s
}
