// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import "path"

// MatchesIgnorePattern reports whether name (a bare directory name, never
// a full path) matches any of patterns, using path.Match glob syntax
// (context-management spec: "Ignore pattern excludes a directory from
// discovery"). Kept separate from env-file pruning (env_prune_dirs), which
// is a plain directory-name set, never a glob (context-management spec:
// "Ignore patterns do not affect env pruning").
func MatchesIgnorePattern(patterns []Glob, name string) (bool, error) {
	for _, p := range patterns {
		ok, err := path.Match(string(p), name)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// MatchesIncludePattern reports whether a repository at relPath (its
// "/"-separated path below the scan root) is wanted by patterns: true when
// patterns is empty, otherwise when any pattern matches (path.Match) the
// repository's folder name or its whole relative path, so "api*" and
// "team-a/*" both work. The error is a malformed pattern.
func MatchesIncludePattern(patterns []Glob, relPath string) (bool, error) {
	if len(patterns) == 0 {
		return true, nil
	}
	leaf := path.Base(relPath)
	for _, p := range patterns {
		for _, name := range []string{leaf, relPath} {
			ok, err := path.Match(string(p), name)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}
