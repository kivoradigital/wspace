// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/spf13/cobra"
)

// newAgentsCommand groups `wspace agents status|install|uninstall`: detect
// the AI coding agents installed on this machine and link the wspace skill
// into them (docs/mcp.md §6). The link always targets the skill
// shipped with this binary (its app bundle, or its embedded copy
// extracted to the user data directory), never a source checkout.
func newAgentsCommand(rt *Runtime) *cobra.Command {
	root := &cobra.Command{
		Use:   "agents",
		Short: "install the wspace skill (and optionally the MCP server) into detected AI coding agents",
	}
	root.AddCommand(newAgentsStatusCommand(rt), newAgentsInstallCommand(rt), newAgentsUninstallCommand(rt), newAgentsMCPCleanCommand(rt))
	return root
}

func addAgentFlag(cmd *cobra.Command, ids *[]string) {
	cmd.Flags().StringArrayVar(ids, "agent", nil, "limit to this agent id (repeatable): claude-code, claude-desktop, codex, cursor, gemini, opencode")
}

func newAgentsStatusCommand(rt *Runtime) *cobra.Command {
	var ids []string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "show each agent: detected, skill state, legacy skill, MCP registration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := app.AgentsStatus(cmd.Context(), rt.AgentsDeps, app.AgentsStatusInput{Agents: ids})
			if err != nil {
				return err
			}
			if jsonRequested(cmd) {
				return printJSON(cmd, agentsReportJSON{Skill: skillJSONOf(r.Source), Agents: agentsJSONOf(r.Agents)})
			}
			return printLines(cmd, app.RenderAgentsStatus(r))
		},
	}
	addAgentFlag(cmd, &ids)
	addJSONFlag(cmd)
	return cmd
}

func newAgentsInstallCommand(rt *Runtime) *cobra.Command {
	var in app.InstallAgentsInput
	cmd := &cobra.Command{
		Use:   "install",
		Short: "link the skill into every detected agent (or each --agent)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := app.InstallAgents(cmd.Context(), rt.AgentsDeps, in)
			if err != nil {
				return err
			}
			return printChanges(cmd, r)
		},
	}
	addAgentFlag(cmd, &in.Agents)
	cmd.Flags().BoolVar(&in.MCP, "mcp", false, "also register the MCP server (agent CLIs are run; file-configured agents get a snippet to paste)")
	cmd.Flags().BoolVar(&in.DisableLegacy, "disable-legacy", false, "move the legacy ws-workspaces skill to <agent>/skills-disabled (never deleted)")
	cmd.Flags().BoolVar(&in.Force, "force", false, "replace an entry wspace did not create (a real directory is moved aside, never deleted)")
	addJSONFlag(cmd)
	return cmd
}

func newAgentsUninstallCommand(rt *Runtime) *cobra.Command {
	var ids []string
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "remove the skill links wspace created (other entries are left alone)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := app.UninstallAgents(cmd.Context(), rt.AgentsDeps, app.UninstallAgentsInput{Agents: ids})
			if err != nil {
				return err
			}
			return printChanges(cmd, r)
		},
	}
	addAgentFlag(cmd, &ids)
	addJSONFlag(cmd)
	return cmd
}

// newAgentsMCPCleanCommand is `wspace agents mcp-clean`: an explicit
// command (rather than an install flag) because it removes registrations
// wspace did not necessarily create. It removes Claude Code's local-scope
// registrations of this installation's server name only, keeping the
// user-scope one (or, without one, exactly one local registration).
func newAgentsMCPCleanCommand(rt *Runtime) *cobra.Command {
	var in app.CleanAgentsMCPInput
	cmd := &cobra.Command{
		Use:   "mcp-clean",
		Short: "remove duplicate local-scope MCP registrations of wspace (claude mcp remove -s local, run in each project)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := app.CleanAgentsMCP(cmd.Context(), rt.AgentsDeps, in)
			if err != nil {
				return err
			}
			return printChanges(cmd, r)
		},
	}
	addAgentFlag(cmd, &in.Agents)
	cmd.Flags().BoolVar(&in.DryRun, "dry-run", false, "only list what would be removed")
	addJSONFlag(cmd)
	return cmd
}

func printChanges(cmd *cobra.Command, r app.AgentsChangeReport) error {
	if jsonRequested(cmd) {
		out := agentsChangeJSON{Skill: skillJSONOf(r.Source), Changes: []agentChangeJSON{}, Agents: agentsJSONOf(r.Agents)}
		for _, c := range r.Changes {
			out.Changes = append(out.Changes, agentChangeJSON{Agent: c.Agent, Kind: string(c.Kind), Path: c.Path, Target: c.Target, Backup: c.Backup, Detail: c.Detail, ManualCommand: c.Manual})
		}
		return printJSON(cmd, out)
	}
	return printLines(cmd, app.RenderAgentChanges(r))
}

func printJSON(cmd *cobra.Command, v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return nil
}

func printLines(cmd *cobra.Command, lines []string) error {
	for _, l := range lines {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), l)
	}
	return nil
}

// skillJSON, agentJSON and agentChangeJSON are agents --json's shapes: the
// rpc agents.* results with the CLI's snake_case field names.
type skillJSON struct {
	Name          string `json:"name"`
	Dir           string `json:"dir"`
	Origin        string `json:"origin"`
	Location      string `json:"location"`
	MCPServer     string `json:"mcp_server"`
	MCPCommand    string `json:"mcp_command"`
	MCPConfigHome string `json:"mcp_config_home,omitempty"`
}

type agentJSON struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Detected      bool   `json:"detected"`
	SkillsDir     string `json:"skills_dir,omitempty"`
	SkillPath     string `json:"skill_path,omitempty"`
	Skill         string `json:"skill"`
	SkillTarget   string `json:"skill_target,omitempty"`
	LegacyPath    string `json:"legacy_path,omitempty"`
	MCP           string `json:"mcp,omitempty"`
	MCPMethod     string `json:"mcp_method,omitempty"`
	MCPConfigPath string `json:"mcp_config_path,omitempty"`
	MCPSnippet    string `json:"mcp_snippet,omitempty"`
	// MCPRegistrations lists every registration found (omitted when none).
	MCPRegistrations []mcpRegistrationJSON `json:"mcp_registrations,omitempty"`
	MCPDuplicate     bool                  `json:"mcp_duplicate,omitempty"`
	MCPStale         bool                  `json:"mcp_stale,omitempty"`
}

type mcpRegistrationJSON struct {
	Scope   string `json:"scope"`
	Project string `json:"project,omitempty"`
	Command string `json:"command,omitempty"`
	Stale   bool   `json:"stale,omitempty"`
}

type agentChangeJSON struct {
	Agent  string `json:"agent,omitempty"`
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Target string `json:"target,omitempty"`
	Backup string `json:"backup,omitempty"`
	Detail string `json:"detail,omitempty"`
	// ManualCommand is, on mcp_failed, the command to run by hand
	// (PowerShell syntax on Windows).
	ManualCommand string `json:"manual_command,omitempty"`
}

type agentsReportJSON struct {
	Skill  skillJSON   `json:"skill"`
	Agents []agentJSON `json:"agents"`
}

type agentsChangeJSON struct {
	Skill   skillJSON         `json:"skill"`
	Changes []agentChangeJSON `json:"changes"`
	Agents  []agentJSON       `json:"agents"`
}

func skillJSONOf(s app.SkillSource) skillJSON {
	return skillJSON{Name: s.Name, Dir: s.Dir, Origin: string(s.Origin), Location: string(s.Location), MCPServer: s.MCPServer, MCPCommand: s.MCPCommand, MCPConfigHome: s.MCPConfigHome}
}

func agentsJSONOf(agents []app.AgentStatus) []agentJSON {
	out := make([]agentJSON, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentJSON{
			ID: a.ID, Name: a.Name, Detected: a.Detected, SkillsDir: a.SkillsDir, SkillPath: a.SkillPath,
			Skill: string(a.Skill), SkillTarget: a.SkillTarget, LegacyPath: a.LegacyPath, MCP: string(a.MCP),
			MCPMethod: string(a.MCPMethod), MCPConfigPath: a.MCPConfigPath, MCPSnippet: a.MCPSnippet,
			MCPRegistrations: registrationsJSONOf(a.MCPRegistrations), MCPDuplicate: a.MCPDuplicate, MCPStale: a.MCPStale,
		})
	}
	return out
}

func registrationsJSONOf(regs []app.MCPRegistration) []mcpRegistrationJSON {
	if regs == nil {
		return nil
	}
	out := make([]mcpRegistrationJSON, 0, len(regs))
	for _, g := range regs {
		out = append(out, mcpRegistrationJSON{Scope: string(g.Scope), Project: g.Project, Command: g.Command, Stale: g.Stale})
	}
	return out
}
