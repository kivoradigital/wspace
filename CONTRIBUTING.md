# Contributing to wspace

Thank you for your interest in contributing. This document explains how to
build and test the project and what we need from every contribution.

By participating you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting started

Requirements: Go (the version in `go.mod`) and git 2.20 or newer.

```sh
make build      # builds dist/wspace
make test       # go test ./...
make test-short # go test -short ./...
make test-e2e   # end-to-end tests: builds the real binary and drives it
make vet        # go vet ./...
make fmt        # gofmt -l . (must print nothing)
make lint       # golangci-lint run (uses .golangci.yml)
```

You can also run `go test ./...` directly. To run the CLI from a checkout
without touching your own configuration, set `WSPACE_CONFIG_HOME` to a
scratch directory (or use `node scripts/run-dev.mjs <args>`).

Every new `.go` file starts with the SPDX header:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.
```

## How contributions work

Nobody outside the maintainers can push to this repository. Every change,
including the maintainers' own, reaches `main` through a pull request:

1. **Issue first.** For anything beyond a typo or an obvious small fix, open
   or comment on an issue and wait until a maintainer agrees on the approach.
   Pull requests for unagreed features or large refactors may be closed
   without a detailed review.
2. **Fork and branch.** Fork the repository and create a branch from `main`
   in your fork (for example `fix/status-json-dirty-state`).
3. **Make the change.** Keep it focused on one thing, with tests.
4. **Open a pull request** against `main` and fill in the template. Link the
   issue (`Closes #123`).
5. **Checks.** CI must pass on Linux, macOS and Windows, plus lint and the DCO
   check. For contributors outside the organization, a maintainer has to
   approve each CI run before it starts.
6. **Review and merge.** A maintainer reviews the change and may ask for
   updates. Only maintainers merge, always as a squash merge.

`main` is protected: no direct pushes, no force pushes, no deletion, linear
history, and every required check must pass. Release tags (`v*`) can only be
created by maintainers.

## How decisions are made

wspace is maintained by Kivora Digital S.L. Maintainers decide the roadmap
and whether a change fits the project. When deciding, we weigh:

- **Scope**: wspace manages git worktree workspaces across repositories. We
  favor changes that make that safer, clearer or more portable, and decline
  features that belong in git itself or in another tool.
- **Safety**: wspace must never lose work. Changes that could discard
  changes, force-push, rewrite history or skip hooks without explicit user
  confirmation will not be accepted.
- **Portability**: behavior must work on macOS, Linux and Windows.
- **Stable interfaces**: the CLI flags, `--json` output, error codes, the
  RPC contract and the MCP tools are public interfaces. Breaking changes need
  a strong reason and a migration note.
- **Maintenance cost**: every feature has to be maintained for years.

A declined proposal is not a judgment on you or your work. You are always free
to maintain a fork under the terms of the license (see
[TRADEMARKS.md](TRADEMARKS.md) for naming).

## Pull request requirements

- Keep pull requests focused; include tests for behavior changes.
- Make sure `go test ./...`, `go vet ./...`, `GOOS=windows go vet ./...`,
  `gofmt -l .` and `golangci-lint run` pass.
- Keep the architecture rules in `internal/archtest` green and put
  user-facing text in `internal/messages`.
- Update the specification in `openspec/specs` and the docs when behavior
  changes.
- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit
  messages, for example `feat(cli): add --json to status` or
  `fix(engine): keep stash on conflict`. The pull request title becomes the
  squash commit title, so it must follow the same format.
- If you used AI tools to write the change, you are still responsible for
  every line: understand it, test it, and make sure you have the right to
  submit it under the DCO.

## Developer Certificate of Origin

We do not use a Contributor License Agreement (CLA). Instead, every commit
must be signed off under the
[Developer Certificate of Origin 1.1](https://developercertificate.org),
which certifies that you wrote the change or otherwise have the right to
submit it under the project's license.

Sign off by adding the `-s` flag when you commit:

```sh
git commit -s -m "fix(cli): handle empty context list"
```

This adds a `Signed-off-by: Your Name <you@example.com>` trailer that must
match the commit author. Commits without a sign-off cannot be merged.

## License of contributions

wspace is licensed under the [Apache License 2.0](LICENSE). As stated in
Section 5 of that license, any contribution you intentionally submit for
inclusion in the project is licensed under the Apache License 2.0, without
any additional terms or conditions.
