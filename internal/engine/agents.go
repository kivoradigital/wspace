// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"
	"io/fs"
	"time"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// AgentsDeps are the dependencies of the agents.* methods (see
// app.AgentsDeps; zero GOOS/Getenv/Executable/Now mean the running
// process's own values).
type AgentsDeps struct {
	FS         ports.AgentFS
	Runner     ports.CommandRunner
	Skill      fs.FS
	Home       domain.Path
	GOOS       string
	Getenv     func(string) string
	Executable func() (string, error)
	Now        func() time.Time
}

func (e *Engine) agentsDeps() (app.AgentsDeps, error) {
	d := e.deps.Agents
	if d == nil {
		return app.AgentsDeps{}, newError(CodeInternal, messages.EngineAgentsUnavailable)
	}
	return app.AgentsDeps{
		FS: d.FS, Runner: d.Runner, Skill: d.Skill, Home: d.Home,
		GOOS: d.GOOS, Getenv: d.Getenv, Executable: d.Executable, Now: d.Now,
	}, nil
}

// AgentsParams selects agents by id (all when empty).
type AgentsParams struct {
	Agents []string `json:"agents,omitempty"`
}

// AgentsInstallParams is agents.install's params. Skill defaults to true;
// false runs only the legacy and MCP steps.
type AgentsInstallParams struct {
	Agents        []string `json:"agents,omitempty"`
	Skill         *bool    `json:"skill,omitempty"`
	MCP           bool     `json:"mcp,omitempty"`
	DisableLegacy bool     `json:"disableLegacy,omitempty"`
	Force         bool     `json:"force,omitempty"`
}

// AgentsMCPCleanParams is agents.mcpClean's params.
type AgentsMCPCleanParams struct {
	Agents []string `json:"agents,omitempty"`
	DryRun bool     `json:"dryRun,omitempty"`
}

// SkillSourceInfo is where agent links point and the MCP server this
// installation registers.
type SkillSourceInfo struct {
	Name       string `json:"name"`
	Dir        string `json:"dir"`
	Origin     string `json:"origin"`
	Location   string `json:"location"`
	MCPServer  string `json:"mcpServer"`
	MCPCommand string `json:"mcpCommand"`
	// MCPConfigHome is set only for the development app's engine.
	MCPConfigHome string `json:"mcpConfigHome,omitempty"`
}

// AgentInfo is one agent's row.
type AgentInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Detected      bool   `json:"detected"`
	SkillsDir     string `json:"skillsDir,omitempty"`
	SkillPath     string `json:"skillPath,omitempty"`
	Skill         string `json:"skill"`
	SkillTarget   string `json:"skillTarget,omitempty"`
	LegacyPath    string `json:"legacyPath,omitempty"`
	MCP           string `json:"mcp,omitempty"`
	MCPMethod     string `json:"mcpMethod,omitempty"`
	MCPConfigPath string `json:"mcpConfigPath,omitempty"`
	MCPSnippet    string `json:"mcpSnippet,omitempty"`
	// MCPRegistrations lists every registration of this installation's
	// server (omitted when none); MCPDuplicate is set when there is more
	// than one, MCPStale when any is stale.
	MCPRegistrations []MCPRegistrationInfo `json:"mcpRegistrations,omitempty"`
	MCPDuplicate     bool                  `json:"mcpDuplicate,omitempty"`
	MCPStale         bool                  `json:"mcpStale,omitempty"`
}

// MCPRegistrationInfo is one MCP registration: scope "user" or "local",
// the project of a local one, the command it starts.
type MCPRegistrationInfo struct {
	Scope   string `json:"scope"`
	Project string `json:"project,omitempty"`
	Command string `json:"command,omitempty"`
	Stale   bool   `json:"stale,omitempty"`
}

// AgentChangeInfo is one install/uninstall action.
type AgentChangeInfo struct {
	Agent  string `json:"agent,omitempty"`
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Target string `json:"target,omitempty"`
	Backup string `json:"backup,omitempty"`
	Detail string `json:"detail,omitempty"`
	// ManualCommand is, on mcp_failed, the command to run by hand
	// (PowerShell syntax on Windows).
	ManualCommand string `json:"manualCommand,omitempty"`
	Message       string `json:"message"`
}

// AgentsStatusResult is agents.status's result.
type AgentsStatusResult struct {
	Skill  SkillSourceInfo `json:"skill"`
	Agents []AgentInfo     `json:"agents"`
}

// AgentsChangeResult is agents.install's and agents.uninstall's result.
type AgentsChangeResult struct {
	Skill   SkillSourceInfo   `json:"skill"`
	Changes []AgentChangeInfo `json:"changes"`
	Agents  []AgentInfo       `json:"agents"`
}

func skillInfo(s app.SkillSource) SkillSourceInfo {
	return SkillSourceInfo{Name: s.Name, Dir: s.Dir, Origin: string(s.Origin), Location: string(s.Location), MCPServer: s.MCPServer, MCPCommand: s.MCPCommand, MCPConfigHome: s.MCPConfigHome}
}

func agentInfos(agents []app.AgentStatus) []AgentInfo {
	out := make([]AgentInfo, 0, len(agents))
	for _, a := range agents {
		out = append(out, AgentInfo{
			ID: a.ID, Name: a.Name, Detected: a.Detected, SkillsDir: a.SkillsDir, SkillPath: a.SkillPath,
			Skill: string(a.Skill), SkillTarget: a.SkillTarget, LegacyPath: a.LegacyPath, MCP: string(a.MCP),
			MCPMethod: string(a.MCPMethod), MCPConfigPath: a.MCPConfigPath, MCPSnippet: a.MCPSnippet,
			MCPRegistrations: registrationInfos(a.MCPRegistrations), MCPDuplicate: a.MCPDuplicate, MCPStale: a.MCPStale,
		})
	}
	return out
}

func registrationInfos(regs []app.MCPRegistration) []MCPRegistrationInfo {
	if regs == nil {
		return nil
	}
	out := make([]MCPRegistrationInfo, 0, len(regs))
	for _, g := range regs {
		out = append(out, MCPRegistrationInfo{Scope: string(g.Scope), Project: g.Project, Command: g.Command, Stale: g.Stale})
	}
	return out
}

func changeResult(r app.AgentsChangeReport) AgentsChangeResult {
	out := AgentsChangeResult{Skill: skillInfo(r.Source), Changes: []AgentChangeInfo{}, Agents: agentInfos(r.Agents)}
	for _, c := range r.Changes {
		lines := app.RenderAgentChanges(app.AgentsChangeReport{Source: r.Source, Changes: []app.AgentChange{c}})
		msg := ""
		if len(lines) > 0 {
			msg = lines[0]
		}
		out.Changes = append(out.Changes, AgentChangeInfo{
			Agent: c.Agent, Kind: string(c.Kind), Path: c.Path, Target: c.Target, Backup: c.Backup, Detail: c.Detail, ManualCommand: c.Manual, Message: msg,
		})
	}
	return out
}

// AgentsStatus reports every supported agent's skill, legacy skill and MCP
// state. It never writes.
func (e *Engine) AgentsStatus(ctx context.Context, p AgentsParams) (AgentsStatusResult, error) {
	deps, err := e.agentsDeps()
	if err != nil {
		return AgentsStatusResult{}, err
	}
	r, err := app.AgentsStatus(ctx, deps, app.AgentsStatusInput{Agents: p.Agents})
	if err != nil {
		return AgentsStatusResult{}, wrap(err)
	}
	return AgentsStatusResult{Skill: skillInfo(r.Source), Agents: agentInfos(r.Agents)}, nil
}

// AgentsInstall links the skill into the selected (default: detected)
// agents, optionally disabling the legacy skill and registering MCP.
func (e *Engine) AgentsInstall(ctx context.Context, p AgentsInstallParams) (AgentsChangeResult, error) {
	deps, err := e.agentsDeps()
	if err != nil {
		return AgentsChangeResult{}, err
	}
	in := app.InstallAgentsInput{Agents: p.Agents, SkipSkill: p.Skill != nil && !*p.Skill, MCP: p.MCP, DisableLegacy: p.DisableLegacy, Force: p.Force}
	r, err := app.InstallAgents(ctx, deps, in)
	if err != nil {
		return AgentsChangeResult{}, wrap(err)
	}
	return changeResult(r), nil
}

// AgentsUninstall removes the skill links wspace created.
func (e *Engine) AgentsUninstall(ctx context.Context, p AgentsParams) (AgentsChangeResult, error) {
	deps, err := e.agentsDeps()
	if err != nil {
		return AgentsChangeResult{}, err
	}
	r, err := app.UninstallAgents(ctx, deps, app.UninstallAgentsInput{Agents: p.Agents})
	if err != nil {
		return AgentsChangeResult{}, wrap(err)
	}
	return changeResult(r), nil
}

// AgentsMCPClean removes duplicate local-scope MCP registrations of this
// installation's server (see app.CleanAgentsMCP); DryRun only reports.
func (e *Engine) AgentsMCPClean(ctx context.Context, p AgentsMCPCleanParams) (AgentsChangeResult, error) {
	deps, err := e.agentsDeps()
	if err != nil {
		return AgentsChangeResult{}, err
	}
	r, err := app.CleanAgentsMCP(ctx, deps, app.CleanAgentsMCPInput{Agents: p.Agents, DryRun: p.DryRun})
	if err != nil {
		return AgentsChangeResult{}, wrap(err)
	}
	return changeResult(r), nil
}
