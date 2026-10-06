// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/mcpserver"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func init() {
	if err := messages.Use("en"); err != nil {
		panic(err)
	}
}

const testPID = 4242

type fixture struct {
	git      *portstest.FakeGit
	fs       *portstest.FakeFS
	store    *portstest.FakeConfigStore
	presence *portstest.FakePresenceStore
	eng      *engine.Engine
	session  *mcp.ClientSession
	progress *progressLog
}

// progressLog records progress notifications at both ends: sent counts
// them as the server sends them (synchronously, inside the tool call), and
// events holds them as the client receives them. The client handles
// notifications on its own goroutine, so a tool result can reach the test
// before the notifications sent ahead of it; sent is the exact count to
// wait for.
type progressLog struct {
	mu     sync.Mutex
	sent   int
	events []*mcp.ProgressNotificationParams
}

func (p *progressLog) countSent(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method == "notifications/progress" {
			p.mu.Lock()
			p.sent++
			p.mu.Unlock()
		}
		return next(ctx, method, req)
	}
}

func (p *progressLog) sentCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sent
}

// waitReceived waits (bounded) until the client has handled every
// notification the server sent, and returns them.
func (p *progressLog) waitReceived(t *testing.T) []*mcp.ProgressNotificationParams {
	t.Helper()
	want := p.sentCount()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		events := p.all()
		if len(events) >= want {
			return events
		}
		if time.Now().After(deadline) {
			t.Fatalf("client received %d of %d progress notifications", len(events), want)
		}
	}
}

func (p *progressLog) add(ev *mcp.ProgressNotificationParams) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *progressLog) all() []*mcp.ProgressNotificationParams {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*mcp.ProgressNotificationParams{}, p.events...)
}

// newEngineFixture seeds engine fakes like internal/rpc's tests: context
// "work" (active) with projects api and web and one workspace "feat"
// mounting api, plus an inactive context "home".
func newEngineFixture(t *testing.T) *fixture {
	t.Helper()
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	git := portstest.NewFakeGit()
	git.WorktreeAddFunc = func(_ domain.Path, spec ports.WorktreeSpec) error { return fs.MkdirAll(spec.Target) }
	store.PutContext(domain.Context{
		Name: "work", WorkspacesRoot: "/fx/workspaces", ProjectsRoot: "/fx/src",
		Projects: []domain.Project{{Key: "api", SourceDir: "/fx/src/api"}, {Key: "web", SourceDir: "/fx/src/web"}},
	})
	store.PutContext(domain.Context{Name: "home", WorkspacesRoot: "/fx/home"})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatal(err)
	}
	store.PutManifest("/fx/workspaces/feat", domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{
		Name: "feat", Root: "/fx/workspaces/feat", Context: "work", Branch: "feat",
		Repos: []domain.RepoEntry{{Alias: "api", Project: "api", SourceDir: "/fx/src/api", Branch: "feat"}},
	}})
	_ = fs.MkdirAll("/fx/workspaces/feat/api")
	_ = fs.MkdirAll("/fx/src/docs/.git")
	date := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	git.StashListFunc = func(domain.Path) ([]domain.StashEntry, error) {
		return []domain.StashEntry{{Index: 0, Ref: "stash@{0}", Hash: "5555555", Branch: "feat", Message: "half done", Date: date}}, nil
	}
	git.CommitDetailFunc = func(_ domain.Path, hash string, _ domain.DiffLimits) (domain.CommitDetail, error) {
		return domain.CommitDetail{CommitInfo: domain.CommitInfo{Hash: hash, ShortHash: hash, Subject: "feat: x", AuthorName: "Ada Acme", AuthorDate: date}, Parents: []string{}}, nil
	}

	presence := portstest.NewFakePresenceStore()
	eng := engine.New(engine.Deps{Store: store, Git: git, FS: fs, Presence: presence, PID: testPID, Version: "1.2.3"})
	return &fixture{git: git, fs: fs, store: store, presence: presence, eng: eng, progress: &progressLog{}}
}

// newFixture connects an MCP client to the server over the SDK's
// in-memory transport.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := newEngineFixture(t)
	srv := mcpserver.New(f.eng, "1.2.3")
	srv.AddSendingMiddleware(f.progress.countSent)
	serverT, clientT := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.9"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) { f.progress.add(req.Params) },
	})
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	f.session = cs
	return f
}

func (f *fixture) call(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s) protocol error: %v", name, err)
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func structured(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func errorBody(t *testing.T, res *mcp.CallToolResult) engine.ErrorBody {
	t.Helper()
	var body engine.ErrorBody
	if !res.IsError {
		t.Fatalf("want a tool error, got success: %s", text(res))
	}
	if err := json.Unmarshal([]byte(text(res)), &body); err != nil {
		t.Fatalf("error content %q is not a JSON error body: %v", text(res), err)
	}
	return body
}

type hints struct {
	readOnly, destructive, idempotent, openWorld bool
}

// wantTools is the complete tool set with its annotations.
var wantTools = map[string]hints{
	"engine_version":          {readOnly: true, idempotent: true},
	"engine_info":             {readOnly: true, idempotent: true},
	"check_update":            {readOnly: true, idempotent: true, openWorld: true},
	"run_doctor":              {idempotent: true},
	"list_contexts":           {readOnly: true, idempotent: true},
	"get_context":             {readOnly: true, idempotent: true},
	"create_context":          {},
	"update_context":          {idempotent: true},
	"switch_context":          {idempotent: true},
	"delete_context":          {destructive: true, idempotent: true},
	"import_legacy_context":   {},
	"list_projects":           {readOnly: true, idempotent: true},
	"scan_projects":           {readOnly: true, idempotent: true},
	"register_project":        {},
	"register_projects":       {},
	"update_project":          {idempotent: true},
	"unregister_project":      {destructive: true, idempotent: true},
	"list_workspaces":         {readOnly: true, idempotent: true},
	"workspace_status":        {readOnly: true, idempotent: true},
	"repo_changes":            {readOnly: true, idempotent: true},
	"teardown_check":          {readOnly: true, idempotent: true},
	"create_workspace":        {},
	"add_project":             {},
	"remove_project":          {destructive: true},
	"destroy_workspace":       {destructive: true},
	"repair_workspace":        {idempotent: true},
	"sync_env":                {destructive: true, idempotent: true},
	"adopt_legacy_workspaces": {idempotent: true},
	"claim_workspaces":        {idempotent: true},
	"update_repo":             {},
	"update_workspace":        {},
	"repo_branch_info":        {readOnly: true, idempotent: true},
	"repo_diff":               {readOnly: true, idempotent: true},
	"repo_commits":            {readOnly: true, idempotent: true},
	"repo_commit":             {readOnly: true, idempotent: true},
	"repo_stashes":            {readOnly: true, idempotent: true},
	"repo_stash":              {readOnly: true, idempotent: true},
	"repo_fetch":              {idempotent: true, openWorld: true},
	"repo_pull_ff":            {openWorld: true},
	"repo_stash_apply":        {},
	"repo_stash_pop":          {},
	"repo_stash_drop":         {destructive: true},
	"repo_delete_untracked":   {destructive: true},
	"repo_stage":              {idempotent: true},
	"repo_unstage":            {idempotent: true},
	"repo_discard":            {destructive: true},
	"repo_commit_changes":     {},
	"repo_push":               {openWorld: true},
	"repo_stash_create":       {},
	"list_addable_projects":   {readOnly: true, idempotent: true},
}

func required(t *testing.T, tool *mcp.Tool) map[string]bool {
	t.Helper()
	schema := map[string]any{}
	b, _ := json.Marshal(tool.InputSchema)
	_ = json.Unmarshal(b, &schema)
	got := map[string]bool{}
	list, _ := schema["required"].([]any)
	for _, r := range list {
		got[r.(string)] = true
	}
	return got
}

func TestToolList_NamesHintsAndSchemas(t *testing.T) {
	f := newFixture(t)
	res, err := f.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		w, ok := wantTools[tool.Name]
		if !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		a := tool.Annotations
		if len(tool.Description) < 60 || a == nil || a.Title == "" {
			t.Errorf("tool %s needs a rich description and a title", tool.Name)
			continue
		}
		if a.ReadOnlyHint != w.readOnly || a.IdempotentHint != w.idempotent {
			t.Errorf("tool %s readOnly/idempotent = %v/%v, want %v/%v", tool.Name, a.ReadOnlyHint, a.IdempotentHint, w.readOnly, w.idempotent)
		}
		if a.OpenWorldHint == nil || *a.OpenWorldHint != w.openWorld {
			t.Errorf("tool %s openWorldHint = %v, want %v", tool.Name, a.OpenWorldHint, w.openWorld)
		}
		if !w.readOnly && (a.DestructiveHint == nil || *a.DestructiveHint != w.destructive) {
			t.Errorf("tool %s destructiveHint = %v, want %v", tool.Name, a.DestructiveHint, w.destructive)
		}
		req := required(t, tool)
		if w.destructive && !req["confirm"] {
			t.Errorf("destructive tool %s must require confirm (required = %v)", tool.Name, req)
		}
		if req["force"] {
			t.Errorf("tool %s must never require force", tool.Name)
		}
		if tool.OutputSchema == nil {
			t.Errorf("tool %s has no output schema", tool.Name)
		}
	}
	sort.Strings(names)
	if len(names) != len(wantTools) {
		t.Fatalf("tools = %v, want %d tools", names, len(wantTools))
	}
}

func TestToolList_RequiredArguments(t *testing.T) {
	f := newFixture(t)
	res, err := f.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"get_context":           {},
		"create_context":        {"name", "workspacesRoot"},
		"update_context":        {"name"},
		"switch_context":        {"name"},
		"delete_context":        {"name", "confirm"},
		"register_project":      {"key", "sourceDir"},
		"register_projects":     {"projects"},
		"update_project":        {"key"},
		"unregister_project":    {"key", "confirm"},
		"workspace_status":      {"workspace"},
		"repo_changes":          {"workspace", "repo"},
		"teardown_check":        {"workspace"},
		"create_workspace":      {"name"},
		"add_project":           {"workspace", "project"},
		"remove_project":        {"workspace", "project", "confirm"},
		"destroy_workspace":     {"workspace", "confirm"},
		"repair_workspace":      {"workspace"},
		"sync_env":              {"workspace", "confirm"},
		"update_repo":           {"workspace", "repo"},
		"update_workspace":      {"workspace"},
		"repo_branch_info":      {"workspace", "repo"},
		"repo_diff":             {"workspace", "repo", "path"},
		"repo_commits":          {"workspace", "repo"},
		"repo_commit":           {"workspace", "repo", "hash"},
		"repo_stashes":          {"workspace", "repo"},
		"repo_stash":            {"workspace", "repo", "index"},
		"repo_fetch":            {"workspace", "repo"},
		"repo_pull_ff":          {"workspace", "repo"},
		"repo_stash_apply":      {"workspace", "repo", "index", "hash"},
		"repo_stash_pop":        {"workspace", "repo", "index", "hash"},
		"repo_stash_drop":       {"workspace", "repo", "index", "hash", "confirm"},
		"repo_delete_untracked": {"workspace", "repo", "paths", "confirm"},
		"repo_stage":            {"workspace", "repo"},
		"repo_unstage":          {"workspace", "repo"},
		"repo_discard":          {"workspace", "repo", "paths", "confirm"},
		"repo_commit_changes":   {"workspace", "repo", "message"},
		"repo_push":             {"workspace", "repo"},
		"repo_stash_create":     {"workspace", "repo"},
		"list_addable_projects": {"workspace"},
	}
	for _, tool := range res.Tools {
		w, ok := want[tool.Name]
		if !ok {
			continue
		}
		got := required(t, tool)
		if len(got) != len(w) {
			t.Errorf("%s required = %v, want %v", tool.Name, got, w)
			continue
		}
		for _, name := range w {
			if !got[name] {
				t.Errorf("%s required = %v, want %v", tool.Name, got, w)
			}
		}
	}
}

// writeTools are the repository inspector's write actions, run against a
// worktree with a staged+unstaged and an untracked change.
var writeTools = map[string]bool{"repo_stage": true, "repo_unstage": true, "repo_discard": true, "repo_commit_changes": true, "repo_push": true, "repo_stash_create": true}

func TestTools_HappyPaths(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
		want string // substring of the structured result
	}{
		{"engine_version", nil, `"version":"1.2.3"`},
		{"engine_info", nil, `"contextName":"work"`},
		{"check_update", nil, `"unavailable":true`},
		{"run_doctor", nil, `"prunedWorktrees":0`},
		{"list_contexts", nil, `"name":"home"`},
		{"get_context", nil, `"name":"work"`},
		{"get_context", map[string]any{"name": "home"}, `"workspacesRoot":"/fx/home"`},
		{"create_context", map[string]any{"name": "lab", "workspacesRoot": "/fx/lab", "activate": true}, `"active":true`},
		{"update_context", map[string]any{"name": "work", "workspacesRoot": "/fx/ws2"}, `"workspacesRoot":"/fx/ws2"`},
		{"update_context", map[string]any{"name": "home", "newName": "house"}, `"name":"house"`},
		{"switch_context", map[string]any{"name": "home"}, `"name":"home"`},
		{"delete_context", map[string]any{"name": "home", "confirm": true}, `"deleted":"home"`},
		{"delete_context", map[string]any{"name": "work", "allowActive": true, "confirm": true}, `"deleted":"work"`},
		{"list_projects", map[string]any{"context": "work"}, `"key":"web"`},
		{"scan_projects", map[string]any{"roots": []string{"/fx/src"}, "depth": 2, "ignore": []string{}, "include": []string{}}, `"maxDepth":2`},
		{"register_project", map[string]any{"key": "docs", "sourceDir": "/fx/src/docs"}, `"key":"docs"`},
		{"register_projects", map[string]any{"projects": []map[string]any{{"key": "docs", "sourceDir": "/fx/src/docs"}}}, `"registered":[{"key":"docs"`},
		{"update_project", map[string]any{"key": "api", "destBranch": "x/{workspace}"}, `"destBranch":"x/{workspace}"`},
		{"unregister_project", map[string]any{"key": "web", "confirm": true}, `"unregistered":"web"`},
		{"list_workspaces", nil, `"name":"feat"`},
		{"workspace_status", map[string]any{"workspace": "feat"}, `"alias":"api"`},
		{"repo_changes", map[string]any{"workspace": "feat", "repo": "api"}, `"changes":[]`},
		{"teardown_check", map[string]any{"workspace": "feat"}, `"reasons":[]`},
		{"create_workspace", map[string]any{"name": "new", "projects": []string{"web"}, "branch": "new-branch"}, `"branch":"new-branch"`},
		{"create_workspace", map[string]any{"name": "fix1", "projects": []string{"web"}, "options": map[string]any{"branchPrefix": "fix"}}, `"branch":"fix/fix1"`},
		{"add_project", map[string]any{"workspace": "feat", "project": "web"}, `"alias":"web"`},
		{"remove_project", map[string]any{"workspace": "feat", "project": "api", "confirm": true}, `"alias":"api"`},
		{"destroy_workspace", map[string]any{"workspace": "feat", "confirm": true}, `"path":"/fx/workspaces/feat"`},
		{"repair_workspace", map[string]any{"workspace": "feat"}, `"recreated":[]`},
		{"sync_env", map[string]any{"workspace": "feat", "confirm": true}, `"copied":[]`},
		{"adopt_legacy_workspaces", nil, `"adopted":[]`},
		{"claim_workspaces", nil, `"claimed":[]`},
		{"claim_workspaces", map[string]any{"workspaces": []string{"feat"}}, `"reason":"claim.skip.already_owned"`},
		{"update_repo", map[string]any{"workspace": "feat", "repo": "api", "strategy": "rebase"}, `"strategy":"rebase"`},
		{"update_workspace", map[string]any{"workspace": "feat", "autostash": true}, `"repo":"api"`},
		{"repo_branch_info", map[string]any{"workspace": "feat", "repo": "api"}, `"repo":"api"`},
		{"repo_diff", map[string]any{"workspace": "feat", "repo": "api", "path": "main.go"}, `"path":"main.go"`},
		{"repo_commits", map[string]any{"workspace": "feat", "repo": "api", "limit": 10}, `"commits":[]`},
		{"repo_commits", map[string]any{"workspace": "feat", "repo": "api", "range": "upstream"}, `"upstream":"origin/feat"`},
		{"repo_commit", map[string]any{"workspace": "feat", "repo": "api", "hash": "abcdef0"}, `"author":"Ada Acme"`},
		{"repo_stashes", map[string]any{"workspace": "feat", "repo": "api"}, `"message":"half done"`},
		{"repo_stash", map[string]any{"workspace": "feat", "repo": "api", "index": 0}, `"includesUntracked":true`},
		{"repo_fetch", map[string]any{"workspace": "feat", "repo": "api"}, `"remote":"origin"`},
		{"repo_pull_ff", map[string]any{"workspace": "feat", "repo": "api"}, `"domainCode":"no_upstream"`},
		{"repo_stash_apply", map[string]any{"workspace": "feat", "repo": "api", "index": 0, "hash": "5555555"}, `"indexRestored":true`},
		{"repo_stash_pop", map[string]any{"workspace": "feat", "repo": "api", "index": 0, "hash": "5555555"}, `"dropped":true`},
		{"repo_stash_drop", map[string]any{"workspace": "feat", "repo": "api", "index": 0, "hash": "5555555", "confirm": true}, `"message":"half done"`},
		{"repo_delete_untracked", map[string]any{"workspace": "feat", "repo": "api", "paths": []string{"notes.txt"}, "confirm": true}, `"removed":["notes.txt"]`},
		{"list_addable_projects", map[string]any{"workspace": "feat"}, `"key":"web"`},
		{"repo_stage", map[string]any{"workspace": "feat", "repo": "api", "paths": []string{"notes.txt"}}, `"paths":["notes.txt"]`},
		{"repo_unstage", map[string]any{"workspace": "feat", "repo": "api", "all": true}, `"paths":["main.go"]`},
		{"repo_discard", map[string]any{"workspace": "feat", "repo": "api", "paths": []string{"main.go"}, "confirm": true}, `"discarded":["main.go"]`},
		{"repo_commit_changes", map[string]any{"workspace": "feat", "repo": "api", "message": "feat: login"}, `"shortHash":"abcdef0"`},
		{"repo_push", map[string]any{"workspace": "feat", "repo": "api", "setUpstream": true}, `"setUpstream":true`},
		{"repo_stash_create", map[string]any{"workspace": "feat", "repo": "api", "message": "wip"}, `"stash":{`},
	}
	seen := map[string]bool{}
	for _, tt := range tests {
		seen[tt.tool] = true
		t.Run(tt.tool, func(t *testing.T) {
			f := newFixture(t)
			if tt.tool == "repo_commits" && tt.args["range"] == "upstream" {
				f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
					return domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin"}, true, nil
				}
			}
			if tt.tool == "repo_delete_untracked" {
				status := []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: "notes.txt"}}
				f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) { return status, nil }
				f.git.CleanUntrackedFunc = func(domain.Path, []string) error { status = nil; return nil }
			}
			if writeTools[tt.tool] {
				f.git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feat", false, nil }
				f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
					return []domain.PorcelainEntry{{X: 'M', Y: 'M', RelPath: "main.go"}, {X: '?', Y: '?', RelPath: "notes.txt"}}, nil
				}
				f.git.CommitLogFunc = func(domain.Path, string, int, int) ([]domain.CommitInfo, error) {
					return []domain.CommitInfo{{Hash: "abcdef0123", ShortHash: "abcdef0", Subject: "feat: login", AuthorName: "Ada Acme"}}, nil
				}
				stashRef := ""
				f.git.StashRefFunc = func(domain.Path) (string, error) { return stashRef, nil }
				f.git.StashPushFunc = func(domain.Path, ports.StashPushSpec) error {
					stashRef = "5555555555555555555555555555555555555555"
					return nil
				}
			}
			if tt.tool == "repo_diff" {
				f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
					return []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "main.go"}}, nil
				}
			}
			res := f.call(t, tt.tool, tt.args)
			if res.IsError {
				t.Fatalf("%s returned a tool error: %s", tt.tool, text(res))
			}
			if got := structured(t, res); !strings.Contains(got, tt.want) {
				t.Fatalf("%s structured result = %s, want it to contain %s", tt.tool, got, tt.want)
			}
			summary := text(res)
			if summary == "" || strings.HasPrefix(summary, "{") || len(summary) > 400 {
				t.Fatalf("%s text content = %q, want a concise human summary", tt.tool, summary)
			}
		})
	}
	for name := range wantTools {
		if name != "import_legacy_context" && !seen[name] {
			t.Errorf("no happy-path case for %s", name)
		}
	}
}

const legacySource = "workspaces_root = /abs/workspaces\nprojects_root = /abs/projects\n[projects]\nalpha\n"

func TestImportLegacyContext_CreatesAndDryRunsMerges(t *testing.T) {
	f := newFixture(t)
	_ = f.fs.WriteFile("/legacy/config", []byte(legacySource), 0o644)

	res := f.call(t, "import_legacy_context", map[string]any{"from": "/legacy/config", "name": "legacy"})
	if res.IsError || !strings.Contains(structured(t, res), `"applied":true`) {
		t.Fatalf("import as a new context = %s", text(res))
	}

	res = f.call(t, "import_legacy_context", map[string]any{"from": "/legacy/config", "into": "work"})
	if res.IsError || !strings.Contains(structured(t, res), `"applied":false`) {
		t.Fatalf("merge without confirm must be a dry run: %s", structured(t, res))
	}
}

func TestTools_ErrorsCarryStableCodes(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
		code engine.Code
	}{
		{"engine_info", map[string]any{"context": "ghost"}, engine.CodeNotFound},
		{"run_doctor", map[string]any{"context": "ghost"}, engine.CodeNotFound},
		{"get_context", map[string]any{"name": "ghost"}, engine.CodeNotFound},
		{"create_context", map[string]any{"name": "work", "workspacesRoot": "/x"}, engine.CodeAlreadyExists},
		{"create_context", map[string]any{"name": "bad", "workspacesRoot": "relative"}, engine.CodeInvalidParams},
		{"update_context", map[string]any{"name": "ghost", "workspacesRoot": "/x"}, engine.CodeNotFound},
		{"switch_context", map[string]any{"name": "ghost"}, engine.CodeNotFound},
		{"delete_context", map[string]any{"name": "work", "confirm": true}, engine.CodeConflict},
		{"delete_context", map[string]any{"name": "ghost", "confirm": false}, engine.CodeNotFound},
		{"import_legacy_context", map[string]any{"from": "/nope", "name": "legacy"}, engine.CodeNotFound},
		{"import_legacy_context", map[string]any{"from": "/nope"}, engine.CodeInvalidParams},
		{"list_projects", map[string]any{"context": "ghost"}, engine.CodeNotFound},
		{"scan_projects", map[string]any{"context": "ghost"}, engine.CodeNotFound},
		{"register_project", map[string]any{"key": "api", "sourceDir": "/fx/src/other"}, engine.CodeAlreadyExists},
		{"register_projects", map[string]any{"context": "ghost", "projects": []map[string]any{}}, engine.CodeNotFound},
		{"update_project", map[string]any{"key": "ghost", "destBranch": "x"}, engine.CodeNotFound},
		{"unregister_project", map[string]any{"key": "ghost", "confirm": true}, engine.CodeNotFound},
		{"unregister_project", map[string]any{"key": "ghost", "confirm": false}, engine.CodeNotFound},
		{"list_workspaces", map[string]any{"context": "ghost"}, engine.CodeNotFound},
		{"workspace_status", map[string]any{"workspace": "ghost"}, engine.CodeNotFound},
		{"workspace_status", map[string]any{"workspace": "../etc"}, engine.CodeInvalidParams},
		{"repo_changes", map[string]any{"workspace": "feat", "repo": "ghost"}, engine.CodeNotFound},
		{"repo_diff", map[string]any{"workspace": "feat", "repo": "api", "path": "../../etc/passwd"}, engine.CodeNotFound},
		{"repo_commit", map[string]any{"workspace": "feat", "repo": "api", "hash": "HEAD~1"}, engine.CodeInvalidParams},
		{"repo_stash", map[string]any{"workspace": "feat", "repo": "api", "index": 5}, engine.CodeNotFound},
		{"repo_branch_info", map[string]any{"workspace": "feat", "repo": "ghost"}, engine.CodeNotFound},
		{"teardown_check", map[string]any{"workspace": "ghost"}, engine.CodeNotFound},
		{"create_workspace", map[string]any{"name": "feat"}, engine.CodeAlreadyExists},
		{"add_project", map[string]any{"workspace": "feat", "project": "ghost"}, engine.CodeNotFound},
		{"remove_project", map[string]any{"workspace": "feat", "project": "ghost", "confirm": false}, engine.CodeNotFound},
		{"destroy_workspace", map[string]any{"workspace": "ghost", "confirm": true}, engine.CodeNotFound},
		{"repair_workspace", map[string]any{"workspace": "ghost"}, engine.CodeNotFound},
		{"sync_env", map[string]any{"workspace": "ghost", "confirm": true}, engine.CodeNotFound},
		{"adopt_legacy_workspaces", map[string]any{"context": "ghost"}, engine.CodeNotFound},
		{"claim_workspaces", map[string]any{"context": "ghost"}, engine.CodeNotFound},
		{"update_repo", map[string]any{"workspace": "feat", "repo": "ghost"}, engine.CodeNotFound},
		{"update_repo", map[string]any{"workspace": "feat", "repo": "api", "strategy": "squash"}, engine.CodeInvalidParams},
		{"update_workspace", map[string]any{"workspace": "ghost"}, engine.CodeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			f := newFixture(t)
			body := errorBody(t, f.call(t, tt.tool, tt.args))
			if body.Code != tt.code {
				t.Fatalf("%s error = %+v, want code %s", tt.tool, body, tt.code)
			}
		})
	}
}

// Every destructive tool refuses without confirm, changing nothing, and a
// call without the confirm argument is rejected by the input schema.
func TestDestructiveTools_RefuseWithoutConfirm(t *testing.T) {
	tests := []struct {
		tool      string
		args      map[string]any
		unchanged func(t *testing.T, f *fixture)
	}{
		{"delete_context", map[string]any{"name": "home"}, func(t *testing.T, f *fixture) {
			if _, err := f.eng.GetContext(context.Background(), engine.ContextRef{Name: "home"}); err != nil {
				t.Fatal("context deleted without confirm")
			}
		}},
		{"delete_context", map[string]any{"name": "work", "allowActive": true}, func(t *testing.T, f *fixture) {
			if c, err := f.eng.GetContext(context.Background(), engine.ContextRef{}); err != nil || c.Name != "work" {
				t.Fatal("active context deleted without confirm")
			}
		}},
		{"unregister_project", map[string]any{"key": "web"}, func(t *testing.T, f *fixture) {
			if list, _ := f.eng.ListProjects(context.Background(), engine.ProjectsRef{}); len(list) != 2 {
				t.Fatal("project unregistered without confirm")
			}
		}},
		{"remove_project", map[string]any{"workspace": "feat", "project": "api"}, func(t *testing.T, f *fixture) {
			if st, _ := f.eng.WorkspaceStatus(context.Background(), engine.WorkspaceRef{Workspace: "feat"}); st.RepoCount != 1 {
				t.Fatal("repo removed without confirm")
			}
		}},
		{"destroy_workspace", map[string]any{"workspace": "feat"}, func(t *testing.T, f *fixture) {
			if exists, _ := f.fs.Exists("/fx/workspaces/feat"); !exists {
				t.Fatal("workspace removed without confirm")
			}
		}},
		{"repo_stash_drop", map[string]any{"workspace": "feat", "repo": "api", "index": 0, "hash": "5555555"}, func(t *testing.T, f *fixture) {
			for _, c := range f.git.Calls {
				if c.Method == "StashDrop" {
					t.Fatal("stash dropped without confirm")
				}
			}
		}},
		{"repo_delete_untracked", map[string]any{"workspace": "feat", "repo": "api", "paths": []string{"notes.txt"}}, func(t *testing.T, f *fixture) {
			for _, c := range f.git.Calls {
				if c.Method == "CleanUntracked" {
					t.Fatal("untracked files deleted without confirm")
				}
			}
		}},
		{"sync_env", map[string]any{"workspace": "feat"}, func(t *testing.T, f *fixture) {
			if data, _ := f.fs.ReadFile("/fx/workspaces/feat/api/.env"); string(data) != "LOCAL=edited" {
				t.Fatalf("worktree env file = %q, overwritten without confirm", data)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			f := newFixture(t)
			_ = f.fs.WriteFile("/fx/src/api/.env", []byte("SOURCE=1"), 0o644)
			_ = f.fs.WriteFile("/fx/workspaces/feat/api/.env", []byte("LOCAL=edited"), 0o644)
			f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
				return []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: "notes.txt"}}, nil
			}
			res := f.call(t, tt.tool, tt.args)
			if !res.IsError || strings.Contains(text(res), `"needs_confirmation"`) {
				t.Fatalf("%s without a confirm argument = %s, want a schema validation error", tt.tool, text(res))
			}
			tt.unchanged(t, f)

			args := map[string]any{"confirm": false}
			for k, v := range tt.args {
				args[k] = v
			}
			body := errorBody(t, f.call(t, tt.tool, args))
			if body.Code != engine.CodeNeedsConfirmation {
				t.Fatalf("%s with confirm=false = %+v, want needs_confirmation", tt.tool, body)
			}
			if _, ok := body.Data["reasons"]; !ok {
				t.Fatalf("%s needs_confirmation carries no reasons list: %+v", tt.tool, body)
			}
			tt.unchanged(t, f)
		})
	}
}

func TestDestroyWorkspace_PreviewsReasonsAndNeverForcesWithoutForce(t *testing.T) {
	f := newFixture(t)
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 4, 0, nil }

	body := errorBody(t, f.call(t, "destroy_workspace", map[string]any{"workspace": "feat", "confirm": false}))
	if body.Code != engine.CodeNeedsConfirmation || !strings.Contains(text(f.call(t, "destroy_workspace", map[string]any{"workspace": "feat", "confirm": false})), "unpushed_commits") {
		t.Fatalf("error = %+v, want needs_confirmation previewing the unpushed commits", body)
	}

	body = errorBody(t, f.call(t, "destroy_workspace", map[string]any{"workspace": "feat", "confirm": true}))
	if body.Code != engine.CodeNeedsConfirmation {
		t.Fatalf("confirmed but unforced destroy over unpushed work = %+v, want needs_confirmation", body)
	}
	if exists, _ := f.fs.Exists("/fx/workspaces/feat"); !exists {
		t.Fatal("workspace removed without force despite blocking reasons")
	}

	if res := f.call(t, "destroy_workspace", map[string]any{"workspace": "feat", "confirm": true, "force": true}); res.IsError {
		t.Fatalf("forced destroy failed: %s", text(res))
	}
}

func TestRemoveProject_PreviewIsScopedToTheRepoAndNeverForcesWithoutForce(t *testing.T) {
	f := newFixture(t)
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: "scratch.txt"}}, nil
	}
	res := f.call(t, "remove_project", map[string]any{"workspace": "feat", "project": "api", "confirm": false})
	if body := errorBody(t, res); body.Code != engine.CodeNeedsConfirmation || !strings.Contains(text(res), "foreign_file") {
		t.Fatalf("preview = %s, want needs_confirmation listing foreign_file", text(res))
	}
	res = f.call(t, "remove_project", map[string]any{"workspace": "feat", "project": "api", "confirm": true})
	if body := errorBody(t, res); body.Code != engine.CodeNeedsConfirmation {
		t.Fatalf("confirmed remove over a foreign file = %s, want needs_confirmation", text(res))
	}
	if res = f.call(t, "remove_project", map[string]any{"workspace": "feat", "project": "api", "confirm": true, "force": true}); res.IsError {
		t.Fatalf("forced remove failed: %s", text(res))
	}
}

func TestCreateWorkspace_SendsProgressNotificationsWhenAsked(t *testing.T) {
	f := newFixture(t)
	params := &mcp.CallToolParams{Name: "create_workspace", Arguments: map[string]any{"name": "new", "projects": []string{"api", "web"}}}
	params.SetProgressToken("tok-1")
	res, err := f.session.CallTool(context.Background(), params)
	if err != nil || res.IsError {
		t.Fatalf("create_workspace = %v, %v", res, err)
	}
	if n := f.progress.sentCount(); n < 4 {
		t.Fatalf("sent %d progress notifications, want at least started+finished per repo", n)
	}
	events := f.progress.waitReceived(t)
	last := 0.0
	var messages []string
	for _, ev := range events {
		if ev.ProgressToken != "tok-1" || ev.Progress <= last {
			t.Fatalf("progress %+v: want token tok-1 and strictly increasing progress", ev)
		}
		last = ev.Progress
		messages = append(messages, ev.Message)
	}
	joined := strings.Join(messages, "|")
	if !strings.Contains(joined, "api") || !strings.Contains(joined, "web") {
		t.Fatalf("progress messages %q do not name every repo", joined)
	}
	if !strings.Contains(structured(t, res), `"repos":[{"alias":"api"`) {
		t.Fatalf("result lacks per-repo entries: %s", structured(t, res))
	}

	// Without a progress token no notification is sent. Counted where it
	// is sent, so the check does not depend on delivery timing.
	before := f.progress.sentCount()
	_ = f.call(t, "create_workspace", map[string]any{"name": "quiet", "projects": []string{"web"}})
	if f.progress.sentCount() != before {
		t.Fatal("progress sent without a progress token")
	}
}

func TestPresence_RecordsClientAndLastToolThenRemovesOnClose(t *testing.T) {
	f := newFixture(t)
	rec, ok := f.presence.Record(testPID)
	if !ok {
		t.Fatal("no presence record after initialize")
	}
	if rec.ClientName != "test-client" || rec.ClientVersion != "0.9" || rec.StartedAt.IsZero() {
		t.Fatalf("presence record = %+v", rec)
	}
	_ = f.call(t, "list_workspaces", nil)
	if rec, _ = f.presence.Record(testPID); rec.LastTool != "list_workspaces" || rec.LastToolAt.IsZero() {
		t.Fatalf("presence after a tool call = %+v, want lastTool list_workspaces", rec)
	}

	_ = f.session.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := f.presence.Record(testPID); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("presence record not removed after the session closed")
}

func TestServe_RemovesPresenceWhenStdinCloses(t *testing.T) {
	f := newEngineFixture(t)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- mcpserver.Serve(context.Background(), f.eng, "1.2.3", inR, outW) }()

	lines := make(chan string, 16)
	go func() {
		buf := make([]byte, 64*1024)
		var pending string
		for {
			n, err := outR.Read(buf)
			pending += string(buf[:n])
			for {
				i := strings.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				lines <- pending[:i]
				pending = pending[i+1:]
			}
			if err != nil {
				close(lines)
				return
			}
		}
	}()
	send := func(s string) {
		if _, err := io.WriteString(inW, s+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke-agent","version":"3"}}}`)
	<-lines
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_contexts","arguments":{}}}`)
	<-lines
	rec, ok := f.presence.Record(testPID)
	if !ok || rec.ClientName != "smoke-agent" || rec.LastTool != "list_contexts" {
		t.Fatalf("presence while serving = %+v (present %v)", rec, ok)
	}

	_ = inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v after EOF", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after stdin closed")
	}
	_ = outW.Close()
	if _, ok := f.presence.Record(testPID); ok {
		t.Fatal("presence record left behind after Serve returned")
	}
}

func TestServe_RemovesPresenceWhenContextIsCancelled(t *testing.T) {
	f := newEngineFixture(t)
	inR, inW := io.Pipe()
	defer func() { _ = inW.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mcpserver.Serve(ctx, f.eng, "1.2.3", inR, io.Discard) }()
	_, _ = io.WriteString(inW, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"sig","version":"1"}}}`+"\n")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := f.presence.Record(testPID); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no presence record after initialize")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after cancellation")
	}
	if _, ok := f.presence.Record(testPID); ok {
		t.Fatal("presence record left behind after cancellation (signal path)")
	}
}

// TestWorkspaceStatus_ReportsAMissingBasePerRepo: a repo without the
// workspace's base branch is flagged in the structured output and named in
// the summary, and the workspace itself is not an error.
func TestWorkspaceStatus_ReportsAMissingBasePerRepo(t *testing.T) {
	f := newFixture(t)
	develop := domain.BranchName("develop")
	f.store.PutManifest("/fx/workspaces/feat", domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{
		Name: "feat", Root: "/fx/workspaces/feat", Context: "work", Branch: "feat",
		Options: domain.Options{BaseBranch: &develop},
		Repos:   []domain.RepoEntry{{Alias: "api", Project: "api", SourceDir: "/fx/src/api", Branch: "feat"}},
	}})
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) {
		return 0, 0, domain.NewOpError("git.ahead_behind", domain.CodeGitFailed, "", "no upstream", nil)
	}
	f.git.ResolveBaseFunc = func(domain.Path, string, domain.BranchName) (ports.BaseRef, error) {
		return ports.BaseRef{Ref: "HEAD"}, nil
	}

	res := f.call(t, "workspace_status", map[string]any{"workspace": "feat"})

	if res.IsError {
		t.Fatalf("workspace_status failed: %s", text(res))
	}
	if s := structured(t, res); !strings.Contains(s, `"baseBranch":"develop"`) || !strings.Contains(s, `"baseMissing":true`) {
		t.Errorf("structured = %s, want baseBranch develop and baseMissing", s)
	}
	if got := text(res); !strings.Contains(got, "api: base branch develop not found") {
		t.Errorf("text = %q, want the missing base named", got)
	}
}

// A per-repo refusal is a successful tool call whose structured result
// carries the error and whose summary says what blocked the update.
func TestUpdateRepo_RefusalIsReportedInTheResult(t *testing.T) {
	f := newFixture(t)
	f.git.RemoteDefaultBranchFunc = func(domain.Path, string) (domain.BranchName, bool, error) { return "main", true, nil }
	f.git.BehindCountFunc = func(domain.Path, string) (int, error) { return 2, nil }
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "main.go"}}, nil
	}

	res := f.call(t, "update_repo", map[string]any{"workspace": "feat", "repo": "api"})

	if res.IsError {
		t.Fatalf("update_repo = tool error %s, want the refusal in the result", text(res))
	}
	if got := structured(t, res); !strings.Contains(got, `"domainCode":"worktree_dirty"`) || !strings.Contains(got, `"files":["main.go"]`) {
		t.Fatalf("structured = %s", got)
	}
	if summary := text(res); !strings.Contains(summary, "api") || !strings.Contains(summary, "uncommitted") {
		t.Fatalf("summary = %q, want the refusal explained", summary)
	}
}
