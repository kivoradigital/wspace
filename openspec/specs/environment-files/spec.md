# Environment Files Specification

## Purpose

Discovery, copy and re-sync of `.env`-style files from a project's source clone into its worktrees, with gitignore-coverage warnings.

## Requirements

### Requirement: Env file discovery

The system MUST discover `.env` and `.env.*` files under a project's `source_dir`, MUST prune directories listed in `env_prune_dirs`, and MUST exclude files matching `*.example`, `*.sample`, `*.template`, or `*.dist`.

#### Scenario: Discovers plain and suffixed env files

- GIVEN a project with `.env` and `.env.local` at its root
- WHEN env discovery runs
- THEN both files are found

#### Scenario: Excludes template-like files

- GIVEN a project with `.env.example` and `.env.template`
- WHEN env discovery runs
- THEN neither file is found

#### Scenario: Prunes configured directories

- GIVEN `env_prune_dirs` includes `node_modules` and an `.env` file exists nested under `node_modules`
- WHEN env discovery runs
- THEN that nested file is excluded from results

### Requirement: Env file copy preserves relative path

When copying discovered env files into a worktree, the system MUST preserve each file's path relative to `source_dir`.

#### Scenario: Nested env file preserved

- GIVEN a discovered file at `source_dir/config/.env.local`
- WHEN copy runs into a worktree
- THEN the file is written at `<worktree>/config/.env.local`

### Requirement: Gitignore coverage warning

For every copied env file, the system MUST check whether the destination repository's ignore rules cover that file's path and MUST warn per file when they do not.

#### Scenario: Warn on uncovered copy

- GIVEN the destination repo's `.gitignore` does not match `config/.env.local`
- WHEN the copy completes
- THEN a warning is emitted naming that specific file

#### Scenario: No warning when covered

- GIVEN the destination repo's `.gitignore` matches `.env*`
- WHEN the copy completes
- THEN no warning is emitted for files matching that pattern

### Requirement: Re-sync on demand

The system MUST support re-running env file copy for every repo in an existing workspace via a dedicated `sync-env` operation, without recreating worktrees.

#### Scenario: Re-sync after source env changes

- GIVEN a workspace already exists and the source project's `.env` file has changed since the workspace was created
- WHEN `sync-env` runs for that workspace
- THEN the updated `.env` content is re-copied into every mounted repo that has one
