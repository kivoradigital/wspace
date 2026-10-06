// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest_test

import (
	"errors"
	"testing"

	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// The fake must behave like a real filesystem for symlinks, or the agent
// installation tests prove nothing.
func TestFakeAgentFS_SymlinksBehaveLikeTheRealFilesystem(t *testing.T) {
	f := portstest.NewFakeAgentFS()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.MkdirAll("/app/skills/s"))
	must(f.WriteFile("/app/skills/s/SKILL.md", []byte("x"), 0o644))
	must(f.MkdirAll("/home/.claude/skills"))

	if err := f.Symlink("/app/skills/s", "/nope/link"); err == nil {
		t.Fatal("Symlink into a missing parent must fail")
	}
	must(f.Symlink("/app/skills/s", "/home/.claude/skills/s"))
	if err := f.Symlink("/app/skills/s", "/home/.claude/skills/s"); err == nil {
		t.Fatal("Symlink over an existing entry must fail")
	}

	e, err := f.Lstat("/home/.claude/skills/s")
	must(err)
	if e.Kind != ports.EntrySymlink || e.LinkTarget != "/app/skills/s" {
		t.Fatalf("Lstat = %+v", e)
	}
	if ok, _ := f.Exists("/home/.claude/skills/s/SKILL.md"); !ok {
		t.Fatal("a file must be reachable through a directory link")
	}
	if data, err := f.ReadFile("/home/.claude/skills/s/SKILL.md"); err != nil || string(data) != "x" {
		t.Fatalf("ReadFile through link = %q, %v", data, err)
	}
	if got, err := f.EvalSymlinks("/home/.claude/skills/s"); err != nil || got != "/app/skills/s" {
		t.Fatalf("EvalSymlinks = %q, %v", got, err)
	}

	if err := f.RemoveLink("/app/skills/s"); !errors.Is(err, ports.ErrNotASymlink) {
		t.Fatalf("RemoveLink(dir) = %v, want ErrNotASymlink", err)
	}
	must(f.RemoveLink("/home/.claude/skills/s"))
	if ok, _ := f.Exists("/app/skills/s/SKILL.md"); !ok {
		t.Fatal("removing a link must not touch its target")
	}

	must(f.Symlink("/gone", "/home/.claude/skills/dangling"))
	if ok, _ := f.Exists("/home/.claude/skills/dangling"); ok {
		t.Fatal("a dangling link does not exist")
	}

	must(f.MkdirAll("/home/.claude/skills/real/sub"))
	must(f.WriteFile("/home/.claude/skills/real/sub/a", []byte("a"), 0o644))
	must(f.MkdirAll("/home/.claude/backup"))
	must(f.Rename("/home/.claude/skills/real", "/home/.claude/backup/real"))
	if data, _ := f.ReadFile("/home/.claude/backup/real/sub/a"); string(data) != "a" {
		t.Fatal("Rename must move the whole tree")
	}
	if e, _ := f.Lstat("/home/.claude/skills/real"); e.Kind != ports.EntryMissing {
		t.Fatal("Rename must leave nothing behind")
	}

	f.SymlinkUnsupported = true
	if err := f.Symlink("/app/skills/s", "/home/.claude/skills/s"); !errors.Is(err, ports.ErrSymlinkUnsupported) {
		t.Fatalf("Symlink with SymlinkUnsupported = %v", err)
	}
	must(f.CopyDir("/app/skills/s", "/home/.claude/skills/s"))
	if e, _ := f.Lstat("/home/.claude/skills/s"); e.Kind != ports.EntryDir {
		t.Fatal("CopyDir must create a real directory")
	}
}
