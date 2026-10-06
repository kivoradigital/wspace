// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package buildinfo_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/buildinfo"
)

// TestBuildinfo_CoordinatesOkFalseWhenUnset covers tasks.md 5.3 (design.md
// §11: "An unbranded local build reports 'update check unavailable' instead
// of querying someone else's repository") — this is the guard that makes
// that possible: Coordinates() must tell the caller when the coordinates
// were never injected, not silently hand back an empty-but-valid pair.
func TestBuildinfo_CoordinatesOkFalseWhenUnset(t *testing.T) {
	origOwner, origName := buildinfo.RepoOwner, buildinfo.RepoName
	t.Cleanup(func() { buildinfo.RepoOwner, buildinfo.RepoName = origOwner, origName })

	buildinfo.RepoOwner, buildinfo.RepoName = "", ""
	if _, ok := buildinfo.Coordinates(); ok {
		t.Fatal("Coordinates() ok = true with both fields unset, want false")
	}

	buildinfo.RepoOwner, buildinfo.RepoName = "someone", ""
	if _, ok := buildinfo.Coordinates(); ok {
		t.Fatal("Coordinates() ok = true with only RepoOwner set, want false")
	}

	buildinfo.RepoOwner, buildinfo.RepoName = "", "repo"
	if _, ok := buildinfo.Coordinates(); ok {
		t.Fatal("Coordinates() ok = true with only RepoName set, want false")
	}
}

// TestBuildinfo_CoordinatesOkTrueWhenBothSet proves the converse: real
// injected coordinates (as -ldflags would set at build time) are reported
// verbatim.
func TestBuildinfo_CoordinatesOkTrueWhenBothSet(t *testing.T) {
	origOwner, origName := buildinfo.RepoOwner, buildinfo.RepoName
	t.Cleanup(func() { buildinfo.RepoOwner, buildinfo.RepoName = origOwner, origName })

	buildinfo.RepoOwner, buildinfo.RepoName = "an-owner", "a-repo"
	got, ok := buildinfo.Coordinates()
	if !ok {
		t.Fatal("Coordinates() ok = false with both fields set, want true")
	}
	if got.Owner != "an-owner" || got.Repo != "a-repo" {
		t.Fatalf("Coordinates() = %+v, want Owner=an-owner Repo=a-repo", got)
	}
}

// TestBuildinfo_IsRelease covers the "Version != \"dev\"" contract design.md
// §11 states verbatim.
func TestBuildinfo_IsRelease(t *testing.T) {
	orig := buildinfo.Version
	t.Cleanup(func() { buildinfo.Version = orig })

	buildinfo.Version = "dev"
	if buildinfo.IsRelease() {
		t.Fatal("IsRelease() = true for Version = \"dev\", want false")
	}

	buildinfo.Version = "v1.0.0"
	if !buildinfo.IsRelease() {
		t.Fatal("IsRelease() = false for Version = \"v1.0.0\", want true")
	}
}
