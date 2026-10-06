# Packaging and Distribution Specification

## Purpose

Native installers per platform, plus the `install` command's PATH/rc integration — never silently elevating privileges.

## Requirements

### Requirement: macOS binary

The system MUST ship the macOS build as a plain CLI binary for both amd64 and arm64.

#### Scenario: Binary runs from PATH

- GIVEN the macOS binary is placed in a PATH-listed directory
- WHEN the user runs `wspace version`
- THEN it reports its version

### Requirement: Windows installer

The system MUST produce a native Windows installer that places the binary in a standard install location and registers it appropriately for the platform.

#### Scenario: Installer completes without manual PATH edit

- GIVEN the Windows installer package
- WHEN it is run to completion
- THEN the CLI is invokable from a new shell without the user manually editing PATH

### Requirement: Linux packaging

The system MUST ship a plain Linux binary plus at least one distribution package definition that installs it.

#### Scenario: Package installs the CLI on PATH

- GIVEN the Linux package is installed
- WHEN the user opens a new shell
- THEN `wspace` is invokable from PATH

### Requirement: install command never elevates

The `install` command MUST locate a writable, PATH-listed directory for the binary and MUST NOT silently escalate privileges when no such directory is available; it MUST instead report the situation and the command the user can run manually.

#### Scenario: No writable PATH directory found

- GIVEN none of the candidate install directories are both writable and on PATH
- WHEN `install` runs
- THEN it reports the problem and prints the manual command instead of attempting an elevated write

#### Scenario: Successful install with rc integration

- GIVEN a writable, PATH-listed directory is found
- WHEN `install` runs and the user accepts the shell rc integration prompt
- THEN the binary is placed there and the shell rc is updated to source `shell-init`
