// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"sort"
	"strings"
)

// ChangeClass is the teardown-safety classification of one changed path.
type ChangeClass int

const (
	// ChangeTracked is a modified/staged/deleted tracked file — blocks.
	ChangeTracked ChangeClass = iota
	// ChangeForeign is an untracked file this tool did not create — blocks.
	ChangeForeign
	// ChangeEnvCopy is an untracked file this tool copied in — never blocks.
	ChangeEnvCopy
)

// Change is one path and its safety classification.
type Change struct {
	RelPath string
	Class   ChangeClass
}

// ChangeSet is the teardown-safety verdict for one worktree.
type ChangeSet struct {
	Alias    string
	Changes  []Change
	Unpushed int // commits on the branch not reachable from upstream (or base)
}

// Blocking returns the Tracked and Foreign changes, in path order.
func (c ChangeSet) Blocking() []Change {
	var out []Change
	for _, ch := range c.Changes {
		if ch.Class == ChangeTracked || ch.Class == ChangeForeign {
			out = append(out, ch)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

// SafeToRemove reports whether teardown may proceed without --force.
func (c ChangeSet) SafeToRemove() bool {
	return len(c.Blocking()) == 0 && c.Unpushed == 0
}

// PorcelainEntry is one `git status --porcelain=v1` record: the XY status
// codes plus the path they describe.
type PorcelainEntry struct {
	X, Y    byte
	RelPath string
	// OrigPath is a rename's or copy's source path (X or Y is 'R'/'C');
	// empty otherwise.
	OrigPath string
}

// Classify is a pure function: no git, no filesystem. The adapter supplies
// entries (from `git status`, worktree-relative) and the manifest supplies
// the env-copy set recorded at creation time (workspace-relative, i.e.
// "<alias>/<path>") — classification never re-derives a guess.
func Classify(alias string, entries []PorcelainEntry, envCopies []string) ChangeSet {
	prefix := alias + "/"
	envSet := make(map[string]bool, len(envCopies))
	for _, e := range envCopies {
		if rel, ok := strings.CutPrefix(e, prefix); ok {
			envSet[rel] = true
		}
	}

	changes := make([]Change, 0, len(entries))
	for _, entry := range entries {
		changes = append(changes, Change{RelPath: entry.RelPath, Class: classifyEntry(entry, envSet)})
	}

	return ChangeSet{Alias: alias, Changes: changes}
}

func classifyEntry(entry PorcelainEntry, envSet map[string]bool) ChangeClass {
	if entry.X == '?' && entry.Y == '?' {
		if envSet[entry.RelPath] {
			return ChangeEnvCopy
		}
		return ChangeForeign
	}
	return ChangeTracked
}
