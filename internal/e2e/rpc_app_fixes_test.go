// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRPC_BranchPrefixWithoutSlashAgainstRealGit: a context prefix
// "feature" (no trailing slash) creates "feature/<name>", and a per-call
// options.branchPrefix (the app's branch type selector) wins.
func TestRPC_BranchPrefixWithoutSlashAgainstRealGit(t *testing.T) {
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	api := NewGitProject(t, projectsRoot, "api", "develop")

	responses, _ := rpcSession(t, fx,
		req("ctx", "contexts.create", map[string]any{"name": "pfx", "workspacesRoot": workspacesRoot, "activate": true, "defaults": map[string]any{"branchPrefix": "feature"}}),
		req("reg", "projects.register", map[string]any{"key": "api", "sourceDir": api.Main}),
		req("c1", "workspaces.create", map[string]any{"name": "test3"}),
		req("c2", "workspaces.create", map[string]any{"name": "login", "options": map[string]any{"branchPrefix": "fix/"}}),
	)
	for _, id := range []string{"ctx", "reg", "c1", "c2"} {
		mustResult(t, responses, id)
	}
	if got := CurrentBranch(t, filepath.Join(workspacesRoot, "test3", "api")); got != "feature/test3" {
		t.Fatalf("branch = %q, want feature/test3", got)
	}
	if got := CurrentBranch(t, filepath.Join(workspacesRoot, "login", "api")); got != "fix/login" {
		t.Fatalf("branch = %q, want fix/login", got)
	}
}

// TestRPC_RepoChangesAgainstRealGit lists modified, staged-renamed,
// deleted and untracked paths, including names with spaces.
func TestRPC_RepoChangesAgainstRealGit(t *testing.T) {
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	api := NewGitProject(t, projectsRoot, "api", "develop")
	for name, body := range map[string]string{"keep.txt": "k\n", "old name.txt": "o\n", "gone.txt": "g\n"} {
		if err := os.WriteFile(filepath.Join(api.Main, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, api.Main, "add", ".")
	runGit(t, api.Main, "commit", "-q", "-m", "files")
	runGit(t, api.Main, "push", "-q", "origin", "develop")

	responses, _ := rpcSession(t, fx,
		req("ctx", "contexts.create", map[string]any{"name": "chg", "workspacesRoot": workspacesRoot, "activate": true}),
		req("reg", "projects.register", map[string]any{"key": "api", "sourceDir": api.Main}),
		req("c", "workspaces.create", map[string]any{"name": "ws", "branch": "chg"}),
	)
	mustResult(t, responses, "c")
	wt := filepath.Join(workspacesRoot, "ws", "api")
	if err := os.WriteFile(filepath.Join(wt, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wt, "mv", "old name.txt", "new name.txt")
	if err := os.Remove(filepath.Join(wt, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wt, "dir with space"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "dir with space", "fresh file.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	responses, _ = rpcSession(t, fx, req("ch", "workspaces.repoChanges", map[string]any{"workspace": "ws", "repo": "api"}))
	var changes []struct {
		Path, OrigPath, Status string
		Staged, Unstaged       bool
	}
	if err := json.Unmarshal(mustResult(t, responses, "ch"), &changes); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range changes {
		got[c.Path] = c.Status
		if c.Status == "renamed" && (c.OrigPath != "old name.txt" || !c.Staged) {
			t.Fatalf("rename = %+v, want origPath \"old name.txt\" and staged", c)
		}
	}
	want := map[string]string{"README.md": "modified", "new name.txt": "renamed", "gone.txt": "deleted", "dir with space/fresh file.txt": "untracked"}
	for p, s := range want {
		if got[p] != s {
			t.Errorf("status of %q = %q, want %q (all %v)", p, got[p], s, got)
		}
	}
}

// TestRPC_DestroyRemovesTheWorkspaceAgainstRealGit covers a clean destroy,
// and a workspace with unpushed commits plus a dirty file: refused
// without force, then removed with force — directory, worktree
// registrations and list entry all gone.
func TestRPC_DestroyRemovesTheWorkspaceAgainstRealGit(t *testing.T) {
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	api := NewGitProject(t, projectsRoot, "api", "develop")
	web := NewGitProject(t, projectsRoot, "web", "develop")

	responses, _ := rpcSession(t, fx,
		req("ctx", "contexts.create", map[string]any{"name": "del", "workspacesRoot": workspacesRoot, "activate": true}),
		req("r1", "projects.register", map[string]any{"key": "api", "sourceDir": api.Main}),
		req("r2", "projects.register", map[string]any{"key": "web", "sourceDir": web.Main}),
		req("clean", "workspaces.create", map[string]any{"name": "clean", "branch": "clean-b"}),
		req("dirty", "workspaces.create", map[string]any{"name": "dirty", "branch": "dirty-b"}),
	)
	mustResult(t, responses, "clean")
	mustResult(t, responses, "dirty")

	dirtyAPI := filepath.Join(workspacesRoot, "dirty", "api")
	if err := os.WriteFile(filepath.Join(dirtyAPI, "work.txt"), []byte("w\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dirtyAPI, "add", "work.txt")
	runGit(t, dirtyAPI, "-c", "user.email=e2e@example.invalid", "-c", "user.name=e2e", "commit", "-q", "-m", "unpushed")
	if err := os.WriteFile(filepath.Join(workspacesRoot, "dirty", "web", "README.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	responses, _ = rpcSession(t, fx,
		req("tc", "workspaces.teardownCheck", map[string]any{"workspace": "clean"}),
		req("d-clean", "workspaces.destroy", map[string]any{"workspace": "clean"}),
		req("tc2", "workspaces.teardownCheck", map[string]any{"workspace": "dirty"}),
		req("d-dirty", "workspaces.destroy", map[string]any{"workspace": "dirty"}),
	)
	if string(mustResult(t, responses, "tc")) != "[]" {
		t.Fatalf("clean teardownCheck = %s, want []", responses["tc"].Result)
	}
	mustResult(t, responses, "d-clean")
	var blockers []map[string]any
	_ = json.Unmarshal(mustResult(t, responses, "tc2"), &blockers)
	if len(blockers) < 2 {
		t.Fatalf("dirty teardownCheck = %v, want the unpushed commit and the tracked change", blockers)
	}
	if r := responses["d-dirty"]; r.Error == nil || r.Error.Code != "needs_confirmation" {
		t.Fatalf("unforced dirty destroy = %+v, want needs_confirmation", r)
	}
	if !PathExists(t, filepath.Join(workspacesRoot, "dirty")) {
		t.Fatal("an unforced, refused destroy removed the workspace")
	}
	responses, _ = rpcSession(t, fx,
		req("d-force", "workspaces.destroy", map[string]any{"workspace": "dirty", "force": true, "deleteBranches": true}),
		req("list", "workspaces.list", map[string]any{}),
	)
	mustResult(t, responses, "d-force")
	if string(mustResult(t, responses, "list")) != "[]" {
		t.Fatalf("workspaces.list after destroying both = %s, want []", responses["list"].Result)
	}
	for _, ws := range []string{"clean", "dirty"} {
		if PathExists(t, filepath.Join(workspacesRoot, ws)) {
			t.Fatalf("workspace %s still on disk", ws)
		}
	}
	for _, main := range []string{api.Main, web.Main} {
		if out := runGit(t, main, "worktree", "list", "--porcelain"); strings.Contains(out, workspacesRoot) {
			t.Fatalf("%s still registers a destroyed worktree:\n%s", main, out)
		}
	}
	if out := runGit(t, api.Main, "branch", "--list", "dirty-b"); out != "" {
		t.Fatalf("deleteBranches left dirty-b behind: %q", out)
	}
	if out := runGit(t, api.Main, "branch", "--list", "clean-b"); !strings.Contains(out, "clean-b") {
		t.Fatalf("a destroy without deleteBranches removed clean-b: %q", out)
	}
}

// TestRPC_ScanAndRegisterManyNestedTreeAgainstRealGit: a nested tree with
// duplicate leaf names, dotted names and ignored folders; the suggested
// keys register in one all-or-nothing call, a conflicting batch is
// refused with per-project reasons, and the depth cap names the folder.
func TestRPC_ScanAndRegisterManyNestedTreeAgainstRealGit(t *testing.T) {
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	repos := []string{
		"api", "team-a/api", "team-b/api", "wixcontrol/wixcontrol", "my.dotted.repo",
		"Guacamole/postgres/apache-guacamole-helm-chart", "a/b/c-helm-chart", "clients/ios/App", "clients/android/app",
		"node_modules/pkg", "dotnet/bin/Debug", "x/y/z/w/too-deep",
	}
	for _, r := range repos {
		dir := filepath.Join(projectsRoot, r)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		runGit(t, dir, "init", "-q", "-b", "develop")
	}

	responses, _ := rpcSession(t, fx,
		req("ctx", "contexts.create", map[string]any{"name": "nest", "workspacesRoot": workspacesRoot, "projectsRoot": projectsRoot, "activate": true,
			"ignorePatterns": []string{"node_modules", "bin", "obj"}, "projectScanMaxDepth": 3}),
		req("scan", "projects.scan", map[string]any{}),
	)
	mustResult(t, responses, "ctx")
	var scan struct {
		Candidates []struct {
			Path         string `json:"path"`
			RelativePath string `json:"relativePath"`
			SuggestedKey string `json:"suggestedKey"`
		} `json:"candidates"`
		Truncated     bool     `json:"truncated"`
		TruncatedDirs []string `json:"truncatedDirs"`
		MaxDepth      int      `json:"maxDepth"`
	}
	if err := json.Unmarshal(mustResult(t, responses, "scan"), &scan); err != nil {
		t.Fatal(err)
	}
	if len(scan.Candidates) != 9 {
		t.Fatalf("candidates = %+v, want 9 (node_modules and bin ignored, too-deep past depth 3)", scan.Candidates)
	}
	if !scan.Truncated || scan.MaxDepth != 3 || len(scan.TruncatedDirs) != 1 || !strings.HasSuffix(scan.TruncatedDirs[0], "x/y/z") {
		t.Fatalf("truncation = %v %d %v, want x/y/z at depth 3", scan.Truncated, scan.MaxDepth, scan.TruncatedDirs)
	}

	var batch []map[string]string
	for _, c := range scan.Candidates {
		batch = append(batch, map[string]string{"key": c.SuggestedKey, "sourceDir": c.Path})
	}
	conflicting := append([]map[string]string{}, batch...)
	conflicting = append(conflicting, map[string]string{"key": "API", "sourceDir": filepath.Join(projectsRoot, "dotnet")})

	responses, _ = rpcSession(t, fx,
		req("bad", "projects.registerMany", map[string]any{"projects": conflicting}),
		req("good", "projects.registerMany", map[string]any{"projects": batch}),
		req("list", "projects.list", map[string]any{}),
	)
	var bad struct {
		Registered []any `json:"registered"`
		Problems   []struct {
			Index  int    `json:"index"`
			Reason string `json:"reason"`
		} `json:"problems"`
	}
	_ = json.Unmarshal(mustResult(t, responses, "bad"), &bad)
	if len(bad.Registered) != 0 || len(bad.Problems) == 0 {
		t.Fatalf("conflicting batch = %+v, want refused with problems", bad)
	}
	for _, p := range bad.Problems {
		if p.Index != len(batch) {
			t.Fatalf("problem %+v blames a valid entry", p)
		}
	}
	var good struct {
		Registered []any `json:"registered"`
		Problems   []any `json:"problems"`
	}
	_ = json.Unmarshal(mustResult(t, responses, "good"), &good)
	if len(good.Problems) != 0 || len(good.Registered) != 9 {
		t.Fatalf("suggested keys batch = %+v, want all 9 registered", good)
	}
	var list []any
	_ = json.Unmarshal(mustResult(t, responses, "list"), &list)
	if len(list) != 9 {
		t.Fatalf("projects.list = %d, want 9", len(list))
	}
}
