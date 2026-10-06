// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

import (
	"io/fs"
	"strings"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
)

// FileSystemPort covers every filesystem touch, including platform paths.
// Methods are deliberately synchronous and context-free: they are single
// non-blocking syscalls, and adding a context would be ceremony that
// implies cancellability that does not exist (design.md §4).
type FileSystemPort interface {
	Exists(p domain.Path) (bool, error)
	IsDir(p domain.Path) (bool, error)
	IsGitDirEntry(p domain.Path) (isDir bool, exists bool, err error) // ".git" dir vs file
	MkdirAll(p domain.Path) error
	RemoveAll(p domain.Path) error
	ListDirs(p domain.Path) ([]string, error)
	// ListFiles returns the names of p's direct regular files, sorted; a
	// missing p is an empty list.
	ListFiles(p domain.Path) ([]string, error)
	ReadFile(p domain.Path) ([]byte, error)
	WriteFile(p domain.Path, data []byte, perm fs.FileMode) error
	CopyFile(src, dst domain.Path) error
	ModTime(p domain.Path) (time.Time, error)
	FindEnvFiles(root domain.Path, pruneDirs []string) ([]string, error) // repo-relative
	WalkUp(start domain.Path, marker string) (domain.Path, bool, error)

	// WalkGitRepos walks the directory tree rooted at root looking for git
	// repositories, recursively — the project-discovery fix for a
	// repository nested more than one level below a context's
	// ProjectsRoot (e.g. "<root>/team/api"), which a single ListDirs call
	// can never see. See WalkGitReposResult and WalkGitReposUsing (this
	// package) for the shared algorithm both FileSystemPort implementations
	// (fsstore.Adapter, portstest.FakeFS) drive through this one method.
	WalkGitRepos(root domain.Path, maxDepth int, prune func(name string) bool) (WalkGitReposResult, error)

	Paths() Paths
	Cwd() (domain.Path, error)

	// LegacyFlatConfigPath resolves the fixed candidate location of a
	// legacy flat workspace configuration tool's own global config file
	// (this change's import feature: "$XDG_CONFIG_HOME/ws/config" else
	// "%APPDATA%\ws\config" on Windows else "~/.config/ws/config"). This is
	// deliberately a *different* directory name ("ws") than this
	// application's own config root (Paths().Config, "<config>/wspace") —
	// the two must never collide, since one names an older, unrelated
	// tool. The returned path is not existence-checked; a caller decides
	// what a missing file there means.
	LegacyFlatConfigPath() domain.Path
}

// Paths are the platform-resolved config/cache/home roots (design.md §5).
type Paths struct {
	Config domain.Path // <config>/ws
	Cache  domain.Path // <cache>/ws
	Home   domain.Path
}

// WalkGitReposResult is WalkGitRepos's own outcome: every main-clone
// directory found, every linked-worktree directory found, and whether the
// walk's own depth cap cut it short.
type WalkGitReposResult struct {
	// MainClones is every directory found whose ".git" entry is itself a
	// directory. None of these is ever descended into: a main clone's own
	// contents are not more candidates (a nested ".git" inside one, if any,
	// belongs to that clone, not to the scan).
	MainClones []domain.Path
	// LinkedWorktrees is every directory found whose ".git" entry is a
	// file, likewise never descended into.
	LinkedWorktrees []domain.Path
	// Truncated is true when maxDepth was reached while at least one
	// directory below it — one that survived the hidden/prune check, so it
	// would genuinely have been explored — was left unvisited. A caller
	// must report this rather than silently presenting a partial scan as a
	// complete one.
	Truncated bool
	// TruncatedDirs lists every directory at the depth cap that still had
	// unexplored (not hidden, not pruned) subdirectories, in walk order —
	// exactly the folders a caller can name when it reports a partial scan
	// ("raise the depth or ignore these"). Truncated == len(TruncatedDirs) > 0.
	TruncatedDirs []domain.Path
}

// dirWalker is the minimal capability WalkGitReposUsing needs from a
// FileSystemPort implementation: list a directory's immediate
// subdirectories, and distinguish a ".git" directory entry from a ".git"
// file entry. Both of this package's own FileSystemPort implementations
// (internal/adapters/fsstore.Adapter, internal/ports/portstest.FakeFS)
// already have both methods, so WalkGitReposUsing lets them share this one
// walk algorithm rather than each reimplementing recursion, depth-capping
// and pruning separately and risking the two silently drifting apart — this
// project's own documented history of exactly that failure (a fake that
// could only express one flat level, unable to catch a bug the real
// adapter had).
type dirWalker interface {
	ListDirs(p domain.Path) ([]string, error)
	IsGitDirEntry(p domain.Path) (isDir bool, exists bool, err error)
}

// isHiddenDirName reports whether name is a hidden directory (a leading
// dot) — always pruned, regardless of prune's own answer, exactly like a
// real shell glob never matches a dotfile by accident.
func isHiddenDirName(name string) bool {
	return strings.HasPrefix(name, ".")
}

// WalkGitReposUsing implements WalkGitRepos's own contract against any
// dirWalker: it walks root's subdirectory tree down to maxDepth levels
// below root (maxDepth <= 0 means unlimited), recording every directory
// whose ".git" entry is a directory (MainClones) or a file
// (LinkedWorktrees) without ever descending into either. Every other
// directory is walked into unless its name is hidden (isHiddenDirName) or
// prune(name) returns true — both checked, and both preventing descent,
// before anything below that directory is touched, so a pruned subtree
// (e.g. "node_modules") is never even listed, which is what keeps this
// genuinely cheap rather than a filter applied after a full walk.
func WalkGitReposUsing(fs dirWalker, root domain.Path, maxDepth int, prune func(name string) bool) (WalkGitReposResult, error) {
	var res WalkGitReposResult
	names, err := fs.ListDirs(root)
	if err != nil {
		return WalkGitReposResult{}, err
	}
	for _, name := range names {
		if isHiddenDirName(name) || (prune != nil && prune(name)) {
			continue
		}
		if err := walkGitReposInto(fs, root.Join(name), 1, maxDepth, prune, &res); err != nil {
			return WalkGitReposResult{}, err
		}
	}
	return res, nil
}

// walkGitReposInto visits dir (already past the hidden/prune check its own
// caller applied to dir's name), at depth levels below the original root.
func walkGitReposInto(fs dirWalker, dir domain.Path, depth, maxDepth int, prune func(name string) bool, res *WalkGitReposResult) error {
	isDir, exists, err := fs.IsGitDirEntry(dir.Join(".git"))
	if err != nil {
		return err
	}
	if exists {
		if isDir {
			res.MainClones = append(res.MainClones, dir)
		} else {
			res.LinkedWorktrees = append(res.LinkedWorktrees, dir)
		}
		return nil // never descend into a repository, main clone or worktree
	}

	// Not a repository at all: an ordinary container directory. Descend
	// into its own subdirectories, subject to the depth cap.
	names, err := fs.ListDirs(dir)
	if err != nil {
		return err
	}

	if maxDepth > 0 && depth >= maxDepth {
		for _, name := range names {
			if isHiddenDirName(name) || (prune != nil && prune(name)) {
				continue
			}
			// At least one directory that would otherwise have been
			// explored survives past the cap: the scan is genuinely
			// partial, not merely empty at this depth.
			res.Truncated = true
			res.TruncatedDirs = append(res.TruncatedDirs, dir)
			break
		}
		return nil
	}

	for _, name := range names {
		if isHiddenDirName(name) || (prune != nil && prune(name)) {
			continue
		}
		if err := walkGitReposInto(fs, dir.Join(name), depth+1, maxDepth, prune, res); err != nil {
			return err
		}
	}
	return nil
}
