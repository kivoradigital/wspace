// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// RenderAgentsStatus renders AgentsStatus's report for the terminal.
func RenderAgentsStatus(r AgentsReport) []string {
	lines := renderAgentsSource(r.Source)
	for _, a := range r.Agents {
		detected := messages.T(messages.AgentsNotDetected)
		if a.Detected {
			detected = messages.T(messages.AgentsDetected)
		}
		mcp := string(a.MCP)
		if mcp == "" {
			mcp = "-"
		}
		lines = append(lines, messages.T(messages.AgentsStatusRow, a.ID, detected, string(a.Skill), mcp))
		if a.SkillTarget != "" && a.Skill != SkillInstalled {
			lines = append(lines, messages.T(messages.AgentsStatusTarget, a.SkillPath, a.SkillTarget))
		}
		if a.LegacyPath != "" {
			lines = append(lines, messages.T(messages.AgentsStatusLegacy, a.LegacyPath))
		}
		if a.MCPDuplicate || a.MCPStale {
			lines = append(lines, renderMCPRegistrations(r.Source, a)...)
		}
	}
	return lines
}

// renderMCPRegistrations lists an agent's registrations when one of them
// needs attention.
func renderMCPRegistrations(src SkillSource, a AgentStatus) []string {
	var lines []string
	for _, g := range a.MCPRegistrations {
		stale := ""
		if g.Stale {
			stale = messages.T(messages.AgentsStatusMCPStale)
		}
		cmd := g.Command
		if cmd == "" {
			cmd = "-"
		}
		if g.Scope == MCPScopeLocal {
			lines = append(lines, messages.T(messages.AgentsStatusMCPLocal, g.Project, cmd, stale))
		} else {
			lines = append(lines, messages.T(messages.AgentsStatusMCPUser, cmd, stale))
		}
	}
	if a.MCPDuplicate {
		lines = append(lines, messages.T(messages.AgentsStatusMCPDuplicate, src.MCPServer))
	}
	return lines
}

func renderAgentsSource(s SkillSource) []string {
	lines := []string{
		messages.T(messages.AgentsSource, s.Name, s.Dir),
		messages.T(messages.AgentsMCPServer, s.MCPServer, s.MCPCommand),
	}
	if s.Location != domain.LocationStable {
		lines = append(lines, messages.T(messages.ErrSkillSourceUnstable))
	}
	return lines
}

var changeKeys = map[AgentChangeKind]messages.Key{
	ChangeLinked:         messages.AgentsChangeLinked,
	ChangeCopied:         messages.AgentsChangeCopied,
	ChangeRefreshed:      messages.AgentsChangeRefreshed,
	ChangeUnchanged:      messages.AgentsChangeUnchanged,
	ChangeReplaced:       messages.AgentsChangeReplaced,
	ChangeConflict:       messages.AgentsChangeConflict,
	ChangeLegacyFound:    messages.AgentsChangeLegacyFound,
	ChangeLegacyDisabled: messages.AgentsChangeLegacyDisabled,
	ChangeRemoved:        messages.AgentsChangeRemoved,
	ChangeKept:           messages.AgentsChangeKept,
	ChangeMCPRegistered:  messages.AgentsChangeMCPRegistered,
	ChangeMCPAlready:     messages.AgentsChangeMCPAlready,
	ChangeMCPFailed:      messages.AgentsChangeMCPFailed,
	ChangeMCPSnippet:     messages.AgentsChangeMCPManual,
	ChangeMCPUnknown:     messages.AgentsChangeMCPUnknown,
	ChangeFailed:         messages.AgentsChangeFailed,
	ChangeNoAgents:       messages.AgentsChangeNoAgents,

	ChangeMCPLocalDuplicate:       messages.AgentsChangeMCPLocalDuplicate,
	ChangeMCPDuplicateRemoved:     messages.AgentsChangeMCPDuplicateRemoved,
	ChangeMCPDuplicateWouldRemove: messages.AgentsChangeMCPDuplicateWouldRemove,
	ChangeMCPDuplicateKept:        messages.AgentsChangeMCPDuplicateKept,
	ChangeMCPCleanFailed:          messages.AgentsChangeMCPCleanFailed,
}

// RenderAgentChanges renders what InstallAgents or UninstallAgents did.
// Every template takes the same arguments: agent, path, target, backup,
// detail, MCP server name.
func RenderAgentChanges(r AgentsChangeReport) []string {
	var lines []string
	for _, c := range r.Changes {
		key, ok := changeKeys[c.Kind]
		if !ok {
			continue
		}
		text := messages.T(key, c.Agent, c.Path, c.Target, c.Backup, c.Detail, r.Source.MCPServer)
		lines = append(lines, strings.Split(text, "\n")...)
		if c.Manual != "" {
			lines = append(lines, messages.T(messages.AgentsChangeMCPRunManually), "  "+c.Manual)
		}
	}
	if len(r.Changes) == 0 {
		lines = append(lines, messages.T(messages.AgentsChangeNothing))
	}
	return lines
}
