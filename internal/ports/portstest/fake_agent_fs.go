// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// FakeAgentFS is an in-memory ports.AgentFS with real symlink semantics:
// links are followed in every path component except the last one for
// Lstat/RemoveLink/Rename, dangling links do not exist, and removing a
// link never touches its target. SymlinkUnsupported simulates a Windows
// account that cannot create symlinks.
type FakeAgentFS struct {
	mu    sync.Mutex
	dirs  map[string]bool
	files map[string][]byte
	links map[string]string

	SymlinkUnsupported bool
}

var _ ports.AgentFS = (*FakeAgentFS)(nil)

// NewFakeAgentFS returns a filesystem holding only "/".
func NewFakeAgentFS() *FakeAgentFS {
	return &FakeAgentFS{dirs: map[string]bool{"/": true}, files: map[string][]byte{}, links: map[string]string{}}
}

// resolveParent follows every symlink in p's directory part, leaving the
// final component as is.
func (f *FakeAgentFS) resolveParent(p string) (string, error) {
	p = path.Clean(p)
	if p == "/" {
		return p, nil
	}
	dir, err := f.resolve(path.Dir(p))
	if err != nil {
		return "", err
	}
	return path.Join(dir, path.Base(p)), nil
}

// resolve follows every symlink in p, including the last component.
func (f *FakeAgentFS) resolve(p string) (string, error) {
	p = path.Clean(p)
	for hops := 0; hops < 40; hops++ {
		parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
		cur := "/"
		changed := false
		for i, part := range parts {
			if part == "" {
				continue
			}
			next := path.Join(cur, part)
			if target, ok := f.links[next]; ok {
				if !path.IsAbs(target) {
					target = path.Join(cur, target)
				}
				p = path.Join(append([]string{target}, parts[i+1:]...)...)
				changed = true
				break
			}
			cur = next
		}
		if !changed {
			return p, nil
		}
	}
	return "", fmt.Errorf("too many levels of symbolic links: %s", p)
}

func (f *FakeAgentFS) Lstat(p domain.Path) (ports.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, err := f.resolveParent(string(p))
	if err != nil {
		return ports.Entry{}, err
	}
	switch {
	case f.isLink(rp):
		return ports.Entry{Kind: ports.EntrySymlink, LinkTarget: f.links[rp]}, nil
	case f.dirs[rp]:
		return ports.Entry{Kind: ports.EntryDir}, nil
	case f.hasFile(rp):
		return ports.Entry{Kind: ports.EntryFile}, nil
	}
	return ports.Entry{Kind: ports.EntryMissing}, nil
}

func (f *FakeAgentFS) Exists(p domain.Path) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, err := f.resolve(string(p))
	if err != nil {
		return false, nil
	}
	return f.dirs[rp] || f.hasFile(rp), nil
}

func (f *FakeAgentFS) EvalSymlinks(p domain.Path) (domain.Path, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, err := f.resolve(string(p))
	if err != nil {
		return "", err
	}
	if !f.dirs[rp] && !f.hasFile(rp) {
		return "", fmt.Errorf("lstat %s: %w", p, fs.ErrNotExist)
	}
	return domain.Path(rp), nil
}

func (f *FakeAgentFS) MkdirAll(p domain.Path) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, err := f.resolve(string(p))
	if err != nil {
		return err
	}
	for d := rp; ; d = path.Dir(d) {
		if f.hasFile(d) {
			return fmt.Errorf("mkdir %s: not a directory", d)
		}
		f.dirs[d] = true
		// "." ends a drive-letter path: "C:/x" -> "C:" -> ".".
		if d == "/" || d == "." {
			return nil
		}
	}
}

func (f *FakeAgentFS) Symlink(target, link domain.Path) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SymlinkUnsupported {
		return ports.ErrSymlinkUnsupported
	}
	rl, err := f.resolveParent(string(link))
	if err != nil {
		return err
	}
	if !f.dirs[path.Dir(rl)] {
		return fmt.Errorf("symlink %s: parent: %w", link, fs.ErrNotExist)
	}
	if f.existsRaw(rl) {
		return fmt.Errorf("symlink %s: %w", link, fs.ErrExist)
	}
	f.links[rl] = string(target)
	return nil
}

func (f *FakeAgentFS) existsRaw(p string) bool {
	return f.dirs[p] || f.hasFile(p) || f.isLink(p)
}

func (f *FakeAgentFS) Rename(from, to domain.Path) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rf, err := f.resolveParent(string(from))
	if err != nil {
		return err
	}
	rt, err := f.resolveParent(string(to))
	if err != nil {
		return err
	}
	if !f.existsRaw(rf) {
		return fmt.Errorf("rename %s: %w", from, fs.ErrNotExist)
	}
	if !f.dirs[path.Dir(rt)] {
		return fmt.Errorf("rename to %s: parent: %w", to, fs.ErrNotExist)
	}
	if f.existsRaw(rt) {
		return fmt.Errorf("rename to %s: %w", to, fs.ErrExist)
	}
	move := func(old string) string { return rt + strings.TrimPrefix(old, rf) }
	under := func(p string) bool { return p == rf || strings.HasPrefix(p, rf+"/") }
	for d := range f.dirs {
		if under(d) {
			delete(f.dirs, d)
			f.dirs[move(d)] = true
		}
	}
	for p, data := range f.files {
		if under(p) {
			delete(f.files, p)
			f.files[move(p)] = data
		}
	}
	for p, target := range f.links {
		if under(p) {
			delete(f.links, p)
			f.links[move(p)] = target
		}
	}
	return nil
}

func (f *FakeAgentFS) RemoveLink(p domain.Path) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, err := f.resolveParent(string(p))
	if err != nil {
		return err
	}
	if !f.isLink(rp) {
		return fmt.Errorf("%s: %w", p, ports.ErrNotASymlink)
	}
	delete(f.links, rp)
	return nil
}

func (f *FakeAgentFS) RemoveAll(p domain.Path) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, err := f.resolveParent(string(p))
	if err != nil {
		return err
	}
	under := func(x string) bool { return x == rp || strings.HasPrefix(x, rp+"/") }
	for d := range f.dirs {
		if under(d) {
			delete(f.dirs, d)
		}
	}
	for x := range f.files {
		if under(x) {
			delete(f.files, x)
		}
	}
	for x := range f.links {
		if under(x) {
			delete(f.links, x)
		}
	}
	return nil
}

func (f *FakeAgentFS) ReadFile(p domain.Path) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, err := f.resolve(string(p))
	if err != nil {
		return nil, err
	}
	data, ok := f.files[rp]
	if !ok {
		return nil, fmt.Errorf("open %s: %w", p, fs.ErrNotExist)
	}
	return append([]byte(nil), data...), nil
}

func (f *FakeAgentFS) WriteFile(p domain.Path, data []byte, _ fs.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rp, err := f.resolve(string(p))
	if err != nil {
		return err
	}
	if !f.dirs[path.Dir(rp)] {
		return fmt.Errorf("open %s: parent: %w", p, fs.ErrNotExist)
	}
	if f.dirs[rp] {
		return fmt.Errorf("open %s: is a directory", p)
	}
	f.files[rp] = append([]byte(nil), data...)
	return nil
}

func (f *FakeAgentFS) CopyDir(src, dst domain.Path) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rs, err := f.resolve(string(src))
	if err != nil {
		return err
	}
	rd, err := f.resolveParent(string(dst))
	if err != nil {
		return err
	}
	if !f.dirs[rs] {
		return fmt.Errorf("copy %s: not a directory", src)
	}
	if f.existsRaw(rd) {
		return fmt.Errorf("copy to %s: %w", dst, fs.ErrExist)
	}
	for d := range f.dirs {
		if d == rs || strings.HasPrefix(d, rs+"/") {
			f.dirs[rd+strings.TrimPrefix(d, rs)] = true
		}
	}
	for p, data := range f.files {
		if strings.HasPrefix(p, rs+"/") {
			f.files[rd+strings.TrimPrefix(p, rs)] = append([]byte(nil), data...)
		}
	}
	return nil
}

// Paths lists every entry (directories, files and links) at or below
// root, sorted; a debugging and assertion aid.
func (f *FakeAgentFS) Paths(root string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	add := func(p string) {
		if p == root || strings.HasPrefix(p, strings.TrimRight(root, "/")+"/") {
			out = append(out, p)
		}
	}
	for d := range f.dirs {
		add(d)
	}
	for p := range f.files {
		add(p)
	}
	for p := range f.links {
		add(p)
	}
	sort.Strings(out)
	return out
}

// FakeCommandRunner is a scripted ports.CommandRunner. OnPath maps a
// program name to its absolute path; Results maps "name arg1 arg2…" to
// what Run returns (a missing entry exits 0 with no output). Calls records
// every Run and RunIn, in order; Dirs holds each call's working directory
// ("" for Run).
type FakeCommandRunner struct {
	mu      sync.Mutex
	OnPath  map[string]string
	Results map[string]ports.CommandResult
	Err     error
	Calls   []string
	Dirs    []string
}

var _ ports.CommandRunner = (*FakeCommandRunner)(nil)

// NewFakeCommandRunner returns a runner with nothing on PATH.
func NewFakeCommandRunner() *FakeCommandRunner {
	return &FakeCommandRunner{OnPath: map[string]string{}, Results: map[string]ports.CommandResult{}}
}

func (r *FakeCommandRunner) LookPath(name string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.OnPath[name]
	return p, ok
}

func (r *FakeCommandRunner) Run(ctx context.Context, name string, args ...string) (ports.CommandResult, error) {
	return r.RunIn(ctx, "", name, args...)
}

func (r *FakeCommandRunner) RunIn(_ context.Context, dir, name string, args ...string) (ports.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	call := strings.Join(append([]string{name}, args...), " ")
	r.Calls = append(r.Calls, call)
	r.Dirs = append(r.Dirs, dir)
	if r.Err != nil {
		return ports.CommandResult{}, r.Err
	}
	return r.Results[call], nil
}

func (f *FakeAgentFS) hasFile(p string) bool { _, ok := f.files[p]; return ok }

func (f *FakeAgentFS) isLink(p string) bool { _, ok := f.links[p]; return ok }
