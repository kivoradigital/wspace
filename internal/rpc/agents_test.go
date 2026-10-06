// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package rpc_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
	"github.com/kivoradigital/wspace/internal/rpc"
)

// agentsServer serves an engine whose agent installation runs over an
// in-memory filesystem: an app bundle at /Applications/wspace.app, a home
// with Claude Code and a legacy skill, the gemini CLI on PATH.
func agentsServer(t *testing.T) (*rpc.Server, *portstest.FakeAgentFS) {
	t.Helper()
	afs := portstest.NewFakeAgentFS()
	const skill = "/Applications/wspace.app/Contents/Resources/skills/wspace-workspaces"
	for _, d := range []string{skill, "/Applications/wspace.app/Contents/Helpers", "/home/u/.claude/skills/ws-workspaces"} {
		if err := afs.MkdirAll(domain.Path(d)); err != nil {
			t.Fatal(err)
		}
	}
	_ = afs.WriteFile(skill+"/SKILL.md", []byte("skill"), 0o644)
	_ = afs.WriteFile("/Applications/wspace.app/Contents/Helpers/wspace", []byte("bin"), 0o755)
	_ = afs.MkdirAll("/work/alpha")
	_ = afs.WriteFile("/home/u/.claude.json", []byte(`{"mcpServers":{"wspace":{"command":"/Applications/wspace.app/Contents/Helpers/wspace"}},"projects":{"/work/alpha":{"mcpServers":{"wspace":{"command":"/old/wspace"}}}}}`), 0o644)
	runner := portstest.NewFakeCommandRunner()
	runner.OnPath["gemini"] = "/opt/bin/gemini"
	runner.OnPath["claude"] = "/opt/bin/claude"

	eng := engine.New(engine.Deps{Version: "1.2.3", Agents: &engine.AgentsDeps{
		FS: afs, Runner: runner, Skill: fstest.MapFS{"wspace-workspaces/SKILL.md": {Data: []byte("x")}},
		Home: "/home/u", GOOS: "darwin", Getenv: func(string) string { return "" },
		Executable: func() (string, error) { return "/Applications/wspace.app/Contents/Helpers/wspace", nil },
		Now:        func() time.Time { return time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC) },
	}})
	return rpc.NewServer(eng, "1.2.3"), afs
}

// TestAgents_Golden pins agents.status, agents.install,
// agents.uninstall and agents.mcpClean on the wire.
func TestAgents_Golden(t *testing.T) {
	srv, afs := agentsServer(t)
	input := strings.Join([]string{
		`{"id":"1","method":"agents.status","params":{"agents":["claude-code","gemini"]}}`,
		`{"id":"2","method":"agents.install","params":{"disableLegacy":true}}`,
		`{"id":"3","method":"agents.install","params":{"agents":["gemini"],"skill":false,"mcp":true}}`,
		`{"id":"4","method":"agents.uninstall","params":{"agents":["claude-code"]}}`,
		`{"id":"5","method":"agents.status","params":{"agents":["vim"]}}`,
		`{"id":"6","method":"agents.install","params":{"bogus":true}}`,
		`{"id":"7","method":"agents.mcpClean","params":{"dryRun":true}}`,
		`{"id":"8","method":"agents.mcpClean","params":{"agents":["claude-code"]}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := srv.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	compareGolden(t, "agents.golden", strings.Split(strings.TrimRight(out.String(), "\n"), "\n"))

	if e, _ := afs.Lstat("/home/u/.gemini/skills/wspace-workspaces"); e.Kind != ports.EntrySymlink {
		t.Fatalf("gemini skill entry = %+v", e)
	}
	if ok, _ := afs.Exists("/home/u/.claude/skills-disabled/ws-workspaces"); !ok {
		t.Fatal("the legacy skill must have been moved aside")
	}
}

func TestAgents_UnavailableWithoutDeps(t *testing.T) {
	srv := rpc.NewServer(engine.New(engine.Deps{Version: "1.2.3"}), "1.2.3")
	var out bytes.Buffer
	if err := srv.Serve(context.Background(), strings.NewReader(`{"id":"1","method":"agents.status"}`+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"code":"internal"`) {
		t.Fatalf("out = %s", out.String())
	}
}
