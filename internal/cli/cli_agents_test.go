// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

const cliBundleSkill = "/Applications/wspace.app/Contents/Resources/skills/wspace-workspaces"

func agentsFixture(t *testing.T) (*fixture, *portstest.FakeAgentFS, *portstest.FakeCommandRunner) {
	t.Helper()
	fx := newFixture(t)
	afs := portstest.NewFakeAgentFS()
	runner := portstest.NewFakeCommandRunner()
	for _, d := range []string{"/home/u/.claude", cliBundleSkill, "/Applications/wspace.app/Contents/Helpers"} {
		if err := afs.MkdirAll(domain.Path(d)); err != nil {
			t.Fatal(err)
		}
	}
	if err := afs.WriteFile(cliBundleSkill+"/SKILL.md", []byte("skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := afs.WriteFile("/Applications/wspace.app/Contents/Helpers/wspace", []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	fx.RT.AgentsDeps = app.AgentsDeps{
		FS: afs, Runner: runner, Skill: fstest.MapFS{"wspace-workspaces/SKILL.md": {Data: []byte("x")}},
		Home: "/home/u", GOOS: "darwin", Getenv: func(string) string { return "" },
		Executable: func() (string, error) { return "/Applications/wspace.app/Contents/Helpers/wspace", nil },
		Now:        func() time.Time { return time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC) },
	}
	return fx, afs, runner
}

func TestCLI_Agents_StatusInstallUninstall(t *testing.T) {
	fx, afs, _ := agentsFixture(t)

	stdout, stderr, code := run(fx.RT, "", "agents", "status")
	if code != 0 || !strings.Contains(stdout, "claude-code") || !strings.Contains(stdout, "missing") {
		t.Fatalf("status = %q (stderr %q, code %d)", stdout, stderr, code)
	}

	stdout, stderr, code = run(fx.RT, "", "agents", "install")
	if code != 0 || !strings.Contains(stdout, "claude-code: linked /home/u/.claude/skills/wspace-workspaces -> "+cliBundleSkill) {
		t.Fatalf("install = %q (stderr %q, code %d)", stdout, stderr, code)
	}
	if e, _ := afs.Lstat("/home/u/.claude/skills/wspace-workspaces"); e.Kind != ports.EntrySymlink || e.LinkTarget != cliBundleSkill {
		t.Fatalf("link = %+v", e)
	}

	stdout, stderr, code = run(fx.RT, "", "agents", "uninstall", "--agent", "claude-code")
	if code != 0 || !strings.Contains(stdout, "claude-code: removed") {
		t.Fatalf("uninstall = %q (stderr %q, code %d)", stdout, stderr, code)
	}
}

func TestCLI_Agents_JSON(t *testing.T) {
	fx, _, _ := agentsFixture(t)

	stdout, stderr, code := run(fx.RT, "", "agents", "install", "--json", "--agent", "claude-code", "--agent", "gemini")
	if code != 0 {
		t.Fatalf("install --json exit %d, stderr %q", code, stderr)
	}
	var res struct {
		Skill struct {
			Name, Dir, Origin, Location string
			MCPServer                   string `json:"mcp_server"`
			MCPCommand                  string `json:"mcp_command"`
		} `json:"skill"`
		Changes []struct{ Agent, Kind, Path, Target string } `json:"changes"`
		Agents  []struct {
			ID        string `json:"id"`
			Detected  bool   `json:"detected"`
			Skill     string `json:"skill"`
			SkillPath string `json:"skill_path"`
			MCP       string `json:"mcp"`
		} `json:"agents"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("install --json output %q: %v", stdout, err)
	}
	if res.Skill.Dir != cliBundleSkill || res.Skill.Origin != "bundle" || res.Skill.MCPServer != "wspace" || res.Skill.Location != "stable" {
		t.Fatalf("skill = %+v", res.Skill)
	}
	if len(res.Changes) != 2 || res.Changes[0].Kind != "linked" || res.Changes[1].Agent != "gemini" {
		t.Fatalf("changes = %+v", res.Changes)
	}
	if len(res.Agents) != 2 || res.Agents[1].ID != "gemini" || res.Agents[1].Skill != "installed" {
		t.Fatalf("agents = %+v", res.Agents)
	}

	stdout, _, code = run(fx.RT, "", "agents", "status", "--json")
	if code != 0 || !strings.Contains(stdout, `"mcp_snippet"`) || !strings.Contains(stdout, `"id": "opencode"`) {
		t.Fatalf("status --json = %s", stdout)
	}
}

func TestCLI_Agents_UnknownAgentFails(t *testing.T) {
	fx, _, _ := agentsFixture(t)
	_, stderr, code := run(fx.RT, "", "agents", "install", "--agent", "vim")
	if code == 0 || !strings.Contains(stderr, "unknown agent") || !strings.Contains(stderr, "vim") {
		t.Fatalf("stderr %q, code %d", stderr, code)
	}
}

func TestCLI_Agents_MCPFlagRunsAgentCLI(t *testing.T) {
	fx, _, runner := agentsFixture(t)
	runner.OnPath["claude"] = "/bin/claude"
	_, stderr, code := run(fx.RT, "", "agents", "install", "--mcp", "--agent", "claude-code")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	want := "claude mcp add --scope user --transport stdio wspace -- /Applications/wspace.app/Contents/Helpers/wspace mcp serve"
	if len(runner.Calls) != 1 || runner.Calls[0] != want {
		t.Fatalf("calls = %v", runner.Calls)
	}
}

func TestCLI_Agents_ReportsAndCleansLocalScopeDuplicates(t *testing.T) {
	const exe = "/Applications/wspace.app/Contents/Helpers/wspace"
	fx, afs, runner := agentsFixture(t)
	runner.OnPath["claude"] = "/bin/claude"
	if err := afs.MkdirAll("/work/alpha"); err != nil {
		t.Fatal(err)
	}
	config := `{"mcpServers":{"wspace":{"command":"` + exe + `"}},"projects":{"/work/alpha":{"mcpServers":{"wspace":{"command":"/old/wspace"}}}}}`
	if err := afs.WriteFile("/home/u/.claude.json", []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := run(fx.RT, "", "agents", "status", "--agent", "claude-code")
	if code != 0 || !strings.Contains(stdout, "mcp local /work/alpha: /old/wspace (stale)") || !strings.Contains(stdout, "wspace agents mcp-clean") {
		t.Fatalf("status = %q (stderr %q, code %d)", stdout, stderr, code)
	}

	stdout, _, code = run(fx.RT, "", "agents", "status", "--agent", "claude-code", "--json")
	var st struct {
		Agents []struct {
			Registrations []struct {
				Scope   string `json:"scope"`
				Project string `json:"project"`
				Command string `json:"command"`
				Stale   bool   `json:"stale"`
			} `json:"mcp_registrations"`
			Duplicate bool `json:"mcp_duplicate"`
			Stale     bool `json:"mcp_stale"`
		} `json:"agents"`
	}
	if err := json.Unmarshal([]byte(stdout), &st); code != 0 || err != nil {
		t.Fatalf("status --json = %s (%v)", stdout, err)
	}
	a := st.Agents[0]
	if !a.Duplicate || !a.Stale || len(a.Registrations) != 2 || a.Registrations[1].Scope != "local" || a.Registrations[1].Project != "/work/alpha" || !a.Registrations[1].Stale {
		t.Fatalf("agent = %+v", a)
	}

	stdout, _, code = run(fx.RT, "", "agents", "mcp-clean", "--dry-run")
	if code != 0 || !strings.Contains(stdout, "would remove the local wspace registration of /work/alpha (/old/wspace)") || len(runner.Calls) != 0 {
		t.Fatalf("dry run = %q, calls %v", stdout, runner.Calls)
	}

	stdout, _, code = run(fx.RT, "", "agents", "mcp-clean", "--agent", "claude-code", "--json")
	if code != 0 || !strings.Contains(stdout, `"kind": "mcp_duplicate_removed"`) {
		t.Fatalf("mcp-clean --json = %s", stdout)
	}
	if len(runner.Calls) != 1 || runner.Calls[0] != "claude mcp remove wspace -s local" || runner.Dirs[0] != "/work/alpha" {
		t.Fatalf("calls = %v in %v", runner.Calls, runner.Dirs)
	}
}

func TestCLI_Agents_FailedMCPRegistrationPrintsTheManualCommand(t *testing.T) {
	fx, _, runner := agentsFixture(t)
	runner.OnPath["claude"] = "/bin/claude"
	const exe = "/Applications/wspace.app/Contents/Helpers/wspace"
	const manual = "claude mcp add --scope user --transport stdio wspace -- " + exe + " mcp serve"
	runner.Results[manual] = ports.CommandResult{ExitCode: 1, Stderr: "not compatible\n"}

	stdout, stderr, code := run(fx.RT, "", "agents", "install", "--agent", "claude-code", "--mcp", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var res struct {
		Changes []struct {
			Kind          string `json:"kind"`
			Detail        string `json:"detail"`
			ManualCommand string `json:"manual_command"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range res.Changes {
		if c.Kind == "mcp_failed" {
			found = c.Detail == "not compatible" && c.ManualCommand == manual
		}
	}
	if !found {
		t.Fatalf("json = %s", stdout)
	}

	stdout, _, _ = run(fx.RT, "", "agents", "install", "--agent", "claude-code", "--mcp")
	if !strings.Contains(stdout, "failed: not compatible") || !strings.Contains(stdout, "\n  "+manual+"\n") {
		t.Fatalf("human output = %q", stdout)
	}
}
