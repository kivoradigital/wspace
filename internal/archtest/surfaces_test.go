// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package archtest

import "testing"

// TestCheckEdge_MachineSurfaces pins the import rules for the
// non-interactive engine facade and the two machine-facing surfaces built
// on it (internal/engine, internal/rpc, internal/mcpserver).
func TestCheckEdge_MachineSurfaces(t *testing.T) {
	const mod = "github.com/kivoradigital/wspace"
	tests := []struct {
		pkg, imp string
		allowed  bool
	}{
		{"internal/engine", mod + "/internal/app", true},
		{"internal/engine", mod + "/internal/ports", true},
		{"internal/engine", mod + "/internal/domain", true},
		{"internal/engine", mod + "/internal/messages", true},
		{"internal/engine", mod + "/internal/adapters/git", false},
		{"internal/engine", mod + "/internal/cli", false},
		{"internal/engine", mod + "/internal/rpc", false},
		{"internal/engine", "github.com/spf13/cobra", false},

		{"internal/rpc", mod + "/internal/engine", true},
		{"internal/rpc", mod + "/internal/messages", true},
		{"internal/rpc", mod + "/internal/app", false},
		{"internal/rpc", mod + "/internal/ports", false},
		{"internal/rpc", mod + "/internal/adapters/fsstore", false},
		{"internal/rpc", mod + "/internal/mcpserver", false},
		{"internal/rpc", "github.com/spf13/cobra", false},

		{"internal/mcpserver", mod + "/internal/engine", true},
		{"internal/mcpserver", mod + "/internal/messages", true},
		{"internal/mcpserver", "github.com/modelcontextprotocol/go-sdk/mcp", true},
		{"internal/mcpserver", mod + "/internal/app", false},
		{"internal/mcpserver", mod + "/internal/ports", false},
		{"internal/mcpserver", mod + "/internal/rpc", false},
		{"internal/mcpserver", mod + "/internal/cli", false},
		{"internal/mcpserver", "github.com/spf13/cobra", false},

		// cli stays on app/domain/messages: the rpc and mcp servers reach
		// it only as functions injected by cmd/wspace.
		{"internal/cli", mod + "/internal/engine", false},
		{"internal/cli", mod + "/internal/rpc", false},
	}
	for _, tt := range tests {
		t.Run(tt.pkg+" -> "+tt.imp, func(t *testing.T) {
			violation := checkEdge(tt.pkg, tt.imp, mod)
			if tt.allowed && violation != "" {
				t.Fatalf("edge rejected: %s", violation)
			}
			if !tt.allowed && violation == "" {
				t.Fatal("edge allowed, want a violation")
			}
		})
	}

	// Test files of the surfaces may build an engine over portstest fakes.
	for _, pkg := range []string{"internal/rpc", "internal/mcpserver"} {
		for _, imp := range []string{mod + "/internal/ports", mod + "/internal/ports/portstest", mod + "/internal/domain"} {
			if v := checkTestEdge(pkg, imp, mod); v != "" {
				t.Fatalf("test edge %s -> %s rejected: %s", pkg, imp, v)
			}
		}
	}
}
