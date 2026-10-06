# Project Configuration Specification

## Purpose

Each managed project is an explicit record — not an implicit name resolved against a shared root — with its own source directory, origin branch, destination branch and worktree folder, replacing convention-based discovery as the only path.

## Requirements

### Requirement: Explicit per-project record

The system MUST store, per project, `source_dir` (absolute path to the project's clone), `origin_branch`, `dest_branch`, and `worktree_dir` (or a worktree path template).

#### Scenario: Project record with all fields set

- GIVEN a project configured with explicit `origin_branch: develop`, `dest_branch: feature-x`, `worktree_dir: /work/feature-x/api`
- WHEN a workspace is created including that project
- THEN the worktree uses `develop` as its base, `feature-x` as its branch, and is created at `/work/feature-x/api`

### Requirement: Field inheritance precedence chain

When a per-project field is empty, the system MUST resolve it through this precedence chain: per-project value → workspace-level value → context-level value → built-in default. This chain applies identically to `dest_branch` (workspace branch), `origin_branch` (context `base_branch`), and `worktree_dir` (derived alias).

#### Scenario: Per-project dest_branch overrides workspace branch

- GIVEN a workspace-level branch `release-1` and a project record with `dest_branch: hotfix-1`
- WHEN a workspace is created including that project
- THEN that project's worktree branch is `hotfix-1`, not `release-1`

#### Scenario: Empty per-project dest_branch inherits workspace branch

- GIVEN a workspace-level branch `release-1` and a project record with an empty `dest_branch`
- WHEN a workspace is created including that project
- THEN that project's worktree branch is `release-1`

#### Scenario: origin_branch inherits context base_branch

- GIVEN a project record with an empty `origin_branch` and a context `base_branch: main`
- WHEN base resolution runs for that project
- THEN `main` is used as the origin branch

#### Scenario: worktree_dir inherits derived alias

- GIVEN a project record with an empty `worktree_dir`
- WHEN a workspace is created including that project
- THEN the worktree path is derived from the workspace's default alias rule

### Requirement: Discovery scan as wizard pre-fill only

Directory scanning under `projects_root` MUST be an optional convenience that pre-fills the per-project wizard; it MUST NOT be the only way to register a project.

#### Scenario: Manual registration without scanning

- GIVEN the user skips directory scanning entirely
- WHEN the user completes the per-project wizard manually
- THEN the project record is saved and usable exactly as a scan-derived one would be

### Requirement: Main-clone detection

The system MUST reject a candidate `source_dir` whose `.git` is a file (indicating a linked worktree) rather than a directory, since a linked worktree's branches live in its parent clone.

#### Scenario: Reject a linked-worktree source

- GIVEN a candidate directory whose `.git` entry is a file, not a directory
- WHEN the project wizard or discovery scan evaluates it
- THEN the candidate MUST be rejected with an explanation, and MUST NOT be registered as a project

#### Scenario: Accept a main clone

- GIVEN a candidate directory whose `.git` entry is a directory
- WHEN the project wizard or discovery scan evaluates it
- THEN the candidate is accepted as a valid project source
