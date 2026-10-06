# Context Management Specification

## Purpose

Contexts are the top-level configuration aggregate: named, switchable option sets, each owning its own roots, defaults, ignore patterns, and workspaces. Resolution never mixes settings across contexts.

## Requirements

### Requirement: Context as top-level aggregate

The system MUST model a context as a named record holding `workspaces_root`, `projects_root`, `base_branch`, `branch_prefix`, `copy_env_default`, `fetch_before_create`, `env_prune_dirs`, and `ignore_patterns`, stored independently per context.

#### Scenario: Two contexts with independent roots

- GIVEN two contexts `work` and `oss` with different `workspaces_root` and `projects_root`
- WHEN the user switches from `work` to `oss`
- THEN all subsequent commands resolve roots, defaults and ignore patterns from `oss` only

### Requirement: Context lifecycle commands

The system MUST support create, list, switch and edit operations on contexts.

#### Scenario: Create a new context

- GIVEN no context named `staging` exists
- WHEN the user runs the context-create flow with name `staging`
- THEN a new context record is persisted and `staging` becomes selectable

#### Scenario: Switch active context

- GIVEN contexts `work` and `oss` both exist
- WHEN the user switches the active context to `oss`
- THEN the root config file records `oss` as the active context

### Requirement: Context resolution precedence

The system MUST resolve the effective context per invocation in this order: (1) an explicit `--context` flag or environment variable, (2) the active context recorded in the root config file.

A local config file found by walking up from the current working directory MUST NOT participate in this decision. It is an option-override layer applied *after* the context is resolved, never a context source — see "Local config as override layer, not a context switch" below. Making the mere presence of a file in a parent directory change the active context would turn `cd` into a silent mode switch across a different workspaces root and a different project set.

#### Scenario: Explicit flag wins

- GIVEN the active context recorded in the root config is `work`
- WHEN the user runs a command with `--context oss`
- THEN the command resolves against `oss`, ignoring the recorded active context and any local config file

#### Scenario: A local config never changes which context is active

- GIVEN no `--context` flag or environment variable is set, the root config's active context is `work`, and a local config file exists in a parent of the cwd
- WHEN a command runs
- THEN the resolved context is still `work`, and the local config contributes only option overrides within it

#### Scenario: Fallback to root active context

- GIVEN no `--context` flag and no environment variable
- WHEN a command runs
- THEN the active context recorded in the root config file is used

### Requirement: Local config as override layer, not a context switch

A local config file MUST only override option values within the resolved context; it MUST NOT define a new context or select a different context by name.

#### Scenario: Local config overrides one option

- GIVEN the resolved context is `work` and a local config file sets `fetch_before_create: false`
- WHEN a command runs from that directory
- THEN the effective `fetch_before_create` is `false` while every other option still comes from `work`

#### Scenario: Local config cannot declare a context

- GIVEN a local config file contains a context name or context-scoped fields
- WHEN the config is loaded
- THEN the system MUST reject those fields as invalid and MUST NOT create or switch to a context from them

### Requirement: Ignore patterns scoped to context

The system MUST accept a glob list of `ignore_patterns` at context initialization, applied during project discovery, and MUST keep it separate from `env_prune_dirs`.

#### Scenario: Ignore pattern excludes a directory from discovery

- GIVEN a context with `ignore_patterns: ["vendor/*"]`
- WHEN project discovery scans `projects_root`
- THEN directories matching `vendor/*` are excluded from discovery results

#### Scenario: Ignore patterns do not affect env pruning

- GIVEN `ignore_patterns` and `env_prune_dirs` are both configured with different values
- WHEN env discovery runs for a project
- THEN only `env_prune_dirs` affects which directories are pruned from env file search
