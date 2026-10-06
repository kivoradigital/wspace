// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// mcpProcess is a running `wspace mcp serve` with its stdin held open.
type mcpProcess struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines *bufio.Scanner
	done  chan error
}

func startMCP(t *testing.T, fx *Fixture, configHome string) *mcpProcess {
	t.Helper()
	cmd := exec.Command(fx.Bin, "mcp", "serve")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + fx.Home, "XDG_CONFIG_HOME=" + fx.XDG, "WSPACE_CONFIG_HOME=" + configHome}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &mcpProcess{cmd: cmd, stdin: stdin, lines: bufio.NewScanner(stdout), done: make(chan error, 1)}
	p.lines.Buffer(make([]byte, 0, 64*1024), 4<<20)
	go func() {
		// Drain stdout after the test stops reading so the child never blocks.
		p.done <- cmd.Wait()
	}()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return p
}

func (p *mcpProcess) send(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(p.stdin, line+"\n"); err != nil {
		t.Fatalf("write to mcp serve: %v", err)
	}
}

func (p *mcpProcess) readResponse(t *testing.T, id int) map[string]any {
	t.Helper()
	for p.lines.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(p.lines.Bytes(), &msg); err != nil {
			t.Fatalf("mcp serve wrote a non-JSON line %q", p.lines.Text())
		}
		if got, ok := msg["id"].(float64); ok && int(got) == id {
			return msg
		}
	}
	t.Fatalf("mcp serve closed stdout before answering id %d (%v)", id, p.lines.Err())
	return nil
}

func (p *mcpProcess) handshake(t *testing.T) {
	t.Helper()
	p.send(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke-agent","version":"1.0"}}}`)
	p.readResponse(t, 1)
	p.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	p.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_contexts","arguments":{}}}`)
	if res := p.readResponse(t, 2); res["error"] != nil {
		t.Fatalf("tools/call failed: %v", res)
	}
}

func (p *mcpProcess) waitExit(t *testing.T) {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		t.Fatal("mcp serve did not exit")
	}
}

func presenceFile(configHome string, pid int) string {
	return filepath.Join(configHome, "run", "mcp", strconv.Itoa(pid)+".json")
}

func TestMCP_PresenceFileLifecycleAndRPCSessions(t *testing.T) {
	fx := NewFixture(t)
	configHome := filepath.Join(fx.Root, "cfg")
	p := startMCP(t, fx, configHome)
	p.handshake(t)

	pid := p.cmd.Process.Pid
	raw, err := os.ReadFile(presenceFile(configHome, pid))
	if err != nil {
		t.Fatalf("presence file missing while serving: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(raw, &rec); err != nil || rec["clientName"] != "smoke-agent" || rec["lastTool"] != "list_contexts" {
		t.Fatalf("presence record = %s (%v)", raw, err)
	}

	// The engine RPC reports the live session.
	stdout, stderr, code := fx.RunEnv([]string{"WSPACE_CONFIG_HOME=" + configHome}, `{"id":"s","method":"mcp.sessions"}`+"\n", "rpc")
	if code != 0 || !strings.Contains(stdout, `"pid":`+strconv.Itoa(pid)) || !strings.Contains(stdout, `"clientName":"smoke-agent"`) {
		t.Fatalf("mcp.sessions = %s (stderr %s, exit %d)", stdout, stderr, code)
	}

	// Closing stdin ends the session and removes the record.
	_ = p.stdin.Close()
	p.waitExit(t)
	if _, err := os.Stat(presenceFile(configHome, pid)); !os.IsNotExist(err) {
		t.Fatalf("presence file left after stdin closed (stat err %v)", err)
	}
	stdout, _, _ = fx.RunEnv([]string{"WSPACE_CONFIG_HOME=" + configHome}, `{"id":"s","method":"mcp.sessions"}`+"\n", "rpc")
	if !strings.Contains(stdout, `"result":[]`) {
		t.Fatalf("mcp.sessions after exit = %s, want []", stdout)
	}
}

func TestMCP_PresenceFileRemovedOnSIGTERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no SIGTERM on windows")
	}
	fx := NewFixture(t)
	configHome := filepath.Join(fx.Root, "cfg")
	p := startMCP(t, fx, configHome)
	p.handshake(t)
	pid := p.cmd.Process.Pid
	if _, err := os.Stat(presenceFile(configHome, pid)); err != nil {
		t.Fatalf("presence file missing while serving: %v", err)
	}
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	p.waitExit(t)
	if _, err := os.Stat(presenceFile(configHome, pid)); !os.IsNotExist(err) {
		t.Fatalf("presence file left after SIGTERM (stat err %v)", err)
	}
}

func TestMCP_StaleRecordOfAKilledServerIsIgnoredAndCleaned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGKILL semantics differ on windows")
	}
	fx := NewFixture(t)
	configHome := filepath.Join(fx.Root, "cfg")
	p := startMCP(t, fx, configHome)
	p.handshake(t)
	pid := p.cmd.Process.Pid
	_ = p.cmd.Process.Kill() // no cleanup possible
	p.waitExit(t)
	if _, err := os.Stat(presenceFile(configHome, pid)); err != nil {
		t.Fatalf("SIGKILL should leave the record behind for readers to clean: %v", err)
	}
	stdout, _, _ := fx.RunEnv([]string{"WSPACE_CONFIG_HOME=" + configHome}, `{"id":"s","method":"mcp.sessions"}`+"\n", "rpc")
	if !strings.Contains(stdout, `"result":[]`) {
		t.Fatalf("mcp.sessions with a stale record = %s, want []", stdout)
	}
	if _, err := os.Stat(presenceFile(configHome, pid)); !os.IsNotExist(err) {
		t.Fatalf("stale record not cleaned by the reader (stat err %v)", err)
	}
}
