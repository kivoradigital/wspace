// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// MCPMethod is how wspace registers its MCP server with an agent.
type MCPMethod string

const (
	// MCPMethodCLI runs the agent's own CLI ("claude mcp add", …).
	MCPMethodCLI MCPMethod = "cli"
	// MCPMethodFile means the agent is configured by a file wspace never
	// edits: the registration snippet is shown for the user to paste.
	MCPMethodFile MCPMethod = "file"
)

type mcpFormat int

const (
	formatMCPServersJSON mcpFormat = iota // {"mcpServers": {"<name>": …}}
	formatOpenCodeJSON                    // {"mcp": {"<name>": …}}
	formatCodexTOML                       // [mcp_servers.<name>]
)

// agentDef is one supported agent. Every path is relative to the home
// directory. Sources (checked 2026-10-03; see docs/mcp.md §6):
//   - Claude Code: personal skills in ~/.claude/skills, symlinked skill
//     directories supported; `claude mcp add --scope user --transport stdio
//     <name> -- <cmd>`, user servers in ~/.claude.json "mcpServers"
//     (code.claude.com/docs/en/skills, /mcp).
//   - Claude Desktop: no skills directory; MCP in
//     ~/Library/Application Support/Claude/claude_desktop_config.json.
//   - Codex: user skills in ~/.agents/skills, symlinks followed;
//     `codex mcp add <name> -- <cmd>`, [mcp_servers.<name>] in
//     ~/.codex/config.toml (learn.chatgpt.com/docs/build-skills, /extend/mcp).
//   - Cursor: user skills in ~/.cursor/skills (also ~/.agents/skills and
//     ~/.claude/skills); MCP in ~/.cursor/mcp.json (cursor.com/docs/skills,
//     /context/mcp).
//   - Gemini CLI: user skills in ~/.gemini/skills (alias ~/.agents/skills);
//     `gemini mcp add --scope user <name> <cmd> [args…]`, user servers in
//     ~/.gemini/settings.json (geminicli.com/docs/cli/skills,
//     /tools/mcp-server; `gemini mcp add --help`).
//   - OpenCode: global skills in ~/.config/opencode/skills (also
//     ~/.claude/skills, ~/.agents/skills); MCP under "mcp" in
//     ~/.config/opencode/opencode.json; its `mcp add` is interactive, so
//     the snippet is shown (opencode.ai/docs/skills, /mcp-servers, /config).
type agentDef struct {
	id, name   string
	configDir  string
	commands   []string
	skillsDir  string // "" when the agent has no skills directory
	darwinOnly bool

	mcpMethod MCPMethod
	mcpConfig string
	mcpFormat mcpFormat
	// cli and cliArgs build the registration command for MCPMethodCLI.
	cli     string
	cliArgs func(server, command string, env []string) []string
	// localArgs, set only for agents with per-project (local-scope)
	// registrations in mcpConfig, builds the CLI command that removes the
	// local registration of the project the CLI runs in.
	localArgs func(server string) []string
}

var agentCatalog = []agentDef{
	{
		id: "claude-code", name: "Claude Code", configDir: ".claude", commands: []string{"claude"},
		skillsDir: ".claude/skills",
		mcpMethod: MCPMethodCLI, mcpConfig: ".claude.json", mcpFormat: formatMCPServersJSON, cli: "claude",
		cliArgs: func(server, command string, env []string) []string {
			args := []string{"mcp", "add", "--scope", "user", "--transport", "stdio"}
			args = append(args, envFlags("--env", env)...)
			return append(args, server, "--", command, "mcp", "serve")
		},
		// `claude mcp remove <name> -s local` resolves the project from its
		// working directory (code.claude.com/docs/en/mcp, "MCP installation
		// scopes"; checked with claude 2.1.273).
		localArgs: func(server string) []string { return []string{"mcp", "remove", server, "-s", "local"} },
	},
	{
		id: "claude-desktop", name: "Claude Desktop", configDir: "Library/Application Support/Claude", darwinOnly: true,
		mcpMethod: MCPMethodFile, mcpConfig: "Library/Application Support/Claude/claude_desktop_config.json", mcpFormat: formatMCPServersJSON,
	},
	{
		id: "codex", name: "Codex", configDir: ".codex", commands: []string{"codex"},
		skillsDir: ".agents/skills",
		mcpMethod: MCPMethodCLI, mcpConfig: ".codex/config.toml", mcpFormat: formatCodexTOML, cli: "codex",
		cliArgs: func(server, command string, env []string) []string {
			args := append([]string{"mcp", "add", server}, envFlags("--env", env)...)
			return append(args, "--", command, "mcp", "serve")
		},
	},
	{
		id: "cursor", name: "Cursor", configDir: ".cursor", commands: []string{"cursor", "cursor-agent"},
		skillsDir: ".cursor/skills",
		mcpMethod: MCPMethodFile, mcpConfig: ".cursor/mcp.json", mcpFormat: formatMCPServersJSON,
	},
	{
		id: "gemini", name: "Gemini CLI", configDir: ".gemini", commands: []string{"gemini"},
		skillsDir: ".gemini/skills",
		mcpMethod: MCPMethodCLI, mcpConfig: ".gemini/settings.json", mcpFormat: formatMCPServersJSON, cli: "gemini",
		cliArgs: func(server, command string, env []string) []string {
			args := append([]string{"mcp", "add", "--scope", "user"}, envFlags("--env", env)...)
			return append(args, server, command, "mcp", "serve")
		},
	},
	{
		id: "opencode", name: "OpenCode", configDir: ".config/opencode", commands: []string{"opencode"},
		skillsDir: ".config/opencode/skills",
		mcpMethod: MCPMethodFile, mcpConfig: ".config/opencode/opencode.json", mcpFormat: formatOpenCodeJSON,
	},
}

// AgentIDs lists every agent id wspace knows, in catalog order.
func AgentIDs() []string {
	ids := make([]string, 0, len(agentCatalog))
	for _, d := range agentCatalog {
		ids = append(ids, d.id)
	}
	return ids
}

func envFlags(flag string, env []string) []string {
	var out []string
	for _, e := range env {
		out = append(out, flag, e)
	}
	return out
}

// mcpSnippet is what the user adds by hand: the CLI command for a CLI
// agent (quoted for PowerShell on Windows, for a POSIX shell elsewhere),
// the configuration fragment for a file-configured one. Paths are in
// goos's native form.
func (d agentDef) mcpSnippet(goos string, src SkillSource) string {
	server, command := src.MCPServer, nativePath(goos, src.MCPCommand)
	var env []string
	envMap := map[string]string{}
	if src.MCPConfigHome != "" {
		home := nativePath(goos, src.MCPConfigHome)
		env = []string{configHomeEnv + "=" + home}
		envMap[configHomeEnv] = home
	}
	if d.mcpMethod == MCPMethodCLI {
		quote := shellQuote
		if goos == "windows" {
			quote = powerShellQuote
		}
		parts := []string{d.cli}
		for _, a := range d.cliArgs(server, command, env) {
			parts = append(parts, quote(a))
		}
		return strings.Join(parts, " ")
	}
	var v any
	switch d.mcpFormat {
	case formatOpenCodeJSON:
		entry := map[string]any{"type": "local", "command": []string{command, "mcp", "serve"}, "enabled": true}
		if len(envMap) > 0 {
			entry["environment"] = envMap
		}
		v = map[string]any{"mcp": map[string]any{server: entry}}
	default:
		entry := map[string]any{"command": command, "args": []string{"mcp", "serve"}}
		if len(envMap) > 0 {
			entry["env"] = envMap
		}
		v = map[string]any{"mcpServers": map[string]any{server: entry}}
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote quotes s for a POSIX shell when it needs it.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// powerShellSafe excludes what PowerShell's parser gives a meaning to in
// a command argument: @ (splatting), commas (arrays), $, quotes, braces,
// parentheses, ; & | < > and backticks.
var powerShellSafe = regexp.MustCompile(`^[A-Za-z0-9_+=:./\\-]+$`)

// powerShellQuote quotes s for PowerShell when it needs it: a single-
// quoted string is literal, with an embedded ' doubled. "--" is always
// quoted, because PowerShell consumes a bare -- as its own end-of-
// parameters marker when the command is a script (npm's claude.ps1).
func powerShellQuote(s string) string {
	if s != "--" && powerShellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// nativePath writes a slash-separated path with goos's separator.
func nativePath(goos, p string) string {
	if goos == "windows" {
		return strings.ReplaceAll(p, "/", `\`)
	}
	return p
}

// rawRegistration is one MCP server entry as read from a configuration
// file: scope, project (local scope only) and command ("" when absent or
// not a string).
type rawRegistration struct {
	scope   MCPScope
	project string
	command string
}

// readMCPRegistrations lists server's registrations in config (the
// agent's configuration file content): the user-scope entry and, when
// local is set, every per-project entry under "projects"
// (~/.claude.json's local scope), sorted by project path. ok is false when
// the content cannot be parsed, so the registration state is unknown; a
// malformed project entry is skipped.
func readMCPRegistrations(format mcpFormat, config []byte, server string, local bool) (regs []rawRegistration, ok bool) {
	if format == formatCodexTOML {
		re := regexp.MustCompile(`(?m)^\s*\[\s*mcp_servers\.(?:` + regexp.QuoteMeta(server) + `|"` + regexp.QuoteMeta(server) + `")\s*\]`)
		if re.Match(config) {
			regs = append(regs, rawRegistration{scope: MCPScopeUser})
		}
		return regs, true
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(config, &top); err != nil {
		return nil, false
	}
	key := "mcpServers"
	if format == formatOpenCodeJSON {
		key = "mcp"
	}
	if raw, present := top[key]; present {
		var servers map[string]json.RawMessage
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, false
		}
		if entry, found := servers[server]; found {
			regs = append(regs, rawRegistration{scope: MCPScopeUser, command: serverCommand(entry)})
		}
	}
	if !local {
		return regs, true
	}
	var projects map[string]json.RawMessage
	if err := json.Unmarshal(top["projects"], &projects); err != nil {
		return regs, true
	}
	paths := make([]string, 0, len(projects))
	for p := range projects {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		var project struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
		}
		if err := json.Unmarshal(projects[p], &project); err != nil {
			continue
		}
		if entry, found := project.MCPServers[server]; found {
			regs = append(regs, rawRegistration{scope: MCPScopeLocal, project: p, command: serverCommand(entry)})
		}
	}
	return regs, true
}

// serverCommand is an entry's program: its "command" string, or the first
// element of OpenCode's command array.
func serverCommand(entry json.RawMessage) string {
	var e struct {
		Command json.RawMessage `json:"command"`
	}
	if json.Unmarshal(entry, &e) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(e.Command, &s) == nil {
		return s
	}
	var list []string
	if json.Unmarshal(e.Command, &list) == nil && len(list) > 0 {
		return list[0]
	}
	return ""
}
