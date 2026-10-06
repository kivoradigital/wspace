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

// rpcLine is one decoded output line of `wspace rpc`: a response (ID set,
// Event empty) or a progress event.
type rpcLine struct {
	ID     *string         `json:"id"`
	Event  string          `json:"event"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	} `json:"error"`
	Data map[string]any `json:"data"`
}

// rpcSession runs the real binary in rpc mode with every request on its
// own stdin line and returns each request's response plus the progress
// events streamed for it, keyed by request id. It fails on any stdout line
// that is not a protocol line.
func rpcSession(t *testing.T, fx *Fixture, requests ...string) (map[string]rpcLine, map[string][]map[string]any) {
	t.Helper()
	stdout, stderr := fx.MustRun(strings.Join(requests, "\n")+"\n", "rpc")
	responses := map[string]rpcLine{}
	events := map[string][]map[string]any{}
	for _, raw := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		var line rpcLine
		if err := json.Unmarshal([]byte(raw), &line); err != nil || line.ID == nil {
			t.Fatalf("stdout line %q is not a protocol line (err=%v); stderr=%s", raw, err, stderr)
		}
		if line.Event == "progress" {
			events[*line.ID] = append(events[*line.ID], line.Data)
			continue
		}
		responses[*line.ID] = line
	}
	return responses, events
}

func mustResult(t *testing.T, responses map[string]rpcLine, id string) json.RawMessage {
	t.Helper()
	r, ok := responses[id]
	if !ok {
		t.Fatalf("no response for request %s", id)
	}
	if r.Error != nil {
		t.Fatalf("request %s failed: %s %s %v", id, r.Error.Code, r.Error.Message, r.Error.Data)
	}
	return r.Result
}

func req(id, method string, params map[string]any) string {
	b, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	return string(b)
}

// TestRPC_FullLifecycleAgainstRealGit drives the built binary in rpc mode
// through context creation, project scan and registration, workspace
// creation (with streamed progress), status, a refused unforced destroy
// and a forced destroy — against real git repositories on disk.
func TestRPC_FullLifecycleAgainstRealGit(t *testing.T) {
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	api := NewGitProject(t, projectsRoot, "api", "develop")

	responses, events := rpcSession(t, fx,
		`{"id":"hello","method":"rpc.hello"}`,
		req("ctx", "contexts.create", map[string]any{"name": "e2e-rpc", "workspacesRoot": workspacesRoot, "projectsRoot": projectsRoot, "activate": true}),
		req("scan", "projects.scan", map[string]any{}),
		req("reg", "projects.register", map[string]any{"key": "api", "sourceDir": api.Main}),
		req("create", "workspaces.create", map[string]any{"name": "feat", "branch": "feat-rpc"}),
		req("status", "workspaces.status", map[string]any{"workspace": "feat"}),
	)

	var hello struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	_ = json.Unmarshal(mustResult(t, responses, "hello"), &hello)
	if hello.ProtocolVersion != 1 {
		t.Fatalf("protocolVersion = %d, want 1", hello.ProtocolVersion)
	}
	mustResult(t, responses, "ctx")

	var scan struct {
		Candidates []struct {
			Path         string `json:"path"`
			SuggestedKey string `json:"suggestedKey"`
		} `json:"candidates"`
	}
	_ = json.Unmarshal(mustResult(t, responses, "scan"), &scan)
	if len(scan.Candidates) != 1 || scan.Candidates[0].SuggestedKey != "api" {
		t.Fatalf("scan = %+v, want the api clone", scan)
	}
	mustResult(t, responses, "reg")

	var ws struct {
		Path  string `json:"path"`
		Repos []struct {
			Alias, Branch string
		} `json:"repos"`
	}
	_ = json.Unmarshal(mustResult(t, responses, "create"), &ws)
	if len(ws.Repos) != 1 || ws.Repos[0].Branch != "feat-rpc" {
		t.Fatalf("create = %+v", ws)
	}
	if got := CurrentBranch(t, filepath.Join(ws.Path, "api")); got != "feat-rpc" {
		t.Fatalf("real worktree branch = %q, want feat-rpc", got)
	}
	var phases []string
	for _, ev := range events["create"] {
		if ev["kind"] == "repo" {
			phases = append(phases, ev["repo"].(string)+":"+ev["phase"].(string))
		}
	}
	if strings.Join(phases, ",") != "api:started,api:finished" {
		t.Fatalf("create progress = %v, want api started then finished before the response", phases)
	}

	var status struct {
		Repos []struct {
			Alias  string `json:"alias"`
			Branch string `json:"branch"`
		} `json:"repos"`
	}
	_ = json.Unmarshal(mustResult(t, responses, "status"), &status)
	if len(status.Repos) != 1 || status.Repos[0].Branch != "feat-rpc" {
		t.Fatalf("status = %+v", status)
	}

	// A stray untracked file makes an unforced destroy need confirmation;
	// force then removes everything.
	if err := os.WriteFile(filepath.Join(ws.Path, "api", "stray.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	responses, _ = rpcSession(t, fx,
		req("d1", "workspaces.destroy", map[string]any{"workspace": "feat"}),
		req("d2", "workspaces.destroy", map[string]any{"workspace": "feat", "force": true}),
	)
	if r := responses["d1"]; r.Error == nil || r.Error.Code != "needs_confirmation" || !strings.Contains(string(mustJSON(r.Error.Data)), "stray.txt") {
		t.Fatalf("unforced destroy = %+v, want needs_confirmation naming stray.txt", r)
	}
	mustResult(t, responses, "d2")
	if PathExists(t, ws.Path) {
		t.Fatalf("workspace %s still exists after forced destroy", ws.Path)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// TestRPC_CreateRollsBackAPartialFailureAgainstRealGit proves the rollback
// with real git: the second project's fetch fails after the first
// project's worktree and branch were already created, and the call must
// leave neither behind (nor the workspace directory), while the first
// project's pre-existing branches stay intact.
func TestRPC_CreateRollsBackAPartialFailureAgainstRealGit(t *testing.T) {
	fx := NewFixture(t)
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	projectsRoot := filepath.Join(fx.Root, "projects")
	if err := os.MkdirAll(projectsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	alpha := NewGitProject(t, projectsRoot, "alpha", "develop")
	beta := NewGitProject(t, projectsRoot, "beta", "develop")
	// Break beta's remote after its origin/develop ref exists locally:
	// pre-flight (which resolves the base from local refs) still passes,
	// the mutate-phase fetch fails.
	if err := os.RemoveAll(beta.Remote); err != nil {
		t.Fatal(err)
	}

	responses, events := rpcSession(t, fx,
		req("ctx", "contexts.create", map[string]any{"name": "e2e-rollback", "workspacesRoot": workspacesRoot, "activate": true}),
		req("a", "projects.register", map[string]any{"key": "alpha", "sourceDir": alpha.Main}),
		req("b", "projects.register", map[string]any{"key": "beta", "sourceDir": beta.Main}),
		req("create", "workspaces.create", map[string]any{"name": "doomed", "branch": "doomed-branch", "projects": []string{"alpha", "beta"}}),
	)
	mustResult(t, responses, "ctx")
	mustResult(t, responses, "a")
	mustResult(t, responses, "b")
	r := responses["create"]
	if r.Error == nil || r.Error.Code != "git_failed" {
		t.Fatalf("create = %+v, want git_failed", r)
	}

	var phases []string
	for _, ev := range events["create"] {
		if ev["kind"] == "repo" {
			phases = append(phases, ev["repo"].(string)+":"+ev["phase"].(string))
		}
	}
	if strings.Join(phases, ",") != "alpha:started,alpha:finished,beta:started,beta:failed,alpha:rolled_back" {
		t.Fatalf("progress = %v", phases)
	}

	if PathExists(t, filepath.Join(workspacesRoot, "doomed")) {
		t.Fatal("workspace directory left behind after rollback")
	}
	if out := runGit(t, alpha.Main, "branch", "--list", "doomed-branch"); out != "" {
		t.Fatalf("alpha still has the branch created by the failed run: %q", out)
	}
	if out := runGit(t, alpha.Main, "worktree", "list", "--porcelain"); strings.Contains(out, "doomed") {
		t.Fatalf("alpha still registers the rolled-back worktree:\n%s", out)
	}
	if out := runGit(t, alpha.Main, "branch", "--list", "develop"); !strings.Contains(out, "develop") {
		t.Fatalf("alpha lost its pre-existing develop branch: %q", out)
	}
}
