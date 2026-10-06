// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package fsstore implements ports.FileSystemPort against the real
// filesystem: every touch this product makes to disk outside of git and
// config-store-specific I/O goes through here (design.md §4).
package fsstore

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Adapter implements ports.FileSystemPort.
type Adapter struct{}

// New constructs an Adapter. It holds no state: every method is a direct,
// synchronous filesystem call.
func New() *Adapter { return &Adapter{} }

var _ ports.FileSystemPort = (*Adapter)(nil)

func (a *Adapter) Exists(p domain.Path) (bool, error) {
	_, err := os.Stat(string(p))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (a *Adapter) IsDir(p domain.Path) (bool, error) {
	info, err := os.Stat(string(p))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// IsGitDirEntry distinguishes a ".git" directory (main clone) from a
// ".git" file (linked worktree) without deciding what that means — that
// judgment belongs to the git adapter's IsMainClone (design.md §7).
func (a *Adapter) IsGitDirEntry(p domain.Path) (isDir bool, exists bool, err error) {
	info, statErr := os.Stat(string(p))
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return false, false, nil
		}
		return false, false, statErr
	}
	return info.IsDir(), true, nil
}

func (a *Adapter) MkdirAll(p domain.Path) error { return os.MkdirAll(string(p), 0o755) }

func (a *Adapter) RemoveAll(p domain.Path) error { return os.RemoveAll(string(p)) }

// ListDirs returns the names of the direct subdirectories of p, sorted. A
// root that does not exist at all reports an empty list, not an error —
// the same translation Exists and IsDir above already apply to
// os.IsNotExist, and for the same reason: "no workspaces have ever been
// created here yet" is p's normal, expected state for a brand-new context,
// never an operational failure (workspace-lifecycle spec: "Workspace
// listing and repair" — CRITICAL-3, verify-report.md). Any other read
// failure (permission denied, I/O error, ...) still propagates, since
// that is a genuine failure this call cannot silently paper over.
func (a *Adapter) ListDirs(p domain.Path) ([]string, error) {
	entries, err := os.ReadDir(string(p))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// ListFiles returns the names of p's direct regular files, sorted; a
// missing p is an empty list, exactly like ListDirs.
func (a *Adapter) ListFiles(p domain.Path) ([]string, error) {
	entries, err := os.ReadDir(string(p))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func (a *Adapter) ReadFile(p domain.Path) ([]byte, error) { return os.ReadFile(string(p)) }

func (a *Adapter) WriteFile(p domain.Path, data []byte, perm fs.FileMode) error {
	return os.WriteFile(string(p), data, perm)
}

// CopyFile copies src to dst, creating dst's parent directories as needed
// and preserving src's permission bits.
func (a *Adapter) CopyFile(src, dst domain.Path) error {
	data, err := os.ReadFile(string(src))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(string(dst)), 0o755); err != nil {
		return err
	}
	perm := fs.FileMode(0o644)
	if info, statErr := os.Stat(string(src)); statErr == nil {
		perm = info.Mode().Perm()
	}
	return os.WriteFile(string(dst), data, perm)
}

func (a *Adapter) ModTime(p domain.Path) (time.Time, error) {
	info, err := os.Stat(string(p))
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func (a *Adapter) Cwd() (domain.Path, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return domain.Path(filepath.ToSlash(dir)), nil
}

// WalkUp walks upward from start looking for marker (a file or directory
// name) in each ancestor directory, returning the first ancestor that
// contains it. Used to find a workspace root from any directory inside it
// (design.md §4).
func (a *Adapter) WalkUp(start domain.Path, marker string) (domain.Path, bool, error) {
	cur := filepath.FromSlash(string(start))
	for {
		candidate := filepath.Join(cur, marker)
		if _, err := os.Stat(candidate); err == nil {
			return domain.Path(filepath.ToSlash(cur)), true, nil
		} else if !os.IsNotExist(err) {
			return "", false, err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false, nil
		}
		cur = parent
	}
}

// WalkGitRepos implements ports.FileSystemPort.WalkGitRepos against the
// real filesystem: it drives ports.WalkGitReposUsing, the algorithm shared
// with portstest.FakeFS's own implementation, over a.ListDirs/a.IsGitDirEntry
// (both already real, synchronous syscalls). See that shared function's own
// doc comment for the walk/prune/depth-cap contract.
func (a *Adapter) WalkGitRepos(root domain.Path, maxDepth int, prune func(name string) bool) (ports.WalkGitReposResult, error) {
	return ports.WalkGitReposUsing(a, root, maxDepth, prune)
}

// excludedEnvSuffixes are template-like files that are never real env files
// (environment-files spec: "Env file discovery").
var excludedEnvSuffixes = []string{".example", ".sample", ".template", ".dist"}

// FindEnvFiles discovers ".env" and ".env.*" files under root, pruning any
// directory named in pruneDirs and excluding template-like suffixes.
// Returned paths are relative to root (environment-files spec).
func (a *Adapter) FindEnvFiles(root domain.Path, pruneDirs []string) ([]string, error) {
	prune := make(map[string]bool, len(pruneDirs))
	for _, d := range pruneDirs {
		prune[d] = true
	}

	rootStr := filepath.FromSlash(string(root))
	var out []string

	err := filepath.WalkDir(rootStr, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != rootStr && prune[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !isEnvFileName(name) || hasExcludedEnvSuffix(name) {
			return nil
		}
		rel, relErr := filepath.Rel(rootStr, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(out)
	return out, nil
}

func isEnvFileName(name string) bool {
	return name == ".env" || strings.HasPrefix(name, ".env.")
}

func hasExcludedEnvSuffix(name string) bool {
	for _, suf := range excludedEnvSuffixes {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}
