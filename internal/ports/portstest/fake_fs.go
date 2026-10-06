// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// fsTestingT is the minimal *testing.T surface FakeFS needs to derive
// realistic, isolated roots (design.md §12: "Paths() returns
// t.TempDir()-based roots").
type fsTestingT interface {
	TempDir() string
}

// FakeFS is an in-memory ports.FileSystemPort. Paths() resolves to
// t.TempDir()-based roots so tests read naturally, but reads/writes never
// touch real disk. Every write is appended to Writes, in order.
type FakeFS struct {
	paths    ports.Paths
	files    map[domain.Path][]byte
	dirs     map[domain.Path]bool
	modTimes map[domain.Path]time.Time
	cwd      domain.Path
	Writes   []domain.Path

	// LegacyPath backs LegacyFlatConfigPath(); it defaults to a path
	// distinct from Paths().Config so a test can tell the two apart
	// exactly as the real fsstore adapter's "ws" vs "wspace" directories
	// do. Tests may overwrite it directly to point at a scripted legacy
	// config location.
	LegacyPath domain.Path

	// DenyWrite, when non-nil, is consulted by WriteFile before every
	// write; a true return simulates a permission-denied directory (e.g.
	// app.Install's "no writable, PATH-listed directory" path) without
	// needing a real, unwritable filesystem in a unit test.
	DenyWrite func(domain.Path) bool
}

// NewFakeFS constructs a FakeFS whose Paths() are rooted under t.TempDir().
func NewFakeFS(t fsTestingT) *FakeFS {
	root := filepath.ToSlash(t.TempDir())
	paths := ports.Paths{
		Config: domain.Path(root + "/config"),
		Cache:  domain.Path(root + "/cache"),
		Home:   domain.Path(root),
	}
	return &FakeFS{
		paths:      paths,
		files:      map[domain.Path][]byte{},
		dirs:       map[domain.Path]bool{paths.Config: true, paths.Cache: true, paths.Home: true},
		modTimes:   map[domain.Path]time.Time{},
		cwd:        paths.Home,
		LegacyPath: domain.Path(root + "/legacy-ws-config/config"),
	}
}

func (f *FakeFS) Paths() ports.Paths { return f.paths }

// LegacyFlatConfigPath returns f.LegacyPath, a fixed candidate location
// distinct from Paths().Config (see LegacyPath's own doc comment).
func (f *FakeFS) LegacyFlatConfigPath() domain.Path { return f.LegacyPath }

func (f *FakeFS) Cwd() (domain.Path, error) { return f.cwd, nil }

// Chdir sets the path Cwd() reports, for tests exercising cwd-relative
// safety checks (workspace-lifecycle spec: "Refuse when cwd is inside the
// workspace"). It does not touch the real process working directory.
func (f *FakeFS) Chdir(p domain.Path) { f.cwd = p }

func (f *FakeFS) Exists(p domain.Path) (bool, error) {
	if _, ok := f.files[p]; ok {
		return true, nil
	}
	return f.dirs[p], nil
}

func (f *FakeFS) IsDir(p domain.Path) (bool, error) {
	return f.dirs[p], nil
}

func (f *FakeFS) IsGitDirEntry(p domain.Path) (isDir bool, exists bool, err error) {
	if f.dirs[p] {
		return true, true, nil
	}
	if _, ok := f.files[p]; ok {
		return false, true, nil
	}
	return false, false, nil
}

// MkdirAll marks p, and every ancestor of p, as an existing directory —
// mirroring what a real os.MkdirAll(p) actually does on disk (it creates
// every missing path component, not only the leaf). A version of this
// method that only marked the leaf directory previously combined with
// ListDirs's own blind prefix-scan (see ListDirs's doc comment) to paper
// over a root directory that was never genuinely registered.
//
// Ancestors are walked by trimming the string after its last "/" rather
// than through domain.Path.Join("..")/path.Clean: some test fixtures build
// synthetic, non-POSIX-rooted paths (e.g. install's Windows candidates,
// "C:/Users/..."), for which repeated ".." cleaning never converges to a
// fixed point and would loop and grow unboundedly. Trimming the last path
// segment is always strictly shortening and always terminates once no "/"
// remains.
func (f *FakeFS) MkdirAll(p domain.Path) error {
	s := string(p)
	for {
		f.dirs[domain.Path(s)] = true
		i := strings.LastIndexByte(s, '/')
		if i <= 0 {
			return nil
		}
		s = s[:i]
	}
}

func (f *FakeFS) RemoveAll(p domain.Path) error {
	prefix := string(p)
	delete(f.dirs, p)
	for d := range f.dirs {
		if strings.HasPrefix(string(d), prefix+"/") {
			delete(f.dirs, d)
		}
	}
	for file := range f.files {
		if string(file) == prefix || strings.HasPrefix(string(file), prefix+"/") {
			delete(f.files, file)
		}
	}
	return nil
}

// ListDirs returns the names of p's direct subdirectories. p that was never
// registered as a directory at all (nobody ever called MkdirAll on it, or
// on anything under it) reports an empty list, not an error — mirroring
// internal/adapters/fsstore.Adapter's own ListDirs, which translates
// os.ReadDir's ErrNotExist into (nil, nil) for the exact same reason: a
// workspaces_root that does not exist yet is the normal state of a
// brand-new context with zero workspaces, not an operational failure
// (CRITICAL-3, verify-report.md).
//
// This method used to scan every known directory by string prefix without
// ever checking whether p itself was a registered directory, which
// happened to return an empty list for a missing p purely by accident —
// indistinguishable, to any caller, from "p exists and is genuinely
// empty". That accidental agreement is exactly why no test built on this
// fake could ever see CRITICAL-3: every fixture that seeded at least one
// workspace also implicitly, coincidentally "created" its root, and the
// one genuinely-missing-root case was never distinguishable from a
// deliberate empty answer. This method now checks f.dirs[p] explicitly so
// a caller can construct a FakeFS that has truly never touched p (no
// MkdirAll call anywhere under it) and get the same well-defined, tested
// answer the real, fixed adapter gives.
func (f *FakeFS) ListDirs(p domain.Path) ([]string, error) {
	if !f.dirs[p] {
		return nil, nil
	}
	prefix := string(p) + "/"
	var out []string
	for d := range f.dirs {
		s := string(d)
		rest, ok := strings.CutPrefix(s, prefix)
		if ok && rest != "" && !strings.Contains(rest, "/") {
			out = append(out, rest)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ListFiles returns the names of the files written directly under p,
// sorted.
func (f *FakeFS) ListFiles(p domain.Path) ([]string, error) {
	prefix := string(p) + "/"
	var out []string
	for file := range f.files {
		rest, ok := strings.CutPrefix(string(file), prefix)
		if ok && rest != "" && !strings.Contains(rest, "/") {
			out = append(out, rest)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (f *FakeFS) ReadFile(p domain.Path) ([]byte, error) {
	data, ok := f.files[p]
	if !ok {
		return nil, fmt.Errorf("fakefs: %q does not exist", p)
	}
	return append([]byte(nil), data...), nil
}

func (f *FakeFS) WriteFile(p domain.Path, data []byte, _ fs.FileMode) error {
	if f.DenyWrite != nil && f.DenyWrite(p) {
		return fmt.Errorf("fakefs: permission denied writing %q", p)
	}
	f.files[p] = append([]byte(nil), data...)
	f.modTimes[p] = time.Now()
	f.Writes = append(f.Writes, p)
	return nil
}

func (f *FakeFS) CopyFile(src, dst domain.Path) error {
	data, ok := f.files[src]
	if !ok {
		return fmt.Errorf("fakefs: copy source %q does not exist", src)
	}
	return f.WriteFile(dst, data, 0)
}

func (f *FakeFS) ModTime(p domain.Path) (time.Time, error) {
	t, ok := f.modTimes[p]
	if !ok {
		return time.Time{}, fmt.Errorf("fakefs: %q has no recorded mod time", p)
	}
	return t, nil
}

// FindEnvFiles returns every recorded file under root whose base name looks
// like an env file (".env" or ".env.<suffix>"), skipping any path with a
// component listed in pruneDirs. Paths are returned repo-relative.
func (f *FakeFS) FindEnvFiles(root domain.Path, pruneDirs []string) ([]string, error) {
	prune := make(map[string]bool, len(pruneDirs))
	for _, d := range pruneDirs {
		prune[d] = true
	}

	prefix := string(root) + "/"
	var out []string
	for file := range f.files {
		s := string(file)
		rel, ok := strings.CutPrefix(s, prefix)
		if !ok {
			continue
		}
		parts := strings.Split(rel, "/")
		pruned := false
		for _, part := range parts[:len(parts)-1] {
			if prune[part] {
				pruned = true
				break
			}
		}
		if pruned {
			continue
		}
		base := parts[len(parts)-1]
		if base == ".env" || strings.HasPrefix(base, ".env.") {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out, nil
}

// WalkGitRepos implements ports.FileSystemPort.WalkGitRepos against this
// fake's own in-memory dirs/files maps: it drives ports.WalkGitReposUsing,
// the exact algorithm internal/adapters/fsstore.Adapter's own real
// implementation shares, over f.ListDirs/f.IsGitDirEntry. Sharing the
// walk/prune/depth-cap algorithm with the real adapter (rather than each
// reimplementing it) is what lets a test built on this fake actually catch
// a defect in that algorithm, instead of only ever exercising a
// hand-rolled imitation of it — see WalkGitReposUsing's own doc comment.
//
// A nested tree is built on this fake exactly like on real disk: MkdirAll
// marks every ancestor directory as it goes (see MkdirAll's own doc
// comment), so "root/team/api" is a genuine three-level structure here, not
// a flat key. A main clone is built by MkdirAll-ing "<dir>/.git"; a linked
// worktree, by WriteFile-ing "<dir>/.git" as a file — exactly the two
// shapes IsGitDirEntry already distinguishes.
func (f *FakeFS) WalkGitRepos(root domain.Path, maxDepth int, prune func(name string) bool) (ports.WalkGitReposResult, error) {
	return ports.WalkGitReposUsing(f, root, maxDepth, prune)
}

func (f *FakeFS) WalkUp(start domain.Path, marker string) (domain.Path, bool, error) {
	cur := start
	for {
		candidate := cur.Join(marker)
		if f.dirs[candidate] {
			return cur, true, nil
		}
		if _, ok := f.files[candidate]; ok {
			return cur, true, nil
		}
		parent := cur.Join("..")
		// "/" is its own parent; a Windows volume ("C:") climbs to "." and
		// then into "..", "../..", ... forever, so stop there too.
		if parent == cur || parent == "." || strings.HasPrefix(string(parent), "..") {
			return "", false, nil
		}
		cur = parent
	}
}
