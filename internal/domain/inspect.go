// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"regexp"
	"strings"
	"time"
)

// ChangeEntry is one changed path on one side (index or worktree) of a
// repository, as the repository inspector lists it.
type ChangeEntry struct {
	Path     string
	OrigPath string
	Status   FileChangeStatus
}

// ChangeSets is a worktree's status split the way a git client shows it:
// what is staged, what is not, untracked files and unmerged paths. A path
// with both a staged and an unstaged change is listed on both sides.
type ChangeSets struct {
	Staged     []ChangeEntry
	Unstaged   []ChangeEntry
	Untracked  []ChangeEntry
	Conflicted []ChangeEntry
}

// ChangeSide names the side of a change a diff is asked for.
type ChangeSide int

const (
	// SideUnstaged is the worktree side (untracked files included).
	SideUnstaged ChangeSide = iota
	// SideStaged is the index side.
	SideStaged
)

// SplitChanges sorts porcelain v1 records into ChangeSets, keeping git's
// order inside each list. Ignored entries are dropped.
func SplitChanges(entries []PorcelainEntry) ChangeSets {
	cs := ChangeSets{Staged: []ChangeEntry{}, Unstaged: []ChangeEntry{}, Untracked: []ChangeEntry{}, Conflicted: []ChangeEntry{}}
	for _, e := range entries {
		switch {
		case e.X == '?' && e.Y == '?':
			cs.Untracked = append(cs.Untracked, ChangeEntry{Path: e.RelPath, Status: FileUntracked})
		case e.X == '!' && e.Y == '!':
		case isConflict(e.X, e.Y):
			cs.Conflicted = append(cs.Conflicted, ChangeEntry{Path: e.RelPath, Status: FileConflicted})
		default:
			if e.X != ' ' {
				cs.Staged = append(cs.Staged, ChangeEntry{Path: e.RelPath, OrigPath: e.OrigPath, Status: statusFor(e.X)})
			}
			if e.Y != ' ' {
				entry := ChangeEntry{Path: e.RelPath, Status: statusFor(e.Y)}
				if e.X == ' ' {
					entry.OrigPath = e.OrigPath
				}
				cs.Unstaged = append(cs.Unstaged, entry)
			}
		}
	}
	return cs
}

// Find returns the change for path on side. The unstaged side includes
// untracked and conflicted paths. Only listed paths are ever found, which
// is what keeps a diff request from naming an arbitrary file.
func (cs ChangeSets) Find(path string, side ChangeSide) (ChangeEntry, bool) {
	lists := [][]ChangeEntry{cs.Staged}
	if side == SideUnstaged {
		lists = [][]ChangeEntry{cs.Unstaged, cs.Untracked, cs.Conflicted}
	}
	for _, list := range lists {
		for _, e := range list {
			if e.Path == path {
				return e, true
			}
		}
	}
	return ChangeEntry{}, false
}

// DiffLimits caps a diff: MaxBytes of git output and MaxLines patch lines.
type DiffLimits struct {
	MaxBytes int
	MaxLines int
}

// DefaultDiffLimits are the inspector's caps (1 MiB, 5000 lines).
var DefaultDiffLimits = DiffLimits{MaxBytes: 1 << 20, MaxLines: 5000}

// DiffHunk is one "@@ -a,b +c,d @@" hunk. Lines keep their one-character
// prefix: ' ' context, '+' added, '-' removed, '\' a "No newline" marker.
type DiffHunk struct {
	Header   string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []string
}

// FileDiff is one file's part of a patch.
type FileDiff struct {
	Path      string
	OrigPath  string
	Status    FileChangeStatus
	Binary    bool
	Additions int
	Deletions int
	Hunks     []DiffHunk
	// Truncated is true when the file's patch was cut by DiffLimits.
	Truncated bool
}

// Patch is a parsed multi-file diff; Truncated is true when any part of
// it was cut by DiffLimits (files after the cut are missing).
type Patch struct {
	Files     []FileDiff
	Truncated bool
}

// CommitInfo is one commit in a list. Author e-mail addresses are never
// read (privacy): only the name.
type CommitInfo struct {
	Hash       string
	ShortHash  string
	Subject    string
	AuthorName string
	AuthorDate time.Time
}

// CommitDetail is one commit with its full message and its diff against
// its first parent (or the empty tree for a root commit).
type CommitDetail struct {
	CommitInfo
	Body    string
	Parents []string
	Patch   Patch
}

// StashEntry is one `git stash list` entry.
type StashEntry struct {
	Index   int
	Ref     string // "stash@{0}"
	Hash    string
	Branch  string // "" when the subject has no "On <branch>:" prefix
	Message string
	Date    time.Time
}

// UpstreamInfo is a branch's configured upstream.
type UpstreamInfo struct {
	Ref    string // "origin/feat"
	Remote string // "origin"; "." for a local upstream
	// Gone is true when the upstream is configured but its ref no longer
	// exists (deleted on the remote and pruned).
	Gone bool
	// RemoteRef is the upstream branch's name on the remote
	// ("refs/heads/feat"); empty when unknown.
	RemoteRef string
}

// ParseStashSubject splits a stash reflog subject ("On main: msg" or
// "WIP on main: abc1234 subject") into branch and message.
func ParseStashSubject(s string) (branch, message string) {
	rest := s
	switch {
	case strings.HasPrefix(s, "WIP on "):
		rest = strings.TrimPrefix(s, "WIP on ")
	case strings.HasPrefix(s, "On "):
		rest = strings.TrimPrefix(s, "On ")
	default:
		return "", s
	}
	i := strings.Index(rest, ": ")
	if i < 0 {
		return "", s
	}
	return rest[:i], rest[i+2:]
}

var commitHashPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// ValidCommitHash accepts an abbreviated or full hexadecimal object name
// only, never a revision expression or anything option-like.
func ValidCommitHash(s string) bool {
	return commitHashPattern.MatchString(s)
}
