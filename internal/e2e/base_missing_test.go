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

type e2eRepoStatus struct {
	Alias       string `json:"alias"`
	Project     string `json:"project"`
	Ahead       int    `json:"ahead"`
	BaseBranch  string `json:"baseBranch"`
	BaseMissing bool   `json:"baseMissing"`
}

type e2eWorkspaceStatus struct {
	Name  string          `json:"name"`
	Repos []e2eRepoStatus `json:"repos"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func repoOf(t *testing.T, ws e2eWorkspaceStatus, alias string) e2eRepoStatus {
	t.Helper()
	for _, r := range ws.Repos {
		if r.Alias == alias {
			return r
		}
	}
	t.Fatalf("repo %s not in %+v", alias, ws)
	return e2eRepoStatus{}
}

// TestRPC_RepoWithoutTheWorkspaceBaseAgainstRealGit reproduces a workspace
// based on develop that mounts a repo with only master: creation succeeds
// (starting that repo from its HEAD), and status/list render the whole
// workspace with that repo flagged baseMissing — until the remote's
// default branch is known, at which point it is compared against master.
func TestRPC_RepoWithoutTheWorkspaceBaseAgainstRealGit(t *testing.T) {
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	api := NewGitProject(t, projectsRoot, "api", "develop")
	hub := NewGitProject(t, projectsRoot, "hub", "master")

	responses, _ := rpcSession(t, fx,
		req("ctx", "contexts.create", map[string]any{"name": "mixed", "workspacesRoot": workspacesRoot, "activate": true, "defaults": map[string]any{"baseBranch": "develop"}}),
		req("r1", "projects.register", map[string]any{"key": "api", "sourceDir": api.Main}),
		req("r2", "projects.register", map[string]any{"key": "hub", "sourceDir": hub.Main}),
		req("c", "workspaces.create", map[string]any{"name": "findings", "branch": "feat"}),
		req("s", "workspaces.status", map[string]any{"workspace": "findings"}),
		req("l", "workspaces.list", map[string]any{}),
	)
	mustResult(t, responses, "c")

	var st e2eWorkspaceStatus
	if err := json.Unmarshal(mustResult(t, responses, "s"), &st); err != nil {
		t.Fatal(err)
	}
	if st.Error != nil || len(st.Repos) != 2 {
		t.Fatalf("status = %+v, want both repos and no error", st)
	}
	if h := repoOf(t, st, "hub"); !h.BaseMissing || h.BaseBranch != "develop" || h.Project != "hub" {
		t.Fatalf("hub = %+v, want baseMissing against develop", h)
	}
	if a := repoOf(t, st, "api"); a.BaseMissing {
		t.Fatalf("api = %+v, want a known comparison", a)
	}
	var list []e2eWorkspaceStatus
	if err := json.Unmarshal(mustResult(t, responses, "l"), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Error != nil || len(list[0].Repos) != 2 {
		t.Fatalf("list = %+v, want findings rendered without an error", list)
	}

	// The CLI renders it too, with an explanatory note.
	out, _ := fx.MustRun("", "status", "findings")
	if !strings.Contains(out, "base branch develop not found in hub") {
		t.Fatalf("status output = %q, want the missing base explained", out)
	}

	// Once origin/HEAD is known, hub is compared against its default
	// branch: one local commit is one ahead.
	runGit(t, hub.Main, "remote", "set-head", "origin", "master")
	wt := filepath.Join(workspacesRoot, "findings", "hub")
	runGit(t, wt, "config", "user.email", "e2e@example.invalid")
	runGit(t, wt, "config", "user.name", "wspace e2e")
	if err := os.WriteFile(filepath.Join(wt, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wt, "add", "x.txt")
	runGit(t, wt, "commit", "-q", "-m", "x")

	responses, _ = rpcSession(t, fx, req("s2", "workspaces.status", map[string]any{"workspace": "findings"}))
	st = e2eWorkspaceStatus{}
	if err := json.Unmarshal(mustResult(t, responses, "s2"), &st); err != nil {
		t.Fatal(err)
	}
	if h := repoOf(t, st, "hub"); h.BaseMissing || h.BaseBranch != "master" || h.Ahead != 1 {
		t.Fatalf("hub = %+v, want ahead 1 against master", h)
	}
}
