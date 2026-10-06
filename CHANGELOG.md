# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- Packaging definitions: GoReleaser configuration, Inno Setup script,
  Debian package script and winget manifest templates.
