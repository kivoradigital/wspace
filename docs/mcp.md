# MCP Server (`wspace mcp serve`)

> Code: `internal/mcpserver`, built on the official Go SDK `github.com/modelcontextprotocol/go-sdk` v1.8.0. Transport: stdio (newline-delimited JSON-RPC). The tools are thin adapters over the same engine as [`wspace rpc`](rpc-contract.md).

## 1. Tools

Every `context` argument is optional; it defaults to the active context. Arguments use the same camelCase names as the [`wspace rpc`](rpc-contract.md#4-methods) parameters of the engine method each tool wraps, with three MCP-only differences: destructive tools take `confirm`, `remove_project` names the repo alias `project`, and list results are wrapped in an object. Every tool returns structured content (the shape below, also declared as the tool's output schema) and a one-line text summary.

Hints: **RO** read-only, **D** destructive, **I** idempotent. Every tool sets `openWorldHint: false` except `check_update` (it asks GitHub), `repo_fetch`, `repo_pull_ff` and `repo_push` (they contact the git remote). A tool without **D** that is not **RO** sets `destructiveHint: false`.

### 1.1 Engine

| Tool | Engine method | Arguments | Result | Hints |
|---|---|---|---|---|
| `engine_version` | `engine.version` | — | `{"version"}` | RO, I |
| `engine_info` | `engine.info` | `context?` | `InfoResult` | RO, I |
| `check_update` | `engine.checkUpdate` | — | `UpdateCheckResult`; `bundledBy` (omitted when empty) names the desktop app wspace is bundled with: no network request is made and the app updates it (see [bundling](bundling.md)) | RO, I, open world |
| `run_doctor` | `engine.doctor` | `context?` | `{"gitTooOld","prunedWorktrees","warnings":[]}` | I |

### 1.2 Contexts

| Tool | Engine method | Arguments | Result | Hints |
|---|---|---|---|---|
| `list_contexts` | `contexts.list` | — | `{"contexts":[ContextSummary]}` | RO, I |
| `get_context` | `contexts.get` | `name?` | `Context` | RO, I |
| `create_context` | `contexts.create` | `name`, `workspacesRoot`, `projectsRoot?`, `ignorePatterns?`, `includePatterns?`, `projectScanMaxDepth?`, `defaults?`, `activate?` | `Context` | — |
| `update_context` | `contexts.update` | `name`, any of `newName`, `workspacesRoot`, `projectsRoot`, `ignorePatterns`, `includePatterns`, `projectScanMaxDepth`, `defaults` | `Context` | I |
| `switch_context` | `contexts.switch` | `name` | `ContextSummary` | I |
| `delete_context` | `contexts.delete` | `name`, `allowActive?`, `confirm` | `{"deleted"}` | D, I |
| `import_legacy_context` | `contexts.importLegacy` | `from?`, exactly one of `name`/`into`, `confirm?` | `ImportLegacyResult` | — |

### 1.3 Projects

| Tool | Engine method | Arguments | Result | Hints |
|---|---|---|---|---|
| `list_projects` | `projects.list` | `context?` | `{"projects":[Project]}` | RO, I |
| `scan_projects` | `projects.scan` | `context?`, `roots?`, `depth?`, `ignore?`, `include?` | `ScanResult` | RO, I |
| `register_project` | `projects.register` | `context?`, `key`, `sourceDir`, `originBranch?`, `destBranch?`, `worktreeDir?` | `Project` | — |
| `register_projects` | `projects.registerMany` | `context?`, `projects: [{"key","sourceDir"}]` | `RegisterManyResult` | — |
| `update_project` | `projects.update` | `context?`, `key`, any of `sourceDir`, `originBranch`, `destBranch`, `worktreeDir` | `Project` | I |
| `unregister_project` | `projects.remove` | `context?`, `key`, `confirm` | `{"unregistered"}` | D, I |

### 1.4 Workspaces

| Tool | Engine method | Arguments | Result | Hints |
|---|---|---|---|---|
| `list_workspaces` | `workspaces.list` | `context?` | `{"workspaces":[WorkspaceStatus]}` | RO, I |
| `workspace_status` | `workspaces.status` | `context?`, `workspace` | `WorkspaceStatus` | RO, I |
| `repo_changes` | `workspaces.repoChanges` | `context?`, `workspace`, `repo` | `{"changes":[FileChange]}` | RO, I |
| `teardown_check` | `workspaces.teardownCheck` | `context?`, `workspace` | `{"safe","reasons":[Blocker]}` | RO, I |
| `create_workspace` | `workspaces.create` | `context?`, `name`, `branch?`, `projects?`, `options?`, `copyNodeModules?` | `Workspace` (one `repos` entry per repo) | — |
| `list_addable_projects` | `workspaces.addableProjects` | `context?`, `workspace` | `{"projects":[Project]}` | RO, I |
| `add_project` | `workspaces.addRepo` | `context?`, `workspace`, `project`, `options?`, `copyNodeModules?` | `RepoEntry` | — |
| `remove_project` | `workspaces.removeRepo` | `context?`, `workspace`, `project` (repo alias), `confirm`, `force?`, `deleteBranch?` | `{"alias"}` | D |
| `destroy_workspace` | `workspaces.destroy` | `context?`, `workspace`, `confirm`, `force?`, `deleteBranches?` | `{"path"}` | D |
| `repair_workspace` | `workspaces.repair` | `context?`, `workspace` | `{"recreated":[]}` | I |
| `sync_env` | `workspaces.syncEnv` | `context?`, `workspace`, `confirm` | `{"copied":[]}` | D, I |
| `adopt_legacy_workspaces` | `workspaces.adoptLegacy` | `context?` | `AdoptLegacyResult` | I |
| `claim_workspaces` | `workspaces.claim` | `context?`, `workspaces?` | `{"claimed":[ClaimedWorkspace],"skipped":[ClaimSkipped]}` | I |
| `update_repo` | `workspaces.updateRepo` | `context?`, `workspace`, `repo`, `strategy?` (`merge`/`rebase`), `autostash?` | `RepoUpdate` | — |
| `update_workspace` | `workspaces.update` | `context?`, `workspace`, `strategy?`, `autostash?` | `{"repos":[RepoUpdate]}` | — |

### 1.4.1 Repository inspector

| Tool | Engine method | Arguments | Result | Hints |
|---|---|---|---|---|
| `repo_branch_info` | `repos.inspect` (branch part) | `context?`, `workspace`, `repo` | `BranchInfo` | RO, I |
| `repo_diff` | `repos.diff` | `context?`, `workspace`, `repo`, `path`, `staged?` | `FileDiff` | RO, I |
| `repo_commits` | `repos.commits` | `context?`, `workspace`, `repo`, `range?`, `offset?`, `limit?` | `RepoCommits` | RO, I |
| `repo_commit` | `repos.commit` | `context?`, `workspace`, `repo`, `hash` | `CommitDetail` | RO, I |
| `repo_stashes` | `repos.stashes` | `context?`, `workspace`, `repo` | `{"stashes":[Stash]}` | RO, I |
| `repo_stash` | `repos.stash` | `context?`, `workspace`, `repo`, `index` | `StashDetail` | RO, I |
| `repo_fetch` | `repos.fetch` | `context?`, `workspace`, `repo` | `{"repo","remote","branch"}` | I, open world |
| `repo_pull_ff` | `repos.pull` | `context?`, `workspace`, `repo` | `RepoPull` | open world |
| `repo_stash_apply` | `repos.stashApply` | `context?`, `workspace`, `repo`, `index`, `hash` | `StashApplyResult` | — |
| `repo_stash_pop` | `repos.stashPop` | `context?`, `workspace`, `repo`, `index`, `hash` | `StashApplyResult` | — |
| `repo_stash_drop` | `repos.stashDrop` | `context?`, `workspace`, `repo`, `index`, `hash`, `confirm` | `{"repo","stash"}` | D |
| `repo_delete_untracked` | `repos.discardUntracked` | `context?`, `workspace`, `repo`, `paths`, `confirm` | `{"repo","removed","kept"}` | D |
| `repo_stage` | `repos.stage` | `context?`, `workspace`, `repo`, `paths?`, `all?` | `{"repo","paths"}` | I |
| `repo_unstage` | `repos.unstage` | `context?`, `workspace`, `repo`, `paths?`, `all?` | `{"repo","paths"}` | I |
| `repo_discard` | `repos.discard` | `context?`, `workspace`, `repo`, `paths`, `confirm` | `{"repo","discarded","backup"?}` | D |
| `repo_commit_changes` | `repos.commitChanges` | `context?`, `workspace`, `repo`, `message` | `{"repo","commit"?,"warnings","error"?}` | — |
| `repo_push` | `repos.push` | `context?`, `workspace`, `repo`, `setUpstream?` | `RepoPush` | open world |
| `repo_stash_create` | `repos.stashCreate` | `context?`, `workspace`, `repo`, `message?`, `includeUntracked?`, `keepIndex?` | `{"repo","stash"}` | — |

`repo_changes` (above) already lists staged and unstaged changes, so it is reused unchanged. Rules and shapes are in [rpc-contract.md §4.5](rpc-contract.md#45-repository-inspector-additive-v1): diffs only for paths the repo's status lists, 1 MiB / 5000-line caps (`truncated`), author names only (never e-mail addresses). The stash and untracked-file actions follow [rpc-contract.md §4.6](rpc-contract.md#46-repository-actions-additive-v1); stage, unstage, discard, commit, push and stash create follow [rpc-contract.md §4.7](rpc-contract.md#47-repository-write-actions-additive-v1). `repo_commit` reads one commit; committing is `repo_commit_changes`.

Shapes are the engine shapes in [rpc-contract.md §4](rpc-contract.md#4-methods). Required arguments are marked required in each tool's input schema; a call missing one is rejected before the handler runs.

### 1.5 Progress

`create_workspace`, `add_project`, `remove_project`, `destroy_workspace`, `repair_workspace`, `sync_env`, `update_repo`, `update_workspace`, `repo_fetch`, `repo_pull_ff`, `repo_push` and `run_doctor` forward the engine's progress events as MCP `notifications/progress` when the request carries a `progressToken` (`_meta.progressToken`). `progress` counts events (1, 2, 3, …); `total` is not set; `message` is `"<repo>: <op> <phase>"` for a per-repo event (for example `api: workspace.create finished`) or the engine's rendered step text. Without a token nothing is sent. The final result lists every repo either way.

## 2. Safety rules

- The destructive tools are `destroy_workspace`, `remove_project`, `delete_context`, `unregister_project`, `sync_env` (it overwrites env file copies, including local edits), `repo_stash_drop`, `repo_delete_untracked` (permanent: it does not use the Trash) and `repo_discard` (it keeps a backup patch). Each one requires `confirm` in its input schema, so a call without it is rejected before anything runs.
- With `confirm: false` a destructive tool changes nothing. It fails with `needs_confirmation`, and `data.reasons` previews what would be lost: for `destroy_workspace` and `remove_project`, the same blockers as `teardown_check` (scoped to the repo for `remove_project`, possibly an empty list); for the others an empty list plus what would be removed (`data.context`, `data.project`, `data.workspace`; `data.stash` and `data.files` for `repo_stash_drop`; `data.paths` for `repo_delete_untracked`; `data.files` with line counts for `repo_discard`).
- With `confirm: true`, `destroy_workspace` and `remove_project` still refuse without `force` when work would be lost (tracked changes, untracked files wspace did not create, unpushed commits). They fail with `needs_confirmation` listing every reason. No tool ever forces by itself.
- `force: true` discards that work. Tool descriptions and the server instructions tell the agent to set it only after the user explicitly agrees.
- `delete_context` never touches workspaces or repositories on disk and is refused (`conflict`) for the active context unless `allowActive` is true, which also clears the active context. The deleted context's workspaces then show as orphans (`orphanOf` in `list_workspaces`) to any other context sharing the root.
- `update_context` with `newName` moves the old name's workspaces to the new name and reports them in `reassignedWorkspaces` (failures in `reassignFailures`; the rename itself is kept).
- `claim_workspaces` is not destructive: it only rewrites the owner of orphaned workspaces (owner context no longer exists) and never takes one owned by an existing context. `unregister_project` never touches the repository or existing workspaces.
- `create_workspace` and `add_project` roll back what they created if they fail part-way. With `copyNodeModules: true` they copy `node_modules` from each Node project's main clone (`list_projects` reports `isNode`/`hasNodeModules`) into the new worktree, copy-on-write where supported ([rpc-contract.md §4](rpc-contract.md)).
- `import_legacy_context` with `into` is a dry run unless `confirm: true` (as in the RPC).
- `repo_fetch` and `repo_pull_ff` are not destructive. `repo_fetch` only updates remote-tracking refs. `repo_pull_ff` only fast-forwards: it never creates a merge commit or rebases, and it refuses, changing nothing, a branch with no upstream (`no_upstream`), a diverged branch (`diverged`, with `data.ahead`/`data.behind`; the description tells the agent to suggest `update_repo` or manual integration) and local changes git would overwrite (`worktree_dirty`, `data.files`). Refusals are a successful tool call whose result carries `error`.
- `repo_stash_apply` and `repo_stash_pop` are not destructive: they never reset the worktree and never drop an entry that did not apply completely. They need the entry's `hash` from `repo_stashes` (a shifted list is refused with `stash_changed`). Refusals (`worktree_dirty` with `data.files`, `integration_in_progress`) change nothing; a conflict (`stash_conflict`, `conflicts`) keeps the entry and leaves conflict markers for the user. `warnings` may hold `index_not_restored` (staged changes came back unstaged). All of these are a successful tool call whose result carries `error`.
- `repo_commit_changes` and `repo_push` change shared history, so their descriptions and the server instructions tell the agent to call them only when the user asked, with a message the user approved. `repo_commit_changes` uses the repo's own identity and hooks: `identity_missing` returns the `git config` commands for the user to run (the agent must not run them), `hook_failed` returns the hook's output (never bypassed). `repo_push` never force-pushes; `no_upstream` (including a branch that tracks a remote branch of another name, such as its base) is resolved only by the user agreeing to publish (`setUpstream: true`), and `push_rejected` points to `repo_pull_ff` or `update_repo`. Refusals are a successful tool call whose result carries `error`.
- `repo_discard` only discards unstaged changes of tracked files (staged changes stay); it refuses untracked files (`path_is_untracked`) and staged-only files (`staged_only`), and writes a backup patch before discarding.
- `list_addable_projects` lists the projects not already in a workspace; `add_project` refuses one that is (`already_exists`, `already_in_workspace`).
- `update_repo` and `update_workspace` are not destructive and not idempotent (a later call may integrate new commits). They refuse, changing nothing, a repo with uncommitted tracked changes unless `autostash: true`, and abort any conflict so the repo returns to its previous HEAD and status ([rpc-contract.md §4.4.1](rpc-contract.md#441-updating-from-the-base-additive-v1)). A refusal or conflict is a successful tool call whose result carries `error` per repo; the summary names it. `strategy: "rebase"` rewrites local commits (a pushed branch then needs a force push); the descriptions tell the agent to use it only when the user asked.

A tool failure is a tool result with `isError: true` whose text content is the engine's error object, for example:

```json
{"code":"needs_confirmation","message":"this destructive operation requires confirm: true","data":{"reasons":[{"repo":"api","kind":"unpushed_commits","count":4,"message":"api: 4 unpushed commit(s)"}]}}
```

The codes are the stable codes of [rpc-contract.md §3](rpc-contract.md#3-errors).

## 3. Registering the server

Replace `<path>` with the directory holding the `wspace` binary (for a development build, `dist`). The server reads the same config as the CLI (`~/.config/wspace`, or `WSPACE_CONFIG_HOME`).

| Agent | Where |
|---|---|
| Claude Code | `claude mcp add`, or `~/.claude.json` / `.mcp.json` |
| Claude Desktop | `mcpServers` in `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Cursor | `~/.cursor/mcp.json` |
| Codex | `codex mcp add`, or `[mcp_servers.wspace]` in `~/.codex/config.toml` |

Claude Code:

```sh
claude mcp add --transport stdio wspace -- <path>/wspace mcp serve
```

Claude Desktop (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "wspace": {
      "command": "<path>/wspace",
      "args": ["mcp", "serve"]
    }
  }
}
```

Cursor (`~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "wspace": {
      "type": "stdio",
      "command": "<path>/wspace",
      "args": ["mcp", "serve"]
    }
  }
}
```

Codex (`~/.codex/config.toml`), or `codex mcp add wspace -- <path>/wspace mcp serve`:

```toml
[mcp_servers.wspace]
command = "<path>/wspace"
args = ["mcp", "serve"]
```

Gemini CLI: `gemini mcp add --scope user wspace <path>/wspace mcp serve` (user servers in `~/.gemini/settings.json`). OpenCode: a `"mcp": {"wspace": {"type": "local", "command": ["<path>/wspace", "mcp", "serve"], "enabled": true}}` entry in `~/.config/opencode/opencode.json` (its `opencode mcp add` is interactive). Sources in §6.2.

`wspace agents install --mcp` (or the equivalent action in a desktop client) runs the CLI commands above for Claude Code, Codex and Gemini CLI and prints the snippet for the file-configured agents; see §6.

## 4. Presence

Each `wspace mcp serve` process is started by one agent. While it runs, it keeps a presence record so a client can show which agents are connected.

| Aspect | Behavior |
|---|---|
| File | `<config>/run/mcp/<pid>.json`, where `<config>` is the wspace config directory (`WSPACE_CONFIG_HOME` or `~/.config/wspace`). |
| Content | `{"pid","clientName","clientVersion","startedAt","lastActivityAt","lastTool"?,"lastToolAt"?}`, times RFC 3339 UTC. |
| Written | When the client identifies itself: after `initialize` (its `clientInfo`), or on the first request of a protocol ≥ 2026-07-28 client, which sends `clientInfo` in each request's `_meta` instead. |
| Updated | On each `tools/call`, before the tool runs. Repeated calls of the same tool within 2 s are not written again; a different tool is always written. |
| Atomic | Written to a temp file in the same folder, then renamed over the record. |
| Removed | When the session ends: stdin closes, the client disconnects, or the process gets SIGINT, SIGTERM or SIGHUP. |
| Stale | A process killed with SIGKILL cannot remove its record. Readers (`mcp.sessions`) check each PID and ignore and delete records of dead processes and their leftover temp files. |
| Failures | Presence is best effort: a failed write never fails a tool call. |

Code: the `ports.PresenceStore` port, its file-system adapter `internal/adapters/presencefs`, `engine.StartPresence`/`engine.MCPSessions`, and the server middleware in `internal/mcpserver/server.go`. A client reads it through [`mcp.sessions`](rpc-contract.md#411-mcp-sessions-additive-v1). Because a killed server leaves no file event, a client that watches `<config>/run/mcp` should also poll (for example every 15 s); a client's own configuration watcher should ignore `<config>/run`, so agent activity never reloads the configuration. Connected is not active: agent apps start their MCP servers at launch and keep idle ones running for hours. A reasonable rule is to treat a session as **active** while its last request (a tool call; `lastToolAt`, or `lastActivityAt` later than `startedAt`) is at most 2 minutes old, computed by the client against its own clock.

## 5. Manual check

The server answers only while stdin stays open, as a real client keeps it:

```sh
(printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 1) | wspace mcp serve
```

## 6. Agent skills

The `wspace-workspaces` skill teaches an agent when and how to drive wspace (MCP tools first, CLI fallback, the confirmation and force rules of §2). One source, two carriers:

| Carrier | Where | How it gets there |
|---|---|---|
| Go binary (every platform) | `embed.FS` in `skills` | `//go:embed`; `TestEmbeddedSkillEqualsSource` checks the embedded copy against the files and the Agent Skills frontmatter rules. |
| Desktop app bundle (optional, macOS) | `Contents/Resources/skills/wspace-workspaces/` | Copied from `skills/` by the app's own build. A development build may ship it as `wspace-dev-workspaces` (frontmatter `name` rewritten) and register the MCP server as `wspace-dev` with its own `WSPACE_CONFIG_HOME`, so it never collides with an installed app. |

There is deliberately **no MCP tool** to install skills or register servers: an agent must not reconfigure itself or other agents. Installation runs from the CLI or the RPC (`agents.*`, [rpc-contract.md §4.1.2](rpc-contract.md#412-agent-skills-additive-v1)), for example on behalf of a desktop client.

### 6.1 Installing

```sh
wspace agents status [--agent <id>]... [--json]
wspace agents install [--agent <id>]... [--mcp] [--disable-legacy] [--force] [--json]
wspace agents uninstall [--agent <id>]... [--json]
wspace agents mcp-clean [--agent <id>]... [--dry-run] [--json]
```

- **Source.** The engine resolves its own executable with symlinks evaluated. Inside `<X>.app/Contents/Helpers/` with the skill in the bundle, agents link to the bundle's copy. Otherwise (Linux, Windows, a standalone CLI) the embedded skill is extracted to `$XDG_DATA_HOME/wspace/skills/wspace-workspaces` (default `~/.local/share/…`; `%LOCALAPPDATA%\wspace\skills` on Windows) and refreshed whenever its content differs from the binary's (a fingerprint file sits next to it). A link never points into a source checkout. A bundle under `/Volumes/` (a mounted DMG) or an App Translocation path is refused: move the app to a stable location (for example Applications) first.
- **Install** creates `<skillsDir>/wspace-workspaces` as a directory symlink. A wspace link to another source (moved app, old extraction) is refreshed. A link elsewhere or a real folder with that name is reported (`conflict`) and replaced only with `--force`: a link is removed, a real folder is moved to `<skillsDir>/.wspace-backup-<UTC timestamp>/`. Nothing is deleted. Where symlinks cannot be created (Windows without Developer Mode) the skill is copied and marked with `.wspace-skill-source`; directory junctions are not implemented.
- **Legacy skill.** `ws-workspaces` (the bash `ws` skill) is reported; with `--disable-legacy` it moves to `<agent root>/skills-disabled/ws-workspaces` (a timestamp suffix if taken).
- **Uninstall** removes only wspace's links (and marked copies); other entries, the legacy skill and MCP registrations stay.
- **MCP (`--mcp`).** The registration state is read from each agent's configuration file (best effort; `unknown` when it cannot be parsed, and then nothing is registered). An existing `wspace` entry is never duplicated. Agents with a CLI get it run with a 30 s timeout; file-configured agents (Claude Desktop, Cursor, OpenCode) get the snippet, never an edited file. The command is `~/.local/bin/wspace` when that is a desktop app's link to this engine, else the engine itself.
- **Finding agent CLIs.** `claude`, `codex` and `gemini` are resolved the way the user's shell does, not by `os/exec`'s lookup: `PATH` in order and, on Windows, the `PATHEXT` extensions in order within each directory (`.COM;.EXE;.BAT;.CMD` when unset). A directory inside a `node_modules` tree is never used (an npm package's internal binary, such as `node_modules\@anthropic-ai\claude-code\bin\claude.exe`, is what its shim runs, not what the user types). Without a match on Windows, `%USERPROFILE%\.local\bin\<name>.exe` (Claude Code's native installer) and `%APPDATA%\npm\<name>.cmd` (npm's default prefix) are tried. A `.cmd`/`.bat` shim is run as `cmd.exe /d /v:off /s /c ""<shim>" "<arg>"…"` with every argument quoted (Go's documented approach for batch files: the command line is built in `SysProcAttr.CmdLine`, since `os/exec` quotes for `CommandLineToArgvW`, not for `cmd.exe`); an argument with `"`, `%` or a line break is refused rather than escaped. A failed registration (`mcp_failed`) carries `manualCommand`: the exact command to paste, quoted for PowerShell on Windows (single-quoted paths, `'--'` quoted so PowerShell does not consume it when `claude` resolves to npm's `claude.ps1`) and for a POSIX shell elsewhere.
- **MCP scopes (Claude Code).** Claude Code has three scopes ([docs](https://code.claude.com/docs/en/mcp), "MCP installation scopes"): *user* (top-level `mcpServers` in `~/.claude.json`), *local* (`projects["<abs path>"].mcpServers` in the same file, the default of `claude mcp add`) and *project* (`<project>/.mcp.json`). Local wins over project over user, so a stale local `wspace` silently shadows the user one and `claude mcp list` warns that the server "is defined in multiple scopes". Status reads user and every local entry and reports them as `mcpRegistrations` (`scope`, `project`, `command`, `stale`), with `mcpDuplicate` (more than one registration) and `mcpStale` (a command that does not exist or is another wspace than this one: neither the running engine, `~/.local/bin/wspace`, nor an engine inside an installed app bundle). Project-scope `.mcp.json` files are **not** scanned: finding them would mean walking the disk. `mcp` stays the user-scope state. `install --mcp` never adds when a user-scope registration exists; when only local ones exist it adds the user one and reports each local one (`mcp_local_duplicate`). Other agents report their single file as `user` scope.
- **Cleanup (`mcp-clean`).** An explicit command rather than an install flag, because it removes registrations wspace may not have created. For this installation's server name only (`wspace`, or `wspace-dev` for a development build of a desktop app) it runs `claude mcp remove <name> -s local` with the working directory set to each project — `claude mcp remove` resolves the local scope from its working directory (normalized to the git root; verified with claude 2.1.273), so an old entry keyed by a subfolder of a git repository fails with "No MCP server named …" and is reported as `mcp_clean_failed`, as is a project folder that no longer exists. With a user-scope registration every local one is removed; without one, exactly one local registration is kept (a current one running this engine's command first). `--dry-run` reports `mcp_duplicate_would_remove`/`mcp_duplicate_kept` and runs nothing; a client should preview with it first.
- **Clients.** A desktop client can expose the same operations (status, install/update, uninstall, disable legacy skill, register MCP, clean up duplicates) through `agents.*`. It should confirm every change with the user, listing the exact paths or command.

### 6.2 Agents and verified locations

Checked 2026-10-03 against the official documentation (and `--help` of the installed CLIs).

| Agent (id) | Detected by | Skills folder | MCP | Source |
|---|---|---|---|---|
| Claude Code (`claude-code`) | `~/.claude`, `claude` | `~/.claude/skills` (symlinked skill folders supported) | `claude mcp add --scope user --transport stdio wspace -- <cmd> mcp serve`; state from `~/.claude.json` `mcpServers` (user) and `projects[*].mcpServers` (local); `claude mcp remove <name> -s local` run in the project removes a local one | code.claude.com/docs/en/skills, /mcp |
| Claude Desktop (`claude-desktop`, macOS) | `~/Library/Application Support/Claude` | none | snippet for `claude_desktop_config.json` `mcpServers` | Claude Desktop MCP documentation |
| Codex (`codex`) | `~/.codex`, `codex` | `~/.agents/skills` (symlinks followed) | `codex mcp add wspace -- <cmd> mcp serve`; state from `~/.codex/config.toml` `[mcp_servers.wspace]` | learn.chatgpt.com/docs/build-skills, /extend/mcp |
| Cursor (`cursor`) | `~/.cursor`, `cursor`/`cursor-agent` | `~/.cursor/skills` | snippet for `~/.cursor/mcp.json` | cursor.com/docs/skills, /context/mcp |
| Gemini CLI (`gemini`) | `~/.gemini`, `gemini` | `~/.gemini/skills` | `gemini mcp add --scope user wspace <cmd> mcp serve`; state from `~/.gemini/settings.json` | geminicli.com/docs/cli/skills, /tools/mcp-server |
| OpenCode (`opencode`) | `~/.config/opencode`, `opencode` | `~/.config/opencode/skills` | snippet for `~/.config/opencode/opencode.json` `mcp` (`opencode.jsonc` is reported `unknown`) | opencode.ai/docs/skills, /mcp-servers, /config |

**Unverified:** whether Cursor, Gemini CLI and OpenCode follow symlinked skill folders (their docs do not say; Claude Code and Codex document it). Cursor, Gemini and OpenCode also read `~/.agents/skills` and/or `~/.claude/skills`, so with several agents installed they may see the skill twice (Claude Code de-duplicates links to the same target; the others are not documented). Cursor walks skill roots recursively, which may include `.wspace-backup-*` folders.

