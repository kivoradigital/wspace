// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TestCLI_Add_Rm_Repair_SyncEnv_Exec_Doctor_Jump covers tasks.md 4b.13: one
// case per remaining command not already covered by its own test file.
func TestCLI_Add_Rm_Repair_SyncEnv_Exec_Doctor_Jump(t *testing.T) {
	t.Run("add mounts a second project", func(t *testing.T) {
		fx := newFixture(t)
		fx.Store.PutContext(domain.Context{
			Name:           "work",
			WorkspacesRoot: "/fixture/workspaces",
			Projects: []domain.Project{
				{Key: "svc", SourceDir: "/fixture/src/svc"},
				{Key: "web", SourceDir: "/fixture/src/web"},
			},
		})
		wsRoot := domain.Path("/fixture/workspaces/ws1")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{Name: "ws1", Root: wsRoot, Branch: "feature-x"}})

		stdout, stderr, code := run(fx.RT, "", "add", "ws1", "web")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		if !strings.Contains(stdout, "web") {
			t.Fatalf("stdout = %q, want it to mention the added repo", stdout)
		}
		m, err := fx.Store.LoadManifest(context.Background(), wsRoot)
		if err != nil {
			t.Fatalf("LoadManifest: %v", err)
		}
		if len(m.Workspace.Repos) != 1 || m.Workspace.Repos[0].Alias != "web" {
			t.Fatalf("manifest repos = %+v, want one repo aliased web", m.Workspace.Repos)
		}
	})

	t.Run("rm removes a mounted repo", func(t *testing.T) {
		fx := newFixture(t)
		wsRoot := domain.Path("/fixture/workspaces/ws1")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{
			Name: "ws1", Root: wsRoot,
			Repos: []domain.RepoEntry{{Alias: "svc", SourceDir: "/fixture/src/svc"}},
		}})

		_, stderr, code := run(fx.RT, "", "rm", "ws1", "svc", "--force")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		m, err := fx.Store.LoadManifest(context.Background(), wsRoot)
		if err != nil {
			t.Fatalf("LoadManifest: %v", err)
		}
		if len(m.Workspace.Repos) != 0 {
			t.Fatalf("manifest repos = %+v, want none", m.Workspace.Repos)
		}
	})

	t.Run("repair recreates a missing worktree", func(t *testing.T) {
		fx := newFixture(t)
		wsRoot := domain.Path("/fixture/workspaces/ws1")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{
			Name: "ws1", Root: wsRoot,
			Repos: []domain.RepoEntry{{Alias: "svc", SourceDir: "/fixture/src/svc", Branch: "feature-x"}},
		}})

		stdout, stderr, code := run(fx.RT, "", "repair", "ws1")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		if !strings.Contains(stdout, "svc") {
			t.Fatalf("stdout = %q, want it to mention the recreated worktree", stdout)
		}
		found := false
		for _, c := range fx.Git.Calls {
			if c.Method == "WorktreeAdd" {
				found = true
			}
		}
		if !found {
			t.Fatalf("git.Calls = %+v, want a WorktreeAdd call", fx.Git.Calls)
		}
	})

	t.Run("sync-env reports how many files were re-copied", func(t *testing.T) {
		fx := newFixture(t)
		wsRoot := domain.Path("/fixture/workspaces/ws1")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{Name: "ws1", Root: wsRoot}})

		stdout, stderr, code := run(fx.RT, "", "sync-env", "ws1")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		if !strings.Contains(stdout, "0") {
			t.Fatalf("stdout = %q, want it to report zero files re-synced (no repos mounted)", stdout)
		}
	})

	t.Run("exec propagates the last non-zero exit code", func(t *testing.T) {
		fx := newFixture(t)
		wsRoot := domain.Path("/fixture/workspaces/ws1")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{
			Name: "ws1", Root: wsRoot,
			Repos: []domain.RepoEntry{{Alias: "svc", SourceDir: "/fixture/src/svc"}},
		}})

		_, _, code := run(fx.RT, "", "exec", "ws1", "--", "false-command-does-not-exist")
		if code == 0 {
			t.Fatal("exit code = 0, want non-zero (the command does not exist)")
		}
	})

	t.Run("exec with no command after -- is rejected", func(t *testing.T) {
		fx := newFixture(t)
		_, stderr, code := run(fx.RT, "", "exec", "ws1", "--")
		if code == 0 {
			t.Fatal("exit code = 0, want non-zero (no command given)")
		}
		if !strings.Contains(stderr, "usage") {
			t.Fatalf("stderr = %q, want a usage hint", stderr)
		}
	})

	t.Run("doctor completes cleanly with no workspaces", func(t *testing.T) {
		fx := newFixture(t)
		_, stderr, code := run(fx.RT, "", "doctor")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
	})

	t.Run("jump prints only the resolved path", func(t *testing.T) {
		fx := newFixture(t)
		wsRoot := domain.Path("/fixture/workspaces/ws1")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{Name: "ws1", Root: wsRoot}})

		stdout, stderr, code := run(fx.RT, "", "jump", "ws1")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		if stdout != "/fixture/workspaces/ws1\n" {
			t.Fatalf("stdout = %q, want exactly the resolved path and nothing else", stdout)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty (stdout is not a TTY in this test)", stderr)
		}
	})

	t.Run("jump on an unknown workspace fails", func(t *testing.T) {
		fx := newFixture(t)
		_, _, code := run(fx.RT, "", "jump", "does-not-exist")
		if code == 0 {
			t.Fatal("exit code = 0, want non-zero")
		}
	})
}

// doctor is what a user runs when nothing works: it must report a missing
// git even before any context exists, instead of only "no context".
func TestCLI_Doctor_ReportsMissingGitWithoutAContext(t *testing.T) {
	fx := newFixture(t)
	if err := fx.Store.DeleteContext(context.Background(), "work"); err != nil {
		t.Fatal(err)
	}
	if err := fx.Store.SaveRoot(context.Background(), domain.RootConfig{}); err != nil {
		t.Fatal(err)
	}
	fx.Git.VersionFunc = func() (ports.Version, error) {
		return ports.Version{}, domain.NewOpError("git.lookup", domain.CodeGitMissing, "", "", nil)
	}

	stdout, stderr, _ := run(fx.RT, "", "doctor")
	out := stdout + stderr
	if !strings.Contains(out, "git") || !strings.Contains(out, "git_missing") && !strings.Contains(strings.ToLower(out), "not found") {
		t.Fatalf("doctor output = %q, want it to report the missing git", out)
	}
	if !strings.Contains(out, "context") {
		t.Fatalf("doctor output = %q, want it to still mention the missing context", out)
	}
}
