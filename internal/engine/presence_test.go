// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) advance(d time.Duration) { c.now = c.now.Add(d) }
func newClock() *clock                   { return &clock{now: time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)} }
func presenceEngine(store ports.PresenceStore, c *clock, pid int) *engine.Engine {
	return engine.New(engine.Deps{Presence: store, Now: c.Now, PID: pid})
}

func TestPresence_StartRecordsClientAndPID(t *testing.T) {
	store := portstest.NewFakePresenceStore()
	c := newClock()
	eng := presenceEngine(store, c, 321)

	p := eng.StartPresence(context.Background(), engine.ClientInfo{Name: "claude-code", Version: "2.1"})
	rec, ok := store.Record(321)
	if !ok {
		t.Fatal("no presence record after StartPresence")
	}
	if rec.ClientName != "claude-code" || rec.ClientVersion != "2.1" || !rec.StartedAt.Equal(c.now) || !rec.LastActivityAt.Equal(c.now) || rec.LastTool != "" {
		t.Fatalf("record = %+v", rec)
	}
	p.Close(context.Background())
	if _, ok := store.Record(321); ok {
		t.Fatal("record still present after Close")
	}
}

func TestPresence_ToolCallsUpdateLastToolThrottled(t *testing.T) {
	store := portstest.NewFakePresenceStore()
	c := newClock()
	eng := presenceEngine(store, c, 7)
	p := eng.StartPresence(context.Background(), engine.ClientInfo{Name: "cursor"})
	start := c.now

	c.advance(100 * time.Millisecond)
	p.ToolCalled(context.Background(), "list_workspaces")
	rec, _ := store.Record(7)
	if rec.LastTool != "list_workspaces" || !rec.LastToolAt.Equal(c.now) || !rec.LastActivityAt.Equal(c.now) || !rec.StartedAt.Equal(start) {
		t.Fatalf("after first tool call record = %+v", rec)
	}
	puts := store.Puts

	// The same tool again within the throttle window is not written.
	c.advance(500 * time.Millisecond)
	p.ToolCalled(context.Background(), "list_workspaces")
	if store.Puts != puts {
		t.Fatalf("repeat call inside the throttle window wrote (%d puts, want %d)", store.Puts, puts)
	}

	// A different tool is always written, so lastTool is never stale.
	c.advance(100 * time.Millisecond)
	p.ToolCalled(context.Background(), "workspace_status")
	if rec, _ = store.Record(7); rec.LastTool != "workspace_status" {
		t.Fatalf("different tool not recorded: %+v", rec)
	}

	// The same tool after the window is written again.
	c.advance(engine.PresenceThrottle)
	p.ToolCalled(context.Background(), "workspace_status")
	if rec, _ = store.Record(7); !rec.LastActivityAt.Equal(c.now) {
		t.Fatalf("call after the window not recorded: %+v", rec)
	}
}

func TestPresence_NilStoreAndNilPresenceAreNoOps(t *testing.T) {
	eng := engine.New(engine.Deps{})
	p := eng.StartPresence(context.Background(), engine.ClientInfo{Name: "x"})
	p.ToolCalled(context.Background(), "t")
	p.Close(context.Background())
	var nilP *engine.Presence
	nilP.ToolCalled(context.Background(), "t")
	nilP.Close(context.Background())

	sessions, err := eng.MCPSessions(context.Background())
	if err != nil || sessions == nil || len(sessions) != 0 {
		t.Fatalf("MCPSessions without a store = %v, %v; want [] and nil", sessions, err)
	}
}

func TestPresence_WriteFailuresNeverPropagate(t *testing.T) {
	store := portstest.NewFakePresenceStore()
	store.PutErr = errors.New("disk full")
	eng := presenceEngine(store, newClock(), 5)
	p := eng.StartPresence(context.Background(), engine.ClientInfo{Name: "x"})
	p.ToolCalled(context.Background(), "t") // must not panic
	p.Close(context.Background())
}

func TestMCPSessions_ListsLiveSessionsAsWireShapes(t *testing.T) {
	store := portstest.NewFakePresenceStore()
	c := newClock()
	started := c.now
	_ = store.Put(context.Background(), ports.AgentSession{PID: 11, ClientName: "claude-code", ClientVersion: "2.1", StartedAt: started, LastActivityAt: started.Add(time.Minute), LastTool: "create_workspace", LastToolAt: started.Add(time.Minute)})
	_ = store.Put(context.Background(), ports.AgentSession{PID: 12, ClientName: "cursor", StartedAt: started.Add(time.Hour), LastActivityAt: started.Add(time.Hour)})
	_ = store.Put(context.Background(), ports.AgentSession{PID: 13, ClientName: "gone", StartedAt: started, LastActivityAt: started})
	store.Dead[13] = true

	sessions, err := presenceEngine(store, c, 1).MCPSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []engine.MCPSession{
		{PID: 11, ClientName: "claude-code", ClientVersion: "2.1", StartedAt: "2026-10-03T09:00:00Z", LastActivityAt: "2026-10-03T09:01:00Z", LastTool: "create_workspace", LastToolAt: "2026-10-03T09:01:00Z"},
		{PID: 12, ClientName: "cursor", StartedAt: "2026-10-03T10:00:00Z", LastActivityAt: "2026-10-03T10:00:00Z"},
	}
	if len(sessions) != len(want) {
		t.Fatalf("sessions = %+v, want %+v", sessions, want)
	}
	for i := range want {
		if sessions[i] != want[i] {
			t.Fatalf("sessions[%d] = %+v, want %+v", i, sessions[i], want[i])
		}
	}
}

func TestMCPSessions_StoreFailureIsInternal(t *testing.T) {
	store := portstest.NewFakePresenceStore()
	store.LiveErr = errors.New("unreadable")
	_, err := presenceEngine(store, newClock(), 1).MCPSessions(context.Background())
	if engine.AsError(err).Code != engine.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}
