# Shell Integration Specification

## Purpose

The binary cannot change its parent shell's working directory; this capability defines how `jump` and `shell-init` cooperate with a shell function to make directory changes appear to the user.

## Requirements

### Requirement: jump never mutates the parent shell

The `jump` command MUST NOT attempt to change the parent shell's working directory. It MUST print only the resolved path to stdout.

#### Scenario: jump prints only the path

- GIVEN a workspace named `feature-x` exists
- WHEN `wspace jump feature-x` runs with stdout captured
- THEN stdout contains exactly the resolved workspace path and nothing else

#### Scenario: jump warns when stdout is a TTY

- GIVEN `wspace jump feature-x` runs with stdout attached to an interactive terminal (not captured)
- WHEN the command completes
- THEN an explanation that the path was not applied is printed to stderr, not stdout

### Requirement: shell-init emits shell-specific functions

The system MUST emit, via `shell-init`, a shell function for POSIX-compatible shells that performs `cd` using the captured output of `jump`, and a distinct fish-syntax variant, each passing all non-`jump` subcommands straight through to the binary.

#### Scenario: POSIX function wraps jump with cd

- GIVEN `shell-init bash` (or `zsh`) is run
- WHEN the emitted function is sourced and the user runs `wspace jump feature-x` through it
- THEN the live shell's cwd changes to the resolved workspace path

#### Scenario: Fish variant emitted for fish

- GIVEN `shell-init fish` is run
- WHEN the output is evaluated
- THEN a fish-syntax function achieving the same `cd` behavior is produced

#### Scenario: Non-jump commands pass through unchanged

- GIVEN the shell function from `shell-init` is active
- WHEN the user runs `wspace status`
- THEN the function forwards the call to the binary and does not attempt any `cd`
