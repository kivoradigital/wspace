# Update Check Specification

## Purpose

Non-blocking release awareness against the project's own GitHub releases, surfaced from both the CLI and the engine's RPC surface for desktop clients.

## Requirements

### Requirement: GitHub releases lookup

The system MUST query the project's GitHub releases endpoint for the latest release, using repository coordinates (owner/repo) injected at build time rather than hardcoded in application logic.

#### Scenario: Build-time coordinates used

- GIVEN the binary was built with owner/repo coordinates `X/Y`
- WHEN an update check runs
- THEN the request targets `X/Y`'s releases endpoint

### Requirement: Result caching

The system MUST cache the result of a successful update check and MUST reuse the cached result to avoid issuing a new request on every invocation.

#### Scenario: Cached result reused within window

- GIVEN an update check succeeded and was cached moments ago
- WHEN another command triggers an update check shortly after
- THEN the cached result is used and no new network request is issued

### Requirement: Graceful offline degradation

An update check MUST NOT block or fail any command when the network is unavailable or the request errors; it MUST degrade silently and leave prior cached state, if any, untouched.

#### Scenario: Offline does not block the command

- GIVEN the network is unreachable
- WHEN a command that triggers an update check runs
- THEN the command still completes successfully, with the update check silently skipped

### Requirement: About surface in CLI and RPC

The system MUST expose the current version and the last known update-check result through both a CLI `version`/About output and the `engine.checkUpdate` RPC method.

#### Scenario: CLI About shows version and update state

- GIVEN a cached "update available" result
- WHEN `wspace version` (or an equivalent About output) runs
- THEN it shows the current version and that an update is available

#### Scenario: RPC mirrors the same state

- GIVEN the same cached result
- WHEN a client calls `engine.checkUpdate`
- THEN it reports the same version and update-available state as the CLI

### Requirement: Bundled builds never check for updates

A binary built with a non-empty `BundledBy` (the display name of a desktop app that embeds and builds the CLI) MUST NOT query the releases endpoint. It MUST report that it is bundled with that app, neither available nor unavailable, on the CLI and through `engine.checkUpdate` (`bundledBy`). The first line of `wspace version` MUST stay `wspace version <version>` in every build. A build with an empty `BundledBy` MUST behave exactly as before.

#### Scenario: Bundled CLI reports its app instead of checking

- GIVEN the binary was built with `BundledBy` set to `App`
- WHEN `wspace version --check` runs or a client calls `engine.checkUpdate`
- THEN no network request is issued, the CLI prints that wspace is bundled with `App` and updates with it, and the RPC result carries `bundledBy: "App"` with `available` and `unavailable` both false

#### Scenario: Version line stays parseable

- GIVEN the binary was built with `BundledBy` set to `App`
- WHEN `wspace version` runs
- THEN the first line is exactly `wspace version <version>` and the bundled note is on the second line
