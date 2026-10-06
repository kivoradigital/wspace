// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package buildinfo carries the values -ldflags -X injects at build time
// (design.md §11): a version/commit/date triple, and the GitHub repository
// coordinates the update checker queries. It is a leaf package (design.md
// §1, R2): data and pure lookups only, no I/O — every field has a safe
// zero-value default so a plain `go build` (no -ldflags at all) still
// produces a working, if unbranded, binary.
package buildinfo

import "github.com/kivoradigital/wspace/internal/domain"

// All five are set via -ldflags -X (see the Makefile's LDFLAGS). Every field
// has a safe zero-value default: an unbuilt/unflagged binary is "dev",
// commit "none", date "unknown", and no repository coordinates at all.
var (
	Version   = "dev"
	Commit    = "none"
	Date      = "unknown"
	RepoOwner = ""
	RepoName  = ""
)

// Coordinates returns the injected repository coordinates. ok is false when
// either field was never set (a local, unbranded build) — the one signal
// app.CheckForUpdate needs to skip the network call entirely rather than
// querying repository coordinates that belong to nobody (design.md §11:
// "Repository coordinates are build-time data, never logic. An unbranded
// local build reports 'update check unavailable' instead of querying
// someone else's repository").
func Coordinates() (domain.RepoCoordinates, bool) {
	if RepoOwner == "" || RepoName == "" {
		return domain.RepoCoordinates{}, false
	}
	return domain.RepoCoordinates{Owner: RepoOwner, Repo: RepoName}, true
}

// IsRelease reports whether this binary was built from a tagged release
// rather than a plain `go build` with no -ldflags at all.
func IsRelease() bool {
	return Version != "dev"
}
