# Repository Operations Specification

## Purpose

Git semantics for worktree lifecycle, base branch resolution, sync, and change classification — shelled out to the system `git` binary.

## Requirements

### Requirement: Base branch resolution order

The system MUST resolve a project's base branch by preferring `origin/<base>` over a local branch of the same name, falling back to the local branch if `origin/<base>` does not exist, and falling back to the current `HEAD` if neither exists. This order exists so a stale local clone never seeds new work.

#### Scenario: Remote branch preferred

- GIVEN both `origin/main` and a local `main` exist and diverge
- WHEN base resolution runs for `base_branch: main`
- THEN `origin/main` is used as the base

#### Scenario: Fallback to local branch

- GIVEN `origin/main` does not exist but a local `main` does
- WHEN base resolution runs
- THEN the local `main` is used as the base

#### Scenario: Fallback to current HEAD

- GIVEN neither `origin/main` nor a local `main` exists
- WHEN base resolution runs
- THEN the current `HEAD` is used as the base

### Requirement: Base sync safety

The system MUST fetch `origin` before resolving or fast-forwarding a base branch, and MUST fast-forward the local base branch only when it is not checked out elsewhere with uncommitted changes. It MUST NOT fast-forward a base branch that is checked out with uncommitted changes anywhere.

#### Scenario: Fetch always runs first

- GIVEN base sync is triggered
- WHEN the operation runs
- THEN `git fetch origin` MUST complete before any fast-forward attempt

#### Scenario: Skip fast-forward when checked out with dirty changes

- GIVEN the local base branch is checked out in another worktree with uncommitted changes
- WHEN base sync runs
- THEN the local base branch MUST NOT be fast-forwarded, and no work is lost

#### Scenario: Fast-forward when safe

- GIVEN the local base branch is not checked out anywhere else
- WHEN base sync runs and origin has new commits
- THEN the local base branch is fast-forwarded to match `origin/<base>`

### Requirement: Worktree creation validation

Before creating a worktree, the system MUST validate that the target branch is not already checked out in another worktree. Creation MUST follow one of two paths: creating a new branch from the resolved base, or attaching to an existing branch that is free.

#### Scenario: Reject branch checked out elsewhere

- GIVEN branch `feature-x` is already checked out in another worktree
- WHEN worktree creation targets `feature-x`
- THEN the system MUST refuse and report which worktree holds the branch

#### Scenario: New branch from base

- GIVEN branch `feature-y` does not exist yet
- WHEN worktree creation runs with base `origin/main`
- THEN a new branch `feature-y` is created from `origin/main` and checked out into the new worktree

#### Scenario: Attach existing free branch

- GIVEN branch `feature-z` exists and is not checked out anywhere
- WHEN worktree creation runs targeting `feature-z`
- THEN the existing branch is checked out into the new worktree without creating a new branch

### Requirement: Worktree removal

The system MUST remove a worktree via `git worktree remove` (optionally `--force`) followed by `git worktree prune`.

#### Scenario: Clean removal

- GIVEN a worktree with no blocking safety issues
- WHEN removal runs
- THEN `git worktree remove` succeeds and a subsequent `git worktree prune` clears any residual registration

### Requirement: Status and change classification

The system MUST report per-repo branch name, ahead/behind counts relative to its upstream, and dirty state, and MUST classify uncommitted changes into exactly three buckets: *tracked* (modified tracked files), *foreign* (untracked files not produced by the tool), and *env-copy* (untracked files the tool copied as environment files).

#### Scenario: Ahead/behind reported

- GIVEN a repo's branch has 2 local commits not pushed and 1 remote commit not merged
- WHEN status runs
- THEN it reports 2 ahead, 1 behind

#### Scenario: Tracked change detected

- GIVEN a tracked file has uncommitted modifications
- WHEN classification runs
- THEN that file is placed in the tracked bucket

#### Scenario: Foreign file detected

- GIVEN an untracked file exists that the tool did not create
- WHEN classification runs
- THEN that file is placed in the foreign bucket

#### Scenario: Env-copy file distinguished from foreign

- GIVEN an untracked file matches a path the tool copied during env sync
- WHEN classification runs
- THEN that file is placed in the env-copy bucket, not the foreign bucket
