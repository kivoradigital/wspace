MODULE := github.com/kivoradigital/wspace

# VERSION/COMMIT/DATE/REPO_OWNER/REPO_NAME are build-time data only
# (design.md §11): nothing in the codebase hardcodes a repository, so the
# update check is told which releases to read rather than assuming any.
# VERSION stays `dev` until a tagged release overrides it, which is what
# makes an untagged local build report itself as a development build.
VERSION    ?= dev
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE       ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
REPO_OWNER ?= kivoradigital
REPO_NAME  ?= wspace
# BUNDLED_BY is the display name of a desktop app that embeds and builds this
# CLI (`make build BUNDLED_BY="App Name"`). A bundled CLI is updated only with
# that app, so it never checks for an update of its own. Package-manager
# builds (GoReleaser) leave it empty.
BUNDLED_BY ?=

LDFLAGS := -s -w \
	-X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
	-X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(MODULE)/internal/buildinfo.Date=$(DATE) \
	-X $(MODULE)/internal/buildinfo.RepoOwner=$(REPO_OWNER) \
	-X $(MODULE)/internal/buildinfo.RepoName=$(REPO_NAME) \
	-X '$(MODULE)/internal/buildinfo.BundledBy=$(BUNDLED_BY)'

.PHONY: build test test-short test-e2e vet fmt lint clean dist release-vars

# This module builds the CLI only; every platform ships the CLI alone.
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/wspace ./cmd/wspace

test:
	go test ./...

test-short:
	go test -short ./...

# test-e2e exists because every other test in this module injects a fake
# into internal/app or internal/cli and asserts on rendered strings — none
# of them ever compiles cmd/wspace and runs it, so a whole class of defect
# (nil composition-root dependencies, subprocess output that never reaches
# the terminal, a cobra flag error rendered as a generic sentence) can ship
# behind a fully green `make test`. internal/e2e builds the real wspace binary
# once and drives it as a subprocess against real git repositories in
# temporary directories, with an isolated HOME/XDG_CONFIG_HOME so a
# developer's own configuration is never touched. It is its own target,
# not part of `test`, because it shells out to `go build` and real `git`
# and is meaningfully slower than the rest of the suite.
test-e2e:
	go test -tags e2e ./internal/e2e/...

vet:
	go vet ./...

fmt:
	gofmt -l .

lint:
	golangci-lint run

clean:
	rm -rf dist

# dist cross-builds the CLI (only) for every packaging-distribution target
# platform (design.md §11). The CLI is pure Go (CGO_ENABLED=0), so one host
# builds every platform.
dist:
	mkdir -p dist
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/wspace-darwin-amd64     ./cmd/wspace
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/wspace-darwin-arm64     ./cmd/wspace
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/wspace-linux-amd64      ./cmd/wspace
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/wspace-linux-arm64      ./cmd/wspace
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/wspace-windows-amd64.exe ./cmd/wspace

# release-vars prints the exact VERSION/COMMIT/DATE/REPO_OWNER/REPO_NAME/BUNDLED_BY
# this Makefile would inject, so a release script (or a human) can confirm
# them — especially REPO_OWNER/REPO_NAME — before running `make dist`.
release-vars:
	@echo "VERSION=$(VERSION)"
	@echo "COMMIT=$(COMMIT)"
	@echo "DATE=$(DATE)"
	@echo "REPO_OWNER=$(REPO_OWNER)"
	@echo "REPO_NAME=$(REPO_NAME)"
	@echo "BUNDLED_BY=$(BUNDLED_BY)"
