// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import "errors"

// UpdateStrategy is how a workspace repository integrates its comparison
// base when it is updated: a merge commit (or fast-forward) or a rebase of
// the local commits onto the base.
type UpdateStrategy string

const (
	// UpdateMerge merges the base into the current branch (the default).
	UpdateMerge UpdateStrategy = "merge"
	// UpdateRebase replays the branch's local commits onto the base; it
	// rewrites those commits.
	UpdateRebase UpdateStrategy = "rebase"
)

// ErrInvalidUpdateStrategy is returned for a strategy other than "merge"
// or "rebase".
var ErrInvalidUpdateStrategy = errors.New(`invalid update strategy: must be "merge" or "rebase"`)

// ParseUpdateStrategy validates s; "" means the default, UpdateMerge.
func ParseUpdateStrategy(s string) (UpdateStrategy, error) {
	switch UpdateStrategy(s) {
	case "", UpdateMerge:
		return UpdateMerge, nil
	case UpdateRebase:
		return UpdateRebase, nil
	default:
		return "", ErrInvalidUpdateStrategy
	}
}

// DirtyForUpdate lists the paths that make a worktree unsafe to update:
// every staged, unstaged or conflicted change to a tracked file, in git's
// order. Untracked files are not listed: neither strategy touches them
// (git refuses, changing nothing, when an incoming file would overwrite
// one) and an autostash would not carry them anyway.
func DirtyForUpdate(entries []PorcelainEntry) []string {
	var out []string
	for _, e := range entries {
		if e.X == '?' && e.Y == '?' {
			continue
		}
		if e.X == '!' && e.Y == '!' {
			continue
		}
		out = append(out, e.RelPath)
	}
	return out
}
