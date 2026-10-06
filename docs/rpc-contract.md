# Engine RPC Contract (`wspace rpc`, protocol v1)

> Normative. This is the contract between the `wspace` engine and any client that drives it over `wspace rpc`, such as a desktop app. Golden file: `internal/rpc/testdata/session.golden`. Code: `internal/rpc` (wire) over `internal/engine` (behavior).

## Contents

1. [Framing](#1-framing)
2. [Requests, responses, events](#2-requests-responses-events)
3. [Errors](#3-errors)
4. [Methods](#4-methods)
5. [Progress events](#5-progress-events)
6. [Versioning policy](#6-versioning-policy)
7. [`--json` on CLI read commands](#7---json-on-cli-read-commands)

## 1. Framing

| Rule | Detail |
|---|---|
| Transport | The client starts `wspace rpc` as a child process and talks over its stdin/stdout. |
| Lines | One JSON object per line, UTF-8, terminated by `\n`. Blank lines are ignored. A line may be up to 16 MiB. |
| stdout | Carries protocol lines only. Nothing else is ever written there. |
| stderr | Free-form diagnostics. Never parse it. |
| Order | Requests are processed **sequentially**, in arrival order. The server reads the next line only after it has written the final response to the current one. A client may pipeline requests; responses come back in the same order. |
| Lifetime | The server exits with status 0 when stdin reaches EOF. |
| State | The server holds no state between requests. The YAML config under `WSPACE_CONFIG_HOME` (default `~/.config/wspace`) is the only source of truth, so the CLI and a client can change it concurrently. |
| Context resolution | Every `context` parameter is optional. When omitted, the active context is used; when there is no active context and exactly one context exists, that one is used and becomes active. `WSPACE_CONTEXT` and `.ws.yaml` directory overlays are **not** applied: they belong to a terminal session's working directory. |

## 2. Requests, responses, events

Request:

```json
{"id":"42","method":"workspaces.status","params":{"workspace":"feat"}}
```

- `id`: required, a non-empty JSON **string**. The client chooses it; the server echoes it.
- `method`: required.
- `params`: an object. It may be omitted or `null` for methods without required parameters. Unknown fields are rejected with `invalid_params`, so a typo never silently becomes "not given".

Success:

```json
{"id":"42","result":{...}}
```

A method with nothing to return answers `{"id":"7","result":{}}`. A list result is always an array, never `null`.

Failure:

```json
{"id":"42","error":{"code":"not_found","message":"workspace not found: /Users/me/demo/workspaces/ghost","data":{"domainCode":"workspace_not_found","subject":"/Users/me/demo/workspaces/ghost"}}}
```

When the id cannot be read (invalid JSON, missing or non-string id), the response has `"id":null`.

Event (server-initiated, always before the final response of the same request):

```json
{"event":"progress","id":"4","data":{"kind":"repo","op":"workspace.create","repo":"api","phase":"started"}}
```

A client tells the three line kinds apart by `event` (events) and `result`/`error` (responses).

## 3. Errors

`code` is stable and is what a client switches on. `message` is human text from the engine's message catalog and may change. `data` is optional machine-readable detail.

| Code | Meaning | `data` |
|---|---|---|
| `invalid_request` | The line is not a valid request (bad JSON, missing/non-string id, missing method). | — |
| `method_not_found` | Unknown method. | — |
| `invalid_params` | A parameter is missing, malformed, or fails validation (bad context name, relative path, invalid branch name, path traversal in a workspace name, malformed ignore pattern, a directory that is not a main clone, a value an import needed but did not get). | `param` (bad parameter), `field` (missing import value), `domainCode`, `subject` |
| `not_found` | A context, workspace, repo, project or legacy config file does not exist; or no context could be resolved. | `domainCode`, `subject` |
| `already_exists` | A context, workspace, project or worktree with that name already exists. | `domainCode`, `subject` |
| `conflict` | The operation is refused in the current state: the branch is checked out elsewhere, the context is active, the server's working directory is inside the workspace, or an update, pull or stash apply found uncommitted changes, a detached HEAD, an operation in progress, a conflict, no upstream, a diverged branch or a shifted stash list. | `domainCode`, `subject`, `files`/`paths`/`restored` (updates), `ahead`/`behind` (diverged pull), `files`/`conflicts` (stash apply) |
| `needs_confirmation` | A destructive operation would discard work. Nothing was changed. Retry with `force: true` (or `confirm: true` for `repos.stashDrop`, `repos.discardUntracked` and `repos.discard`) only after the user agrees. | `reasons` (see below); `stash`/`files`, `paths` or `files` for those three |
| `git_failed` | A git command failed, git is missing or too old, or it timed out. | `domainCode`, `subject` |
| `internal` | Anything else. | — |

`domainCode` is the engine's finer code (for example `context_not_found`, `workspace_exists`, `branch_checked_out`, `not_a_main_clone`). Raw git output is never included.

`needs_confirmation` reasons:

```json
{"id":"6","error":{"code":"needs_confirmation","message":"destroying workspace feat would discard work (1 blocking item(s)); review the reasons and retry with force to proceed","data":{"reasons":[{"repo":"api","kind":"unpushed_commits","count":2,"message":"api: 2 unpushed commit(s)"}]}}}
```

| `kind` | Extra field | Meaning |
|---|---|---|
| `tracked_change` | `path` | A modified, staged or deleted tracked file. |
| `foreign_file` | `path` | An untracked file wspace did not create. Env files wspace copied never block. |
| `unpushed_commits` | `count` | Commits not on the upstream (or the base, without an upstream). |

## 4. Methods

Paths in the examples are illustrative. Fields marked optional may be omitted.

### 4.1 Protocol and engine

| Method | Params | Result |
|---|---|---|
| `rpc.hello` | — | `{"protocolVersion":1,"engineVersion":"<build version>","methods":[...sorted names...]}` |
| `engine.version` | — | `{"version":"dev"}` |
| `engine.info` | `context?` | Config directory and the context's resolved options with the layer that won each one. |
| `engine.doctor` | `context?` | `{"gitTooOld":false,"prunedWorktrees":0}`. Findings are also streamed as `warn` events. It only prunes stale worktree registrations. |
| `engine.checkUpdate` | — | `{"currentVersion":"dev","available":false,"unavailable":true}` (`latestTag` when known). Never fails on network problems; reports `unavailable`. |

Call the handshake first and compare `protocolVersion` with the version the client was built for. A mismatch means the client and the engine are out of sync.

```json
{"id":"8","result":{"configDir":"/Users/me/.config/wspace","contextName":"demo","options":{"baseBranch":{"value":"develop","from":"builtin"},"branchPrefix":{"value":"","from":"builtin"},"copyEnv":{"value":true,"from":"builtin"},"envPruneDirs":{"value":["node_modules","vendor","dist","target",".git"],"from":"builtin"},"fetchBeforeCreate":{"value":true,"from":"builtin"},"remote":{"value":"origin","from":"builtin"}}}}
```

`from` is one of `flag`, `project`, `workspace`, `overlay`, `context`, `builtin`.

### 4.1.1 MCP sessions (additive, v1)

| Method | Params | Result |
|---|---|---|
| `mcp.sessions` | — | `[MCPSession]`, oldest first. Empty when no agent is connected. |

Each running `wspace mcp serve` process ([mcp.md §4](mcp.md#4-presence)) keeps a presence record. `mcp.sessions` returns the records whose process is still alive. A record whose process is gone (killed without cleanup) is left out and deleted. On darwin and linux, liveness is `kill(pid, 0)`; on Windows it is best effort (`OpenProcess` succeeds).

```json
{"id":"8","method":"mcp.sessions"}
{"id":"8","result":[{"pid":4242,"clientName":"claude-code","clientVersion":"2.1.0","startedAt":"2026-10-03T09:00:00Z","lastActivityAt":"2026-10-03T09:01:00Z","lastTool":"list_workspaces","lastToolAt":"2026-10-03T09:01:00Z"}]}
```

`MCPSession`: `{"pid","clientName","clientVersion"?,"startedAt","lastActivityAt","lastTool"?,"lastToolAt"?}`. Times are RFC 3339, UTC. `clientName`/`clientVersion` are what the agent sent as `clientInfo`. `lastTool`/`lastToolAt` are absent until the session's first tool call. An engine older than this method answers `method_not_found`; a client treats that as "no sessions".

### 4.1.2 Agent skills (additive, v1)

| Method | Params | Result |
|---|---|---|
| `agents.status` | `agents?` (ids) | `{"skill":SkillSource,"agents":[Agent]}`. Never writes. |
| `agents.install` | `agents?`, `skill?` (default `true`), `mcp?`, `disableLegacy?`, `force?` | `{"skill","changes":[AgentChange],"agents":[Agent]}` |
| `agents.uninstall` | `agents?` | Same shape as `agents.install`. |
| `agents.mcpClean` | `agents?`, `dryRun?` | Same shape as `agents.install`. Removes duplicate local-scope Claude Code registrations of this installation's server ([mcp.md §6.1](mcp.md#61-installing)). |

Agent ids: `claude-code`, `claude-desktop` (darwin only), `codex`, `cursor`, `gemini`, `opencode`. An unknown id is `invalid_params` (`domainCode` `unknown_agent`). Without `agents`, `agents.install` acts on every *detected* agent (its configuration folder exists or its command is on PATH) and `agents.uninstall` on every agent; a named agent is installed even when not detected. `skill: false` skips the link and runs only the legacy and MCP steps (for example a client's "Disable Legacy Skill" and "Register MCP" actions).

`SkillSource`: `{"name","dir","origin","location","mcpServer","mcpCommand","mcpConfigHome"?}`. `origin` is `bundle` (the engine runs from `<X>.app/Contents/Helpers/` and the bundle has `Contents/Resources/skills/<name>/SKILL.md`) or `embedded` (the skill compiled into the binary, extracted to `$XDG_DATA_HOME/wspace/skills` — default `~/.local/share/wspace/skills` — or `%LOCALAPPDATA%\wspace\skills`). `location` is `stable`, `disk_image` (`/Volumes/…`) or `translocated` (App Translocation); `agents.install` with the skill step refuses anything but `stable` with `conflict` (`domainCode` `skill_source_unstable`). `name`/`mcpServer` are `wspace-workspaces`/`wspace`, or `wspace-dev-workspaces`/`wspace-dev` for a development build of a desktop app, whose `mcpConfigHome` carries its `WSPACE_CONFIG_HOME`.

`Agent`: `{"id","name","detected","skillsDir"?,"skillPath"?,"skill","skillTarget"?,"legacyPath"?,"mcp"?,"mcpMethod"?,"mcpConfigPath"?,"mcpSnippet"?}`. `skill` is `installed` (a link to `dir`), `outdated` (a wspace link to another bundle or extraction), `missing`, `conflict` (anything wspace did not create) or `unsupported` (no skills folder). `mcp` is `registered`, `not_registered` or `unknown` (the configuration file could not be parsed), read from the agent's configuration file — for Claude Code, the *user* scope only; `mcpMethod` is `cli` or `file`. Additive: `"mcpRegistrations"?:[{"scope","project"?,"command"?,"stale"?}]` lists every registration of this installation's server (`scope` `user`, or `local` with its `project` path — Claude Code's `~/.claude.json` `projects[*].mcpServers`; project-scope `.mcp.json` files are not scanned), `"mcpDuplicate"?` is `true` when there is more than one, `"mcpStale"?` when a `command` does not exist or is another wspace than this one.

`AgentChange`: `{"agent"?,"kind","path"?,"target"?,"backup"?,"detail"?,"manualCommand"?,"message"}`. On `mcp_failed`, `manualCommand` is the exact registration command for the user to run (PowerShell quoting on Windows: single-quoted paths, a quoted `'--'`). `kind`: `linked`, `copied`, `refreshed`, `unchanged`, `replaced`, `conflict`, `legacy_found`, `legacy_disabled`, `removed`, `kept`, `mcp_registered`, `mcp_already_registered`, `mcp_failed`, `mcp_manual` (`detail` is the snippet to add by hand), `mcp_unknown`, `mcp_local_duplicate` (`path` a project with a local registration, `target` its command), `mcp_duplicate_removed`, `mcp_duplicate_would_remove`, `mcp_duplicate_kept`, `mcp_clean_failed` (`agents.mcpClean`, same `path`/`target`; `detail` the reason), `failed`, `no_agents`. Per-agent problems are changes, not errors. Golden file: `internal/rpc/testdata/agents.golden`.

### 4.2 Contexts

| Method | Params | Result |
|---|---|---|
| `contexts.list` | — | `[ContextSummary]` |
| `contexts.get` | `name?` (omit for the active one) | `Context` |
| `contexts.create` | `name`, `workspacesRoot`, `projectsRoot?`, `ignorePatterns?`, `includePatterns?`, `projectScanMaxDepth?`, `defaults?` (Options), `activate?` | `Context` |
| `contexts.update` | `name`, and any of `newName`, `workspacesRoot`, `projectsRoot`, `ignorePatterns`, `includePatterns`, `projectScanMaxDepth`, `defaults` | `Context` |
| `contexts.delete` | `name`, `allowActive?` (additive, v1) | `{}`. Refused with `conflict` for the active context unless `allowActive: true`, which deletes it (even the only one) and clears the active context; clients that want another context active switch to it first. Never touches disk outside the config. |
| `contexts.switch` | `name` | `ContextSummary` |
| `contexts.importLegacy` | `from?`, exactly one of `name` (new context) or `into` (merge), `confirm?` | `ImportLegacyResult` |

`contexts.update` replaces each given field. `newName` renames the context, re-points the active context if needed, and refuses (`already_exists`) to overwrite another context. Projects are never changed here; use `projects.*`.

A rename also moves the context's workspaces to the new name: after the renamed context is saved, every manifest under the old and the new `workspacesRoot` whose owner (`workspace.context`) is the old name is rewritten to the new name (atomically, one file at a time; only that field changes; legacy `.ws/` files are never touched). The result `Context` then carries (additive, v1; omitted when empty):
- `reassignedWorkspaces`: the names of the workspaces moved;
- `reassignFailures`: `[{"name"?,"root","message"}]`, each workspace whose manifest could not be rewritten. The rename is never rolled back because of one; such a workspace is listed as an orphan (`orphanOf`) and can be recovered with `workspaces.claim`.

`contexts.delete` never rewrites manifests either: the deleted context's workspaces become **orphans** (see `orphanOf` in §4.4) for any other context sharing the `workspacesRoot`, which can take them over with `workspaces.claim`.

`contexts.importLegacy`:
- `from` forces a source file. Without it, the global legacy file and a folder-level `ws.config` above the server's working directory are discovered; finding both is `invalid_params`.
- With `into`, `confirm: false` (the default) is a **dry run**: the result describes the change (`fieldChanges`, `importedProjects`, `projectsKept`) with `applied: false`, and nothing is written. Send it again with `confirm: true` to apply.
- A required value missing from the source (for example `workspaces_root`) fails with `invalid_params` and `data.field`.
- Once the context is written (a new context, or a merge with `confirm: true`), every orphaned workspace under its `workspacesRoot` (its owner context no longer exists) is first claimed for it exactly as `workspaces.claim` without `workspaces` does; then the legacy workspaces there are adopted exactly as `workspaces.adoptLegacy` does (§4.4). The outcome is in `claimedWorkspaces` (`[ClaimedWorkspace]`), `adoptedWorkspaces`/`skippedWorkspaces` (all additive, v1) and in `summary`. A dry run claims and adopts nothing and returns all three empty. A claim or adoption problem never fails the import.

```json
{"id":"1","method":"contexts.create","params":{"name":"demo","workspacesRoot":"/Users/me/demo/workspaces","projectsRoot":"/Users/me/demo/projects","activate":true}}
{"id":"1","result":{"name":"demo","active":true,"workspacesRoot":"/Users/me/demo/workspaces","projectsRoot":"/Users/me/demo/projects","ignorePatterns":[],"defaults":{},"projects":[]}}
```

`ContextSummary`: `{"name","active","workspacesRoot","projectsRoot"?, "projectCount"}`.

`Context`: `{"name","active","workspacesRoot","projectsRoot"?, "ignorePatterns":[], "includePatterns":[], "projectScanMaxDepth"?, "defaults":Options, "projects":[Project], "reassignedWorkspaces"?, "reassignFailures"?}`. The last two are set by `contexts.update` only (see §4.2).

`includePatterns` (added in v1, additive) limits project discovery to repositories whose folder name **or** root-relative path matches one of the `path.Match` globs (`api*`, `team-a/*`); `[]` offers every repository. It is stored as `include_patterns` in the context YAML. A malformed pattern is `invalid_params` (`param: "includePatterns"`).

`Options` (every field optional; absent means "unset at this layer"): `{"baseBranch","branchPrefix","copyEnv","fetchBeforeCreate","envPruneDirs","remote"}`. `envPruneDirs: []` means "prune nothing", which differs from absent.

`branchPrefix` is normalized before use: surrounding spaces and leading `/` are dropped. A prefix ending in `/`, a letter or a digit is a branch namespace joined with exactly one `/`, so `feature`, `feature/` and `feature//` all give `feature/<name>`. A prefix ending in any other character is a literal kept as written, so `alice-` gives `alice-<name>`. `""` gives `<name>`. The same normalized value (with its trailing `/`) is what `{prefix}` expands to in `destBranch`/`worktreeDir` templates. The CLI and the RPC share this rule.

`ImportLegacyResult`: `{"sourcePath","context","created","applied","importedProjects":[],"skippedProjects":[{"name","reason","message"}],"unmappedKeys":[{"key","reason","detail"?}],"fieldIssues":[...],"fieldChanges":[{"field","current","incoming"}],"projectsKept":[],"summary":["human lines"],"adoptedWorkspaces":[],"skippedWorkspaces":[],"claimedWorkspaces":[]}`.

### 4.3 Projects

| Method | Params | Result |
|---|---|---|
| `projects.scan` | `context?`, `roots?` (absolute paths; default the context's `projectsRoot`), `depth?` (default the context's, then 5), `ignore?` (default the context's `ignorePatterns`; `[]` ignores nothing), `include?` (default the context's `includePatterns`; `[]` includes everything) | `ScanResult` |
| `projects.list` | `context?` | `[Project]` |
| `projects.register` | `context?`, `key`, `sourceDir`, `originBranch?`, `destBranch?`, `worktreeDir?` | `Project` |
| `projects.registerMany` | `context?`, `projects: [{"key","sourceDir"}]` | `RegisterManyResult` |
| `projects.update` | `context?`, `key`, any of `sourceDir`, `originBranch`, `destBranch`, `worktreeDir` (`""` restores inheritance) | `Project` |
| `projects.remove` | `context?`, `key` | `{}`. Never touches the repository or existing workspaces. |

`projects.scan` only discovers; it registers nothing. Hidden directories and ignored names are pruned while walking, repositories are never descended into, and linked worktrees are reported but never offered. `suggestedKey` follows the wizard and is registrable as a set: the leaf name while it is unique among the candidates and the context's keys, **ignoring case** (a key is a worktree folder name; macOS file systems ignore case); otherwise the root-relative path with `/` replaced by `-`; otherwise that with `-2`, `-3`, … appended. Leading `.`/`-` are dropped. A registered candidate's `suggestedKey` is its registered key.

`truncated` is true when the depth cap stopped the walk with folders left unsearched. `truncatedDirs` (additive) names those folders (the folders at the cap whose subfolders were not searched, at most 20), `truncatedCount` is their full number and `maxDepth` the cap that was used, so a client can say exactly where the scan stopped and what to raise or ignore. A folder at the cap whose only subfolders are hidden or ignored does not truncate.

```json
{"id":"2","result":{"candidates":[{"path":"/Users/me/demo/projects/api","root":"/Users/me/demo/projects","relativePath":"api","suggestedKey":"api","registered":false}],"linkedWorktrees":[],"truncated":true,"truncatedDirs":["/Users/me/demo/projects/docs/a/b/c/d"],"truncatedCount":1,"maxDepth":5}}
```

`projects.registerMany` registers a batch **all-or-nothing**. Every project is validated first: the key (as for `projects.register`), keys unique within the batch and against the context ignoring case, source folders absolute, unique and not already registered, each a git main clone. When any project has a problem nothing is written and the result lists every problem; otherwise all are registered with one config write. Problems are a result, not an error: an operational failure (the config cannot be read or written) is still an error response.

`RegisterManyResult`: `{"registered":[Project], "problems":[{"index","key","sourceDir","reason","message"}]}`. Exactly one of the two lists is non-empty (both are empty for an empty batch). `reason` is one of `invalid_key`, `key_taken` (a project in the context has it), `key_duplicate` (an earlier entry of the batch has it), `source_invalid`, `source_registered`, `source_duplicate`, `not_a_main_clone`, `git_failed`; `message` names the project and says what to change.

`projects.register` validates the key (one path component, not starting with `.` or `-`), requires an absolute `sourceDir` that is a git main clone, validates the branch and the `{workspace}`/`{project}`/`{branch}`/`{prefix}` templates, and refuses a duplicate key or source directory (`already_exists`).

`Project`: `{"key","sourceDir","originBranch"?, "destBranch"?, "worktreeDir"?, "options":Options, "isNode"?, "hasNodeModules"?}`. `isNode` (a `package.json` at the main clone's root) and `hasNodeModules` (it also has a `node_modules` directory) are additive and set only by `projects.list`, `contexts.get` and `workspaces.addableProjects`; a UI offers `copyNodeModules` when a selected project has `hasNodeModules`.

### 4.4 Workspaces

| Method | Params | Result | Progress |
|---|---|---|---|
| `workspaces.list` | `context?` | `[WorkspaceStatus]` | — |
| `workspaces.status` | `context?`, `workspace` | `WorkspaceStatus` | — |
| `workspaces.create` | `context?`, `name`, `branch?`, `projects?` (keys; default all), `options?` (Options override; `options.branchPrefix` picks the branch type for this workspace, e.g. `fix`), `copyNodeModules?` (additive, default `false`) | `Workspace` | yes |
| `workspaces.teardownCheck` | `context?`, `workspace` | `[Blocker]` (empty: an unforced destroy would proceed) | — |
| `workspaces.repoChanges` | `context?`, `workspace`, `repo` (alias) | `[FileChange]` | — |
| `workspaces.destroy` | `context?`, `workspace`, `force?`, `deleteBranches?` | `{"path":...}` | yes |
| `workspaces.addRepo` | `context?`, `workspace`, `project`, `options?`, `copyNodeModules?` (additive, default `false`) | `RepoEntry` | yes |
| `workspaces.addableProjects` (additive, v1) | `context?`, `workspace` | `[Project]` | — |
| `workspaces.removeRepo` | `context?`, `workspace`, `repo` (alias), `force?`, `deleteBranch?` | `{"alias":...}` | yes |
| `workspaces.repair` | `context?`, `workspace` | `{"recreated":[aliases]}` | yes |
| `workspaces.syncEnv` | `context?`, `workspace` | `{"copied":["alias/path"]}` | yes |
| `workspaces.adoptLegacy` (additive, v1) | `context?` | `{"adopted":[AdoptedWorkspace],"skipped":[SkippedWorkspace]}` | — |
| `workspaces.claim` (additive, v1) | `context?`, `workspaces?` (folder names; omit for every orphan) | `{"claimed":[ClaimedWorkspace],"skipped":[ClaimSkipped]}` | — |
| `workspaces.updateRepo` (additive, v1) | `context?`, `workspace`, `repo` (alias), `strategy?` (`merge` default, or `rebase`), `autostash?` | `RepoUpdate` | yes |
| `workspaces.update` (additive, v1) | `context?`, `workspace`, `strategy?`, `autostash?` | `{"repos":[RepoUpdate]}` | yes |

Rules:
- `workspace` and `name` are one directory name under the context's `workspacesRoot`. `/`, `\`, a leading `.` or `-` are rejected, so a name can never escape the root.
- `workspaces.create` validates every project before touching any. If a later step fails, it **rolls back** what this call created: each new worktree is removed, each branch this call created is deleted, and the new workspace directory is removed. Branches that already existed are kept. A rollback step that fails is reported as a `warn` event, and the directory is then left for inspection. The original error is returned.
- `workspaces.addRepo` rolls back the same way, scoped to the one repo; the workspace and its manifest are unchanged. A project already in the workspace is refused with `already_exists` (`domainCode` `already_in_workspace`), changing nothing.
- `copyNodeModules: true` copies `node_modules` from each selected Node project's main clone (a `package.json` and a `node_modules` directory at its root) into the new worktree, after the env files, so dependencies need no reinstall. It is copy-on-write where the filesystem allows: one `clonefile(2)` of the whole tree on macOS/APFS (no extra disk space), the `FICLONE` ioctl per file on Linux (Btrfs, XFS), a regular copy otherwise and on Windows. Symbolic links inside (pnpm) are recreated, never followed; permission bits (executables in `.bin`) are kept. The repo's events include a `step` (`node_modules.copying`) and an `info` (`node_modules.copied`, naming the method). A worktree that already has `node_modules` is left alone (`info` `node_modules.already_present`). A failed copy is a `warn` (`node_modules.copy_failed`), not a failed call: the partial copy is removed and the user installs dependencies as usual. The copy lives inside the worktree, so a rollback removes it. An ignored `node_modules` (the usual `.gitignore`) is never a "foreign" change for `workspaces.destroy` (git status leaves ignored files out); when the worktree does not ignore it, a `warn` (`node_modules.not_ignored`) says destroy will report its files as untracked. A lockfile changed on the new branch may still need an install.
- `workspaces.addableProjects` lists, in the context's order, the projects `workspaces.addRepo` accepts: every registered project **not** already in the workspace. A project is "already in" it when a repo of the manifest has the same project key, the same main clone (`sourceDir`, compared after path cleaning; an adopted legacy workspace may record another key for the same clone) or an alias equal to the project key (the worktree directory is taken). An empty list means every project is already there.
- `workspaces.adoptLegacy` gives each workspace the legacy bash `ws` tool created in the context's `workspacesRoot` (a folder with `.ws/workspace.conf` and no `.wspace/workspace.yaml`) a wspace manifest recorded under that context. It only writes `.wspace/workspace.yaml`; `.ws/` is never changed or removed, so the legacy tool keeps working on the same workspace. It is idempotent: a legacy folder that already has a manifest is never re-adopted; it is reported in `skipped` with its ownership (`adopt.skip.already_adopted`, `adopt.skip.owned_by_other`), and when its owner context no longer exists it is claimed for this context (`adopt.skip.orphan_claimed`). A workspace whose data cannot be resolved is left alone and listed in `skipped`; it never fails the call. Only an unreadable `workspacesRoot` is an error; a missing one adopts nothing.
- `workspaces.claim` makes the context the owner of **orphaned** workspaces: those whose manifest names a context that no longer exists (it was deleted, or renamed by an engine older than this rule). Only the manifest's `workspace.context` is rewritten (atomically). It never takes a workspace owned by another existing context, and never changes a shared one (a manifest naming no context). Without `workspaces` it claims every orphan under the `workspacesRoot` and reports nothing else; with `workspaces`, every name not claimed is listed in `skipped` with a reason. Idempotent: a second call has nothing left to claim. Only an unreadable context registry or `workspacesRoot` is an error.
- `workspaces.destroy` and `workspaces.removeRepo` without `force` return `needs_confirmation` with every reason and change nothing. With `force` they remove the work. Branches are kept unless `deleteBranches`/`deleteBranch` is set.

```json
{"id":"4","method":"workspaces.create","params":{"name":"feat","branch":"feat-x"}}
{"event":"progress","id":"4","data":{"kind":"step","key":"workspace.create_project","message":"Creating worktree for api…"}}
{"event":"progress","id":"4","data":{"kind":"repo","op":"workspace.create","repo":"api","phase":"started"}}
{"event":"progress","id":"4","data":{"kind":"repo","op":"workspace.create","repo":"api","phase":"finished"}}
{"id":"4","result":{"name":"feat","path":"/Users/me/demo/workspaces/feat","context":"demo","branch":"feat-x","created":"2026-10-03T02:46:46Z","repos":[{"alias":"api","project":"api","sourceDir":"/Users/me/demo/projects/api","branch":"feat-x"}]}}
```

```json
{"id":"5","method":"workspaces.status","params":{"workspace":"feat"}}
{"id":"5","result":{"name":"feat","path":"/Users/me/demo/workspaces/feat","repoCount":1,"dirty":false,"repos":[{"alias":"api","project":"api","branch":"feat-x","detached":false,"ahead":0,"behind":0,"dirty":false,"changes":[]}]}}
```

`WorkspaceStatus`: `{"name","path","repoCount","dirty","repos":[RepoStatus],"error"?,"legacy"?,"orphanOf"?}`. In `workspaces.list`, a workspace that cannot be read is listed with `error` (an error object as in §3) and empty `repos`; the others are still returned. `legacy: true` (additive, v1; absent means `false`) marks a folder only the legacy `ws` tool manages: it is listed, with empty `repos` and no live status, so a client can offer `workspaces.adoptLegacy`. A legacy manifest names no context, so such a folder is listed in every context that shares the `workspacesRoot`.

Ownership in `workspaces.list`: several contexts may share one `workspacesRoot`. A workspace is listed for a context when its manifest names that context, names no context (shared), or names a context that **no longer exists**. In the last case it carries `orphanOf` (additive, v1; absent when owned or shared): the removed context's name, so a client can offer `workspaces.claim`. A workspace whose manifest names another existing context is not listed.

`ClaimedWorkspace`: `{"name","root","previousContext"}`. `ClaimSkipped`: `{"name","root"?,"reason","message"}`, where `reason` is a stable code (clients must accept unknown values):

| `reason` | Meaning |
|---|---|
| `claim.skip.already_owned` | The workspace already belongs to this context. |
| `claim.skip.shared` | Its manifest names no context, so it already belongs to every context sharing the root. |
| `claim.skip.owned_by_other` | It belongs to another context that still exists; it is never taken. |
| `claim.skip.not_found` | No wspace workspace with that folder name in the `workspacesRoot`. |
| `claim.skip.invalid_name` | The name is not a valid workspace name. |
| `claim.skip.manifest_unreadable` | Its manifest could not be read; it is not changed. |
| `claim.skip.write_failed` | Its manifest could not be written. |
| `adopt.skip.scan_failed` | Import only: the `workspacesRoot` could not be scanned. |

```json
{"id":"7","method":"workspaces.claim","params":{"workspaces":["lost","feat"]}}
{"id":"7","result":{"claimed":[{"name":"lost","root":"/Users/me/demo/workspaces/lost","previousContext":"gone"}],"skipped":[{"name":"feat","root":"/Users/me/demo/workspaces/feat","reason":"claim.skip.already_owned","message":"already belongs to context demo"}]}}
```

`AdoptedWorkspace`: `{"name","root"}`. `SkippedWorkspace`: `{"root","reason","message"}`. `reason` is a stable code; clients must accept unknown values:

| `reason` | Meaning |
|---|---|
| `adopt.skip.unresolved_project` | A legacy repo's project is not registered in the context (matched by source folder `<projects_root>/<project>`, then by project key). |
| `adopt.skip.branch_unknown` | A repo's branch is neither in the legacy manifest nor readable from its worktree (missing or detached). |
| `adopt.skip.branch_ambiguous` | The legacy manifest names no workspace branch and the repos are on different branches. |
| `adopt.skip.no_repos` | The legacy manifest declares no repositories. |
| `adopt.skip.invalid_name` | The folder name is not a valid workspace name. |
| `adopt.skip.conf_unreadable` | `.ws/workspace.conf` could not be read. |
| `adopt.skip.manifest_unreadable` | A `.wspace/workspace.yaml` exists but could not be read; it is not overwritten. |
| `adopt.skip.write_failed` | The new manifest could not be written. |
| `adopt.skip.scan_failed` | Import only: the `workspacesRoot` could not be scanned (`root` is that folder). |
| `adopt.skip.already_adopted` | The folder already has a manifest owned by this context (or by none). |
| `adopt.skip.owned_by_other` | The folder already has a manifest owned by another existing context; it is left alone. |
| `adopt.skip.orphan_claimed` | The folder's manifest named a context that no longer exists; it was claimed for this context (`message` names the old one). |

```json
{"id":"6","method":"workspaces.adoptLegacy","params":{"context":"demo"}}
{"id":"6","result":{"adopted":[{"name":"old","root":"/Users/me/demo/workspaces/old"}],"skipped":[{"root":"/Users/me/demo/workspaces/stray","reason":"adopt.skip.unresolved_project","message":"project unknown is not registered in this context"}]}}
```

`RepoStatus`: `{"alias","project"?,"branch","detached","ahead","behind","dirty","changes":[{"path","class"}],"baseBranch"?,"baseMissing"?,"baseBehind"?}`, where `class` is `tracked`, `foreign` or `env_copy`. `project`, `baseBranch`, `baseMissing` and `baseBehind` are additive (v1): `project` is the context project key the repo was created from (use it with `projects.update`). `baseBehind` counts the comparison base's commits not yet in the branch, against the last fetch (what `workspaces.updateRepo` would integrate); it is present only when `baseBranch` was counted against, so it is absent (unknown) for a branch with an upstream, a missing base or a failed count.

`ahead`/`behind` compare against the branch's upstream when it has one; then `baseBranch` is absent. Without an upstream, `ahead` counts the commits not in the repo's **comparison base** (`behind` is 0) and `baseBranch` names that base. The base is resolved per repo, first match wins:

1. the project's `originBranch` (in the workspace's context);
2. the project's own `options.baseBranch`;
3. the base recorded for that repo in the workspace manifest (written by `workspaces.create` and `workspaces.adoptLegacy` only when it differs from the workspace base);
4. the workspace's `baseBranch`;
5. the remote's default branch (`<remote>/HEAD`), when the clone records it.

Each candidate matches as `<remote>/<b>` first, then as the local branch `<b>`. When none exists, the repo is reported with `baseMissing: true`, `ahead`/`behind` 0 (unknown) and `baseBranch` naming the first candidate. This never fails the workspace: its other repos and the workspace itself render normally. To fix it, set the project's `originBranch` with `projects.update`. A repo whose worktree is missing still fails the whole workspace (`error`), which `workspaces.repair` fixes.

`workspaces.create` picks each new worktree's start point with the same order (an explicit `options.baseBranch` overrides items 1–4). If no candidate exists it uses the remote's default branch, then the clone's current `HEAD`, and emits a `warn` event (`workspace.base_branch_fallback` or `workspace.base_branch_missing`).

`Workspace`: `{"name","path","context","branch","created"` (RFC 3339, UTC)`,"repos":[RepoEntry]}`. `RepoEntry`: `{"alias","project","sourceDir","branch"}`.

`Blocker`: `{"repo","kind","path"?,"count"?,"message"}` (§3).

### 4.4.1 Updating from the base (additive, v1)

`workspaces.updateRepo` brings one repo up to date with its comparison base; `workspaces.update` does it for every repo, one at a time, in manifest order. Per repo:

1. Refuse, changing nothing, when HEAD is detached (`detached_head`) or a merge or rebase is already in progress (`integration_in_progress`; the user's own operation is never aborted).
2. `git fetch --prune <remote>`.
3. Resolve the base with the same precedence as `RepoStatus` (list above). None found: `base_missing` (`base` names the expected branch).
4. Count the base's commits not in HEAD (`git rev-list --count HEAD..<base>`). Zero: success with `upToDate: true`, even when the worktree is dirty.
5. Uncommitted changes to tracked files (staged, unstaged or conflicted; untracked files do not count) without `autostash`: `worktree_dirty`, with `data.files`.
6. `git merge --no-edit [--autostash] <base>` (a fast-forward when possible) or `git rebase [--autostash] <base>`. `--autostash` is passed only when the worktree is dirty and `autostash` is set. Merge `--autostash` needs git 2.27; an older git gives `git_too_old` (subject `2.27 (merge --autostash)`) and changes nothing. Rebase `--autostash` predates the supported minimum.
7. A stop with conflicts is aborted (`git merge --abort` / `git rebase --abort`): the result is `update_conflict` with the unmerged paths (`git diff --name-only --diff-filter=U -z`) in `conflicts` and `data.paths`. When git re-applies an autostash after a successful integration and that conflicts, git keeps the stash; the engine then runs `git reset --hard <beforeHead>` and `git stash pop --index` and reports `update_conflict` too. `data.restored` is `true` only when HEAD, `git status` and the absence of an operation in progress were verified to match the state before the update. Git re-applies its own autostash without the index, so staged changes come back unstaged after an aborted autostash update: `restored` is then `false`, while the content is kept.

A repo is never left mid-merge or mid-rebase. `rebase` rewrites the branch's local commits; a branch already pushed then needs a force push. Only an unknown workspace or repo, or a bad parameter, fails the call; every per-repo outcome is in the result, and one repo's refusal never stops the others.

`RepoUpdate`: `{"repo","strategy","base","beforeHead","afterHead","upToDate","commitsIntegrated","conflicts":[],"error"?}`. `error` (an error object as in §3) is absent on success. `domainCode` values: `detached_head`, `integration_in_progress`, `update_conflict` (`conflict`); `worktree_dirty` (`conflict`, `data.files`); `base_missing` (`not_found`); `git_too_old`, `git_failed` (`git_failed`). Progress events: `op` `workspace.update`, phases `started`, then `finished` or `failed`.

```json
{"id":"9","method":"workspaces.updateRepo","params":{"workspace":"feat","repo":"api"}}
{"event":"progress","id":"9","data":{"kind":"repo","op":"workspace.update","repo":"api","phase":"started"}}
{"event":"progress","id":"9","data":{"kind":"repo","op":"workspace.update","repo":"api","phase":"failed","error":"worktree has uncommitted changes: api"}}
{"id":"9","result":{"repo":"api","strategy":"merge","base":"origin/develop","beforeHead":"1111111","afterHead":"1111111","upToDate":false,"commitsIntegrated":0,"conflicts":[],"error":{"code":"conflict","message":"worktree has uncommitted changes: api","data":{"domainCode":"worktree_dirty","files":["main.go"],"subject":"api"}}}}
```

`FileChange` (additive): `{"path","origPath"?,"status","staged","unstaged"}`, one per changed path of the repository's worktree in git's order, read with `git status --porcelain=v1 -z --untracked-files=all` (NUL-separated, so spaces, quotes and newlines in names survive). `status` is `modified`, `added`, `deleted`, `renamed`, `copied`, `typechange`, `untracked` or `conflicted` (clients must accept unknown values): the index side when the index holds a change, else the worktree side. `staged` means the index holds a change, `unstaged` that the worktree holds a further one. `origPath` is a rename's or copy's source. An alias the workspace does not hold is `not_found` with `domainCode` `repo_not_found`.

### 4.5 Repository inspector (additive, v1)

Read-only queries over one repo of a workspace, plus a fetch and a fast-forward-only pull. Every method takes `context?`, `workspace`, `repo` (alias); an unknown alias is `not_found` (`repo_not_found`). Nothing here stages, commits, stashes, merges with a merge commit or rebases.

| Method | Extra params | Result | Progress |
|---|---|---|---|
| `repos.inspect` | — | `RepoInspection` | — |
| `repos.changes` | — | `RepoChangeSets` | — |
| `repos.diff` | `path`, `staged?` | `FileDiff` | — |
| `repos.commits` | `range?` (`base` default, `upstream`; additive, v1), `offset?` (≥ 0), `limit?` (1–200, default 50) | `RepoCommits` | — |
| `repos.commit` | `hash` | `CommitDetail` | — |
| `repos.stashes` | — | `[Stash]` | — |
| `repos.stash` | `index` | `StashDetail` | — |
| `repos.fetch` | — | `{"repo","remote","branch":BranchInfo}` | yes (`op` `repo.fetch`) |
| `repos.pull` | — | `RepoPull` | yes (`op` `repo.pull`) |

Shapes:

- `BranchInfo`: `{"repo","branch","detached","head","upstream"?:{"ref","remote","gone","ahead","behind"},"baseBranch"?,"baseRef"?,"baseFound","baseAhead"?,"baseBehind"?,"lastFetch"?}`. `upstream` is absent without one; `gone` means it is configured but its ref no longer exists. The base is the comparison base `RepoStatus` uses (same precedence, §4.4); `baseAhead`/`baseBehind` count `git rev-list --count <base>..HEAD` and `HEAD..<base>` and are absent when unknown. Counts are against the last fetch. `lastFetch` (RFC 3339, UTC) is the newer modification time of the worktree's and the main clone's `FETCH_HEAD`; absent when never fetched.
- `RepoInspection`: `{"branch":BranchInfo,"staged","unstaged","untracked","conflicted","stashes"}` (counts).
- `RepoChangeSets`: `{"staged":[ChangeEntry],"unstaged":[…],"untracked":[…],"conflicted":[…]}`, `ChangeEntry` `{"path","origPath"?,"status"}` (`status` as in `FileChange`). Read from `git status --porcelain=v1 -z --untracked-files=all`; a path with staged and unstaged changes is listed on both sides; `unstaged` statuses come from the worktree column.
- `FileDiff`: `{"path","origPath"?,"status","binary","truncated","additions","deletions","hunks":[{"header","oldStart","oldLines","newStart","newLines","lines":[]}]}`. Each line keeps its one-character prefix: `' '` context, `+` added, `-` removed, `\` a "No newline at end of file" marker. A binary file has `binary: true` and no hunks.
- `RepoCommits`: `{"range","base"?,"upstream"?,"total","offset","hasMore","commits":[Commit]}`. With `range` `base` (the default) the listed range is `<base>..HEAD` (the branch's own commits since it left its base), or `HEAD` when no base exists. With `range` `upstream` it is `<upstream>..HEAD` (the commits a push would send; `upstream` is set, `base` absent); a branch without an upstream, or whose upstream is gone, is `conflict` (`no_upstream`). The two can differ: a branch that took commits from its base (update from base) after it was pushed is ahead of its upstream with no commits since the base. `Commit`: `{"hash","shortHash","subject","author","date"}`.
- `CommitDetail`: `Commit` plus `{"body","parents":[],"files":[FileDiff],"truncated"}`; the diff is against the first parent (the empty tree for a root commit).
- `Stash`: `{"index","ref","hash","branch"?,"message","date"}`; `branch`/`message` are split from the reflog subject (`On <branch>: <message>`, `WIP on <branch>: …`). `StashDetail`: `{"stash":Stash,"files":[FileDiff],"truncated","includesUntracked"}`.
- `RepoPull`: `{"repo","upstream","beforeHead","afterHead","upToDate","commitsPulled","ahead","behind","error"?}`.

Rules:

- **Privacy.** Only the author **name** is read (`%an`); e-mail addresses are never requested from git, so they never reach any surface.
- **Diff safety.** `repos.diff` only accepts a `path` the repo's status lists on the requested side (`staged` → index side; otherwise worktree side, untracked and conflicted included); anything else is `not_found` (`path_not_changed`), so no file outside the changes can be read. Paths are passed as literal pathspecs (`:(literal)…`).
- **Caps.** Every diff (file, commit, stash) is capped at 1 MiB of git output and 5000 hunk lines. Past the cap git is stopped, the partial last line is dropped and `truncated` is `true` on the file and the result; files after the cut are absent.
- **Deterministic, side-effect-free diffs.** All diffs run with `--no-color --no-ext-diff --no-textconv -M --src-prefix=a/ --dst-prefix=b/`. External diff drivers and textconv filters are disabled because both run user-configured programs; such files show their raw content or as binary. Names are read from the patch headers and C-unquoted, so spaces, quotes and newlines survive.
- **Timeouts.** Queries time out after 30 s, fetch and pull after 2 min (`git_failed`, `domainCode` `timeout`).
- `repos.commit` accepts only a hexadecimal hash (4–64 digits); a revision expression is `invalid_params`, an unknown hash `not_found` (`ref_not_found`). `repos.stash` with an index not in the list is `not_found` (`ref_not_found`). Untracked files of a stash need git 2.32 (`git stash show --include-untracked`); with an older git `includesUntracked` is `false` and only tracked changes are shown.

Git commands (all through the adapter's fixed `-C <repo>` prefix and sanitized environment):

| Query | Command |
|---|---|
| changes | `status --porcelain=v1 -z --untracked-files=all` |
| staged / unstaged diff | `diff [--cached] <flags> -- :(literal)<path> [:(literal)<origPath>]` |
| untracked diff | `diff --no-index <flags> -- /dev/null <path>` (exit 1 = differences) |
| commits | `rev-list --count <range>`; `log -z --no-show-signature --format=%H%x1f%h%x1f%an%x1f%aI%x1f%s --skip=N --max-count=M <range>` |
| commit | `rev-parse --verify --quiet <hash>^{commit}`; `show -s --format=%H%x1f%h%x1f%an%x1f%aI%x1f%P%x1f%B`; `diff <flags> <parent1> <hash>` or `diff-tree -p -r --root --no-commit-id <flags> <hash>` |
| stashes | `stash list -z --format=%gd%x1f%H%x1f%gs%x1f%aI`; `stash show -p [--include-untracked] <flags> stash@{N}` |
| upstream | `symbolic-ref -q HEAD`; `for-each-ref --format=%(upstream)%1f%(upstream:short)%1f%(upstream:remotename)%1f%(upstream:track,nobracket) <ref>`; `rev-list --left-right --count @{upstream}...HEAD` |
| last fetch | `rev-parse --git-path FETCH_HEAD --git-common-dir`, then a file stat |
| fetch | `fetch --prune --quiet -- <remote>` (the repo's configured remote, plus the upstream's remote when different) |
| pull | `fetch` of the upstream's remote, then `merge --ff-only --no-edit <upstream>` |

`repos.pull` per repo: refuse, changing nothing, a detached HEAD (`detached_head`), a merge or rebase in progress (`integration_in_progress`), no upstream or a gone one (`no_upstream`); fetch; nothing behind → `upToDate: true`; local commits the upstream lacks → `diverged` (`conflict`, `data.ahead`/`data.behind`; integrate with `workspaces.updateRepo` or by hand); otherwise `git merge --ff-only`. When git refuses because local changes or untracked files would be overwritten the result is `worktree_dirty` with the paths git listed in `data.files`. It never creates a merge commit. Like `workspaces.updateRepo`, refusals are in the result's `error`, not a failed call.

```json
{"id":"10","method":"repos.pull","params":{"workspace":"feat","repo":"api"}}
{"event":"progress","id":"10","data":{"kind":"repo","op":"repo.pull","repo":"api","phase":"started"}}
{"event":"progress","id":"10","data":{"kind":"repo","op":"repo.pull","repo":"api","phase":"failed","error":"the branch and its upstream have diverged: a fast-forward pull is not possible: origin/feat"}}
{"id":"10","result":{"repo":"api","upstream":"origin/feat","beforeHead":"1111111","afterHead":"1111111","upToDate":false,"commitsPulled":0,"ahead":1,"behind":2,"error":{"code":"conflict","message":"the branch and its upstream have diverged: a fast-forward pull is not possible: origin/feat","data":{"ahead":1,"behind":2,"domainCode":"diverged","subject":"origin/feat"}}}}
```

The full wire examples are pinned in `internal/rpc/testdata/repos.golden`.

### 4.6 Repository actions (additive, v1)

Local actions on one repo of a workspace: apply, pop or drop a stash entry, and delete untracked files. Every method takes `context?`, `workspace`, `repo` (alias). None of them ever resets the worktree or discards a tracked change.

| Method | Extra params | Result |
|---|---|---|
| `repos.stashApply` | `index`, `hash` | `StashApplyResult` |
| `repos.stashPop` | `index`, `hash` | `StashApplyResult` |
| `repos.stashDrop` | `index`, `hash`, `confirm?` | `{"repo","stash":Stash}` |
| `repos.validateUntracked` | `paths` | `{"repo","worktree","paths":[{"path","absolutePath"}]}` |
| `repos.discardUntracked` | `paths`, `confirm?` | `{"repo","removed":[],"kept":[]}` |

`StashApplyResult`: `{"repo","stash"?:Stash,"indexRestored","dropped","conflicts":[],"warnings":[],"error"?}`. Like `repos.pull`, refusals and conflicts are in the result's `error`, not a failed call; only an unknown workspace or repo, or a bad parameter, fails the call.

Stash rules:

- **Pinned entry.** `hash` is required (4–64 hex digits, as `repos.stashes` lists it; a prefix is accepted). If `stash@{index}` no longer has that hash (the list shifted, or the entry is gone), the action is refused with `conflict` (`domainCode` `stash_changed`) and nothing changes. Apply and pop then run `git stash apply` on the **commit**, not on `stash@{N}`, so they act on exactly the verified entry.
- **Refused up front, changing nothing:** a merge or rebase in progress (`integration_in_progress`); unresolved conflicts already in the worktree (`worktree_dirty`, `data.files` = the unmerged paths); an untracked file of the stash that already exists in the worktree (`worktree_dirty`, `data.files`). The last check is needed because git applies the tracked part before it fails on such a file.
- **Index.** The apply runs `git stash apply -q --index <hash>`, restoring staged changes as staged. When git refuses that because the stashed index no longer applies (`conflicts in index`; git changed nothing), the engine applies again without `--index` and reports `indexRestored: false` with the warning `index_not_restored`: every change is kept but comes back unstaged. It is never silent.
- **Local changes git would overwrite** (`would be overwritten by merge`; git changed nothing): `worktree_dirty` with the paths in `data.files`.
- **Conflicts.** When git stops with merge conflicts, the result is `conflict` (`domainCode` `stash_conflict`) with the unmerged paths (`git diff --name-only --diff-filter=U -z`) in `conflicts` and `data.conflicts`. The worktree is **left as git left it**, with conflict markers, and the stash entry is **kept**: resetting would destroy the stash's content that did apply. The user resolves the conflicts (or resets) by hand. Any other stop is `git_failed`, also with the entry kept.
- **Pop** drops the entry (`git stash drop -q stash@{index}`, after re-verifying the hash) **only** when the apply was complete: no conflict and `indexRestored: true`. Otherwise `dropped` is `false` and the entry stays in the list, so a pop can never lose the stashed staged/unstaged split or conflicting content.
- **Drop** deletes the entry. Without `confirm: true` it changes nothing and fails with `needs_confirmation`, `data.stash` (the entry) and `data.files` (how many files it holds), after the same hash check.

Untracked-file rules:

- Every path must be exactly a path `repos.changes` lists as **untracked** right now (status is re-read on every call). Anything else is refused as a whole, changing nothing: `not_found` (`domainCode` `path_not_untracked`) for a tracked, ignored, missing or absolute path, a path escaping the worktree, a non-canonical spelling (`a/../b`) and a directory entry (a nested repository, listed with a trailing `/`). Duplicates collapse.
- `repos.validateUntracked` only validates and resolves each path to an absolute path inside the worktree. A desktop client can use it before moving the files to the system **Trash** itself, which is recoverable.
- `repos.discardUntracked` **permanently** deletes the files with `git clean -f -q -- :(literal)<path>...` (no `-d`, no `-x`: git itself never removes a tracked file, an ignored file or a directory here). Without `confirm: true` it changes nothing and fails with `needs_confirmation` and `data.paths`. `removed` lists what is gone afterwards; `kept` what status still lists (for example recreated meanwhile). The CLI and the MCP tool use this method.

Git commands:

| Action | Command |
|---|---|
| stash untracked files | `rev-parse -q --verify <hash>^3`; `ls-tree -r -z --name-only <tree>` |
| apply / pop | `stash apply -q [--index] <hash>`; on exit 1, `diff --name-only --diff-filter=U -z` |
| drop / pop | `stash drop -q stash@{N}` |
| delete untracked | `clean -f -q -- :(literal)<path>...` |

```json
{"id":"12","method":"repos.stashPop","params":{"workspace":"feat","repo":"api","index":1,"hash":"6666666666666666666666666666666666666666"}}
{"id":"12","result":{"repo":"api","stash":{"index":1,"ref":"stash@{1}","hash":"6666666666666666666666666666666666666666","branch":"feat","message":"older","date":"2026-10-02T08:00:00Z"},"indexRestored":false,"dropped":false,"conflicts":["main.go"],"warnings":[],"error":{"code":"conflict","message":"applying the stash stopped with conflicts; the stash entry was kept: stash@{1}","data":{"conflicts":["main.go"],"domainCode":"stash_conflict","subject":"stash@{1}"}}}}
```

The full wire examples are pinned in `internal/rpc/testdata/repo_actions.golden`.

### 4.7 Repository write actions (additive, v1)

Stage, unstage, discard, commit, push and create a stash in one repo of a workspace. Every method takes `context?`, `workspace`, `repo` (alias) and re-reads the repo's status right before acting. Paths are worktree-relative, exactly as `repos.changes` lists them; each reaches git as a literal pathspec (`:(literal)<path>`) after `--`.

| Method | Extra params | Result |
|---|---|---|
| `repos.stage` | `paths` or `all: true` | `{"repo","paths":[]}` |
| `repos.unstage` | `paths` or `all: true` | `{"repo","paths":[]}` |
| `repos.discard` | `paths`, `confirm?` | `{"repo","discarded":[],"backup"?}` |
| `repos.commitChanges` | `message`, `amend?` (must be false) | `{"repo","commit"?:Commit,"warnings":[],"error"?}` |
| `repos.push` | `setUpstream?` | `{"repo","branch","remote","upstream"?,"setUpstream","upToDate","pushed","error"?}` (progress `op` `repo.push`) |
| `repos.stashCreate` | `message?`, `includeUntracked?`, `keepIndex?` | `{"repo","stash":Stash}` |

`repos.commit` (one commit's detail, §4.5) is unchanged; committing is `repos.commitChanges`.

Rules:

- **Stage** accepts paths listed as unstaged (modified, deleted, type change) or untracked; **unstage** accepts paths listed as staged (a staged rename also unstages its source path). Any other path (staged-only for stage, unstaged-only for unstage, conflicted, unknown, absolute, escaping, non-canonical, a nested repository) refuses the whole call with `not_found` (`domainCode` `path_not_changed`) and changes nothing. `all: true` takes every applicable path instead of `paths`.
- **Discard** restores tracked files from the **index**: only unstaged changes are discarded, staged changes stay (for a file with no staged change this equals restoring from `HEAD`). Refused, changing nothing: an untracked file or one marked intent-to-add (`conflict`, `path_is_untracked`; deleting untracked files is `repos.discardUntracked`, or a client moving the files to the Trash), a file whose changes are all staged (`conflict`, `staged_only`: unstage first), a conflicted or unknown path (`path_not_changed`). Without `confirm: true` it fails with `needs_confirmation` and `data.files` = `[{"path","status","additions","deletions","binary"}]`. With it, the engine first writes the unstaged changes as a binary-safe patch to `<cache>/discarded/<repo>-<UTC timestamp>.patch` (mode 0600; a header names the `git -C <worktree> apply <patch>` command that restores it; the newest 20 are kept), then restores the files. If the backup cannot be written nothing is discarded. `backup` is its absolute path.
- **Commit** needs staged changes (`nothing_staged`) and a non-empty message (`invalid_params`; surrounding whitespace and trailing spaces are trimmed). A subject line over 72 characters is the warning `subject_too_long`, not an error. It uses the repo's configured identity and never sets one: when git has none (`git var GIT_AUTHOR_IDENT`/`GIT_COMMITTER_IDENT` fail) the result is `identity_missing` with `data.commands`, the two `git config --global user.name|user.email` commands for the user to run (omit `--global` for this repo only). Hooks run (never `--no-verify`); a refusal by an installed `pre-commit`, `prepare-commit-msg` or `commit-msg` hook is `hook_failed` (`git_failed`) with `data.output` (git's and the hook's output, last 4000 bytes). Also refused: detached HEAD (`detached_head`), a merge or rebase in progress (`integration_in_progress`), unresolved conflicts (`worktree_dirty`, `data.files`). Never `--amend`. Refusals are in the result's `error`, not a failed call.
- **Push** never forces (no `--force`, `--force-with-lease`, or `+` refspec). The branch is pushed to its upstream only when that is a remote branch of the **same name** (git's `push.default=simple` rule): a workspace branch created from `origin/develop` tracks `origin/develop`, and pushing there would land its commits on the base branch. Otherwise (no upstream, a gone or local one, another name) the result is `no_upstream` (`conflict`) with `data.remote` (the context's remote, default `origin`), and nothing is pushed; with `setUpstream: true` the branch is published as `git push -u <remote> refs/heads/<b>:refs/heads/<b>`, which also makes it the upstream. Nothing ahead of the upstream: `upToDate: true`, no push. A non-fast-forward is `push_rejected` (`conflict`; pull or update from base, then push); an unreachable remote or refused credentials is `auth_failed` (`git_failed`); any other refusal (a pre-push hook, a protected branch) is `git_failed`. All three carry git's message in `data.output`, with credentials in URLs masked. `GIT_TERMINAL_PROMPT=0` means git never prompts. Timeout: 2 minutes.
- **Stash create** refuses a merge or rebase in progress and unresolved conflicts, and `nothing_to_stash` (`conflict`) when there is nothing to save (only untracked files without `includeUntracked`, or git saved nothing: `refs/stash` unchanged).

Git commands:

| Action | Command |
|---|---|
| stage | `add -A -- :(literal)<path>...` |
| unstage | `restore --staged -- ...`; without a HEAD commit `rm --cached -q -- ...` |
| discard backup | `diff --binary --full-index --no-ext-diff --no-textconv --no-color -- ...` |
| discard | `restore --worktree -- ...` |
| identity check | `var GIT_AUTHOR_IDENT`, `var GIT_COMMITTER_IDENT` |
| commit | `commit -q -m <message>`; on failure `rev-parse --git-path hooks/<name>...` |
| push | `push -q [-u] <remote> refs/heads/<b>:refs/heads/<b>` |
| stash create | `stash push -q [--include-untracked] [--keep-index] [-m <message>]` |

```json
{"id":"14","method":"repos.push","params":{"workspace":"feat","repo":"api"}}
{"id":"14","result":{"repo":"api","branch":"feat","remote":"origin","upstream":"origin/feat","setUpstream":false,"upToDate":false,"pushed":1,"error":{"code":"conflict","message":"the remote has commits this branch lacks; pull or update from base, then push again: origin/feat","data":{"domainCode":"push_rejected","output":" ! [rejected]        feat -\u003e feat (fetch first)","subject":"origin/feat"}}}}
```

The full wire examples are pinned in `internal/rpc/testdata/repo_write.golden`.

## 5. Progress events

`data` fields:

| Field | Set for | Meaning |
|---|---|---|
| `kind` | all | `repo`, `step`, `info` or `warn`. |
| `op` | `repo` | `workspace.create`, `workspace.add_repo`, `workspace.destroy`, `workspace.remove_repo`, `workspace.repair`, `workspace.sync_env`, `workspace.update`, `repo.fetch`, `repo.pull`, `repo.push`. |
| `repo` | `repo` | Project key or repo alias. |
| `phase` | `repo` | `started`, `finished`, `failed`, `rolled_back`. |
| `error` | `repo` with `failed` | Human error text. The final response carries the structured error. |
| `key`, `message` | `step`, `info`, `warn` | Catalog key (stable) and rendered text (may change). |

A failed create emits, for example: `alpha:started`, `alpha:finished`, `beta:started`, `beta:failed`, `alpha:rolled_back`, then the error response. Unknown `kind` or `phase` values must be ignored (they may be added).

## 6. Versioning policy

| Change | Version |
|---|---|
| New method, new optional parameter, new result field, new event `kind`/`phase`, new `domainCode` | Keeps `protocolVersion` 1. Clients must ignore unknown fields. |
| Removing or renaming a method, parameter or field; changing a type or meaning; adding a required parameter; changing an error `code` | Increments `protocolVersion`. |

The ten error codes in §3 are closed for v1. Every wire shape is pinned by a golden test (`internal/rpc/testdata`); changing one needs a golden update and, if breaking, a version bump.

## 7. `--json` on CLI read commands

For scripts and agents without the RPC server. These shapes are separate from the RPC shapes and use snake_case; their golden files are in `internal/cli/testdata`.

| Command | Shape |
|---|---|
| `wspace info --json` | `{"context_name","options":{"<key>":{"value","from"}}}` |
| `wspace list --json` | `[{"name","path","project_count","error"?,"legacy"?,"orphan_of"?}]` |
| `wspace adopt-legacy --json` | `{"adopted":[{"name","root"}],"skipped":[{"root","reason","message"}]}` (same as `workspaces.adoptLegacy`) |
| `wspace claim [names...] --json` | `{"claimed":[{"name","root","previous_context"}],"skipped":[{"name","root"?,"reason","message"}]}` (same as `workspaces.claim`) |
| `wspace status <ws> --json` | `[{"alias","branch","ahead","behind","dirty","base_branch"?,"base_missing"?}]` |
| `wspace context list --json` | `[{"name","active","workspaces_root","projects_root"?,"project_count"}]` |
| `wspace update [ws] --json` | `[{"repo","strategy","base","before_head","after_head","up_to_date","commits_integrated","conflicts","dirty_files"?,"restored"?,"error"?:{"code","message"}}]` (`code` is the domain code; exit status 1 when any repo was not updated) |
| `wspace agents status --json` | `{"skill":{"name","dir","origin","location","mcp_server","mcp_command","mcp_config_home"?},"agents":[{"id","name","detected","skills_dir"?,"skill_path"?,"skill","skill_target"?,"legacy_path"?,"mcp"?,"mcp_method"?,"mcp_config_path"?,"mcp_snippet"?,"mcp_registrations"?:[{"scope","project"?,"command"?,"stale"?}],"mcp_duplicate"?,"mcp_stale"?}]}` (same as `agents.status`) |
| `wspace agents mcp-clean --json` | Same shape as `agents install --json` |
| `wspace agents install\|uninstall --json` | `{"skill","changes":[{"agent"?,"kind","path"?,"target"?,"backup"?,"detail"?,"manual_command"?}],"agents"}` (as `agents.install`, without `message`) |
| `wspace install [--yes] [--uninstall] --json` | `{"installed","already_installed"?,"path"?,"dir"?,"manual_command"?,"on_path","path_configured"?,"path_updated","path_declined"?,"path_targets"?,"new_terminal","shell_rc_updated"?,"shell_rc_path"?,"uninstall"?,"removed"?,"path_removed"?,"pending_removal"?}`. Installs to `~/.local/bin` (Linux, macOS) or `%LOCALAPPDATA%\Programs\wspace` (Windows); with `--yes`, adds that directory to PATH: a marked block in the rc file(s) of `$SHELL`, or the user `Path` in `HKCU\Environment` on Windows (`path_targets` is then `HKCU\Environment\Path`). `--json` never prompts. Paths are native. |
| `wspace repo <ws> <repo> status --json` | `{"branch":{"repo","branch","detached","head","upstream"?:{"ref","remote","gone","ahead","behind"},"base_branch"?,"base_ref"?,"base_found","base_ahead"?,"base_behind"?,"last_fetch"?},"staged":[{"path","orig_path"?,"status"}],"unstaged","untracked","conflicted","stashes"}` |
| `wspace repo <ws> <repo> log --json` | `{"range","base"?,"total","offset","has_more","commits":[{"hash","short_hash","subject","author","date"}]}` (`--offset`, `--limit`) |
| `wspace repo <ws> <repo> show <hash> --json` | a commit plus `{"body","parents","files":[FileDiff],"truncated"}` |
| `wspace repo <ws> <repo> stash [index] --json` | `[{"index","ref","hash","branch"?,"message","date"}]`, or with an index `{"stash","files","truncated","includes_untracked"}` |
| `wspace repo <ws> <repo> diff [path] [--staged] --json` | `[FileDiff]`, FileDiff `{"path","orig_path"?,"status","binary","truncated","additions","deletions","hunks":[{"header","old_start","old_lines","new_start","new_lines","lines"}]}` (no path: every unstaged and untracked change, or every staged one) |
| `wspace repo <ws> <repo> fetch\|pull --json` | fetch: `{"repo","remote","branch"}`; pull: `{"repo","upstream","before_head","after_head","up_to_date","commits_pulled","ahead","behind","dirty_files"?,"error"?:{"code","message"}}` (exit status 1 when refused) |
| `wspace repo <ws> <repo> stash apply\|pop <n> --json` | `{"repo","stash"?,"index_restored","dropped","conflicts","files"?,"warnings","error"?:{"code","message"}}` (exit status 1 when refused or conflicted; the entry is then kept) |
| `wspace repo <ws> <repo> stash drop <n> [--yes] --json` | `{"repo","stash"}` (asks first unless `--yes`) |
| `wspace repo <ws> <repo> clean <path>... [--yes] --json` | `{"repo","removed","kept"}` (permanent delete of untracked files; asks first unless `--yes`) |
| `wspace repo <ws> <repo> stage\|unstage <path>...\|--all --json` | `{"repo","paths"}` |
| `wspace repo <ws> <repo> discard <path>... [--yes] --json` | `{"repo","discarded","backup"?}` (asks first, listing line counts, unless `--yes`) |
| `wspace repo <ws> <repo> commit -m <message> --json` | `{"repo","commit"?:{"hash","short_hash","subject","author","date"},"warnings","commands"?,"files"?,"output"?,"error"?:{"code","message"}}` (exit status 1 when refused) |
| `wspace repo <ws> <repo> push [--set-upstream] --json` | `{"repo","branch","remote","upstream"?,"set_upstream","up_to_date","pushed","output"?,"error"?:{"code","message"}}` (exit status 1 when refused) |
| `wspace repo <ws> <repo> stash push [-m <msg>] [-u] [--keep-index] --json` | `{"repo","stash"}` |
| `wspace project list --json` | `[{"key","source_dir","origin_branch"?,"dest_branch"?,"worktree_dir"?}]` |
