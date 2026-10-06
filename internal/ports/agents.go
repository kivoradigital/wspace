// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

import (
	"context"
	"errors"
	"io/fs"

	"github.com/kivoradigital/wspace/internal/domain"
)

// EntryKind is what a path names without following a final symlink.
type EntryKind int

const (
	EntryMissing EntryKind = iota
	EntryDir
	EntryFile
	EntrySymlink
)

// Entry describes one path as lstat sees it. LinkTarget is the raw link
// text (slash-separated) when Kind is EntrySymlink.
type Entry struct {
	Kind       EntryKind
	LinkTarget string
}

// ErrSymlinkUnsupported is returned by AgentFS.Symlink when the platform
// or account cannot create a directory symlink (Windows without Developer
// Mode); callers fall back to copying.
var ErrSymlinkUnsupported = errors.New("symbolic links are not supported here")

// ErrNotASymlink is returned by AgentFS.RemoveLink for a path that is not
// a symbolic link: removing a link must never delete real content.
var ErrNotASymlink = errors.New("not a symbolic link")

// AgentFS is the filesystem surface agent skill installation needs:
// lstat-level inspection, directory symlinks, and moves. Every path is a
// slash-separated absolute domain.Path.
type AgentFS interface {
	// Lstat describes p without following a final symlink; a missing p is
	// EntryMissing, not an error.
	Lstat(p domain.Path) (Entry, error)
	// Exists reports whether p exists, following symlinks (a dangling
	// link does not exist).
	Exists(p domain.Path) (bool, error)
	// EvalSymlinks resolves every symlink in p.
	EvalSymlinks(p domain.Path) (domain.Path, error)
	MkdirAll(p domain.Path) error
	// Symlink creates link pointing at the directory target.
	Symlink(target, link domain.Path) error
	// Rename moves from to to (both on the same volume).
	Rename(from, to domain.Path) error
	// RemoveLink deletes p only when it is a symlink (ErrNotASymlink
	// otherwise).
	RemoveLink(p domain.Path) error
	// RemoveAll deletes p recursively. Agent installation only uses it on
	// directories wspace itself created (its extracted skill, its marked
	// copies).
	RemoveAll(p domain.Path) error
	ReadFile(p domain.Path) ([]byte, error)
	WriteFile(p domain.Path, data []byte, perm fs.FileMode) error
	// CopyDir copies the directory tree src to dst (dst must not exist).
	CopyDir(src, dst domain.Path) error
}

// CommandResult is a finished subprocess.
type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// CommandRunner finds and runs other programs (an agent's own CLI) with a
// bounded run time.
type CommandRunner interface {
	// LookPath returns name's absolute path when it is on PATH.
	LookPath(name string) (string, bool)
	// Run executes name with args and waits for it. A non-zero exit is a
	// result, not an error; err is set only when the program could not be
	// run or timed out.
	Run(ctx context.Context, name string, args ...string) (CommandResult, error)
	// RunIn is Run with the working directory set to dir (the caller's
	// own when dir is empty): agent CLIs resolve project-scoped settings
	// from it.
	RunIn(ctx context.Context, dir, name string, args ...string) (CommandResult, error)
}
