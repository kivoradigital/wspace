# Workspace Lifecycle Specification

## Purpose

Workspaces are per-context, multi-repo checkouts described by a manifest. This capability covers create, list, repair, destroy, doctor, and manifest read/write with destructive-action safety.

## Requirements

### Requirement: Workspace manifest format

The system MUST persist a workspace manifest as a flat `key = value` header (`name`, `created`, `projects_root`, `base_branch`, `branch`, `copy_env`) followed by a `[repos]` section of `alias|project|branch` records, one per mounted repository.

#### Scenario: Manifest round-trip

- GIVEN a workspace with two repos mounted at aliases `api` and `web`
- WHEN the manifest is read back
- THEN both `[repos]` entries resolve to the correct project, branch and alias

### Requirement: Workspace creation

The system MUST create one worktree per selected project under the workspace directory and MUST write the resulting manifest only after all worktrees succeed.

#### Scenario: Successful creation

- GIVEN a valid project selection and resolvable base branches
- WHEN `create` runs
- THEN one worktree per project is created and the manifest lists all of them

#### Scenario: Worktree failure aborts manifest write

- GIVEN one project's worktree creation fails (e.g. branch already checked out elsewhere)
- WHEN `create` runs
- THEN the manifest MUST NOT be written for that workspace, and the failure MUST be reported

### Requirement: Workspace listing and repair

The system MUST list all workspaces in the active context and MUST support repairing a workspace whose manifest declares worktrees no longer present on disk.

#### Scenario: Repair recreates a missing worktree

- GIVEN a workspace manifest lists repo `api` but its worktree directory was deleted manually
- WHEN `repair` runs for that workspace
- THEN the `api` worktree is recreated at its manifest-recorded branch and path

### Requirement: Destructive teardown safety

Teardown (`destroy`, `rm`) MUST refuse when the current working directory is inside the target workspace, MUST classify uncommitted changes (tracked, foreign, env-copy per repository-operations) and block on tracked or foreign changes unless forced, MUST archive the workspace manifest before removal, and MUST prune stale worktree registrations after removal.

#### Scenario: Refuse when cwd is inside the workspace

- GIVEN the user's current working directory is inside the workspace being destroyed
- WHEN `destroy` runs without relocating first
- THEN the system MUST refuse and report the reason, performing no removal

#### Scenario: Block on foreign changes without --force

- GIVEN a repo in the workspace has a genuinely new untracked file (foreign)
- WHEN `destroy` runs without `--force`
- THEN the system MUST refuse to remove that repo's worktree

#### Scenario: Env-copy files never block teardown

- GIVEN a repo's only uncommitted content is untracked files the tool itself copied as env files
- WHEN `destroy` runs without `--force`
- THEN the system MUST proceed, since env-copy files are ignored for safety classification

#### Scenario: Archive before removal

- GIVEN a workspace passes all safety checks (or is forced)
- WHEN `destroy` runs
- THEN the manifest MUST be archived before any worktree is removed, and worktree registrations MUST be pruned after removal completes

### Requirement: Doctor diagnostics

The system MUST report diagnostics for all workspaces in the active context and MUST prune stale worktree registrations discovered during the check, without any other mutation.

#### Scenario: Doctor prunes stale registrations

- GIVEN a worktree registration exists for a directory that no longer exists on disk
- WHEN `doctor` runs
- THEN the stale registration is pruned and reported, and no other workspace state changes
