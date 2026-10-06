// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ownersJSON runs "list --json" for ctxName and returns name -> orphan_of
// ("" when owned).
func ownersJSON(t *testing.T, fx *Fixture, ctxName string) map[string]string {
	t.Helper()
	stdout, _ := fx.MustRun("", "--context", ctxName, "list", "--json")
	var items []struct {
		Name     string `json:"name"`
		OrphanOf string `json:"orphan_of"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("list --json %q: %v", stdout, err)
	}
	out := map[string]string{}
	for _, it := range items {
		if it.Error != "" {
			t.Fatalf("workspace %s damaged: %s", it.Name, it.Error)
		}
		out[it.Name] = it.OrphanOf
	}
	return out
}

// TestContextRename_KeepsWorkspacesAndClaimRecoversOrphans: a context with
// two real-git workspaces is renamed; both stay listed under the new name
// and their manifests name it. A manifest left naming a context that no
// longer exists (the state an older version's rename produced) is listed
// as an orphan and recovered with "wspace claim".
func TestContextRename_KeepsWorkspacesAndClaimRecoversOrphans(t *testing.T) {
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	api := NewGitProject(t, projectsRoot, "api", "develop")

	responses, _ := rpcSession(t, fx,
		req("ctx", "contexts.create", map[string]any{"name": "alpha", "workspacesRoot": workspacesRoot, "projectsRoot": projectsRoot, "activate": true}),
		req("reg", "projects.register", map[string]any{"key": "api", "sourceDir": api.Main}),
		req("c1", "workspaces.create", map[string]any{"name": "findings", "branch": "findings"}),
		req("c2", "workspaces.create", map[string]any{"name": "memoryleak", "branch": "memoryleak"}),
		req("rename", "contexts.update", map[string]any{"name": "alpha", "newName": "beta"}),
	)
	for _, id := range []string{"ctx", "reg", "c1", "c2"} {
		mustResult(t, responses, id)
	}
	var renamed struct {
		Name                 string   `json:"name"`
		ReassignedWorkspaces []string `json:"reassignedWorkspaces"`
	}
	if err := json.Unmarshal(mustResult(t, responses, "rename"), &renamed); err != nil {
		t.Fatal(err)
	}
	sort.Strings(renamed.ReassignedWorkspaces)
	if renamed.Name != "beta" || strings.Join(renamed.ReassignedWorkspaces, ",") != "findings,memoryleak" {
		t.Fatalf("rename result = %+v, want both workspaces reassigned to beta", renamed)
	}

	for _, name := range []string{"findings", "memoryleak"} {
		m := ReadManifest(t, filepath.Join(workspacesRoot, name))
		if m.Workspace.Context != "beta" || len(m.Workspace.Repos) != 1 || m.Workspace.Branch != name {
			t.Fatalf("%s manifest = %+v, want owner beta with its other fields kept", name, m.Workspace)
		}
	}
	if got := ownersJSON(t, fx, "beta"); len(got) != 2 || got["findings"] != "" || got["memoryleak"] != "" {
		t.Fatalf("list under beta = %v, want both owned", got)
	}

	// Simulate the pre-fix state: the manifest names a context that no
	// longer exists.
	manifestPath := filepath.Join(workspacesRoot, "findings", ".wspace", "workspace.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	stale := strings.Replace(string(data), "context: beta", "context: gone", 1)
	if stale == string(data) {
		t.Fatalf("manifest has no owner line to rewrite:\n%s", data)
	}
	if err := os.WriteFile(manifestPath, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := ownersJSON(t, fx, "beta"); got["findings"] != "gone" || got["memoryleak"] != "" {
		t.Fatalf("list under beta = %v, want findings flagged orphan_of gone", got)
	}

	stdout, _ := fx.MustRun("", "--context", "beta", "claim", "findings")
	if !strings.Contains(stdout, "findings") || !strings.Contains(stdout, "gone") {
		t.Fatalf("claim stdout = %q, want findings claimed from gone", stdout)
	}
	if m := ReadManifest(t, filepath.Join(workspacesRoot, "findings")); m.Workspace.Context != "beta" || len(m.Workspace.Repos) != 1 {
		t.Fatalf("findings manifest after claim = %+v", m.Workspace)
	}
	if got := ownersJSON(t, fx, "beta"); got["findings"] != "" {
		t.Fatalf("list under beta = %v, want findings owned after claim", got)
	}
}
