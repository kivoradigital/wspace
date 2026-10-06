// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

// FileChangeStatus names what happened to one path, for display.
type FileChangeStatus string

const (
	FileModified   FileChangeStatus = "modified"
	FileAdded      FileChangeStatus = "added"
	FileDeleted    FileChangeStatus = "deleted"
	FileRenamed    FileChangeStatus = "renamed"
	FileCopied     FileChangeStatus = "copied"
	FileTypeChange FileChangeStatus = "typechange"
	FileUntracked  FileChangeStatus = "untracked"
	FileIgnored    FileChangeStatus = "ignored"
	FileConflicted FileChangeStatus = "conflicted"
)

// FileChange is one changed path in a worktree, as a person reads it:
// what happened (the index side when staged, else the worktree side),
// whether the index holds a change (Staged) and whether the worktree holds
// a further, unstaged one (Unstaged).
type FileChange struct {
	Path     string
	OrigPath string
	Status   FileChangeStatus
	Staged   bool
	Unstaged bool
}

// DescribeChange maps one porcelain v1 record onto a FileChange.
func DescribeChange(e PorcelainEntry) FileChange {
	fc := FileChange{Path: e.RelPath, OrigPath: e.OrigPath}
	switch {
	case e.X == '?' && e.Y == '?':
		fc.Status = FileUntracked
		return fc
	case e.X == '!' && e.Y == '!':
		fc.Status = FileIgnored
		return fc
	case isConflict(e.X, e.Y):
		fc.Status, fc.Staged, fc.Unstaged = FileConflicted, true, true
		return fc
	}
	fc.Staged = e.X != ' '
	fc.Unstaged = e.Y != ' '
	code := e.Y
	if fc.Staged {
		code = e.X
	}
	fc.Status = statusFor(code)
	return fc
}

func isConflict(x, y byte) bool {
	return x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D')
}

func statusFor(code byte) FileChangeStatus {
	switch code {
	case 'A':
		return FileAdded
	case 'D':
		return FileDeleted
	case 'R':
		return FileRenamed
	case 'C':
		return FileCopied
	case 'T':
		return FileTypeChange
	default:
		return FileModified
	}
}
