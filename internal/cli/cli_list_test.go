// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// TestCLI_List_JSONContract covers tasks.md 4b.7: list --json's schema
// (cli-surface spec: "name, path, project_count" per workspace), golden
// file normalized against a real FakeFS-derived (t.TempDir()-based) root
// so normalizeGolden's tmp-path replacement is genuinely exercised, not
// merely present.
func TestCLI_List_JSONContract(t *testing.T) {
	fx := newFixture(t)

	root := fx.FS.Paths().Home.Join("workspaces")
	fx.Store.PutManifest(root.Join("ws1"), domain.Manifest{
		Workspace: domain.Workspace{Name: "ws1", Root: root.Join("ws1"), Repos: []domain.RepoEntry{{Alias: "svc"}}},
	})
	fx.Store.PutManifest(root.Join("ws2"), domain.Manifest{
		Workspace: domain.Workspace{Name: "ws2", Root: root.Join("ws2"), Repos: []domain.RepoEntry{{Alias: "svc"}, {Alias: "web"}}},
	})
	if err := fx.FS.MkdirAll(root.Join("ws1")); err != nil {
		t.Fatalf("MkdirAll ws1: %v", err)
	}
	if err := fx.FS.MkdirAll(root.Join("ws2")); err != nil {
		t.Fatalf("MkdirAll ws2: %v", err)
	}
	fx.Store.PutContext(domain.Context{Name: "work", WorkspacesRoot: root})
	if err := fx.Store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	stdout, stderr, code := run(fx.RT, "", "list", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	compareGolden(t, "list.json.golden", stdout)
}

// TestCLI_List_HumanOutputEmpty covers the empty case's human rendering.
func TestCLI_List_HumanOutputEmpty(t *testing.T) {
	fx := newFixture(t)

	stdout, stderr, code := run(fx.RT, "", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if stdout != "no workspaces found\n" {
		t.Fatalf("stdout = %q, want the empty-list message", stdout)
	}
}

// TestCLI_List_OneBrokenWorkspaceStillListsHealthyOnes covers the CLI-level
// half of phase 6's disclosed defect (app.List used to abort entirely on
// the first per-workspace collection error): `ws list` must still print
// every healthy workspace, with the damaged one marked rather than
// silently missing or blanking the whole command's output.
func TestCLI_List_OneBrokenWorkspaceStillListsHealthyOnes(t *testing.T) {
	fx := newFixture(t)
	root := domain.Path("/fixture/workspaces")

	seed := func(name string) domain.Path {
		wsRoot := root.Join(name)
		fx.Store.PutManifest(wsRoot, domain.Manifest{
			Workspace: domain.Workspace{Name: name, Root: wsRoot, Repos: []domain.RepoEntry{{Alias: "api"}}},
		})
		if err := fx.FS.MkdirAll(wsRoot); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		return wsRoot
	}
	seed("healthy")
	brokenRoot := seed("broken")

	fx.Git.CurrentBranchFunc = func(worktree domain.Path) (domain.BranchName, bool, error) {
		if worktree == brokenRoot.Join("api") {
			return "", false, domain.NewOpError("git.current_branch", domain.CodeWorktreeMissing, "", "", nil)
		}
		return "main", false, nil
	}

	stdout, stderr, code := run(fx.RT, "", "list")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "healthy") || !strings.Contains(stdout, "broken") {
		t.Fatalf("stdout = %q, want both the healthy and the broken workspace listed", stdout)
	}
	if !strings.Contains(stdout, "unable to load status") {
		t.Fatalf("stdout = %q, want the damaged workspace's row marked", stdout)
	}

	stdoutJSON, stderrJSON, code := run(fx.RT, "", "list", "--json")
	if code != 0 {
		t.Fatalf("--json exit code = %d, want 0; stderr=%q", code, stderrJSON)
	}
	if !strings.Contains(stdoutJSON, `"error"`) {
		t.Fatalf("--json stdout = %q, want an \"error\" field for the damaged workspace", stdoutJSON)
	}
	if !strings.Contains(stdoutJSON, `"healthy"`) {
		t.Fatalf("--json stdout = %q, want the healthy workspace still present", stdoutJSON)
	}
}
