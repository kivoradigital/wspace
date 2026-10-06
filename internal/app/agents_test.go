// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

const (
	agentsHome   = "/home/u"
	bundle       = "/Applications/wspace.app"
	bundleExe    = bundle + "/Contents/Helpers/wspace"
	bundleSkill  = bundle + "/Contents/Resources/skills/wspace-workspaces"
	extractedDir = agentsHome + "/.local/share/wspace/skills/wspace-workspaces"
)

var embeddedSkill = fstest.MapFS{
	"wspace-workspaces/SKILL.md":            {Data: []byte("---\nname: wspace-workspaces\n---\nv1\n")},
	"wspace-workspaces/references/tools.md": {Data: []byte("tools v1\n")},
}

type agentsFixture struct {
	deps   app.AgentsDeps
	fs     *portstest.FakeAgentFS
	runner *portstest.FakeCommandRunner
	exe    string
}

// newAgentsFixture is a darwin machine with an installed app bundle whose
// engine is the running executable, an empty home, and nothing on PATH.
func newAgentsFixture(t *testing.T) *agentsFixture {
	t.Helper()
	fs := portstest.NewFakeAgentFS()
	runner := portstest.NewFakeCommandRunner()
	f := &agentsFixture{fs: fs, runner: runner, exe: bundleExe}
	f.mkdir(t, agentsHome)
	f.write(t, bundleExe, "binary")
	f.write(t, bundleSkill+"/SKILL.md", "skill")
	f.deps = app.AgentsDeps{
		FS:         fs,
		Runner:     runner,
		Skill:      embeddedSkill,
		Home:       agentsHome,
		GOOS:       "darwin",
		Getenv:     func(string) string { return "" },
		Executable: func() (string, error) { return f.exe, nil },
		Now:        func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) },
	}
	return f
}

func (f *agentsFixture) mkdir(t *testing.T, p string) {
	t.Helper()
	if err := f.fs.MkdirAll(domain.Path(p)); err != nil {
		t.Fatal(err)
	}
}

func (f *agentsFixture) write(t *testing.T, p, content string) {
	t.Helper()
	f.mkdir(t, p[:strings.LastIndex(p, "/")])
	if err := f.fs.WriteFile(domain.Path(p), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *agentsFixture) link(t *testing.T, target, link string) {
	t.Helper()
	f.mkdir(t, link[:strings.LastIndex(link, "/")])
	if err := f.fs.Symlink(domain.Path(target), domain.Path(link)); err != nil {
		t.Fatal(err)
	}
}

func (f *agentsFixture) entry(t *testing.T, p string) ports.Entry {
	t.Helper()
	e, err := f.fs.Lstat(domain.Path(p))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func agentByID(t *testing.T, agents []app.AgentStatus, id string) app.AgentStatus {
	t.Helper()
	for _, a := range agents {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("agent %q not in %+v", id, agents)
	return app.AgentStatus{}
}

func changesOf(r app.AgentsChangeReport, agent string) []app.AgentChange {
	var out []app.AgentChange
	for _, c := range r.Changes {
		if c.Agent == agent {
			out = append(out, c)
		}
	}
	return out
}

func hasChange(r app.AgentsChangeReport, agent string, kind app.AgentChangeKind) bool {
	for _, c := range changesOf(r, agent) {
		if c.Kind == kind {
			return true
		}
	}
	return false
}

// --- skill source resolution -------------------------------------------

func TestAgentsStatus_BundleSourceResolvedThroughTheCommandLineToolSymlink(t *testing.T) {
	f := newAgentsFixture(t)
	f.link(t, bundleExe, agentsHome+"/.local/bin/wspace")
	f.exe = agentsHome + "/.local/bin/wspace"

	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	want := app.SkillSource{
		Name: "wspace-workspaces", Dir: bundleSkill, Origin: app.SkillOriginBundle,
		Location: domain.LocationStable, MCPServer: "wspace", MCPCommand: agentsHome + "/.local/bin/wspace",
	}
	if r.Source != want {
		t.Fatalf("Source = %+v\nwant     %+v", r.Source, want)
	}
}

func TestAgentsStatus_MCPCommandIsTheEngineWhenNoCommandLineToolLinkPointsAtIt(t *testing.T) {
	f := newAgentsFixture(t)
	f.write(t, agentsHome+"/.local/bin/wspace", "a copied binary, not our link")

	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Source.MCPCommand != bundleExe {
		t.Fatalf("MCPCommand = %q, want %q", r.Source.MCPCommand, bundleExe)
	}
}

func TestAgentsStatus_DevelopmentBundleUsesItsOwnSkillAndServerNames(t *testing.T) {
	f := newAgentsFixture(t)
	_ = f.fs.RemoveAll(bundleSkill)
	f.write(t, bundle+"/Contents/Resources/skills/wspace-dev-workspaces/SKILL.md", "dev skill")

	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Source.Name != "wspace-dev-workspaces" || r.Source.MCPServer != "wspace-dev" ||
		r.Source.Dir != bundle+"/Contents/Resources/skills/wspace-dev-workspaces" {
		t.Fatalf("Source = %+v", r.Source)
	}
	claude := agentByID(t, r.Agents, "claude-code")
	if claude.SkillPath != agentsHome+"/.claude/skills/wspace-dev-workspaces" {
		t.Fatalf("SkillPath = %q", claude.SkillPath)
	}
}

func TestAgentsStatus_StandaloneBinaryUsesTheDataDirectoryWithoutWriting(t *testing.T) {
	f := newAgentsFixture(t)
	f.exe = "/usr/local/bin/wspace"
	f.write(t, f.exe, "binary")

	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Source.Dir != extractedDir || r.Source.Origin != app.SkillOriginEmbedded || r.Source.MCPCommand != "/usr/local/bin/wspace" {
		t.Fatalf("Source = %+v", r.Source)
	}
	if ok, _ := f.fs.Exists(extractedDir); ok {
		t.Fatal("status must never extract the skill")
	}
}

func TestInstallAgents_StandaloneExtractsTheEmbeddedSkillAndRefreshesItWhenItChanges(t *testing.T) {
	f := newAgentsFixture(t)
	f.exe = "/usr/local/bin/wspace"
	f.write(t, f.exe, "binary")
	f.mkdir(t, agentsHome+"/.claude")

	if _, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{}); err != nil {
		t.Fatal(err)
	}
	if data, _ := f.fs.ReadFile(extractedDir + "/references/tools.md"); string(data) != "tools v1\n" {
		t.Fatalf("extracted reference = %q", data)
	}
	if e := f.entry(t, agentsHome+"/.claude/skills/wspace-workspaces"); e.Kind != ports.EntrySymlink || e.LinkTarget != extractedDir {
		t.Fatalf("claude skill entry = %+v", e)
	}

	// A newer binary embeds different content: the extraction follows it,
	// and stale files from the old version disappear.
	f.write(t, extractedDir+"/stale.md", "old")
	f.deps.Skill = fstest.MapFS{"wspace-workspaces/SKILL.md": {Data: []byte("v2\n")}}
	if _, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{}); err != nil {
		t.Fatal(err)
	}
	if data, _ := f.fs.ReadFile(extractedDir + "/SKILL.md"); string(data) != "v2\n" {
		t.Fatalf("refreshed SKILL.md = %q", data)
	}
	if ok, _ := f.fs.Exists(extractedDir + "/stale.md"); ok {
		t.Fatal("refresh must drop files the new version no longer has")
	}
}

func TestInstallAgents_RefusesASourceInsideADiskImageOrATranslocatedApp(t *testing.T) {
	for _, b := range []string{"/Volumes/wspace/wspace.app", "/private/var/folders/x/T/AppTranslocation/ABC/d/wspace.app"} {
		t.Run(b, func(t *testing.T) {
			f := newAgentsFixture(t)
			f.exe = b + "/Contents/Helpers/wspace"
			f.write(t, f.exe, "binary")
			f.write(t, b+"/Contents/Resources/skills/wspace-workspaces/SKILL.md", "skill")
			f.mkdir(t, agentsHome+"/.claude")

			status, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{})
			if err != nil {
				t.Fatal(err)
			}
			if status.Source.Location == domain.LocationStable {
				t.Fatalf("status must report the unstable location, got %+v", status.Source)
			}

			_, err = app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{})
			if domain.Code(err) != domain.CodeSkillSourceUnstable {
				t.Fatalf("InstallAgents error = %v, want %s", err, domain.CodeSkillSourceUnstable)
			}
			if e := f.entry(t, agentsHome+"/.claude/skills/wspace-workspaces"); e.Kind != ports.EntryMissing {
				t.Fatal("nothing may be linked from an unstable source")
			}
		})
	}
}

// --- detection and status ----------------------------------------------

func TestAgentsStatus_DetectsAgentsByConfigDirectoryOrExecutable(t *testing.T) {
	f := newAgentsFixture(t)
	f.mkdir(t, agentsHome+"/.claude")
	f.mkdir(t, agentsHome+"/.config/opencode")
	f.runner.OnPath["gemini"] = "/opt/bin/gemini"

	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, a := range r.Agents {
		ids = append(ids, a.ID)
	}
	if got := strings.Join(ids, ","); got != "claude-code,claude-desktop,codex,cursor,gemini,opencode" {
		t.Fatalf("agents = %s", got)
	}
	for id, want := range map[string]bool{"claude-code": true, "opencode": true, "gemini": true, "codex": false, "cursor": false, "claude-desktop": false} {
		if got := agentByID(t, r.Agents, id).Detected; got != want {
			t.Errorf("%s detected = %v, want %v", id, got, want)
		}
	}
	for id, dir := range map[string]string{
		"claude-code": "/.claude/skills", "opencode": "/.config/opencode/skills", "gemini": "/.gemini/skills",
		"cursor": "/.cursor/skills", "codex": "/.agents/skills", "claude-desktop": "",
	} {
		a := agentByID(t, r.Agents, id)
		want := ""
		if dir != "" {
			want = agentsHome + dir
		}
		if a.SkillsDir != want {
			t.Errorf("%s skills dir = %q, want %q", id, a.SkillsDir, want)
		}
		if dir == "" && a.Skill != app.SkillUnsupported {
			t.Errorf("%s skill = %q, want unsupported", id, a.Skill)
		}
	}
}

func TestAgentsStatus_ClaudeDesktopOnlyOnMacOS(t *testing.T) {
	f := newAgentsFixture(t)
	f.deps.GOOS = "linux"
	f.exe = "/usr/bin/wspace"
	f.write(t, f.exe, "binary")
	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r.Agents {
		if a.ID == "claude-desktop" {
			t.Fatal("Claude Desktop has no Linux configuration location")
		}
	}
}

func TestAgentsStatus_SkillStates(t *testing.T) {
	f := newAgentsFixture(t)
	f.link(t, bundleSkill, agentsHome+"/.claude/skills/wspace-workspaces")
	f.link(t, "/Users/u/Downloads/old/wspace.app/Contents/Resources/skills/wspace-workspaces", agentsHome+"/.gemini/skills/wspace-workspaces")
	f.link(t, "/Users/u/dotfiles/wspace-workspaces", agentsHome+"/.cursor/skills/wspace-workspaces")
	f.write(t, agentsHome+"/.agents/skills/wspace-workspaces/SKILL.md", "hand-written")
	f.write(t, agentsHome+"/.claude/skills/ws-workspaces/SKILL.md", "legacy")

	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]app.SkillState{
		"claude-code": app.SkillInstalled, "gemini": app.SkillOutdated, "cursor": app.SkillConflict,
		"codex": app.SkillConflict, "opencode": app.SkillMissing,
	} {
		if got := agentByID(t, r.Agents, id).Skill; got != want {
			t.Errorf("%s skill = %q, want %q", id, got, want)
		}
	}
	if got := agentByID(t, r.Agents, "cursor").SkillTarget; got != "/Users/u/dotfiles/wspace-workspaces" {
		t.Errorf("cursor skill target = %q", got)
	}
	if got := agentByID(t, r.Agents, "claude-code").LegacyPath; got != agentsHome+"/.claude/skills/ws-workspaces" {
		t.Errorf("claude legacy path = %q", got)
	}
	if got := agentByID(t, r.Agents, "gemini").LegacyPath; got != "" {
		t.Errorf("gemini legacy path = %q, want none", got)
	}
}

func TestAgentsStatus_FiltersByAgentAndRejectsUnknownIDs(t *testing.T) {
	f := newAgentsFixture(t)
	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{Agents: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Agents) != 1 || r.Agents[0].ID != "codex" {
		t.Fatalf("agents = %+v", r.Agents)
	}
	_, err = app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{Agents: []string{"vim"}})
	if domain.Code(err) != domain.CodeUnknownAgent {
		t.Fatalf("error = %v, want %s", err, domain.CodeUnknownAgent)
	}
}

func TestAgentsStatus_ReadsMCPRegistrationsFromConfigFiles(t *testing.T) {
	f := newAgentsFixture(t)
	f.write(t, agentsHome+"/.claude.json", `{"numStartups":3,"mcpServers":{"wspace":{"type":"stdio","command":"/x"}}}`)
	f.write(t, agentsHome+"/.codex/config.toml", "model = \"o3\"\n\n[mcp_servers.other]\ncommand = \"x\"\n")
	f.write(t, agentsHome+"/.gemini/settings.json", `{"mcpServers": {"wspace": {}}}`)
	f.write(t, agentsHome+"/.cursor/mcp.json", `{ not json`)
	f.write(t, agentsHome+"/.config/opencode/opencode.json", `{"mcp":{"wspace":{"type":"local"}}}`)

	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]app.MCPState{
		"claude-code": app.MCPRegistered, "codex": app.MCPNotRegistered, "gemini": app.MCPRegistered,
		"cursor": app.MCPUnknown, "opencode": app.MCPRegistered, "claude-desktop": app.MCPNotRegistered,
	} {
		if got := agentByID(t, r.Agents, id).MCP; got != want {
			t.Errorf("%s mcp = %q, want %q", id, got, want)
		}
	}
	if got := agentByID(t, r.Agents, "codex").MCPConfigPath; got != agentsHome+"/.codex/config.toml" {
		t.Errorf("codex config path = %q", got)
	}
	f.write(t, agentsHome+"/.codex/config.toml", "[mcp_servers.wspace]\ncommand = \"x\"\n")
	r, _ = app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{Agents: []string{"codex"}})
	if r.Agents[0].MCP != app.MCPRegistered {
		t.Errorf("codex mcp = %q after adding [mcp_servers.wspace]", r.Agents[0].MCP)
	}
}

// --- install -------------------------------------------------------------

func TestInstallAgents_LinksDetectedAgentsIntoTheBundle(t *testing.T) {
	f := newAgentsFixture(t)
	f.mkdir(t, agentsHome+"/.claude")
	f.mkdir(t, agentsHome+"/.gemini")

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{agentsHome + "/.claude/skills/wspace-workspaces", agentsHome + "/.gemini/skills/wspace-workspaces"} {
		if e := f.entry(t, p); e.Kind != ports.EntrySymlink || e.LinkTarget != bundleSkill {
			t.Fatalf("%s = %+v, want a link to the bundle", p, e)
		}
	}
	if e := f.entry(t, agentsHome+"/.cursor"); e.Kind != ports.EntryMissing {
		t.Fatal("an undetected agent must be left alone")
	}
	if !hasChange(r, "claude-code", app.ChangeLinked) || !hasChange(r, "gemini", app.ChangeLinked) {
		t.Fatalf("changes = %+v", r.Changes)
	}
	if got := agentByID(t, r.Agents, "claude-code").Skill; got != app.SkillInstalled {
		t.Fatalf("status after install = %q", got)
	}

	// A second run changes nothing.
	r, err = app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(r, "claude-code", app.ChangeUnchanged) {
		t.Fatalf("second run changes = %+v", r.Changes)
	}
}

func TestInstallAgents_NamedAgentIsInstalledEvenWhenNotDetected(t *testing.T) {
	f := newAgentsFixture(t)
	if _, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{Agents: []string{"cursor"}}); err != nil {
		t.Fatal(err)
	}
	if e := f.entry(t, agentsHome+"/.cursor/skills/wspace-workspaces"); e.Kind != ports.EntrySymlink {
		t.Fatalf("cursor entry = %+v", e)
	}
}

func TestInstallAgents_NoAgentDetectedReportsIt(t *testing.T) {
	f := newAgentsFixture(t)
	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Changes) != 1 || r.Changes[0].Kind != app.ChangeNoAgents {
		t.Fatalf("changes = %+v", r.Changes)
	}
}

func TestInstallAgents_RefreshesAnOutdatedLinkOfOurs(t *testing.T) {
	f := newAgentsFixture(t)
	old := "/Users/u/Downloads/wspace.app/Contents/Resources/skills/wspace-workspaces"
	f.link(t, old, agentsHome+"/.claude/skills/wspace-workspaces")

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if e := f.entry(t, agentsHome+"/.claude/skills/wspace-workspaces"); e.LinkTarget != bundleSkill {
		t.Fatalf("link = %+v", e)
	}
	if !hasChange(r, "claude-code", app.ChangeRefreshed) {
		t.Fatalf("changes = %+v", r.Changes)
	}
}

func TestInstallAgents_ConflictsNeedForceAndNeverDeleteAnything(t *testing.T) {
	f := newAgentsFixture(t)
	foreign := "/Users/u/dotfiles/wspace-workspaces"
	f.link(t, foreign, agentsHome+"/.claude/skills/wspace-workspaces")
	f.write(t, agentsHome+"/.gemini/skills/wspace-workspaces/SKILL.md", "mine")

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(r, "claude-code", app.ChangeConflict) || !hasChange(r, "gemini", app.ChangeConflict) {
		t.Fatalf("changes = %+v", r.Changes)
	}
	if e := f.entry(t, agentsHome+"/.claude/skills/wspace-workspaces"); e.LinkTarget != foreign {
		t.Fatal("without force a foreign link stays")
	}

	r, err = app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if e := f.entry(t, agentsHome+"/.claude/skills/wspace-workspaces"); e.LinkTarget != bundleSkill {
		t.Fatalf("forced link = %+v", e)
	}
	if e := f.entry(t, agentsHome+"/.gemini/skills/wspace-workspaces"); e.LinkTarget != bundleSkill {
		t.Fatalf("forced gemini entry = %+v", e)
	}
	backup := agentsHome + "/.gemini/skills/.wspace-backup-20261003-120000/wspace-workspaces/SKILL.md"
	if data, _ := f.fs.ReadFile(domain.Path(backup)); string(data) != "mine" {
		t.Fatalf("the real directory must be moved to %s, got %q", backup, data)
	}
	var gem app.AgentChange
	for _, c := range changesOf(r, "gemini") {
		if c.Kind == app.ChangeReplaced {
			gem = c
		}
	}
	if gem.Backup != agentsHome+"/.gemini/skills/.wspace-backup-20261003-120000/wspace-workspaces" {
		t.Fatalf("gemini change = %+v", gem)
	}
}

func TestInstallAgents_LegacySkillIsReportedAndDisabledOnlyOnRequest(t *testing.T) {
	f := newAgentsFixture(t)
	legacy := agentsHome + "/.claude/skills/ws-workspaces"
	f.write(t, legacy+"/SKILL.md", "legacy")

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(r, "claude-code", app.ChangeLegacyFound) {
		t.Fatalf("changes = %+v", r.Changes)
	}
	if e := f.entry(t, legacy); e.Kind != ports.EntryDir {
		t.Fatal("legacy skill must stay without --disable-legacy")
	}

	r, err = app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{DisableLegacy: true})
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(r, "claude-code", app.ChangeLegacyDisabled) {
		t.Fatalf("changes = %+v", r.Changes)
	}
	if data, _ := f.fs.ReadFile(agentsHome + "/.claude/skills-disabled/ws-workspaces/SKILL.md"); string(data) != "legacy" {
		t.Fatal("legacy skill must be moved to skills-disabled, not deleted")
	}
	if e := f.entry(t, legacy); e.Kind != ports.EntryMissing {
		t.Fatal("legacy skill must be gone from the skills directory")
	}
}

func TestInstallAgents_DisablingLegacyTwiceKeepsBothCopies(t *testing.T) {
	f := newAgentsFixture(t)
	f.write(t, agentsHome+"/.claude/skills-disabled/ws-workspaces/SKILL.md", "first")
	f.write(t, agentsHome+"/.claude/skills/ws-workspaces/SKILL.md", "second")

	if _, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{DisableLegacy: true, SkipSkill: true}); err != nil {
		t.Fatal(err)
	}
	if data, _ := f.fs.ReadFile(agentsHome + "/.claude/skills-disabled/ws-workspaces/SKILL.md"); string(data) != "first" {
		t.Fatal("an earlier disabled copy must survive")
	}
	if data, _ := f.fs.ReadFile(agentsHome + "/.claude/skills-disabled/ws-workspaces-20261003-120000/SKILL.md"); string(data) != "second" {
		t.Fatal("the second copy must land next to it with a timestamp")
	}
	if e := f.entry(t, agentsHome+"/.claude/skills/wspace-workspaces"); e.Kind != ports.EntryMissing {
		t.Fatal("SkipSkill must not link the skill")
	}
}

func TestInstallAgents_FallsBackToAMarkedCopyWhenSymlinksAreUnsupported(t *testing.T) {
	f := newAgentsFixture(t)
	f.mkdir(t, agentsHome+"/.claude")
	f.fs.SymlinkUnsupported = true

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(r, "claude-code", app.ChangeCopied) {
		t.Fatalf("changes = %+v", r.Changes)
	}
	dst := agentsHome + "/.claude/skills/wspace-workspaces"
	if data, _ := f.fs.ReadFile(domain.Path(dst + "/SKILL.md")); string(data) != "skill" {
		t.Fatal("the copy must hold the skill")
	}
	if got := agentByID(t, r.Agents, "claude-code").Skill; got != app.SkillInstalled {
		t.Fatalf("a marked copy of the current source is installed, got %q", got)
	}

	u, err := app.UninstallAgents(context.Background(), f.deps, app.UninstallAgentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(u, "claude-code", app.ChangeRemoved) {
		t.Fatalf("uninstall changes = %+v", u.Changes)
	}
	if e := f.entry(t, dst); e.Kind != ports.EntryMissing {
		t.Fatal("uninstall must remove our marked copy")
	}
}

// --- uninstall -----------------------------------------------------------

func TestUninstallAgents_RemovesOnlyOurLinks(t *testing.T) {
	f := newAgentsFixture(t)
	f.link(t, bundleSkill, agentsHome+"/.claude/skills/wspace-workspaces")
	f.link(t, "/Users/u/Downloads/wspace.app/Contents/Resources/skills/wspace-workspaces", agentsHome+"/.gemini/skills/wspace-workspaces")
	f.link(t, "/Users/u/dotfiles/wspace-workspaces", agentsHome+"/.cursor/skills/wspace-workspaces")
	f.write(t, agentsHome+"/.agents/skills/wspace-workspaces/SKILL.md", "hand-written")
	f.write(t, agentsHome+"/.claude/skills/ws-workspaces/SKILL.md", "legacy")

	r, err := app.UninstallAgents(context.Background(), f.deps, app.UninstallAgentsInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{agentsHome + "/.claude/skills/wspace-workspaces", agentsHome + "/.gemini/skills/wspace-workspaces"} {
		if e := f.entry(t, p); e.Kind != ports.EntryMissing {
			t.Fatalf("%s must be removed", p)
		}
	}
	if e := f.entry(t, agentsHome+"/.cursor/skills/wspace-workspaces"); e.Kind != ports.EntrySymlink {
		t.Fatal("a foreign link must stay")
	}
	if e := f.entry(t, agentsHome+"/.agents/skills/wspace-workspaces"); e.Kind != ports.EntryDir {
		t.Fatal("a real directory must stay")
	}
	if e := f.entry(t, agentsHome+"/.claude/skills/ws-workspaces"); e.Kind != ports.EntryDir {
		t.Fatal("uninstall never touches the legacy skill")
	}
	if ok, _ := f.fs.Exists(bundleSkill + "/SKILL.md"); !ok {
		t.Fatal("removing a link must not touch the bundle")
	}
	if !hasChange(r, "cursor", app.ChangeKept) || !hasChange(r, "codex", app.ChangeKept) {
		t.Fatalf("changes = %+v", r.Changes)
	}
}

// --- MCP registration ----------------------------------------------------

func TestInstallAgents_MCPRegistersThroughAgentCLIsAndNeverDuplicates(t *testing.T) {
	f := newAgentsFixture(t)
	for _, d := range []string{"/.claude", "/.codex", "/.gemini", "/.cursor", "/.config/opencode"} {
		f.mkdir(t, agentsHome+d)
	}
	f.write(t, agentsHome+"/.claude.json", `{"mcpServers":{"wspace":{}}}`)
	f.runner.OnPath["claude"] = "/bin/claude"
	f.runner.OnPath["codex"] = "/bin/codex"
	f.runner.OnPath["gemini"] = "/bin/gemini"
	f.runner.Results["gemini mcp add --scope user wspace "+bundleExe+" mcp serve"] = ports.CommandResult{ExitCode: 1, Stderr: "boom\nmore"}

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{MCP: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.runner.Calls, "\n"); got != "codex mcp add wspace -- "+bundleExe+" mcp serve\ngemini mcp add --scope user wspace "+bundleExe+" mcp serve" {
		t.Fatalf("calls:\n%s", got)
	}
	if !hasChange(r, "claude-code", app.ChangeMCPAlready) {
		t.Fatalf("claude changes = %+v", changesOf(r, "claude-code"))
	}
	if !hasChange(r, "codex", app.ChangeMCPRegistered) {
		t.Fatalf("codex changes = %+v", changesOf(r, "codex"))
	}
	for _, c := range changesOf(r, "gemini") {
		if c.Kind == app.ChangeMCPFailed && c.Detail != "boom" {
			t.Fatalf("gemini failure detail = %q", c.Detail)
		}
	}
	if !hasChange(r, "gemini", app.ChangeMCPFailed) {
		t.Fatalf("gemini changes = %+v", changesOf(r, "gemini"))
	}
	for _, id := range []string{"cursor", "opencode"} {
		var snippet app.AgentChange
		for _, c := range changesOf(r, id) {
			if c.Kind == app.ChangeMCPSnippet {
				snippet = c
			}
		}
		if !strings.Contains(snippet.Detail, bundleExe) || snippet.Path == "" {
			t.Fatalf("%s snippet change = %+v", id, snippet)
		}
	}
	if ok, _ := f.fs.Exists(agentsHome + "/.cursor/mcp.json"); ok {
		t.Fatal("file-based agents are never edited")
	}
}

func TestInstallAgents_MCPWithUnreadableConfigIsNotRegisteredBlindly(t *testing.T) {
	f := newAgentsFixture(t)
	f.mkdir(t, agentsHome+"/.gemini")
	f.write(t, agentsHome+"/.gemini/settings.json", "{ broken")
	f.runner.OnPath["gemini"] = "/bin/gemini"

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{MCP: true, SkipSkill: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runner.Calls) != 0 {
		t.Fatalf("calls = %v", f.runner.Calls)
	}
	if !hasChange(r, "gemini", app.ChangeMCPUnknown) {
		t.Fatalf("changes = %+v", r.Changes)
	}
}

func TestInstallAgents_MCPForACLIAgentWithoutItsCLIPrintsTheCommand(t *testing.T) {
	f := newAgentsFixture(t)
	f.mkdir(t, agentsHome+"/.claude")

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{MCP: true, SkipSkill: true})
	if err != nil {
		t.Fatal(err)
	}
	var c app.AgentChange
	for _, ch := range changesOf(r, "claude-code") {
		c = ch
	}
	if c.Kind != app.ChangeMCPSnippet || c.Detail != "claude mcp add --scope user --transport stdio wspace -- "+bundleExe+" mcp serve" {
		t.Fatalf("change = %+v", c)
	}
}

func TestInstallAgents_RunnerErrorIsReportedPerAgent(t *testing.T) {
	f := newAgentsFixture(t)
	f.mkdir(t, agentsHome+"/.codex")
	f.runner.OnPath["codex"] = "/bin/codex"
	f.runner.Err = errors.New("timed out")

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{MCP: true, SkipSkill: true})
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(r, "codex", app.ChangeMCPFailed) {
		t.Fatalf("changes = %+v", r.Changes)
	}
}

func TestRenderAgentChanges_EveryKindRendersWithoutFormatErrors(t *testing.T) {
	kinds := []app.AgentChangeKind{
		app.ChangeLinked, app.ChangeCopied, app.ChangeRefreshed, app.ChangeUnchanged, app.ChangeReplaced,
		app.ChangeConflict, app.ChangeLegacyFound, app.ChangeLegacyDisabled, app.ChangeRemoved, app.ChangeKept,
		app.ChangeMCPRegistered, app.ChangeMCPAlready, app.ChangeMCPFailed, app.ChangeMCPSnippet, app.ChangeMCPUnknown,
		app.ChangeNoAgents, app.ChangeMCPLocalDuplicate, app.ChangeMCPDuplicateRemoved, app.ChangeMCPDuplicateWouldRemove,
		app.ChangeMCPDuplicateKept, app.ChangeMCPCleanFailed,
	}
	for _, k := range kinds {
		r := app.AgentsChangeReport{Source: app.SkillSource{MCPServer: "wspace"}, Changes: []app.AgentChange{{Agent: "claude-code", Kind: k, Path: "/p", Target: "/t", Backup: "/b", Detail: "d"}}}
		lines := app.RenderAgentChanges(r)
		if len(lines) == 0 {
			t.Fatalf("%s renders nothing", k)
		}
		for _, l := range lines {
			if strings.Contains(l, "%!") || strings.HasPrefix(l, "agents.") {
				t.Fatalf("%s renders %q", k, l)
			}
		}
	}
}

// The development app hands its engine its own configuration directory;
// its MCP registration must carry it, or the dev server would read the
// installed app's configuration.
func TestInstallAgents_DevelopmentRegistrationCarriesItsConfigHome(t *testing.T) {
	f := newAgentsFixture(t)
	_ = f.fs.RemoveAll(bundleSkill)
	f.write(t, bundle+"/Contents/Resources/skills/wspace-dev-workspaces/SKILL.md", "dev skill")
	f.deps.Getenv = func(k string) string {
		if k == "WSPACE_CONFIG_HOME" {
			return "/home/u/.config/wspace-dev"
		}
		return ""
	}
	f.mkdir(t, agentsHome+"/.claude")
	f.mkdir(t, agentsHome+"/.cursor")
	f.runner.OnPath["claude"] = "/bin/claude"

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{MCP: true, SkipSkill: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "claude mcp add --scope user --transport stdio --env WSPACE_CONFIG_HOME=/home/u/.config/wspace-dev wspace-dev -- " + bundleExe + " mcp serve"
	if len(f.runner.Calls) != 1 || f.runner.Calls[0] != want {
		t.Fatalf("calls = %v\nwant %s", f.runner.Calls, want)
	}
	for _, c := range changesOf(r, "cursor") {
		if !strings.Contains(c.Detail, `"WSPACE_CONFIG_HOME": "/home/u/.config/wspace-dev"`) || !strings.Contains(c.Detail, `"wspace-dev"`) {
			t.Fatalf("cursor snippet = %s", c.Detail)
		}
	}
	if r.Source.MCPConfigHome != "/home/u/.config/wspace-dev" {
		t.Fatalf("source = %+v", r.Source)
	}
}

// A release engine never pins a configuration directory, even when the
// variable is set in its environment.
func TestAgentsStatus_ReleaseRegistrationNeverPinsAConfigHome(t *testing.T) {
	f := newAgentsFixture(t)
	f.deps.Getenv = func(k string) string {
		if k == "WSPACE_CONFIG_HOME" {
			return "/tmp/x"
		}
		return ""
	}
	r, err := app.AgentsStatus(context.Background(), f.deps, app.AgentsStatusInput{Agents: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Source.MCPConfigHome != "" || strings.Contains(r.Agents[0].MCPSnippet, "WSPACE_CONFIG_HOME") {
		t.Fatalf("source = %+v, snippet %q", r.Source, r.Agents[0].MCPSnippet)
	}
}

func TestInstallAgents_FailedRegistrationCarriesTheManualCommand(t *testing.T) {
	f := newAgentsFixture(t)
	f.mkdir(t, agentsHome+"/.claude")
	f.runner.OnPath["claude"] = "/bin/claude"
	f.runner.Results["claude mcp add --scope user --transport stdio wspace -- "+bundleExe+" mcp serve"] = ports.CommandResult{ExitCode: 1, Stderr: "incompatible\n"}

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{MCP: true, SkipSkill: true})
	if err != nil {
		t.Fatal(err)
	}
	c := changesOf(r, "claude-code")[0]
	if c.Kind != app.ChangeMCPFailed || c.Detail != "incompatible" || c.Manual != "claude mcp add --scope user --transport stdio wspace -- "+bundleExe+" mcp serve" {
		t.Fatalf("change = %+v", c)
	}
}

// On Windows the manual command is for PowerShell, the default shell:
// single-quoted paths (doubling a quote inside) and a quoted '--', which
// PowerShell would otherwise consume as its own end-of-parameters token
// when claude resolves to npm's claude.ps1 shim.
func TestInstallAgents_WindowsManualCommandIsQuotedForPowerShell(t *testing.T) {
	f := newAgentsFixture(t)
	const exe = "C:/Users/O'Neil Q/AppData/Local/Programs/wspace/wspace.exe"
	f.exe = exe
	f.write(t, exe, "binary")
	f.deps.GOOS = "windows"
	f.mkdir(t, agentsHome+"/.claude")
	f.runner.OnPath["claude"] = `C:\Users\Q\AppData\Roaming\npm\claude.cmd`
	f.runner.Err = errors.New("argument cannot be passed safely to a batch file")

	r, err := app.InstallAgents(context.Background(), f.deps, app.InstallAgentsInput{MCP: true, SkipSkill: true})
	if err != nil {
		t.Fatal(err)
	}
	c := changesOf(r, "claude-code")[0]
	want := `claude mcp add --scope user --transport stdio wspace '--' 'C:\Users\O''Neil Q\AppData\Local\Programs\wspace\wspace.exe' mcp serve`
	if c.Kind != app.ChangeMCPFailed || c.Manual != want {
		t.Fatalf("change = %+v\nwant manual %s", c, want)
	}
	st := agentByID(t, r.Agents, "claude-code")
	if st.MCPSnippet != want {
		t.Fatalf("status snippet = %s", st.MCPSnippet)
	}
}
