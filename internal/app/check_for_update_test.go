// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestCheckForUpdate_NeverBlocksOtherCommands covers tasks.md 5.12. The
// "never blocks other commands" contract this test proves has two parts:
// (1) the structural part is that ports.ReleaseChecker only ever appears in
// CheckForUpdateDeps — Deps (the bundle CreateWorkspace, Status, List,
// Doctor and every other phase-4a use case takes) has no Checker field at
// all, so nothing outside a direct CheckForUpdate call can ever reach one;
// (2) the behavioral part, which this test actually exercises, is that
// CheckForUpdate calls the checker exactly once per invocation — never
// zero (it would silently skip a real check) and never more than once (it
// would defeat the adapter's own caching).
func TestCheckForUpdate_NeverBlocksOtherCommands(t *testing.T) {
	checker := portstest.NewFakeReleaseChecker()
	checker.Response = domain.ReleaseInfo{Tag: "v2.0.0", URL: "https://example.invalid/v2.0.0"}

	got, err := app.CheckForUpdate(context.Background(), app.CheckForUpdateDeps{Checker: checker}, app.CheckForUpdateInput{
		Coordinates:    domain.RepoCoordinates{Owner: "acme", Repo: "widget"},
		CurrentVersion: "v1.0.0",
	})
	if err != nil {
		t.Fatalf("CheckForUpdate() unexpected error: %v", err)
	}
	if checker.Calls != 1 {
		t.Fatalf("checker.Calls = %d, want exactly 1", checker.Calls)
	}
	if !got.Available || got.LatestTag != "v2.0.0" || got.Unavailable {
		t.Fatalf("CheckForUpdate() = %+v, want Available=true LatestTag=v2.0.0 Unavailable=false", got)
	}
}

// TestCheckForUpdate_UnbrandedBuildNeverCallsChecker covers the "unbranded
// build" outcome design.md §11 requires: when Coordinates is the zero
// value (buildinfo.Coordinates()'s ok=false case), CheckForUpdate must
// report Unavailable without ever querying a repository nobody chose.
func TestCheckForUpdate_UnbrandedBuildNeverCallsChecker(t *testing.T) {
	checker := portstest.NewFakeReleaseChecker()
	checker.Response = domain.ReleaseInfo{Tag: "v2.0.0"}

	got, err := app.CheckForUpdate(context.Background(), app.CheckForUpdateDeps{Checker: checker}, app.CheckForUpdateInput{
		CurrentVersion: "v1.0.0",
	})
	if err != nil {
		t.Fatalf("CheckForUpdate() unexpected error: %v", err)
	}
	if checker.Calls != 0 {
		t.Fatalf("checker.Calls = %d, want 0 for an unbranded build (no coordinates)", checker.Calls)
	}
	if !got.Unavailable {
		t.Fatalf("CheckForUpdate() = %+v, want Unavailable=true for an unbranded build", got)
	}
}

// TestCheckForUpdate_UpToDate covers the "current version already latest"
// outcome: Available must be false, not merely absent.
func TestCheckForUpdate_UpToDate(t *testing.T) {
	checker := portstest.NewFakeReleaseChecker()
	checker.Response = domain.ReleaseInfo{Tag: "v1.0.0"}

	got, err := app.CheckForUpdate(context.Background(), app.CheckForUpdateDeps{Checker: checker}, app.CheckForUpdateInput{
		Coordinates:    domain.RepoCoordinates{Owner: "acme", Repo: "widget"},
		CurrentVersion: "v1.0.0",
	})
	if err != nil {
		t.Fatalf("CheckForUpdate() unexpected error: %v", err)
	}
	if got.Available || got.Unavailable {
		t.Fatalf("CheckForUpdate() = %+v, want Available=false Unavailable=false when already up to date", got)
	}
}

// TestCheckForUpdate_ForwardsUnavailableFromChecker covers the network-
// failure/rate-limited outcome surfacing through unchanged: when the
// checker itself cannot determine anything (no cache, offline), the app
// layer must forward Unavailable rather than compute a false "up to date".
func TestCheckForUpdate_ForwardsUnavailableFromChecker(t *testing.T) {
	checker := portstest.NewFakeReleaseChecker()
	checker.Response = domain.ReleaseInfo{Unavailable: true}

	got, err := app.CheckForUpdate(context.Background(), app.CheckForUpdateDeps{Checker: checker}, app.CheckForUpdateInput{
		Coordinates:    domain.RepoCoordinates{Owner: "acme", Repo: "widget"},
		CurrentVersion: "v1.0.0",
	})
	if err != nil {
		t.Fatalf("CheckForUpdate() unexpected error: %v", err)
	}
	if !got.Unavailable || got.Available {
		t.Fatalf("CheckForUpdate() = %+v, want Unavailable=true Available=false", got)
	}
}
