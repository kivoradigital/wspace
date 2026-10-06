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

## Pull requests

- Open an issue first for anything larger than a small fix, so we can agree
  on the approach.
- Keep pull requests focused; include tests for behavior changes.
- Make sure `go test ./...`, `go vet ./...`, `gofmt -l .` and
  `golangci-lint run` pass.
- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit
  messages, for example `feat(cli): add --json to status` or
  `fix(engine): keep stash on conflict`.

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
