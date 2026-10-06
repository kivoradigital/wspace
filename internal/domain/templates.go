// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// BranchTemplate carries exactly one substitution pass over Vars, e.g.
// "{prefix}{workspace}". Deliberately not text/template: no arbitrary
// expressions, no I/O, no panics.
type BranchTemplate string

// PathTemplate carries exactly one substitution pass over Vars, e.g.
// "{workspace}/{project}".
type PathTemplate string

// Vars is the fixed variable set every template substitution draws from.
type Vars struct {
	Workspace string
	Project   string
	Branch    string
	Prefix    string
}

// ErrInvalidTemplate is the sentinel wrapped when a template references an
// unknown variable.
var ErrInvalidTemplate = errors.New("invalid template")

var templateVarPattern = regexp.MustCompile(`\{[a-zA-Z]+\}`)

// Resolve substitutes v into t and validates the result as a BranchName.
func (t BranchTemplate) Resolve(v Vars) (BranchName, error) {
	s, err := substitute(string(t), v)
	if err != nil {
		return "", err
	}
	return NewBranchName(s)
}

// Resolve substitutes v into t, rejects any resulting ".." component, and
// joins the result onto root.
func (t PathTemplate) Resolve(v Vars, root Path) (Path, error) {
	s, err := substitute(string(t), v)
	if err != nil {
		return "", err
	}
	normalized := strings.ReplaceAll(s, `\`, "/")
	if hasDotDotComponent(normalized) {
		return "", fmt.Errorf("%w: resolved path %q escapes its root via \"..\"", ErrInvalidPath, s)
	}
	return root.Join(s), nil
}

// substitute performs the single, non-recursive substitution pass every
// template gets. An unknown "{name}" token is rejected rather than left in
// place, so a typo never becomes a literal directory or branch name.
func substitute(tmpl string, v Vars) (string, error) {
	var unknown []string
	result := templateVarPattern.ReplaceAllStringFunc(tmpl, func(token string) string {
		switch token[1 : len(token)-1] {
		case "workspace":
			return v.Workspace
		case "project":
			return v.Project
		case "branch":
			return v.Branch
		case "prefix":
			return v.Prefix
		default:
			unknown = append(unknown, token)
			return token
		}
	})
	if len(unknown) > 0 {
		return "", fmt.Errorf("%w: unknown template variable(s) %v", ErrInvalidTemplate, unknown)
	}
	return result, nil
}
