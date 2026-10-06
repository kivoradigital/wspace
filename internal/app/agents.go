// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// AgentsDeps bundles the agent skill use cases' dependencies. GOOS, Getenv,
// Executable and Now default to the running process's own values when
// left zero; tests set every one.
type AgentsDeps struct {
	FS     ports.AgentFS
	Runner ports.CommandRunner
	// Skill holds the embedded skill files under a top-level
	// domain.SkillName directory (the skills package's FS()).
	Skill fs.FS
	Home  domain.Path

	GOOS       string
	Getenv     func(string) string
	Executable func() (string, error)
	Now        func() time.Time
}

func (d AgentsDeps) resolved() AgentsDeps {
	if d.GOOS == "" {
		d.GOOS = runtime.GOOS
	}
	if d.Getenv == nil {
		d.Getenv = os.Getenv
	}
	if d.Executable == nil {
		d.Executable = os.Executable
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return d
}

// SkillOrigin says where the skill source comes from.
type SkillOrigin string

const (
	// SkillOriginBundle is the copy inside the desktop app bundle the
	// engine runs from.
	SkillOriginBundle SkillOrigin = "bundle"
	// SkillOriginEmbedded is the binary's embedded copy, extracted to the
	// user data directory.
	SkillOriginEmbedded SkillOrigin = "embedded"
)

// SkillSource is the directory agent links point at, and the MCP server
// this installation registers.
type SkillSource struct {
	Name     string
	Dir      string
	Origin   SkillOrigin
	Location domain.SourceLocation
	// MCPServer is the server name agents register ("wspace", or
	// "wspace-dev" for the development app).
	MCPServer string
	// MCPCommand is the program agents start: the command line tool link
	// (~/.local/bin/wspace) when it points at this engine, else the engine
	// itself.
	MCPCommand string
	// MCPConfigHome is the WSPACE_CONFIG_HOME the registration passes to
	// the server: only for the development skill, whose app gives its
	// engine its own configuration directory.
	MCPConfigHome string
}

// SkillState is one agent's skill entry state.
type SkillState string

const (
	SkillInstalled   SkillState = "installed"   // links to (or is a marked copy of) the current source
	SkillOutdated    SkillState = "outdated"    // a wspace link to another source (moved app, old extraction)
	SkillMissing     SkillState = "missing"     // nothing there
	SkillConflict    SkillState = "conflict"    // something wspace did not place
	SkillUnsupported SkillState = "unsupported" // the agent has no skills directory
)

// MCPState is one agent's MCP registration state, read from its
// configuration file.
type MCPState string

const (
	MCPRegistered    MCPState = "registered"
	MCPNotRegistered MCPState = "not_registered"
	MCPUnknown       MCPState = "unknown" // the configuration could not be read
)

// MCPScope is where an MCP registration lives. Claude Code has three
// scopes (code.claude.com/docs/en/mcp): user (top-level "mcpServers" in
// ~/.claude.json), local (that file's "projects"["<path>"]."mcpServers")
// and project (<project>/.mcp.json). wspace reads the first two; project
// files are not scanned, since finding them would mean walking the disk.
// Agents with a single configuration file report user scope.
type MCPScope string

const (
	MCPScopeUser  MCPScope = "user"
	MCPScopeLocal MCPScope = "local"
)

// MCPRegistration is one registration of this installation's MCP server.
// Project is the project path of a local-scope one. Command is the program
// it starts ("" when the configuration does not say, as for Codex); Stale
// marks a command that no longer exists or is another wspace than this
// one (neither the running engine, ~/.local/bin/wspace, nor an engine
// inside an installed app bundle).
type MCPRegistration struct {
	Scope   MCPScope
	Project string
	Command string
	Stale   bool
}

// AgentStatus is one agent's row.
type AgentStatus struct {
	ID       string
	Name     string
	Detected bool
	// SkillsDir is "" when the agent has no skills directory.
	SkillsDir string
	SkillPath string
	Skill     SkillState
	// SkillTarget is the current link target (or a copy's recorded source)
	// when the entry is wspace's or a link of someone else's.
	SkillTarget string
	// LegacyPath is the legacy ws-workspaces skill's path, "" when absent.
	LegacyPath string
	// MCP is the user-scope registration state.
	MCP MCPState
	// MCPRegistrations lists every registration found, user scope first;
	// MCPDuplicate is set when there is more than one (Claude Code then
	// uses the local one in that project and warns), MCPStale when any is
	// stale.
	MCPRegistrations []MCPRegistration
	MCPDuplicate     bool
	MCPStale         bool
	MCPMethod        MCPMethod
	MCPConfigPath    string
	MCPSnippet       string
}

// AgentsReport is AgentsStatus's result.
type AgentsReport struct {
	Source SkillSource
	Agents []AgentStatus
}

// AgentChangeKind is what install or uninstall did for one agent.
type AgentChangeKind string

const (
	ChangeLinked         AgentChangeKind = "linked"
	ChangeCopied         AgentChangeKind = "copied"
	ChangeRefreshed      AgentChangeKind = "refreshed"
	ChangeUnchanged      AgentChangeKind = "unchanged"
	ChangeReplaced       AgentChangeKind = "replaced"
	ChangeConflict       AgentChangeKind = "conflict"
	ChangeLegacyFound    AgentChangeKind = "legacy_found"
	ChangeLegacyDisabled AgentChangeKind = "legacy_disabled"
	ChangeRemoved        AgentChangeKind = "removed"
	ChangeKept           AgentChangeKind = "kept"
	ChangeMCPRegistered  AgentChangeKind = "mcp_registered"
	ChangeMCPAlready     AgentChangeKind = "mcp_already_registered"
	ChangeMCPFailed      AgentChangeKind = "mcp_failed"
	ChangeMCPSnippet     AgentChangeKind = "mcp_manual"
	ChangeMCPUnknown     AgentChangeKind = "mcp_unknown"
	// ChangeMCPLocalDuplicate reports a local-scope registration (Path is
	// its project, Target its command) next to the user-scope one.
	ChangeMCPLocalDuplicate AgentChangeKind = "mcp_local_duplicate"
	// CleanAgentsMCP's outcomes, per local registration (Path, Target as
	// above).
	ChangeMCPDuplicateRemoved     AgentChangeKind = "mcp_duplicate_removed"
	ChangeMCPDuplicateWouldRemove AgentChangeKind = "mcp_duplicate_would_remove"
	ChangeMCPDuplicateKept        AgentChangeKind = "mcp_duplicate_kept"
	ChangeMCPCleanFailed          AgentChangeKind = "mcp_clean_failed"
	ChangeFailed                  AgentChangeKind = "failed"
	ChangeNoAgents                AgentChangeKind = "no_agents"
)

// AgentChange is one action (or deliberate non-action) for one agent.
// Path is the entry acted on; Target a link target; Backup where a
// replaced or disabled entry was moved; Detail a CLI error line or the
// manual MCP snippet; Manual, on a failed MCP registration, the exact
// command for the user to run instead (PowerShell syntax on Windows).
type AgentChange struct {
	Agent  string
	Kind   AgentChangeKind
	Path   string
	Target string
	Backup string
	Detail string
	Manual string
}

// AgentsChangeReport is InstallAgents' and UninstallAgents' result: what
// changed, then every touched agent's state afterwards.
type AgentsChangeReport struct {
	Source  SkillSource
	Changes []AgentChange
	Agents  []AgentStatus
}

// AgentsStatusInput filters the report to some agent ids (all when empty).
type AgentsStatusInput struct {
	Agents []string
}

// InstallAgentsInput parameterizes InstallAgents. Agents empty means every
// detected agent; a named agent is installed even when not detected.
type InstallAgentsInput struct {
	Agents        []string
	SkipSkill     bool // only the legacy and MCP steps
	MCP           bool
	DisableLegacy bool
	Force         bool
}

// CleanAgentsMCPInput parameterizes CleanAgentsMCP. Agents empty means
// every agent with local-scope registrations; DryRun only reports.
type CleanAgentsMCPInput struct {
	Agents []string
	DryRun bool
}

// UninstallAgentsInput names the agents to uninstall from (all when empty).
type UninstallAgentsInput struct {
	Agents []string
}

// copyMarker marks a skill directory wspace copied because symlinks are
// unavailable (Windows without Developer Mode); it holds the source path.
const copyMarker = ".wspace-skill-source"

// configHomeEnv is the engine's configuration directory override.
const configHomeEnv = "WSPACE_CONFIG_HOME"

// agentsRun is one use-case invocation: resolved deps and skill source.
type agentsRun struct {
	ctx     context.Context
	deps    AgentsDeps
	src     SkillSource
	dataDir string
	exe     string // the running engine, symlinks resolved
}

func newAgentsRun(ctx context.Context, deps AgentsDeps) (*agentsRun, error) {
	r := &agentsRun{ctx: ctx, deps: deps.resolved()}
	r.dataDir = domain.SkillDataDir(r.deps.GOOS, string(r.deps.Home), r.deps.Getenv)
	src, err := r.resolveSource()
	if err != nil {
		return nil, err
	}
	r.src = src
	return r, nil
}

func slash(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// resolveSource finds the skill this installation links to (see
// SkillSource): the running engine's own app bundle, else the user data
// directory the embedded copy is extracted to.
func (r *agentsRun) resolveSource() (SkillSource, error) {
	exe, err := r.deps.Executable()
	if err != nil {
		return SkillSource{}, err
	}
	exe = slash(exe)
	if real, err := r.deps.FS.EvalSymlinks(domain.Path(exe)); err == nil {
		exe = string(real)
	}

	src := SkillSource{Name: domain.SkillName, Origin: SkillOriginEmbedded, Location: domain.LocationStable}
	src.Dir = r.dataDir + "/" + domain.SkillName
	if bundle, ok := domain.AppBundleOf(exe); ok {
		for _, name := range domain.BundleSkillNames() {
			dir := domain.BundleSkillDir(bundle, name)
			if ok, _ := r.deps.FS.Exists(domain.Path(dir + "/SKILL.md")); ok {
				src = SkillSource{Name: name, Dir: dir, Origin: SkillOriginBundle, Location: domain.ClassifySkillSourceLocation(bundle)}
				break
			}
		}
	}
	src.MCPServer = domain.MCPServerNameFor(src.Name)
	if src.Name == domain.DevSkillName {
		src.MCPConfigHome = r.deps.Getenv(configHomeEnv)
	}
	r.exe = exe
	src.MCPCommand = exe
	if r.deps.GOOS != "windows" {
		link := string(r.deps.Home) + "/.local/bin/wspace"
		if e, err := r.deps.FS.Lstat(domain.Path(link)); err == nil && e.Kind == ports.EntrySymlink {
			if target, err := r.deps.FS.EvalSymlinks(domain.Path(link)); err == nil && string(target) == exe {
				src.MCPCommand = link
			}
		}
	}
	return src, nil
}

// agents returns the catalog for this OS, filtered to ids (all when
// empty, unknown ids rejected).
func (r *agentsRun) agents(ids []string) ([]agentDef, error) {
	var all []agentDef
	for _, d := range agentCatalog {
		if d.darwinOnly && r.deps.GOOS != "darwin" {
			continue
		}
		all = append(all, d)
	}
	if len(ids) == 0 {
		return all, nil
	}
	var out []agentDef
	for _, id := range ids {
		found := false
		for _, d := range all {
			if d.id == id {
				out = append(out, d)
				found = true
				break
			}
		}
		if !found {
			return nil, domain.NewOpError("agents", domain.CodeUnknownAgent, id, "known: "+strings.Join(AgentIDs(), ", "), nil)
		}
	}
	return out, nil
}

func (r *agentsRun) home(rel string) string { return string(r.deps.Home) + "/" + rel }

func (r *agentsRun) detected(d agentDef) bool {
	if ok, _ := r.deps.FS.Exists(domain.Path(r.home(d.configDir))); ok {
		return true
	}
	for _, c := range d.commands {
		if _, ok := r.deps.Runner.LookPath(c); ok {
			return true
		}
	}
	return false
}

// skillState inspects d's skill entry.
func (r *agentsRun) skillState(d agentDef) (state SkillState, entry ports.Entry, target string) {
	if d.skillsDir == "" {
		return SkillUnsupported, ports.Entry{}, ""
	}
	p := r.home(d.skillsDir) + "/" + r.src.Name
	e, err := r.deps.FS.Lstat(domain.Path(p))
	if err != nil {
		return SkillConflict, e, ""
	}
	switch e.Kind {
	case ports.EntryMissing:
		return SkillMissing, e, ""
	case ports.EntrySymlink:
		t := slash(e.LinkTarget)
		if !path.IsAbs(t) && !strings.Contains(t, ":") {
			t = path.Join(r.home(d.skillsDir), t)
		}
		switch {
		case r.samePath(t, r.src.Dir):
			return SkillInstalled, e, t
		case domain.IsManagedSkillTarget(t, r.src.Name, r.dataDir):
			return SkillOutdated, e, t
		}
		return SkillConflict, e, t
	case ports.EntryDir:
		marker, err := r.deps.FS.ReadFile(domain.Path(p + "/" + copyMarker))
		if err != nil {
			return SkillConflict, e, ""
		}
		t := strings.TrimSpace(string(marker))
		if r.samePath(t, r.src.Dir) {
			return SkillInstalled, e, t
		}
		return SkillOutdated, e, t
	}
	return SkillConflict, e, ""
}

func (r *agentsRun) samePath(a, b string) bool {
	if path.Clean(a) == path.Clean(b) {
		return true
	}
	ra, errA := r.deps.FS.EvalSymlinks(domain.Path(a))
	rb, errB := r.deps.FS.EvalSymlinks(domain.Path(b))
	return errA == nil && errB == nil && ra == rb
}

// mcpState reads d's configuration: the user-scope state and every
// registration of this installation's server.
func (r *agentsRun) mcpState(d agentDef) (MCPState, []MCPRegistration) {
	cfg := r.home(d.mcpConfig)
	data, err := r.deps.FS.ReadFile(domain.Path(cfg))
	if err != nil {
		if e, _ := r.deps.FS.Lstat(domain.Path(cfg)); e.Kind != ports.EntryMissing {
			return MCPUnknown, nil
		}
		if d.mcpFormat == formatOpenCodeJSON {
			if e, _ := r.deps.FS.Lstat(domain.Path(cfg + "c")); e.Kind != ports.EntryMissing {
				return MCPUnknown, nil // opencode.jsonc: comments, not parsed
			}
		}
		return MCPNotRegistered, nil
	}
	raw, ok := readMCPRegistrations(d.mcpFormat, data, r.src.MCPServer, d.localArgs != nil)
	if !ok {
		return MCPUnknown, nil
	}
	state := MCPNotRegistered
	var regs []MCPRegistration
	for _, g := range raw {
		if g.scope == MCPScopeUser {
			state = MCPRegistered
		}
		regs = append(regs, MCPRegistration{Scope: g.scope, Project: g.project, Command: g.command, Stale: g.command != "" && !r.currentCommand(g.command)})
	}
	return state, regs
}

// currentCommand reports whether a registration's command starts this
// installation's wspace: the running engine, the command line tool link,
// or an engine inside an installed app bundle.
func (r *agentsRun) currentCommand(cmd string) bool {
	cmd = slash(cmd)
	if ok, _ := r.deps.FS.Exists(domain.Path(cmd)); !ok {
		return false
	}
	if cmd == r.src.MCPCommand || (r.deps.GOOS != "windows" && cmd == r.home(".local/bin/wspace")) || r.samePath(cmd, r.exe) {
		return true
	}
	real, err := r.deps.FS.EvalSymlinks(domain.Path(cmd))
	if err != nil {
		return false
	}
	bundle, ok := domain.AppBundleOf(string(real))
	return ok && domain.ClassifySkillSourceLocation(bundle) == domain.LocationStable
}

func (r *agentsRun) status(d agentDef) AgentStatus {
	s := AgentStatus{ID: d.id, Name: d.name, Detected: r.detected(d)}
	s.Skill, _, s.SkillTarget = r.skillState(d)
	if d.skillsDir != "" {
		s.SkillsDir = r.home(d.skillsDir)
		s.SkillPath = s.SkillsDir + "/" + r.src.Name
		legacy := s.SkillsDir + "/" + domain.LegacySkillName
		if e, _ := r.deps.FS.Lstat(domain.Path(legacy)); e.Kind != ports.EntryMissing {
			s.LegacyPath = legacy
		}
	}
	if d.mcpMethod != "" {
		s.MCP, s.MCPRegistrations = r.mcpState(d)
		s.MCPDuplicate = len(s.MCPRegistrations) > 1
		for _, g := range s.MCPRegistrations {
			s.MCPStale = s.MCPStale || g.Stale
		}
		s.MCPMethod = d.mcpMethod
		s.MCPConfigPath = r.home(d.mcpConfig)
		s.MCPSnippet = d.mcpSnippet(r.deps.GOOS, r.src)
	}
	return s
}

func (r *agentsRun) statuses(defs []agentDef) []AgentStatus {
	out := make([]AgentStatus, 0, len(defs))
	for _, d := range defs {
		out = append(out, r.status(d))
	}
	return out
}

// AgentsStatus reports, for every supported agent, whether it is
// installed on this machine and the state of wspace's skill, the legacy
// ws-workspaces skill and the MCP registration. It never writes.
func AgentsStatus(ctx context.Context, deps AgentsDeps, in AgentsStatusInput) (AgentsReport, error) {
	r, err := newAgentsRun(ctx, deps)
	if err != nil {
		return AgentsReport{}, err
	}
	defs, err := r.agents(in.Agents)
	if err != nil {
		return AgentsReport{}, err
	}
	return AgentsReport{Source: r.src, Agents: r.statuses(defs)}, nil
}

// InstallAgents links the skill into each selected agent's skills
// directory (a directory symlink to SkillSource.Dir), optionally moves the
// legacy ws-workspaces skill aside and registers the MCP server. Nothing
// is ever deleted: an entry wspace did not place is only replaced with
// Force, and then moved to "<skillsDir>/.wspace-backup-<timestamp>/".
func InstallAgents(ctx context.Context, deps AgentsDeps, in InstallAgentsInput) (AgentsChangeReport, error) {
	r, err := newAgentsRun(ctx, deps)
	if err != nil {
		return AgentsChangeReport{}, err
	}
	defs, err := r.agents(in.Agents)
	if err != nil {
		return AgentsChangeReport{}, err
	}
	if len(in.Agents) == 0 {
		var detected []agentDef
		for _, d := range defs {
			if r.detected(d) {
				detected = append(detected, d)
			}
		}
		defs = detected
	}
	out := AgentsChangeReport{Source: r.src}
	if len(defs) == 0 {
		out.Changes = []AgentChange{{Kind: ChangeNoAgents, Detail: strings.Join(AgentIDs(), ", ")}}
		return out, nil
	}

	if !in.SkipSkill {
		if r.src.Location != domain.LocationStable {
			return AgentsChangeReport{}, domain.NewOpError("agents.install", domain.CodeSkillSourceUnstable, r.src.Dir, string(r.src.Location), nil)
		}
		if r.src.Origin == SkillOriginEmbedded {
			if err := r.extract(); err != nil {
				return AgentsChangeReport{}, err
			}
		}
	}

	for _, d := range defs {
		if !in.SkipSkill && d.skillsDir != "" {
			out.Changes = append(out.Changes, r.installSkill(d, in.Force))
		}
		if d.skillsDir != "" {
			if c, ok := r.legacy(d, in.DisableLegacy); ok {
				out.Changes = append(out.Changes, c)
			}
		}
		if in.MCP && d.mcpMethod != "" {
			out.Changes = append(out.Changes, r.registerMCP(d)...)
		}
	}
	out.Agents = r.statuses(defs)
	return out, nil
}

func (r *agentsRun) timestamp() string { return r.deps.Now().UTC().Format("20060102-150405") }

func failed(agent, p string, err error) AgentChange {
	return AgentChange{Agent: agent, Kind: ChangeFailed, Path: p, Detail: err.Error()}
}

func (r *agentsRun) installSkill(d agentDef, force bool) AgentChange {
	dir := r.home(d.skillsDir)
	p := dir + "/" + r.src.Name
	state, entry, target := r.skillState(d)
	change := AgentChange{Agent: d.id, Path: p, Target: r.src.Dir}

	switch state {
	case SkillInstalled:
		if entry.Kind == ports.EntrySymlink {
			change.Kind = ChangeUnchanged
			return change
		}
		fallthrough // a marked copy is re-copied: its content may be stale
	case SkillOutdated:
		if err := r.removeOurs(p, entry); err != nil {
			return failed(d.id, p, err)
		}
		if _, err := r.place(p); err != nil {
			return failed(d.id, p, err)
		}
		change.Kind = ChangeRefreshed
		return change
	case SkillConflict:
		if !force {
			change.Kind = ChangeConflict
			change.Target = target
			return change
		}
		if entry.Kind == ports.EntrySymlink {
			if err := r.deps.FS.RemoveLink(domain.Path(p)); err != nil {
				return failed(d.id, p, err)
			}
		} else {
			backupDir := dir + "/.wspace-backup-" + r.timestamp()
			if err := r.deps.FS.MkdirAll(domain.Path(backupDir)); err != nil {
				return failed(d.id, p, err)
			}
			change.Backup = backupDir + "/" + r.src.Name
			if err := r.deps.FS.Rename(domain.Path(p), domain.Path(change.Backup)); err != nil {
				return failed(d.id, p, err)
			}
		}
		if _, err := r.place(p); err != nil {
			return failed(d.id, p, err)
		}
		change.Kind = ChangeReplaced
		return change
	}

	if err := r.deps.FS.MkdirAll(domain.Path(dir)); err != nil {
		return failed(d.id, p, err)
	}
	copied, err := r.place(p)
	if err != nil {
		return failed(d.id, p, err)
	}
	change.Kind = ChangeLinked
	if copied {
		change.Kind = ChangeCopied
	}
	return change
}

// place links p to the source, or copies it with a marker when this
// platform cannot create symlinks.
func (r *agentsRun) place(p string) (copied bool, err error) {
	err = r.deps.FS.Symlink(domain.Path(r.src.Dir), domain.Path(p))
	if !errors.Is(err, ports.ErrSymlinkUnsupported) {
		return false, err
	}
	if err := r.deps.FS.CopyDir(domain.Path(r.src.Dir), domain.Path(p)); err != nil {
		return true, err
	}
	return true, r.deps.FS.WriteFile(domain.Path(p+"/"+copyMarker), []byte(r.src.Dir+"\n"), 0o644)
}

// removeOurs removes an entry wspace placed: a link, or a marked copy.
func (r *agentsRun) removeOurs(p string, entry ports.Entry) error {
	if entry.Kind == ports.EntrySymlink {
		return r.deps.FS.RemoveLink(domain.Path(p))
	}
	return r.deps.FS.RemoveAll(domain.Path(p))
}

// legacy reports the legacy skill and, when disable is set, moves it to
// "<agent root>/skills-disabled/ws-workspaces" (timestamped if taken).
func (r *agentsRun) legacy(d agentDef, disable bool) (AgentChange, bool) {
	dir := r.home(d.skillsDir)
	p := dir + "/" + domain.LegacySkillName
	if e, _ := r.deps.FS.Lstat(domain.Path(p)); e.Kind == ports.EntryMissing {
		return AgentChange{}, false
	}
	disabledDir := path.Dir(dir) + "/skills-disabled"
	dst := disabledDir + "/" + domain.LegacySkillName
	if !disable {
		return AgentChange{Agent: d.id, Kind: ChangeLegacyFound, Path: p, Backup: dst}, true
	}
	if err := r.deps.FS.MkdirAll(domain.Path(disabledDir)); err != nil {
		return failed(d.id, p, err), true
	}
	if e, _ := r.deps.FS.Lstat(domain.Path(dst)); e.Kind != ports.EntryMissing {
		dst += "-" + r.timestamp()
	}
	if err := r.deps.FS.Rename(domain.Path(p), domain.Path(dst)); err != nil {
		return failed(d.id, p, err), true
	}
	return AgentChange{Agent: d.id, Kind: ChangeLegacyDisabled, Path: p, Backup: dst}, true
}

// registerMCP adds the user-scope registration unless there is one (or
// the configuration cannot be read), and reports every local-scope one as
// a duplicate to clean up: they are never removed here.
func (r *agentsRun) registerMCP(d agentDef) []AgentChange {
	state, regs := r.mcpState(d)
	var dups []AgentChange
	for _, g := range regs {
		if g.Scope == MCPScopeLocal {
			dups = append(dups, AgentChange{Agent: d.id, Kind: ChangeMCPLocalDuplicate, Path: g.Project, Target: g.Command})
		}
	}
	return append([]AgentChange{r.addUserMCP(d, state)}, dups...)
}

func (r *agentsRun) addUserMCP(d agentDef, state MCPState) AgentChange {
	c := AgentChange{Agent: d.id, Path: r.home(d.mcpConfig)}
	snippet := d.mcpSnippet(r.deps.GOOS, r.src)
	switch state {
	case MCPRegistered:
		c.Kind = ChangeMCPAlready
		return c
	case MCPUnknown:
		c.Kind, c.Detail = ChangeMCPUnknown, snippet
		return c
	}
	if d.mcpMethod != MCPMethodCLI {
		c.Kind, c.Detail = ChangeMCPSnippet, snippet
		return c
	}
	if _, ok := r.deps.Runner.LookPath(d.cli); !ok {
		c.Kind, c.Detail = ChangeMCPSnippet, snippet
		return c
	}
	res, err := r.deps.Runner.Run(r.ctx, d.cli, d.cliArgs(r.src.MCPServer, nativePath(r.deps.GOOS, r.src.MCPCommand), r.mcpEnv())...)
	switch {
	case err != nil:
		c.Kind, c.Detail, c.Manual = ChangeMCPFailed, err.Error(), snippet
	case res.ExitCode != 0:
		c.Kind, c.Detail, c.Manual = ChangeMCPFailed, firstLine(res.Stderr, res.Stdout, fmt.Sprintf("exit status %d", res.ExitCode)), snippet
	default:
		c.Kind = ChangeMCPRegistered
	}
	return c
}

// CleanAgentsMCP removes duplicate local-scope registrations of this
// installation's MCP server (only its own name: "wspace", or "wspace-dev"
// for the development app) through the agent's CLI, run in each project
// directory. With a user-scope registration every local one goes; without
// one, exactly one local registration is kept (a current command first,
// this engine's own command first among those). DryRun reports the plan
// and runs nothing.
func CleanAgentsMCP(ctx context.Context, deps AgentsDeps, in CleanAgentsMCPInput) (AgentsChangeReport, error) {
	r, err := newAgentsRun(ctx, deps)
	if err != nil {
		return AgentsChangeReport{}, err
	}
	defs, err := r.agents(in.Agents)
	if err != nil {
		return AgentsChangeReport{}, err
	}
	out := AgentsChangeReport{Source: r.src}
	var acted []agentDef
	for _, d := range defs {
		if d.localArgs == nil {
			continue
		}
		acted = append(acted, d)
		out.Changes = append(out.Changes, r.cleanMCP(d, in.DryRun)...)
	}
	if len(in.Agents) > 0 {
		acted = defs
	}
	out.Agents = r.statuses(acted)
	return out, nil
}

func (r *agentsRun) cleanMCP(d agentDef, dryRun bool) []AgentChange {
	state, regs := r.mcpState(d)
	if state == MCPUnknown {
		return []AgentChange{{Agent: d.id, Kind: ChangeMCPCleanFailed, Path: r.home(d.mcpConfig), Detail: messages.T(messages.AgentsMCPConfigUnreadable)}}
	}
	var locals []MCPRegistration
	for _, g := range regs {
		if g.Scope == MCPScopeLocal {
			locals = append(locals, g)
		}
	}
	var out []AgentChange
	if state != MCPRegistered {
		if len(locals) < 2 {
			return nil
		}
		keep := preferredLocal(locals, r.src.MCPCommand)
		out = append(out, AgentChange{Agent: d.id, Kind: ChangeMCPDuplicateKept, Path: locals[keep].Project, Target: locals[keep].Command})
		locals = append(locals[:keep:keep], locals[keep+1:]...)
	}
	_, haveCLI := r.deps.Runner.LookPath(d.cli)
	for _, g := range locals {
		c := AgentChange{Agent: d.id, Path: g.Project, Target: g.Command}
		switch ok, _ := r.deps.FS.Exists(domain.Path(g.Project)); {
		case dryRun:
			c.Kind = ChangeMCPDuplicateWouldRemove
		case !haveCLI:
			c.Kind, c.Detail = ChangeMCPCleanFailed, messages.T(messages.AgentsMCPCLIMissing, d.cli)
		case !ok:
			c.Kind, c.Detail = ChangeMCPCleanFailed, messages.T(messages.AgentsMCPProjectMissing)
		default:
			c.Kind = ChangeMCPDuplicateRemoved
			res, err := r.deps.Runner.RunIn(r.ctx, g.Project, d.cli, d.localArgs(r.src.MCPServer)...)
			switch {
			case err != nil:
				c.Kind, c.Detail = ChangeMCPCleanFailed, err.Error()
			case res.ExitCode != 0:
				c.Kind, c.Detail = ChangeMCPCleanFailed, firstLine(res.Stderr, res.Stdout, fmt.Sprintf("exit status %d", res.ExitCode))
			}
		}
		out = append(out, c)
	}
	return out
}

// preferredLocal picks the local registration to keep when there is no
// user-scope one: a current one running this engine's command, else any
// current one, else the first (locals are sorted by project path).
func preferredLocal(locals []MCPRegistration, command string) int {
	best := -1
	for i, g := range locals {
		if g.Stale {
			continue
		}
		if g.Command == command {
			return i
		}
		if best < 0 {
			best = i
		}
	}
	return max(best, 0)
}

// mcpEnv is the KEY=VALUE environment the registration passes, if any.
func (r *agentsRun) mcpEnv() []string {
	if r.src.MCPConfigHome == "" {
		return nil
	}
	return []string{configHomeEnv + "=" + nativePath(r.deps.GOOS, r.src.MCPConfigHome)}
}

func firstLine(candidates ...string) string {
	for _, s := range candidates {
		for _, line := range strings.Split(s, "\n") {
			if l := strings.TrimSpace(line); l != "" {
				return l
			}
		}
	}
	return ""
}

// UninstallAgents removes wspace's skill entries (its links and marked
// copies) from the selected agents, all of them by default. Entries
// wspace did not place, the legacy skill and MCP registrations are left
// alone.
func UninstallAgents(ctx context.Context, deps AgentsDeps, in UninstallAgentsInput) (AgentsChangeReport, error) {
	r, err := newAgentsRun(ctx, deps)
	if err != nil {
		return AgentsChangeReport{}, err
	}
	defs, err := r.agents(in.Agents)
	if err != nil {
		return AgentsChangeReport{}, err
	}
	out := AgentsChangeReport{Source: r.src}
	for _, d := range defs {
		state, entry, target := r.skillState(d)
		p := ""
		if d.skillsDir != "" {
			p = r.home(d.skillsDir) + "/" + r.src.Name
		}
		switch state {
		case SkillInstalled, SkillOutdated:
			if err := r.removeOurs(p, entry); err != nil {
				out.Changes = append(out.Changes, failed(d.id, p, err))
				continue
			}
			out.Changes = append(out.Changes, AgentChange{Agent: d.id, Kind: ChangeRemoved, Path: p, Target: target})
		case SkillConflict:
			out.Changes = append(out.Changes, AgentChange{Agent: d.id, Kind: ChangeKept, Path: p, Target: target})
		}
	}
	out.Agents = r.statuses(defs)
	return out, nil
}

// extract writes the embedded skill to the data directory unless the
// copy there already matches it (a fingerprint of every embedded file is
// kept next to it), replacing the whole directory so files a newer
// version dropped do not linger.
func (r *agentsRun) extract() error {
	type file struct {
		rel  string
		data []byte
	}
	var files []file
	err := fs.WalkDir(r.deps.Skill, domain.SkillName, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(r.deps.Skill, p)
		if err != nil {
			return err
		}
		files = append(files, file{rel: strings.TrimPrefix(p, domain.SkillName+"/"), data: data})
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	h := sha256.New()
	for _, f := range files {
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", f.rel, len(f.data))
		_, _ = h.Write(f.data)
	}
	sum := hex.EncodeToString(h.Sum(nil))

	dir := r.src.Dir
	stamp := domain.Path(r.dataDir + "/." + domain.SkillName + ".sha256")
	if cur, err := r.deps.FS.ReadFile(stamp); err == nil && strings.TrimSpace(string(cur)) == sum {
		if ok, _ := r.deps.FS.Exists(domain.Path(dir + "/SKILL.md")); ok {
			return nil
		}
	}
	if err := r.deps.FS.RemoveAll(domain.Path(dir)); err != nil {
		return err
	}
	for _, f := range files {
		target := dir + "/" + f.rel
		if err := r.deps.FS.MkdirAll(domain.Path(path.Dir(target))); err != nil {
			return err
		}
		if err := r.deps.FS.WriteFile(domain.Path(target), f.data, 0o644); err != nil {
			return err
		}
	}
	return r.deps.FS.WriteFile(stamp, []byte(sum+"\n"), 0o644)
}
