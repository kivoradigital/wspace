# wspace

**wspace creates and manages multi-repository workspaces built from git
worktrees.**

A *workspace* is one directory that holds a git worktree of each project you
need for a task, all checked out on the same branch. You get an isolated place
to work on a feature that spans several repositories (an API, a web client, a
shared library...) without stashing, switching branches or otherwise
disturbing your main clones. When the work is merged, you destroy the
workspace; wspace refuses if that would lose uncommitted or unpushed work.

```text
~/code/                         (your main clones, untouched)
├── api/
├── web/
└── shared/

~/workspaces/feature-x/         (wspace create feature-x)
├── .wspace/workspace.yaml      manifest: which repos, which branch
├── api/                        git worktree of ~/code/api    on feature-x
├── web/                        git worktree of ~/code/web    on feature-x
└── shared/                     git worktree of ~/code/shared on feature-x
```

wspace is a single, self-contained Go binary for macOS, Linux and Windows.
It offers three interfaces over the same engine:

| Interface | Command | For |
|-----------|---------|-----|
| CLI | `wspace <command>` | day-to-day use in a terminal |
| MCP server | `wspace mcp serve` | AI coding agents (Claude Code, Codex, Cursor, Gemini CLI, OpenCode...) |
| JSON-lines RPC | `wspace rpc` | desktop clients and other programs |

## Concepts

- **Project**: a main git clone that wspace knows about (for example
  `~/code/api`). Projects are discovered under the context's roots or
  registered explicitly.
- **Context**: a named set of roots, a workspaces folder and default options,
  for example `work` and `personal`. One context is active at a time;
  `--context <name>` overrides it per command.
- **Workspace**: a directory under the context's workspaces folder with one
  worktree per selected project, all on one branch. Its manifest lives in
  `.wspace/workspace.yaml`.
- **Safety first**: destructive operations (destroy, remove a repo, discard,
  delete untracked files) list every reason work would be lost and require
  explicit confirmation. Nothing is ever force-pushed, amended or reset
  behind your back.

## Features

- Create a workspace from every project or a selection (`--project`), on a
  branch of your choice (`--branch`).
- Copy `.env` files from the main clone into each worktree, and re-sync them
  later (`sync-env`).
- Optionally copy `node_modules` into new worktrees, copy-on-write where the
  file system supports it (`--copy-node-modules`).
- Status across every repo: branch, ahead/behind, dirty state.
- Bring every repo up to date with its base branch (merge or `--rebase`),
  aborting cleanly on conflicts.
- Per-repo git operations without leaving wspace: status, log, diff, stash,
  stage, commit, fetch, fast-forward pull and push.
- Run any command in every repo of a workspace (`exec`).
- Add or remove repos from an existing workspace, and repair missing
  worktrees.
- Install an agent skill and register the MCP server with the AI coding
  agents found on your machine.

## Install

Requirements: **git 2.20 or newer**.

**macOS (Homebrew)**

```sh
brew install --cask kivoradigital/tap/wspace
```

**Windows**

```powershell
# Scoop
scoop bucket add kivoradigital https://github.com/kivoradigital/scoop-bucket
scoop install kivoradigital/wspace

# winget (available once Microsoft merges the package)
winget install KivoraDigital.wspace

# Chocolatey (available once chocolatey.org approves the package)
choco install wspace
```

**Linux**: download the `.deb`, `.rpm` or `.tar.gz` for your architecture
from the [latest release](https://github.com/kivoradigital/wspace/releases/latest):

```sh
sudo apt install ./wspace_<version>_linux_amd64.deb   # Debian, Ubuntu
sudo dnf install ./wspace_<version>_linux_amd64.rpm   # Fedora, RHEL
```

**From source** (requires Go, see the version in [go.mod](go.mod)):

```sh
go install github.com/kivoradigital/wspace/cmd/wspace@latest
```

Every release archive and package has a signed build provenance attestation.
Verify a download with the [GitHub CLI](https://cli.github.com):

```sh
gh attestation verify <file> -R kivoradigital/wspace
```

To install a downloaded binary for your user and add it to your `PATH`:

```sh
./wspace install --yes
```

## Quick start

```sh
wspace doctor                    # check git and the setup
wspace context create            # create a context: where projects and workspaces live
wspace project list              # projects registered in the active context
wspace create feature-x          # new workspace with every project, branch named after it
wspace create fix-y --project api --project web --branch fix/y
wspace list                      # every workspace in the context
wspace status feature-x          # branch, ahead/behind and dirty state per repo
wspace update feature-x          # bring each repo up to date with its base branch
wspace exec feature-x -- git log -1 --oneline
wspace repo feature-x api status # inspect or act on one repo of the workspace
wspace destroy feature-x         # remove the worktrees (blocked by unsafe changes)
```

To let `wspace jump <workspace>` change your shell's directory, add the shell
integration to your shell startup file:

```sh
eval "$(wspace shell-init bash)"   # or zsh, sh; fish: wspace shell-init fish | source
```

Run `wspace --help` or `wspace <command> --help` for every command and flag.
Most read commands accept `--json`.

### Commands

| Command | What it does |
|---------|--------------|
| `create`, `add`, `rm` | create a workspace; mount or remove one project's worktree |
| `list`, `status`, `info` | list workspaces; per-repo state; resolved context and options |
| `update` | fetch and merge (or `--rebase`) each repo's base branch |
| `repo` | status, log, show, diff, stash, stage, commit, fetch, pull, push on one repo |
| `exec` | run a command in every repo (no shell involved) |
| `sync-env` | copy `.env` files from the main clones again |
| `repair` | recreate worktrees the manifest declares but are missing |
| `destroy` | tear down a workspace (blocked by unsafe changes unless `--force`) |
| `jump`, `shell-init` | print a workspace path; shell function so `jump` can `cd` |
| `context`, `project` | manage contexts; list registered projects |
| `claim`, `adopt-legacy` | take ownership of orphaned or legacy workspaces |
| `agents` | install the skill and MCP registration into AI agents |
| `mcp serve`, `rpc` | MCP server and JSON-lines RPC over stdin/stdout |
| `doctor`, `install`, `version` | health check, user install, version and update check |

## AI agents (MCP)

Install the bundled agent skill and register the MCP server with the agents
detected on your machine:

```sh
wspace agents status
wspace agents install --agent claude-code --mcp
```

Supported agents: Claude Code, Claude Desktop, Codex, Cursor, Gemini CLI and
OpenCode. Read-only tools change nothing; destructive tools always require
explicit confirmation and never force on their own. See
[docs/mcp.md](docs/mcp.md).

## Configuration

| Item | Location |
|------|----------|
| Config root (macOS, Linux) | `$XDG_CONFIG_HOME/wspace`, or `~/.config/wspace` |
| Config root (Windows) | `%APPDATA%\wspace` |
| Contexts | `<config root>/contexts/<name>/config.yaml` |
| Cache (update check) | `$XDG_CACHE_HOME/wspace` or `~/.cache/wspace`; `%LOCALAPPDATA%\wspace` on Windows |
| Workspace manifest | `<workspace>/.wspace/workspace.yaml` |

`WSPACE_CONFIG_HOME` replaces the config root entirely. Use it to keep a
development or test setup apart from your real configuration.

## Development

### Requirements

- Go, at the version declared in [go.mod](go.mod)
- git 2.20 or newer
- Optional: [golangci-lint](https://golangci-lint.run) v2 for `make lint`,
  Node.js for `scripts/run-dev.mjs`

### Build, run and test

```sh
make build            # dist/wspace for this machine (CGO_ENABLED=0)
make dist             # cross-builds for darwin, linux and windows
make test             # go test ./...
make test-short       # go test -short ./...
make test-e2e         # builds the real binary and drives it against real git repos
make vet              # go vet ./...
make fmt              # gofmt -l . (must print nothing)
make lint             # golangci-lint run (uses .golangci.yml)
make release-vars     # print the version data a build would embed
```

Run the CLI from the checkout without touching your real configuration:

```sh
node scripts/run-dev.mjs list          # go run ./cmd/wspace with WSPACE_CONFIG_HOME=~/.config/wspace-dev
WSPACE_CONFIG_HOME=/tmp/wspace-dev go run ./cmd/wspace doctor
```

Before opening a pull request, run at least `go test ./...`,
`go vet ./...`, `GOOS=windows go vet ./...`, `gofmt -l .` and
`golangci-lint run`. CI runs the tests on Linux, macOS and Windows.

### Build-time data

Version information and the repository the update check reads are injected
with `-ldflags` (see the [Makefile](Makefile)): `Version`, `Commit`, `Date`,
`RepoOwner` and `RepoName` in `internal/buildinfo`. A plain `go build` or
`go install` reports itself as a development build and has no update check.

Desktop apps that embed the CLI build it with
`make build BUNDLED_BY="App Name" VERSION=<cli version>`. A bundled CLI never
checks for an update of its own; the app updates it. See
[docs/bundling.md](docs/bundling.md).

### Architecture

wspace follows a hexagonal (ports and adapters) architecture. The import
rules below are enforced by tests in `internal/archtest`, so a violation
fails `go test ./...`.

```text
cmd/wspace            composition root: wires adapters into the app
internal/domain       pure model and rules; no I/O, no third-party imports
internal/ports        interfaces the app needs (git, file system, prompts...)
internal/app          use cases; depends on domain and ports only
internal/adapters/*   implementations of the ports (git, fsstore, configstore,
                      agenthost, treecopy, winenv, githubrelease...)
internal/engine       non-interactive facade over the use cases
internal/cli          cobra commands; renders through internal/messages
internal/rpc          JSON-lines protocol over the engine (wspace rpc)
internal/mcpserver    MCP tools over the engine (wspace mcp serve)
internal/messages     every user-facing string, keyed and testable
skills/               the agent skill embedded in the binary
```

Conventions worth knowing before you change code:

- User-facing text lives in `internal/messages`; a test rejects inline
  strings in `internal/cli`.
- Errors carry a stable code (`internal/domain`) that the CLI, RPC and MCP
  surfaces all report. Clients branch on codes, never on text.
- Git always runs as a direct subprocess with an allowlisted environment
  and the C locale, so its output parses the same everywhere; wspace never
  goes through a user shell.
- Behavior is specified in [openspec/specs](openspec/specs); update the spec
  together with the code when behavior changes.
- Every `.go` file starts with the SPDX license header.

### Project layout

| Path | Contents |
|------|----------|
| `cmd/wspace` | the `main` package |
| `internal/` | engine, interfaces and adapters (see above) |
| `skills/` | the `wspace-workspaces` agent skill |
| `docs/` | [MCP server](docs/mcp.md), [RPC contract](docs/rpc-contract.md) and [bundling in a desktop app](docs/bundling.md) |
| `openspec/specs/` | behavior specifications per capability |
| `packaging/` | Linux `.deb`, Windows installer and winget templates, icons, [release checklist](packaging/RELEASE_CHECKLIST.md) |
| `scripts/` | development helpers |
| `.goreleaser.yaml` | release builds and package manifests (publishing disabled until the first public release) |

## Desktop apps

Official desktop apps are built by Kivora Digital on top of this CLI. They
use the same engine through `wspace rpc` and `wspace mcp serve`.

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md): every
commit needs a Developer Certificate of Origin sign-off (`git commit -s`), and
commit messages follow Conventional Commits. Please also read the
[Code of Conduct](CODE_OF_CONDUCT.md), and report security issues privately
as described in [SECURITY.md](SECURITY.md).

## License

Copyright 2026 Kivora Digital S.L.

Licensed under the [Apache License, Version 2.0](LICENSE). See
[NOTICE](NOTICE) and [THIRD_PARTY_LICENSES](THIRD_PARTY_LICENSES).

"wspace" and the wspace logo are trademarks of Kivora Digital S.L.; see
[TRADEMARKS.md](TRADEMARKS.md).
