// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"context"
	"io"
	"strings"
	"testing"
)

// TestCLI_ContextList_JSONContract pins `context list --json`.
func TestCLI_ContextList_JSONContract(t *testing.T) {
	fx := newFixture(t)
	stdout, stderr, code := run(fx.RT, "", "context", "list", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	compareGolden(t, "context_list.json.golden", stdout)
}

// TestCLI_ProjectList_JSONContract pins `project list --json`.
func TestCLI_ProjectList_JSONContract(t *testing.T) {
	fx := newFixture(t)
	stdout, stderr, code := run(fx.RT, "", "project", "list", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	compareGolden(t, "project_list.json.golden", stdout)
}

func TestCLI_ProjectList_Human(t *testing.T) {
	fx := newFixture(t)
	stdout, _, code := run(fx.RT, "", "project", "list")
	if code != 0 || !strings.Contains(stdout, "svc") || !strings.Contains(stdout, "/fixture/src/svc") {
		t.Fatalf("exit=%d stdout=%q, want the svc project and its source dir", code, stdout)
	}
}

// echoServer is an injected ServeRPC/ServeMCP stand-in: it copies stdin to
// stdout, proving the command hands the process's own streams straight to
// the server and writes nothing else on stdout.
func echoServer(called *bool) func(context.Context, io.Reader, io.Writer) error {
	return func(_ context.Context, in io.Reader, out io.Writer) error {
		*called = true
		_, err := io.Copy(out, in)
		return err
	}
}

func TestCLI_RPCAndMCPServe_HandStdioToTheInjectedServer(t *testing.T) {
	tests := []struct {
		name string
		args []string
		set  func(fx *fixture, called *bool)
	}{
		{"rpc", []string{"rpc"}, func(fx *fixture, c *bool) { fx.RT.ServeRPC = echoServer(c) }},
		{"mcp serve", []string{"mcp", "serve"}, func(fx *fixture, c *bool) { fx.RT.ServeMCP = echoServer(c) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			var called bool
			tt.set(fx, &called)
			stdout, stderr, code := run(fx.RT, "{\"id\":\"1\"}\n", tt.args...)
			if code != 0 || !called {
				t.Fatalf("exit=%d called=%v stderr=%q", code, called, stderr)
			}
			if stdout != "{\"id\":\"1\"}\n" {
				t.Fatalf("stdout = %q, want exactly the server's own output", stdout)
			}
		})
	}
}
