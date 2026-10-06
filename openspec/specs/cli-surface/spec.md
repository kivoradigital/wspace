# CLI Surface Specification

## Purpose

Command parsing, flags, human-readable output, the `--json` automation contract on read-only commands, and a centralized English message catalog for all user-facing strings.

## Requirements

### Requirement: Full command parity

The system MUST expose `create`, `list`, `status`, `add`, `rm`, `repair`, `sync-env`, `exec`, `destroy`, `doctor`, `jump`, `shell-init`, `info`, `install`, and `version` as top-level commands.

#### Scenario: Every listed command is invokable

- GIVEN the built binary
- WHEN each of the 14 listed commands is invoked with `--help`
- THEN each prints usage without error

### Requirement: --json contract on read-only commands

`info`, `list`, and `status` MUST support a `--json` flag emitting stable, schema-conformant JSON. `info --json` MUST emit the resolved context name and its resolved options. `list --json` MUST emit an array of workspace summaries (name, path, project count). `status --json` MUST emit, per workspace, an array of per-repo records (alias, branch, ahead, behind, dirty boolean).

#### Scenario: info --json schema

- GIVEN a resolved context `work`
- WHEN `wspace info --json` runs
- THEN the output is valid JSON containing at least `context_name` and the resolved option keys

#### Scenario: list --json schema

- GIVEN two workspaces exist
- WHEN `wspace list --json` runs
- THEN the output is a JSON array with one object per workspace, each containing `name`, `path`, and `project_count`

#### Scenario: status --json schema

- GIVEN a workspace with two repos, one dirty
- WHEN `wspace status <name> --json` runs
- THEN the output is a JSON array of two objects, each with `alias`, `branch`, `ahead`, `behind`, and `dirty`

#### Scenario: Non-JSON commands remain human-output only

- GIVEN a command not in {`info`, `list`, `status`}
- WHEN it is invoked with `--json`
- THEN the system MUST reject the flag as unsupported for that command

### Requirement: Centralized message catalog

Every user-facing string emitted by CLI code MUST be resolved through a single centralized message catalog package; no string literal MUST be emitted directly from command code.

#### Scenario: New string added through the catalog

- GIVEN a new CLI error condition needs a message
- WHEN the message is implemented
- THEN it is added as a catalog entry and referenced by key, not written inline in the command handler

#### Scenario: Catalog resolution failure is visible

- GIVEN a message key referenced by a command does not exist in the catalog
- WHEN that code path executes
- THEN the system MUST surface a clear internal error rather than emit a blank or malformed string
