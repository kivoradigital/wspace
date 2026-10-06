// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

// ShellInitResult carries the shell function source ShellInit renders for
// each supported shell family. The CLI's `shell-init` command (phase 4b)
// prints one of these verbatim; internal/app only builds the data
// (design.md §15 phase mapping: "the ShellInit data the CLI layer will
// render in 4b").
type ShellInitResult struct {
	POSIX string
	Fish  string
}

// ShellInit returns the "wspace" shell function source for POSIX shells
// (bash/zsh/sh) and fish. Jump itself never changes a process's working
// directory (see Jump's doc comment); only this shell-owned function can,
// by wrapping the real binary and running `cd` on the path a `jump`
// invocation resolves.
func ShellInit() ShellInitResult {
	return ShellInitResult{
		POSIX: posixShellFunction,
		Fish:  fishShellFunction,
	}
}

// Both functions invoke the real binary's own "jump" subcommand (which
// only ever prints a path, per Jump's doc comment above) and never a
// separate hidden name — "jump" is a real, directly-invokable top-level
// command in its own right (cli-surface spec: "every listed command is
// invokable"), so the wrapper needs no alias for it.
const posixShellFunction = `wspace() {
  if [ "$1" = "jump" ]; then
    shift
    __wspace_target="$(command wspace jump "$@")" || return $?
    cd "$__wspace_target"
  else
    command wspace "$@"
  fi
}
`

const fishShellFunction = `function wspace
    if test "$argv[1]" = "jump"
        set -e argv[1]
        set -l __wspace_target (command wspace jump $argv)
        or return $status
        cd $__wspace_target
    else
        command wspace $argv
    end
end
`
