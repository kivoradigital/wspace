// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package rpc_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
	"github.com/kivoradigital/wspace/internal/rpc"
)

func checkUpdateOver(t *testing.T, deps engine.Deps) string {
	t.Helper()
	srv := rpc.NewServer(engine.New(deps), deps.Version)
	var out bytes.Buffer
	if err := srv.Serve(context.Background(), strings.NewReader(`{"id":"1","method":"engine.checkUpdate"}`+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// TestCheckUpdate_BundledByField: engine.checkUpdate carries the additive
// bundledBy field for a bundled build and omits it otherwise.
func TestCheckUpdate_BundledByField(t *testing.T) {
	checker := portstest.NewFakeReleaseChecker()
	out := checkUpdateOver(t, engine.Deps{
		Version: "0.1.0", BundledBy: "Wspace Dev", Checker: checker,
		Coordinates: domain.RepoCoordinates{Owner: "acme", Repo: "widget"},
	})
	for _, want := range []string{`"bundledBy":"Wspace Dev"`, `"available":false`, `"unavailable":false`} {
		if !strings.Contains(out, want) {
			t.Fatalf("out = %s, want it to contain %s", out, want)
		}
	}
	if checker.Calls != 0 {
		t.Fatalf("checker.Calls = %d, want 0", checker.Calls)
	}

	if out := checkUpdateOver(t, engine.Deps{Version: "0.1.0"}); strings.Contains(out, "bundledBy") {
		t.Fatalf("out = %s, want no bundledBy for a normal build", out)
	}
}
