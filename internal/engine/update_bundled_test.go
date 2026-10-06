// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestCheckForUpdate_Bundled: a bundled engine reports who bundles it and
// never calls the checker, even with real coordinates injected.
func TestCheckForUpdate_Bundled(t *testing.T) {
	checker := portstest.NewFakeReleaseChecker()
	checker.Response = domain.ReleaseInfo{Tag: "v9.9.9"}
	eng := engine.New(engine.Deps{
		Checker: checker, Version: "0.1.0", BundledBy: "Wspace Dev",
		Coordinates: domain.RepoCoordinates{Owner: "acme", Repo: "widget"},
	})

	got, err := eng.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v", err)
	}
	want := engine.UpdateCheckResult{CurrentVersion: "0.1.0", BundledBy: "Wspace Dev"}
	if got != want {
		t.Fatalf("CheckForUpdate() = %+v, want %+v", got, want)
	}
	if checker.Calls != 0 {
		t.Fatalf("checker.Calls = %d, want 0", checker.Calls)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"bundledBy":"Wspace Dev"`) {
		t.Fatalf("json = %s, want bundledBy", b)
	}
}

// TestCheckForUpdate_BundledWithoutChecker: the bundled answer does not
// depend on a checker being wired at all.
func TestCheckForUpdate_BundledWithoutChecker(t *testing.T) {
	eng := engine.New(engine.Deps{Version: "0.1.0", BundledBy: "Wspace Dev"})
	got, err := eng.CheckForUpdate(context.Background())
	if err != nil || got.BundledBy != "Wspace Dev" || got.Unavailable || got.Available {
		t.Fatalf("CheckForUpdate() = %+v, %v, want bundled, not available, not unavailable", got, err)
	}
}

// TestCheckForUpdate_NotBundledOmitsField: the field is additive and
// omitted from the wire for a normal build.
func TestCheckForUpdate_NotBundledOmitsField(t *testing.T) {
	eng := engine.New(engine.Deps{Version: "0.1.0", Checker: portstest.NewFakeReleaseChecker()})
	got, err := eng.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), "bundledBy") {
		t.Fatalf("json = %s, want no bundledBy for a normal build", b)
	}
}
