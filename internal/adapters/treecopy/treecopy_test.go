// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package treecopy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// nodeModules builds a small node_modules: a package file, an executable
// .bin script, a pnpm-style relative directory symlink and a dangling one.
func nodeModules(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "node_modules")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(src, ".pnpm", "left-pad@1.0.0", "node_modules", "left-pad"), 0o755))
	must(os.MkdirAll(filepath.Join(src, ".bin"), 0o755))
	must(os.WriteFile(filepath.Join(src, ".pnpm", "left-pad@1.0.0", "node_modules", "left-pad", "index.js"), []byte("module.exports = 1\n"), 0o644))
	must(os.WriteFile(filepath.Join(src, ".bin", "tool"), []byte("#!/bin/sh\necho hi\n"), 0o755))
	if runtime.GOOS != "windows" {
		must(os.Symlink(filepath.Join(".pnpm", "left-pad@1.0.0", "node_modules", "left-pad"), filepath.Join(src, "left-pad")))
		must(os.Symlink("missing-target", filepath.Join(src, "dangling")))
	}
	return src
}

func checkCopy(t *testing.T, dst string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dst, ".pnpm", "left-pad@1.0.0", "node_modules", "left-pad", "index.js"))
	if err != nil || string(data) != "module.exports = 1\n" {
		t.Fatalf("index.js = %q, %v", data, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(filepath.Join(dst, ".bin", "tool"))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("tool mode = %v, %v; want executable", info, err)
	}
	for name, want := range map[string]string{
		"left-pad": filepath.Join(".pnpm", "left-pad@1.0.0", "node_modules", "left-pad"),
		"dangling": "missing-target",
	} {
		li, err := os.Lstat(filepath.Join(dst, name))
		if err != nil || li.Mode()&fs.ModeSymlink == 0 {
			t.Fatalf("%s: %v, %v; want a symlink (not followed)", name, li, err)
		}
		if got, _ := os.Readlink(filepath.Join(dst, name)); got != want {
			t.Fatalf("%s -> %q, want %q", name, got, want)
		}
	}
}

func TestCopyTree_PreservesSymlinksAndModes(t *testing.T) {
	src := nodeModules(t)
	dst := filepath.Join(t.TempDir(), "node_modules")
	if err := copyTree(src, dst, nil); err != nil {
		t.Fatal(err)
	}
	checkCopy(t, dst)
}

func TestCloneTree_UsesTheFastestMethodAndPreservesTheTree(t *testing.T) {
	src := nodeModules(t)
	dst := filepath.Join(t.TempDir(), "wt", "node_modules")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	method, err := New().CloneTree(domain.Path(filepath.ToSlash(src)), domain.Path(filepath.ToSlash(dst)))
	if err != nil {
		t.Fatal(err)
	}
	checkCopy(t, dst)
	switch runtime.GOOS {
	case "darwin":
		// t.TempDir is on the APFS system volume: clonefile must be used.
		if method != MethodClone {
			t.Fatalf("method = %q, want %q", method, MethodClone)
		}
	case "linux":
		if method != MethodReflink && method != MethodCopy {
			t.Fatalf("method = %q", method)
		}
	default:
		if method != MethodCopy {
			t.Fatalf("method = %q", method)
		}
	}
}

func TestCloneTree_RefusesAnExistingDestination(t *testing.T) {
	src := nodeModules(t)
	dst := t.TempDir()
	_, err := New().CloneTree(domain.Path(filepath.ToSlash(src)), domain.Path(filepath.ToSlash(dst)))
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("err = %v, want fs.ErrExist", err)
	}
}
