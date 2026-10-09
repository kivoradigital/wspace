# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Release archives for macOS are named `wspace_<version>_macos_<arch>.tar.gz`
  (previously `darwin`), and every release starts with a download guide.

## [0.2.1] - 2026-10-06

### Fixed

- Releases always publish build provenance attestations, even when
  chocolatey.org rejects the package push. v0.2.0 shipped without
  attestations; v0.2.1 has the same code with attestations.

## [0.2.0] - 2026-10-06

### Added

- `BUNDLED_BY` build flag (`make build BUNDLED_BY="App Name"`) for desktop
  apps that embed the CLI. A bundled CLI never checks for updates on its own:
  `wspace version` adds a second line `(bundled with App Name)`,
  `wspace version --check` says it updates with that app, and
  `engine.checkUpdate` and the MCP `check_update` tool return an additive
  `bundledBy` field. See `docs/bundling.md`.

## [0.1.0] - 2026-10-06

### Added

- Initial open-source release of wspace under the Apache License 2.0.
- CLI to create, inspect, update, repair and destroy multi-repository
  workspaces built from git worktrees, organized in contexts.
- Per-repository operations (`wspace repo`): status, log, diff, stash,
  fetch, fast-forward pull, push, stage, unstage, discard and commit.
- Shell integration (`wspace shell-init`) so `wspace jump` changes directory.
- `wspace install` for a per-user install with PATH integration on Linux,
  macOS and Windows.
- MCP server (`wspace mcp serve`) exposing workspace tools to AI coding
  agents, plus `wspace agents` to install the bundled agent skill and
  register the MCP server.
- JSON-lines RPC engine (`wspace rpc`) for desktop clients, documented in
  `docs/rpc-contract.md`.
- Update check against GitHub releases.
- `wspace create --copy-node-modules` copies `node_modules` into new
  worktrees, copy-on-write where the file system supports it.
- Releases for macOS, Linux and Windows (amd64 and arm64): archives, `.deb`,
  `.rpm`, a Homebrew cask, Scoop and winget manifests and a Chocolatey
  package, all with signed build provenance attestations.

### Fixed

- Stash diffs no longer contain garbage path prefixes with git 2.55.
- On Windows: agent CLIs installed as `.cmd` shims are found and run, commit
  hooks are detected, cancelled git commands no longer hang, and discarding
  changes keeps the newest backup when file times tie.

[Unreleased]: https://github.com/kivoradigital/wspace/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/kivoradigital/wspace/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/kivoradigital/wspace/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/kivoradigital/wspace/releases/tag/v0.1.0
