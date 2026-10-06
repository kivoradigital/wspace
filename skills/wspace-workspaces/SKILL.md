---
name: wspace-workspaces
description: "Trigger: workspace, worktree, multi-repo branch, wspace, .wspace/workspace.yaml, legacy .ws/. Drive wspace git worktree workspaces safely through its MCP tools or CLI."
metadata:
  author: "Kivora Digital S.L."
  version: "1.0"
---

## Activation Contract

Load this skill when:
- The user mentions a workspace, worktrees across several repositories, a wspace context, or the `wspace` tool.
- The cwd is inside a directory that contains `.wspace/workspace.yaml` (a wspace workspace) or a legacy `.ws/workspace.conf` (created by the old bash `ws`).
- The user asks to create, inspect, update, tear down or clean up such a workspace, or to manage contexts and projects.

## Hard Rules

- Prefer the `wspace` MCP tools (server `wspace`; `wspace-dev` for a development build). Fall back to the `wspace` CLI only when the tools are unavailable.
- Never call `destroy_workspace`, `remove_project`, `delete_context`, `unregister_project` or `sync_env` unless the user explicitly asked for that removal.
- Call a destructive tool with `confirm: false` first, show the user every `needs_confirmation` reason, and pass `confirm: true` only after the user confirms.
- Never pass `force: true` (CLI `--force`) unless the user explicitly agreed to lose the listed work.
- Update from base with `strategy: "merge"` (default). Use `"rebase"` only when the user asks; it rewrites pushed history.
- Conflicts are auto-aborted and the repo is restored; report the conflicting paths, never resolve them silently.
- Never edit `.wspace/` or `.ws/` files by hand.
- Commit (`repo_commit_changes`) and push (`repo_push`) only when the user asked, with a commit message the user approved. Never amend, skip hooks, or force-push (the tools cannot).
- On `identity_missing`, show the user `data.commands`; never run `git config` for them. On `hook_failed`, report `data.output`; never bypass the hook.
- On `no_upstream`, ask before publishing (`setUpstream: true`). On `push_rejected`, suggest `repo_pull_ff` or `update_repo`, never a force push.
- When `check_update` returns `bundledBy` (or `wspace version` says "bundled with"), wspace updates only with that app: never suggest updating wspace separately.
- `repo_discard` and `repo_delete_untracked` lose work: call them with `confirm: false` first, show the files, and confirm only after the user agrees. `repo_discard` keeps a backup patch; name its path.

## Decision Gates

| Situation | Action |
|---|---|
| What exists? | `list_contexts`, `list_workspaces`, `workspace_status`, `list_projects` |
| Before tearing down | `teardown_check`, then the confirm flow above |
| New feature branch across repos | `create_workspace` (`projects?`, `branch?`) |
| Workspace or repo behind its base | `update_workspace` / `update_repo` |
| `orphanOf` set on a workspace | Its context was removed or renamed: offer `claim_workspaces` |
| `legacy: true` or a `.ws/` directory | Offer `adopt_legacy_workspaces` (legacy files stay untouched) |
| Legacy bash `ws` configuration | `import_legacy_context` (dry run unless `confirm: true`) |
| Worktree missing or broken | `repair_workspace` |
| Inspect one repo | `repo_changes`, `repo_diff`, `repo_branch_info`, `repo_commits` |
| User asks to commit / push one repo | `repo_stage`, then `repo_commit_changes`, then `repo_push` |

## Execution Steps

1. Resolve the context: tools default to the active context; pass `context` when the user names another one.
2. Read state with the read-only tools before changing anything.
3. Run the change; on `needs_confirmation`, list `data.reasons` (repo, kind, message) and stop until the user decides.
4. Without MCP, use the CLI with `--json` where supported: `wspace list --json`, `wspace status <ws> --json`, `wspace info --json`, `wspace context list --json`, `wspace project list --json`, `wspace update [ws] --json`, `wspace claim --json`, `wspace adopt-legacy --json`. Other commands: `create <name> [--project k]... [--branch b]`, `add <ws> <project>`, `rm <ws> <alias>`, `destroy <ws>`, `repair <ws>`, `sync-env <ws>`. Select a context with `--context <name>`.

## Output Contract

- Summarize per repo: branch, ahead/behind, dirty state, and any error.
- Quote every refusal or confirmation reason verbatim; name what was changed and where.

## References

- `references/tools.md` — MCP tool arguments, error codes and CLI equivalents.
