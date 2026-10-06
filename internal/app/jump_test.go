// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func TestJump_ResolvesWorkspacePathWithoutMutatingShell(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	store.PutManifest(fs.Paths().Home.Join("workspaces", "ws1"), domain.Manifest{Workspace: domain.Workspace{Name: "ws1"}})
	before, err := fs.Cwd()
	if err != nil {
		t.Fatalf("Cwd() error: %v", err)
	}

	deps := app.Deps{Store: store, FS: fs}
	in := app.JumpInput{WorkspacesRoot: fs.Paths().Home.Join("workspaces"), Name: "ws1"}

	got, err := app.Jump(context.Background(), deps, in)
	if err != nil {
		t.Fatalf("Jump() unexpected error: %v", err)
	}
	want := fs.Paths().Home.Join("workspaces", "ws1")
	if got != want {
		t.Fatalf("Jump() = %q, want %q", got, want)
	}

	// Jump must never change the process's (or FakeFS's) working
	// directory: only the shell function ShellInit renders may `cd`
	// (workspace-lifecycle / shell-integration: "Jump ... MUST NOT attempt
	// to change any process's working directory").
	after, err := fs.Cwd()
	if err != nil {
		t.Fatalf("Cwd() error: %v", err)
	}
	if after != before {
		t.Fatalf("cwd changed from %q to %q; Jump must not mutate it", before, after)
	}
}

func TestJump_UnknownWorkspaceErrors(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	deps := app.Deps{Store: store, FS: fs}

	_, err := app.Jump(context.Background(), deps, app.JumpInput{WorkspacesRoot: fs.Paths().Home.Join("workspaces"), Name: "missing"})
	if err == nil {
		t.Fatal("Jump() error = nil for an unknown workspace, want an error")
	}
	if got := domain.Code(err); got != domain.CodeWorkspaceNotFound {
		t.Fatalf("domain.Code(err) = %q, want %q", got, domain.CodeWorkspaceNotFound)
	}
}

func TestShellInit_ReturnsPOSIXAndFish(t *testing.T) {
	result := app.ShellInit()
	if result.POSIX == "" || result.Fish == "" {
		t.Fatalf("ShellInit() = %+v, want both POSIX and Fish non-empty", result)
	}
	if !containsAll(result.POSIX, "wspace()", "cd ") {
		t.Fatalf("POSIX function = %q, want a wspace() function that cd's", result.POSIX)
	}
	if !containsAll(result.Fish, "function ws", "cd ") {
		t.Fatalf("fish function = %q, want a ws function that cd's", result.Fish)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
