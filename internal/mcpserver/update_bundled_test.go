// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/mcpserver"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestCheckUpdate_Bundled: check_update reports the bundling app in both
// the structured result and the text summary, and never calls the checker.
func TestCheckUpdate_Bundled(t *testing.T) {
	checker := portstest.NewFakeReleaseChecker()
	eng := engine.New(engine.Deps{Version: "0.1.0", BundledBy: "Wspace Dev", Checker: checker})
	serverT, clientT := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := mcpserver.New(eng, "0.1.0").Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.9"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "check_update"})
	if err != nil || res.IsError {
		t.Fatalf("check_update = %+v, %v", res, err)
	}
	if got := structured(t, res); !strings.Contains(got, `"bundledBy":"Wspace Dev"`) || !strings.Contains(got, `"unavailable":false`) {
		t.Fatalf("structured = %s, want bundledBy and unavailable=false", got)
	}
	if got := text(res); !strings.Contains(got, "bundled with Wspace Dev") {
		t.Fatalf("text = %q, want it to name the bundling app", got)
	}
	if checker.Calls != 0 {
		t.Fatalf("checker.Calls = %d, want 0", checker.Calls)
	}
}

// TestCheckUpdate_NotBundledOmitsField keeps the normal result unchanged.
func TestCheckUpdate_NotBundledOmitsField(t *testing.T) {
	f := newFixture(t)
	if got := structured(t, f.call(t, "check_update", nil)); strings.Contains(got, "bundledBy") {
		t.Fatalf("structured = %s, want no bundledBy for a normal build", got)
	}
}
