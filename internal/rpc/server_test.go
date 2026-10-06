// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package rpc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
	"github.com/kivoradigital/wspace/internal/rpc"
)

var update = flag.Bool("update", false, "update golden files")

func init() {
	if err := messages.Use("en"); err != nil {
		panic(err)
	}
}

// fixture seeds fakes with fixed literal paths so wire output is
// deterministic: context "work" (active) with projects api and web, and
// one workspace "feat" mounting api.
type fixture struct {
	presence *portstest.FakePresenceStore
	store    *portstest.FakeConfigStore
	git      *portstest.FakeGit
	fs       *portstest.FakeFS
	srv      *rpc.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	git := portstest.NewFakeGit()
	git.WorktreeAddFunc = func(_ domain.Path, spec ports.WorktreeSpec) error { return fs.MkdirAll(spec.Target) }
	git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feat", false, nil }

	store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: "/fx/workspaces",
		ProjectsRoot:   "/fx/src",
		Projects: []domain.Project{
			{Key: "api", SourceDir: "/fx/src/api"},
			{Key: "web", SourceDir: "/fx/src/web"},
		},
	})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatal(err)
	}
	store.PutManifest("/fx/workspaces/feat", domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{
		Name: "feat", Root: "/fx/workspaces/feat", Context: "work", Branch: "feat",
		Repos: []domain.RepoEntry{{Alias: "api", Project: "api", SourceDir: "/fx/src/api", Branch: "feat"}},
	}})
	_ = fs.MkdirAll("/fx/workspaces/feat/api")

	// One live MCP session (pid 4242) and one whose process is gone.
	presence := portstest.NewFakePresenceStore()
	started := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	_ = presence.Put(context.Background(), ports.AgentSession{
		PID: 4242, ClientName: "claude-code", ClientVersion: "2.1.0",
		StartedAt: started, LastActivityAt: started.Add(time.Minute),
		LastTool: "list_workspaces", LastToolAt: started.Add(time.Minute),
	})
	_ = presence.Put(context.Background(), ports.AgentSession{PID: 4243, ClientName: "gone", StartedAt: started, LastActivityAt: started})
	presence.Dead[4243] = true

	eng := engine.New(engine.Deps{Store: store, Git: git, FS: fs, Presence: presence, Version: "1.2.3"})
	return &fixture{presence: presence, store: store, git: git, fs: fs, srv: rpc.NewServer(eng, "1.2.3")}
}

// serve runs one session: every line of input is a request; the returned
// slice holds every output line.
func (f *fixture) serve(t *testing.T, input string) []string {
	t.Helper()
	var out bytes.Buffer
	if err := f.srv.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	return strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
}

var timePattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?`)

func compareGolden(t *testing.T, name string, lines []string) {
	t.Helper()
	got := timePattern.ReplaceAllString(strings.Join(lines, "\n")+"\n", "<TIME>")
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s:\n--- got ---\n%s--- want ---\n%s", name, got, want)
	}
}

// TestWireFormat_Golden pins the exact bytes of the protocol: framing,
// field names and order, error shapes, and progress events emitted before
// the final response of a long operation.
func TestWireFormat_Golden(t *testing.T) {
	f := newFixture(t)
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 2, 0, nil }
	input := strings.Join([]string{
		`{"id":"1","method":"rpc.hello"}`,
		`not json`,
		`{"method":"rpc.hello"}`,
		`{"id":7,"method":"rpc.hello"}`,
		`{"id":"2","method":"nope.nothing"}`,
		`{"id":"3","method":"contexts.get","params":{"name":"work","bogus":1}}`,
		`{"id":"4","method":"contexts.list"}`,
		`{"id":"5","method":"contexts.get","params":{"name":"missing"}}`,
		`{"id":"6","method":"workspaces.destroy","params":{"workspace":"feat"}}`,
		``,
		`{"id":"7","method":"workspaces.destroy","params":{"workspace":"feat","force":true}}`,
		`{"id":"8","method":"mcp.sessions"}`,
	}, "\n") + "\n"
	compareGolden(t, "session.golden", f.serve(t, input))
}

func TestHello_ListsEveryMethodAndTheProtocolVersion(t *testing.T) {
	f := newFixture(t)
	lines := f.serve(t, `{"id":"h","method":"rpc.hello"}`+"\n")
	var resp struct {
		ID     string `json:"id"`
		Result struct {
			ProtocolVersion int      `json:"protocolVersion"`
			EngineVersion   string   `json:"engineVersion"`
			Methods         []string `json:"methods"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "h" || resp.Result.ProtocolVersion != rpc.ProtocolVersion || resp.Result.EngineVersion != "1.2.3" {
		t.Fatalf("hello = %+v", resp)
	}
	want := []string{
		"agents.install", "agents.mcpClean", "agents.status", "agents.uninstall",
		"contexts.create", "contexts.delete", "contexts.get", "contexts.importLegacy", "contexts.list", "contexts.switch", "contexts.update",
		"engine.checkUpdate", "engine.doctor", "engine.info", "engine.version",
		"mcp.sessions",
		"projects.list", "projects.register", "projects.registerMany", "projects.remove", "projects.scan", "projects.update",
		"repos.changes", "repos.commit", "repos.commitChanges", "repos.commits", "repos.diff", "repos.discard", "repos.discardUntracked", "repos.fetch", "repos.inspect", "repos.pull", "repos.push", "repos.stage", "repos.stash", "repos.stashApply", "repos.stashCreate", "repos.stashDrop", "repos.stashPop", "repos.stashes", "repos.unstage", "repos.validateUntracked",
		"rpc.hello",
		"workspaces.addRepo", "workspaces.addableProjects", "workspaces.adoptLegacy", "workspaces.claim", "workspaces.create", "workspaces.destroy", "workspaces.list", "workspaces.removeRepo",
		"workspaces.repair", "workspaces.repoChanges", "workspaces.status", "workspaces.syncEnv", "workspaces.teardownCheck",
		"workspaces.update", "workspaces.updateRepo",
	}
	if strings.Join(resp.Result.Methods, ",") != strings.Join(want, ",") {
		t.Fatalf("methods = %v\nwant      %v", resp.Result.Methods, want)
	}
}

// TestMethods covers every method's dispatch: a happy call decodes params,
// reaches the engine and returns a result; a bad call returns the right
// stable code.
// wantNoContexts asserts the engine has no context left.
func wantNoContexts() func(t *testing.T, f *fixture, result json.RawMessage) {
	return func(t *testing.T, f *fixture, _ json.RawMessage) {
		t.Helper()
		list, err := f.store.ListContexts(context.Background())
		if err != nil || len(list) != 0 {
			t.Fatalf("store.ListContexts() = %v, %v, want none", list, err)
		}
	}
}

func TestMethods(t *testing.T) {
	tests := []struct {
		method   string
		params   string
		wantCode string // "" means success
		check    func(t *testing.T, f *fixture, result json.RawMessage)
	}{
		{method: "engine.version", check: wantField("version", "1.2.3")},
		{method: "engine.info", params: `{"context":"work"}`, check: wantField("contextName", "work")},
		{method: "engine.doctor"},
		{method: "engine.checkUpdate", check: wantField("unavailable", true)},
		{method: "contexts.list"},
		{method: "mcp.sessions", check: wantSessions(4242)},
		{method: "mcp.sessions", params: `{"bogus":1}`, wantCode: "invalid_params"},
		{method: "contexts.get", params: `{}`, check: wantField("name", "work")},
		{method: "contexts.create", params: `{"name":"home","workspacesRoot":"/h"}`, check: wantField("name", "home")},
		{method: "contexts.create", params: `{"name":"work","workspacesRoot":"/h"}`, wantCode: "already_exists"},
		{method: "contexts.create", params: `{"name":"home"}`, wantCode: "invalid_params"},
		{method: "contexts.update", params: `{"name":"work","workspacesRoot":"/fx/ws2"}`, check: wantField("workspacesRoot", "/fx/ws2")},
		{method: "contexts.delete", params: `{"name":"work"}`, wantCode: "conflict"},
		{method: "contexts.delete", params: `{"name":"work","allowActive":true}`, check: wantNoContexts()},
		{method: "contexts.switch", params: `{"name":"work"}`, check: wantField("active", true)},
		{method: "contexts.importLegacy", params: `{"from":"/nope","name":"legacy"}`, wantCode: "not_found"},
		{method: "projects.list", check: wantLen(2)},
		{method: "projects.scan", params: `{"roots":["/fx/src"]}`},
		{method: "projects.register", params: `{"key":"docs","sourceDir":"/fx/src/docs"}`, check: wantField("key", "docs")},
		{method: "projects.update", params: `{"key":"api","destBranch":"x/{workspace}"}`, check: wantField("destBranch", "x/{workspace}")},
		{method: "projects.registerMany", params: `{"projects":[{"key":"docs","sourceDir":"/fx/src/docs"},{"key":"DOCS","sourceDir":"/fx/src/docs2"}]}`, check: wantProblems(1)},
		{method: "projects.registerMany", params: `{"projects":[{"key":"docs","sourceDir":"/fx/src/docs"}]}`},
		{method: "projects.remove", params: `{"key":"web"}`},
		{method: "workspaces.list", check: wantLen(1)},
		{method: "workspaces.adoptLegacy", check: wantAdoption(0, 0)},
		{method: "workspaces.adoptLegacy", params: `{"context":"missing"}`, wantCode: "not_found"},
		{method: "workspaces.status", params: `{"workspace":"feat"}`, check: wantField("name", "feat")},
		{method: "workspaces.status", params: `{"workspace":"../etc"}`, wantCode: "invalid_params"},
		{method: "workspaces.create", params: `{"name":"new","projects":["web"]}`, check: wantField("name", "new")},
		{method: "workspaces.create", params: `{"name":"feat"}`, wantCode: "already_exists"},
		{method: "workspaces.teardownCheck", params: `{"workspace":"feat"}`, check: wantLen(0)},
		{method: "workspaces.repoChanges", params: `{"workspace":"feat","repo":"api"}`, check: wantLen(0)},
		{method: "workspaces.repoChanges", params: `{"workspace":"feat","repo":"ghost"}`, wantCode: "not_found"},
		{method: "workspaces.addRepo", params: `{"workspace":"feat","project":"web"}`, check: wantField("alias", "web")},
		{method: "workspaces.removeRepo", params: `{"workspace":"feat","repo":"api"}`, check: wantField("alias", "api")},
		{method: "workspaces.repair", params: `{"workspace":"feat"}`},
		{method: "workspaces.syncEnv", params: `{"workspace":"feat"}`},
		{method: "workspaces.updateRepo", params: `{"workspace":"feat","repo":"api","strategy":"rebase","autostash":true}`, check: wantField("strategy", "rebase")},
		{method: "workspaces.updateRepo", params: `{"workspace":"feat","repo":"ghost"}`, wantCode: "not_found"},
		{method: "workspaces.updateRepo", params: `{"workspace":"feat","repo":"api","strategy":"squash"}`, wantCode: "invalid_params"},
		{method: "workspaces.update", params: `{"workspace":"feat"}`},
		{method: "workspaces.update", params: `{"workspace":"feat","bogus":1}`, wantCode: "invalid_params"},
		{method: "workspaces.destroy", params: `{"workspace":"feat"}`, check: wantField("path", "/fx/workspaces/feat")},
		{method: "workspaces.destroy", params: `{"workspace":"ghost"}`, wantCode: "not_found"},
		{method: "workspaces.destroy", params: `[1,2]`, wantCode: "invalid_params"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.params, func(t *testing.T) {
			f := newFixture(t)
			req := `{"id":"x","method":"` + tt.method + `"`
			if tt.params != "" {
				req += `,"params":` + tt.params
			}
			lines := f.serve(t, req+"}\n")
			var resp struct {
				ID     string            `json:"id"`
				Result json.RawMessage   `json:"result"`
				Error  *engine.ErrorBody `json:"error"`
				Event  string            `json:"event"`
			}
			last := lines[len(lines)-1]
			if err := json.Unmarshal([]byte(last), &resp); err != nil {
				t.Fatalf("bad response line %q: %v", last, err)
			}
			if resp.ID != "x" || resp.Event != "" {
				t.Fatalf("final line %q is not the response to id x", last)
			}
			if tt.wantCode != "" {
				if resp.Error == nil || string(resp.Error.Code) != tt.wantCode {
					t.Fatalf("response = %s, want error code %s", last, tt.wantCode)
				}
				return
			}
			if resp.Error != nil || resp.Result == nil {
				t.Fatalf("response = %s, want a result", last)
			}
			if tt.check != nil {
				tt.check(t, f, resp.Result)
			}
		})
	}
}

func wantField(name string, want any) func(*testing.T, *fixture, json.RawMessage) {
	return func(t *testing.T, _ *fixture, raw json.RawMessage) {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("result %s is not an object: %v", raw, err)
		}
		if m[name] != want {
			t.Fatalf("result.%s = %v, want %v (result %s)", name, m[name], want, raw)
		}
	}
}

// wantSessions checks mcp.sessions lists exactly these live pids, in order.
func wantSessions(pids ...int) func(*testing.T, *fixture, json.RawMessage) {
	return func(t *testing.T, _ *fixture, raw json.RawMessage) {
		t.Helper()
		var sessions []engine.MCPSession
		if err := json.Unmarshal(raw, &sessions); err != nil || len(sessions) != len(pids) {
			t.Fatalf("result %s: want %d sessions (%v)", raw, len(pids), err)
		}
		for i, pid := range pids {
			if sessions[i].PID != pid || sessions[i].ClientName == "" || sessions[i].StartedAt == "" {
				t.Fatalf("sessions[%d] = %+v, want pid %d with client and start time", i, sessions[i], pid)
			}
		}
	}
}

func wantLen(n int) func(*testing.T, *fixture, json.RawMessage) {
	return func(t *testing.T, _ *fixture, raw json.RawMessage) {
		t.Helper()
		var a []any
		if err := json.Unmarshal(raw, &a); err != nil || len(a) != n {
			t.Fatalf("result %s: want an array of %d (%v)", raw, n, err)
		}
	}
}

// TestServe_StopsAtEOFAndOnContextCancel guards the server lifecycle.
func TestServe_StopsWhenContextIsCancelled(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	err := f.srv.Serve(ctx, strings.NewReader(`{"id":"1","method":"rpc.hello"}`+"\n"), &out)
	if err == nil || out.Len() != 0 {
		t.Fatalf("Serve() on a cancelled context = %v, output %q; want an error and no output", err, out.String())
	}
}

func wantProblems(n int) func(*testing.T, *fixture, json.RawMessage) {
	return func(t *testing.T, _ *fixture, raw json.RawMessage) {
		t.Helper()
		var r struct {
			Registered []any `json:"registered"`
			Problems   []any `json:"problems"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatalf("result %s: %v", raw, err)
		}
		if len(r.Problems) != n || len(r.Registered) != 0 {
			t.Fatalf("result %s: want %d problem(s) and nothing registered", raw, n)
		}
	}
}

func wantAdoption(adopted, skipped int) func(*testing.T, *fixture, json.RawMessage) {
	return func(t *testing.T, _ *fixture, raw json.RawMessage) {
		t.Helper()
		var r struct {
			Adopted []any `json:"adopted"`
			Skipped []any `json:"skipped"`
		}
		if err := json.Unmarshal(raw, &r); err != nil || r.Adopted == nil || r.Skipped == nil {
			t.Fatalf("result %s: want adopted and skipped arrays (%v)", raw, err)
		}
		if len(r.Adopted) != adopted || len(r.Skipped) != skipped {
			t.Fatalf("result %s: want %d adopted, %d skipped", raw, adopted, skipped)
		}
	}
}

// TestAdoptLegacy_Golden pins workspaces.adoptLegacy's wire shape and the
// legacy flag workspaces.list carries before and after adoption: a
// workspace only the legacy bash tool manages, one it cannot resolve, and
// the fixture's native workspace.
func TestAdoptLegacy_Golden(t *testing.T) {
	f := newFixture(t)
	seed := func(name, conf string) {
		root := domain.Path("/fx/workspaces/" + name)
		_ = f.fs.MkdirAll(root.Join(".ws"))
		_ = f.fs.MkdirAll(root.Join("api"))
		if err := f.fs.WriteFile(root.Join(".ws", "workspace.conf"), []byte(conf), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	seed("old", "name = old\nbranch = feat\n[repos]\napi|api|feat\n")
	seed("stray", "name = stray\nbranch = feat\n[repos]\napi|unknown|feat\n")
	input := strings.Join([]string{
		`{"id":"1","method":"workspaces.list"}`,
		`{"id":"2","method":"workspaces.adoptLegacy","params":{"context":"work"}}`,
		`{"id":"3","method":"workspaces.list"}`,
		`{"id":"4","method":"workspaces.adoptLegacy"}`,
	}, "\n") + "\n"
	compareGolden(t, "adopt_legacy.golden", f.serve(t, input))
}

// TestBaseMissing_Golden pins the additive RepoStatus fields: project,
// baseBranch (the base ahead was counted against when there is no
// upstream) and baseMissing (no base resolvable in that repo), while the
// workspace itself still renders without an error.
func TestBaseMissing_Golden(t *testing.T) {
	f := newFixture(t)
	develop := domain.BranchName("develop")
	f.store.PutManifest("/fx/workspaces/feat", domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{
		Name: "feat", Root: "/fx/workspaces/feat", Context: "work", Branch: "feat",
		Options: domain.Options{BaseBranch: &develop},
		Repos: []domain.RepoEntry{
			{Alias: "api", Project: "api", SourceDir: "/fx/src/api", Branch: "feat"},
			{Alias: "web", Project: "web", SourceDir: "/fx/src/web", Branch: "feat"},
		},
	}})
	_ = f.fs.MkdirAll("/fx/workspaces/feat/web")
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) {
		return 0, 0, domain.NewOpError("git.ahead_behind", domain.CodeGitFailed, "", "no upstream", nil)
	}
	f.git.ResolveBaseFunc = func(repo domain.Path, remote string, b domain.BranchName) (ports.BaseRef, error) {
		if repo == "/fx/workspaces/feat/api" {
			return ports.BaseRef{Ref: remote + "/" + string(b), Remote: true}, nil
		}
		return ports.BaseRef{Ref: "HEAD"}, nil
	}
	f.git.UnpushedCountFunc = func(domain.Path, string) (int, error) { return 1, nil }
	input := strings.Join([]string{
		`{"id":"1","method":"workspaces.status","params":{"workspace":"feat"}}`,
		`{"id":"2","method":"workspaces.list"}`,
	}, "\n") + "\n"
	compareGolden(t, "base_missing.golden", f.serve(t, input))
}

// TestUpdate_Golden pins workspaces.update's and workspaces.updateRepo's
// wire shape: per-repo progress events, a merged repo, and a dirty
// refusal carrying the files.
func TestUpdate_Golden(t *testing.T) {
	f := newFixture(t)
	f.git.ResolveBaseFunc = func(_ domain.Path, remote string, b domain.BranchName) (ports.BaseRef, error) {
		return ports.BaseRef{Ref: remote + "/" + string(b), Remote: true}, nil
	}
	f.git.RemoteDefaultBranchFunc = func(domain.Path, string) (domain.BranchName, bool, error) { return "develop", true, nil }
	merged := false
	f.git.HeadCommitFunc = func(domain.Path) (string, error) {
		if merged {
			return "2222222", nil
		}
		return "1111111", nil
	}
	f.git.BehindCountFunc = func(domain.Path, string) (int, error) {
		if merged {
			return 0, nil
		}
		return 3, nil
	}
	f.git.IntegrateFunc = func(domain.Path, ports.IntegrateSpec) error { merged = true; return nil }
	dirty := true
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		if dirty {
			return []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "main.go"}}, nil
		}
		return nil, nil
	}
	lines := f.serve(t, `{"id":"1","method":"workspaces.updateRepo","params":{"workspace":"feat","repo":"api"}}`+"\n")
	dirty = false
	lines = append(lines, f.serve(t, `{"id":"2","method":"workspaces.update","params":{"workspace":"feat"}}`+"\n")...)
	compareGolden(t, "update.golden", lines)
}

// TestClaim_Golden pins workspaces.claim's wire shape, the orphanOf flag
// workspaces.list carries for a workspace whose owner context no longer
// exists, and the reassignedWorkspaces a rename reports.
func TestClaim_Golden(t *testing.T) {
	f := newFixture(t)
	f.store.PutManifest("/fx/workspaces/lost", domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{
		Name: "lost", Root: "/fx/workspaces/lost", Context: "gone", Branch: "lost",
	}})
	_ = f.fs.MkdirAll("/fx/workspaces/lost")
	input := strings.Join([]string{
		`{"id":"1","method":"workspaces.list"}`,
		`{"id":"2","method":"workspaces.claim","params":{"workspaces":["lost","feat","nope"]}}`,
		`{"id":"3","method":"workspaces.list"}`,
		`{"id":"4","method":"workspaces.claim"}`,
		`{"id":"5","method":"contexts.update","params":{"name":"work","newName":"job"}}`,
		`{"id":"6","method":"workspaces.list","params":{"context":"job"}}`,
	}, "\n") + "\n"
	compareGolden(t, "claim.golden", f.serve(t, input))
}
