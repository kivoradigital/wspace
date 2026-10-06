// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/ports"
)

const (
	projA = "/work/alpha"
	projB = "/work/beta"
)

// claudeJSON builds a ~/.claude.json with an optional user-scope wspace
// command and local-scope wspace commands keyed by project path.
func claudeJSON(user string, local map[string]string) string {
	var b strings.Builder
	b.WriteString(`{"numStartups":3`)
	if user != "" {
		b.WriteString(`,"mcpServers":{"wspace":{"type":"stdio","command":"` + user + `","args":["mcp","serve"]},"other":{"command":"/x"}}`)
	}
	b.WriteString(`,"projects":{"/work/none":{"allowedTools":[]},"/work/other":{"mcpServers":{"other":{"command":"/x"}}}`)
	for p, cmd := range local {
		b.WriteString(`,"` + p + `":{"mcpServers":{"wspace":{"type":"stdio","command":"` + cmd + `","args":["mcp","serve"]}}}`)
	}
	b.WriteString(`}}`)
	return b.String()
}

func claudeStatus(t *testing.T, f *agentsFixture) app.AgentStatus {
	t.Helper()
	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{Agents: []string{"claude-code"}})
	if err != nil {
		t.Fatal(err)
	}
	return r.Agents[0]
}

func TestAgentsStatus_ClaudeCodeReportsEveryScope(t *testing.T) {
	const gone = "/old/build/wspace"
	tests := []struct {
		name      string
		config    string
		wantMCP   app.MCPState
		wantRegs  []app.MCPRegistration
		duplicate bool
		stale     bool
	}{
		{
			name:     "user scope only",
			config:   claudeJSON(bundleExe, nil),
			wantMCP:  app.MCPRegistered,
			wantRegs: []app.MCPRegistration{{Scope: app.MCPScopeUser, Command: bundleExe}},
		},
		{
			name:     "local scope only is not a user registration",
			config:   claudeJSON("", map[string]string{projA: bundleExe}),
			wantMCP:  app.MCPNotRegistered,
			wantRegs: []app.MCPRegistration{{Scope: app.MCPScopeLocal, Project: projA, Command: bundleExe}},
		},
		{
			name:    "user and local scopes are duplicates",
			config:  claudeJSON(bundleExe, map[string]string{projB: bundleExe, projA: bundleExe}),
			wantMCP: app.MCPRegistered,
			wantRegs: []app.MCPRegistration{
				{Scope: app.MCPScopeUser, Command: bundleExe},
				{Scope: app.MCPScopeLocal, Project: projA, Command: bundleExe},
				{Scope: app.MCPScopeLocal, Project: projB, Command: bundleExe},
			},
			duplicate: true,
		},
		{
			name:    "a command that no longer exists is stale",
			config:  claudeJSON(gone, map[string]string{projA: bundleExe}),
			wantMCP: app.MCPRegistered,
			wantRegs: []app.MCPRegistration{
				{Scope: app.MCPScopeUser, Command: gone, Stale: true},
				{Scope: app.MCPScopeLocal, Project: projA, Command: bundleExe},
			},
			duplicate: true,
			stale:     true,
		},
		{
			name:    "malformed configuration is unknown",
			config:  `{"mcpServers": {`,
			wantMCP: app.MCPUnknown,
		},
		{
			name:      "a malformed project entry is skipped",
			config:    `{"mcpServers":{"wspace":{"command":"` + bundleExe + `"}},"projects":{"/p":"x","/q":{"mcpServers":[1]},"/r":{"mcpServers":{"wspace":{"command":7}}}}}`,
			wantMCP:   app.MCPRegistered,
			wantRegs:  []app.MCPRegistration{{Scope: app.MCPScopeUser, Command: bundleExe}, {Scope: app.MCPScopeLocal, Project: "/r"}},
			duplicate: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAgentsFixture(t)
			f.write(t, agentsHome+"/.claude.json", tt.config)
			s := claudeStatus(t, f)
			if s.MCP != tt.wantMCP {
				t.Errorf("mcp = %q, want %q", s.MCP, tt.wantMCP)
			}
			if !equalRegs(s.MCPRegistrations, tt.wantRegs) {
				t.Errorf("registrations = %+v, want %+v", s.MCPRegistrations, tt.wantRegs)
			}
			if s.MCPDuplicate != tt.duplicate || s.MCPStale != tt.stale {
				t.Errorf("duplicate/stale = %v/%v, want %v/%v", s.MCPDuplicate, s.MCPStale, tt.duplicate, tt.stale)
			}
		})
	}
}

func equalRegs(a, b []app.MCPRegistration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAgentsStatus_StableCommandsAreNotStale(t *testing.T) {
	const otherBuild = "/src/ws/apps/wspace/dist/wspace"
	f := newAgentsFixture(t)
	f.link(t, bundleExe, agentsHome+"/.local/bin/wspace")
	f.write(t, otherBuild, "binary")
	f.write(t, "/Applications/Other.app/Contents/Helpers/wspace", "binary")
	f.write(t, agentsHome+"/.claude.json", claudeJSON(agentsHome+"/.local/bin/wspace", map[string]string{
		projA: bundleExe, projB: otherBuild, "/work/gamma": "/Applications/Other.app/Contents/Helpers/wspace",
	}))
	s := claudeStatus(t, f)
	for _, reg := range s.MCPRegistrations {
		if want := reg.Command == otherBuild; reg.Stale != want {
			t.Errorf("%+v: stale = %v, want %v", reg, reg.Stale, want)
		}
	}
	if !s.MCPStale {
		t.Error("a registration pointing at another wspace build must flag the agent stale")
	}
}

func TestAgentsStatus_OtherAgentsReportTheirUserRegistration(t *testing.T) {
	f := newAgentsFixture(t)
	f.write(t, agentsHome+"/.gemini/settings.json", `{"mcpServers":{"wspace":{"command":"`+bundleExe+`"}}}`)
	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{Agents: []string{"gemini", "codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Agents[0].MCPRegistrations; !equalRegs(got, []app.MCPRegistration{{Scope: app.MCPScopeUser, Command: bundleExe}}) {
		t.Errorf("gemini registrations = %+v", got)
	}
	if r.Agents[1].MCPRegistrations != nil || r.Agents[1].MCPDuplicate {
		t.Errorf("codex = %+v", r.Agents[1])
	}
}

func TestInstallAgents_MCPNeverAddsWhenTheUserScopeHasIt(t *testing.T) {
	f := newAgentsFixture(t)
	f.runner.OnPath["claude"] = "/bin/claude"
	f.write(t, agentsHome+"/.claude.json", claudeJSON(bundleExe, map[string]string{projA: bundleExe}))

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{Agents: []string{"claude-code"}, MCP: true, SkipSkill: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runner.Calls) != 0 {
		t.Fatalf("calls = %v", f.runner.Calls)
	}
	if !hasChange(r, "claude-code", app.ChangeMCPAlready) || !hasChange(r, "claude-code", app.ChangeMCPLocalDuplicate) {
		t.Fatalf("changes = %+v", r.Changes)
	}
}

func TestInstallAgents_MCPAddsTheUserScopeAndReportsLocalDuplicates(t *testing.T) {
	f := newAgentsFixture(t)
	f.runner.OnPath["claude"] = "/bin/claude"
	f.write(t, agentsHome+"/.claude.json", claudeJSON("", map[string]string{projA: "/old/wspace"}))

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{Agents: []string{"claude-code"}, MCP: true, SkipSkill: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "claude mcp add --scope user --transport stdio wspace -- " + bundleExe + " mcp serve"
	if len(f.runner.Calls) != 1 || f.runner.Calls[0] != want {
		t.Fatalf("calls = %v", f.runner.Calls)
	}
	var dup app.AgentChange
	for _, c := range changesOf(r, "claude-code") {
		if c.Kind == app.ChangeMCPLocalDuplicate {
			dup = c
		}
	}
	if dup.Path != projA || dup.Target != "/old/wspace" {
		t.Fatalf("duplicate change = %+v in %+v", dup, r.Changes)
	}
}

func TestCleanAgentsMCP_RemovesLocalDuplicatesFromEachProjectKeepingTheUserScope(t *testing.T) {
	f := newAgentsFixture(t)
	f.runner.OnPath["claude"] = "/bin/claude"
	f.mkdir(t, projA)
	f.mkdir(t, projB)
	f.write(t, agentsHome+"/.claude.json", claudeJSON(bundleExe, map[string]string{projA: bundleExe, projB: "/gone"}))

	r, err := app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{})
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{"claude mcp remove wspace -s local", "claude mcp remove wspace -s local"}
	if strings.Join(f.runner.Calls, "|") != strings.Join(wantCalls, "|") || strings.Join(f.runner.Dirs, "|") != projA+"|"+projB {
		t.Fatalf("calls = %v in %v", f.runner.Calls, f.runner.Dirs)
	}
	if n := len(changesOf(r, "claude-code")); n != 2 || !hasChange(r, "claude-code", app.ChangeMCPDuplicateRemoved) {
		t.Fatalf("changes = %+v", r.Changes)
	}
}

func TestCleanAgentsMCP_DryRunRunsNothing(t *testing.T) {
	f := newAgentsFixture(t)
	f.runner.OnPath["claude"] = "/bin/claude"
	f.mkdir(t, projA)
	f.write(t, agentsHome+"/.claude.json", claudeJSON(bundleExe, map[string]string{projA: bundleExe}))

	r, err := app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runner.Calls) != 0 {
		t.Fatalf("calls = %v", f.runner.Calls)
	}
	c := changesOf(r, "claude-code")
	if len(c) != 1 || c[0].Kind != app.ChangeMCPDuplicateWouldRemove || c[0].Path != projA || c[0].Target != bundleExe {
		t.Fatalf("changes = %+v", r.Changes)
	}
}

func TestCleanAgentsMCP_WithoutAUserScopeKeepsExactlyOnePreferringTheStableCommand(t *testing.T) {
	f := newAgentsFixture(t)
	f.runner.OnPath["claude"] = "/bin/claude"
	f.mkdir(t, projA)
	f.mkdir(t, projB)
	f.write(t, agentsHome+"/.claude.json", claudeJSON("", map[string]string{projA: "/gone", projB: bundleExe}))

	r, err := app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runner.Calls) != 1 || f.runner.Dirs[0] != projA {
		t.Fatalf("calls = %v in %v", f.runner.Calls, f.runner.Dirs)
	}
	var kept app.AgentChange
	for _, c := range changesOf(r, "claude-code") {
		if c.Kind == app.ChangeMCPDuplicateKept {
			kept = c
		}
	}
	if kept.Path != projB {
		t.Fatalf("kept = %+v in %+v", kept, r.Changes)
	}
}

func TestCleanAgentsMCP_ASingleLocalRegistrationIsLeftAlone(t *testing.T) {
	f := newAgentsFixture(t)
	f.runner.OnPath["claude"] = "/bin/claude"
	f.mkdir(t, projA)
	f.write(t, agentsHome+"/.claude.json", claudeJSON("", map[string]string{projA: bundleExe}))

	r, err := app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runner.Calls) != 0 || len(r.Changes) != 0 {
		t.Fatalf("calls = %v, changes = %+v", f.runner.Calls, r.Changes)
	}
}

func TestCleanAgentsMCP_FailuresAreReportedPerProject(t *testing.T) {
	f := newAgentsFixture(t)
	f.runner.OnPath["claude"] = "/bin/claude"
	f.mkdir(t, projA)
	f.write(t, agentsHome+"/.claude.json", claudeJSON(bundleExe, map[string]string{projA: bundleExe, projB: bundleExe}))
	f.runner.Results["claude mcp remove wspace -s local"] = ports.CommandResult{ExitCode: 1, Stderr: "No MCP server named \"wspace\" in local scope\n"}

	r, err := app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runner.Calls) != 1 {
		t.Fatalf("a missing project directory must not run claude: calls = %v", f.runner.Calls)
	}
	failures := map[string]string{}
	for _, c := range changesOf(r, "claude-code") {
		if c.Kind == app.ChangeMCPCleanFailed {
			failures[c.Path] = c.Detail
		}
	}
	if !strings.Contains(failures[projA], "No MCP server") || failures[projB] == "" {
		t.Fatalf("failures = %v", failures)
	}
}

func TestCleanAgentsMCP_NeedsTheClaudeCLIAndAReadableConfig(t *testing.T) {
	f := newAgentsFixture(t)
	f.mkdir(t, projA)
	f.write(t, agentsHome+"/.claude.json", claudeJSON(bundleExe, map[string]string{projA: bundleExe}))
	r, err := app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(r, "claude-code", app.ChangeMCPCleanFailed) || len(f.runner.Calls) != 0 {
		t.Fatalf("without claude on PATH: changes = %+v, calls = %v", r.Changes, f.runner.Calls)
	}

	f.runner.OnPath["claude"] = "/bin/claude"
	f.write(t, agentsHome+"/.claude.json", "{ broken")
	r, err = app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(r, "claude-code", app.ChangeMCPCleanFailed) || len(f.runner.Calls) != 0 {
		t.Fatalf("unreadable config: changes = %+v, calls = %v", r.Changes, f.runner.Calls)
	}

	f.write(t, agentsHome+"/.claude.json", claudeJSON(bundleExe, map[string]string{projA: bundleExe}))
	f.runner.Err = errors.New("timed out")
	r, _ = app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{})
	if !hasChange(r, "claude-code", app.ChangeMCPCleanFailed) {
		t.Fatalf("runner error: changes = %+v", r.Changes)
	}
}

func TestCleanAgentsMCP_OnlyTouchesThisInstallationsServerName(t *testing.T) {
	f := newAgentsFixture(t)
	f.runner.OnPath["claude"] = "/bin/claude"
	f.mkdir(t, projA)
	f.write(t, agentsHome+"/.claude.json", `{"mcpServers":{"wspace-dev":{"command":"/d"}},"projects":{"`+projA+`":{"mcpServers":{"wspace-dev":{"command":"/d"}}}}}`)
	r, err := app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runner.Calls) != 0 || len(r.Changes) != 0 {
		t.Fatalf("calls = %v, changes = %+v", f.runner.Calls, r.Changes)
	}
}

func TestCleanAgentsMCP_UnknownAgentIsRejected(t *testing.T) {
	f := newAgentsFixture(t)
	if _, err := app.CleanAgentsMCP(context.Background(), f.deps, app.CleanAgentsMCPInput{Agents: []string{"vim"}}); err == nil {
		t.Fatal("want an error")
	}
}
