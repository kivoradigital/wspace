// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package agenthost_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/adapters/agenthost"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

func p(s string) domain.Path { return domain.Path(filepath.ToSlash(s)) }

func TestFS_LinksMovesAndSafeRemoval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need Developer Mode on Windows")
	}
	root := t.TempDir()
	fs := agenthost.NewFS()
	src := filepath.Join(root, "app", "skills", "s")
	skills := filepath.Join(root, "home", ".claude", "skills")
	link := filepath.Join(skills, "s")

	if err := fs.MkdirAll(p(src)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile(p(filepath.Join(src, "SKILL.md")), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if e, err := fs.Lstat(p(link)); err != nil || e.Kind != ports.EntryMissing {
		t.Fatalf("Lstat(missing) = %+v, %v", e, err)
	}
	if err := fs.MkdirAll(p(skills)); err != nil {
		t.Fatal(err)
	}
	if err := fs.Symlink(p(src), p(link)); err != nil {
		t.Fatal(err)
	}
	e, err := fs.Lstat(p(link))
	if err != nil || e.Kind != ports.EntrySymlink || e.LinkTarget != filepath.ToSlash(src) {
		t.Fatalf("Lstat(link) = %+v, %v", e, err)
	}
	if ok, _ := fs.Exists(p(filepath.Join(link, "SKILL.md"))); !ok {
		t.Fatal("file not reachable through the link")
	}
	real, err := fs.EvalSymlinks(p(link))
	if err != nil {
		t.Fatal(err)
	}
	wantReal, _ := filepath.EvalSymlinks(src)
	if string(real) != filepath.ToSlash(wantReal) {
		t.Fatalf("EvalSymlinks = %q, want %q", real, wantReal)
	}

	if err := fs.RemoveLink(p(src)); !errors.Is(err, ports.ErrNotASymlink) {
		t.Fatalf("RemoveLink(dir) = %v", err)
	}
	if err := fs.RemoveLink(p(link)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err != nil {
		t.Fatal("removing the link touched its target")
	}

	moved := filepath.Join(root, "moved")
	if err := fs.Rename(p(src), p(moved)); err != nil {
		t.Fatal(err)
	}
	copyDst := filepath.Join(root, "copy")
	if err := fs.CopyDir(p(moved), p(copyDst)); err != nil {
		t.Fatal(err)
	}
	if data, err := fs.ReadFile(p(filepath.Join(copyDst, "SKILL.md"))); err != nil || string(data) != "x" {
		t.Fatalf("copied SKILL.md = %q, %v", data, err)
	}
	if err := fs.CopyDir(p(moved), p(copyDst)); err == nil {
		t.Fatal("CopyDir onto an existing directory must fail")
	}
}

func TestRunner_LookPathRunAndTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	r := agenthost.NewRunner(200 * time.Millisecond)
	if _, ok := r.LookPath("definitely-not-a-program-wspace"); ok {
		t.Fatal("LookPath found a missing program")
	}
	sh, ok := r.LookPath("sh")
	if !ok {
		t.Skip("no sh on PATH")
	}
	res, err := r.Run(context.Background(), sh, "-c", "echo out; echo err >&2; exit 3")
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 || res.Stdout != "out\n" || res.Stderr != "err\n" {
		t.Fatalf("Run = %+v", res)
	}
	if _, err := r.Run(context.Background(), sh, "-c", "sleep 5"); err == nil {
		t.Fatal("Run must time out")
	}
}

func TestRunner_RunInSetsTheWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	r := agenthost.NewRunner(2 * time.Second)
	sh, ok := r.LookPath("sh")
	if !ok {
		t.Skip("no sh on PATH")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.RunIn(context.Background(), filepath.ToSlash(dir), sh, "-c", "pwd -P")
	if err != nil || strings.TrimSpace(res.Stdout) != dir {
		t.Fatalf("RunIn = %+v, %v; want %s", res, err, dir)
	}
}
