# wspace MCP tools and CLI equivalents

`context` is optional on every tool and defaults to the active context.
Destructive tools (D) require `confirm`; with `confirm: false` they change
nothing and fail with `needs_confirmation` plus `data.reasons`.

| Tool | Arguments | CLI equivalent |
|---|---|---|
| `engine_version` | — | `wspace version` |
| `engine_info` | `context?` | `wspace info --json` |
| `check_update` | — | `wspace version --check` |
| `run_doctor` | `context?` | `wspace doctor` |
| `list_contexts` | — | `wspace context list --json` |
| `get_context` | `name?` | `wspace context show [name]` |
| `create_context` | `name`, `workspacesRoot`, `projectsRoot?`, `activate?`, … | `wspace context create` (interactive) |
| `update_context` | `name`, `newName?`, … | `wspace context edit [name]` |
| `switch_context` | `name` | `wspace context switch <name>` |
| `delete_context` (D) | `name`, `allowActive?`, `confirm` | `wspace context remove <name>` |
| `import_legacy_context` | `from?`, `name` or `into`, `confirm?` | `wspace context import [name]` |
| `list_projects` | `context?` | `wspace project list --json` |
| `scan_projects` | `context?`, `roots?`, `depth?` | — |
| `register_project` / `register_projects` | `key`, `sourceDir` | — |
| `update_project` | `key`, … | — |
| `unregister_project` (D) | `key`, `confirm` | — |
| `list_workspaces` | `context?` | `wspace list --json` |
| `workspace_status` | `workspace` | `wspace status <ws> --json` |
| `teardown_check` | `workspace` | — |
| `create_workspace` | `name`, `branch?`, `projects?` | `wspace create <name>` |
| `add_project` | `workspace`, `project` | `wspace add <ws> <project>` |
| `remove_project` (D) | `workspace`, `project` (alias), `confirm`, `force?`, `deleteBranch?` | `wspace rm <ws> <alias>` |
| `destroy_workspace` (D) | `workspace`, `confirm`, `force?`, `deleteBranches?` | `wspace destroy <ws>` |
| `repair_workspace` | `workspace` | `wspace repair <ws>` |
| `sync_env` (D) | `workspace`, `confirm` | `wspace sync-env <ws>` |
| `adopt_legacy_workspaces` | `context?` | `wspace adopt-legacy --json` |
| `claim_workspaces` | `workspaces?` | `wspace claim [names...] --json` |
| `update_repo` | `workspace`, `repo`, `strategy?`, `autostash?` | `wspace update <ws> --repo <alias>` |
| `update_workspace` | `workspace`, `strategy?`, `autostash?` | `wspace update <ws> [--rebase] [--autostash]` |
| `list_addable_projects` | `workspace` | — |

## Repository inspector (one repo of a workspace)

Read-only:

| Tool | Arguments | CLI equivalent |
|---|---|---|
| `repo_changes` | `workspace`, `repo` | `wspace repo <ws> <repo> status --json` |
| `repo_branch_info` | `workspace`, `repo` | `wspace repo <ws> <repo> status --json` |
| `repo_diff` | `workspace`, `repo`, `path`, `staged?` | `wspace repo <ws> <repo> diff [path] [--staged]` |
| `repo_commits` | `workspace`, `repo`, `range?` (`base`/`upstream`), `offset?`, `limit?` | `wspace repo <ws> <repo> log [--range upstream]` |
| `repo_commit` (reads one commit) | `workspace`, `repo`, `hash` | `wspace repo <ws> <repo> show <hash>` |
| `repo_stashes` | `workspace`, `repo` | `wspace repo <ws> <repo> stash` |
| `repo_stash` | `workspace`, `repo`, `index` | `wspace repo <ws> <repo> stash <n>` |

Network (remote-tracking refs / fast-forward only):

| Tool | Arguments | CLI equivalent |
|---|---|---|
| `repo_fetch` | `workspace`, `repo` | `wspace repo <ws> <repo> fetch` |
| `repo_pull_ff` | `workspace`, `repo` | `wspace repo <ws> <repo> pull` |

Changing the repo:

| Tool | Arguments | CLI equivalent |
|---|---|---|
| `repo_stage` | `workspace`, `repo`, `paths?` or `all?` | `wspace repo <ws> <repo> stage <path>...\|--all` |
| `repo_unstage` | `workspace`, `repo`, `paths?` or `all?` | `wspace repo <ws> <repo> unstage <path>...\|--all` |
| `repo_discard` (D) | `workspace`, `repo`, `paths`, `confirm` | `wspace repo <ws> <repo> discard <path>...` |
| `repo_commit_changes` | `workspace`, `repo`, `message` | `wspace repo <ws> <repo> commit -m <message>` |
| `repo_push` | `workspace`, `repo`, `setUpstream?` | `wspace repo <ws> <repo> push [--set-upstream]` |
| `repo_stash_create` | `workspace`, `repo`, `message?`, `includeUntracked?`, `keepIndex?` | `wspace repo <ws> <repo> stash push [-m msg] [-u] [--keep-index]` |
| `repo_stash_apply` | `workspace`, `repo`, `index`, `hash` | `wspace repo <ws> <repo> stash apply <n>` |
| `repo_stash_pop` | `workspace`, `repo`, `index`, `hash` | `wspace repo <ws> <repo> stash pop <n>` |
| `repo_stash_drop` (D) | `workspace`, `repo`, `index`, `hash`, `confirm` | `wspace repo <ws> <repo> stash drop <n>` |
| `repo_delete_untracked` (D) | `workspace`, `repo`, `paths`, `confirm` | `wspace repo <ws> <repo> clean <path>...` |

Paths are exactly as `repo_changes` lists them. `hash` is the entry's hash
from `repo_stashes`; a shifted list is refused with `stash_changed`.
Refusals of commit, push, pull and stash apply/pop come back as a successful
call whose result carries `error` (`data.domainCode`): `nothing_staged`,
`identity_missing` (`data.commands` for the user to run), `hook_failed`
(`data.output`), `no_upstream` (`data.remote`), `push_rejected`,
`auth_failed`, `diverged`, `worktree_dirty`, `stash_conflict`.

## Error codes

`invalid_params`, `not_found`, `already_exists`, `conflict`,
`needs_confirmation`, `git_failed`, `internal`. A `needs_confirmation`
error lists `data.reasons` (`repo`, `kind`, `message`); kinds are
`tracked_change`, `foreign_file` and `unpushed_commits`.

## Update results

`update_repo` / `update_workspace` return one result per repo. A refusal
(uncommitted tracked changes without `autostash`) or a conflict is reported
in that repo's `error`; on a conflict the operation was aborted, the repo is
back at its previous HEAD, and `conflicts` lists the paths.

## Contexts and orphans

A workspace whose `orphanOf` is set belongs to a context that no longer
exists. `claim_workspaces` moves it to the current context; it never takes a
workspace owned by an existing context. `delete_context` never touches files
on disk.
