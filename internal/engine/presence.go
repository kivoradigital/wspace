// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"
	"sync"
	"time"

	"github.com/kivoradigital/wspace/internal/ports"
)

// PresenceThrottle is the shortest interval between two presence writes
// for repeated calls of the same tool. A call of a different tool is always
// written, so lastTool is never stale.
const PresenceThrottle = 2 * time.Second

// ClientInfo identifies the agent that started an MCP session (the MCP
// initialize request's clientInfo).
type ClientInfo struct {
	Name    string
	Version string
}

// MCPSession is one row of mcp.sessions: a live `wspace mcp serve`
// process. Times are RFC 3339, UTC; lastTool/lastToolAt are omitted until
// the session's first tool call.
type MCPSession struct {
	PID            int    `json:"pid"`
	ClientName     string `json:"clientName"`
	ClientVersion  string `json:"clientVersion,omitempty"`
	StartedAt      string `json:"startedAt"`
	LastActivityAt string `json:"lastActivityAt"`
	LastTool       string `json:"lastTool,omitempty"`
	LastToolAt     string `json:"lastToolAt,omitempty"`
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// MCPSessions lists the live MCP server sessions, oldest first. Records of
// processes that are gone are dropped (and deleted by the store).
func (e *Engine) MCPSessions(ctx context.Context) ([]MCPSession, error) {
	out := []MCPSession{}
	if e.deps.Presence == nil {
		return out, nil
	}
	live, err := e.deps.Presence.Live(ctx)
	if err != nil {
		return nil, wrap(err)
	}
	for _, s := range live {
		out = append(out, MCPSession{
			PID: s.PID, ClientName: s.ClientName, ClientVersion: s.ClientVersion,
			StartedAt: rfc3339(s.StartedAt), LastActivityAt: rfc3339(s.LastActivityAt),
			LastTool: s.LastTool, LastToolAt: rfc3339(s.LastToolAt),
		})
	}
	return out, nil
}

// Presence is this process's presence record while an MCP session is
// open. Its methods are safe for concurrent use and on a nil *Presence.
// Presence is best effort: a failed write never fails the session.
type Presence struct {
	store ports.PresenceStore
	now   func() time.Time

	mu          sync.Mutex
	rec         ports.AgentSession
	lastWrite   time.Time
	writtenTool string
	closed      bool
}

// StartPresence writes this process's presence record for client and
// returns the handle that updates and removes it. With no presence store
// it returns nil (every method is then a no-op).
func (e *Engine) StartPresence(ctx context.Context, client ClientInfo) *Presence {
	if e.deps.Presence == nil {
		return nil
	}
	now := e.deps.Now()
	p := &Presence{
		store: e.deps.Presence,
		now:   e.deps.Now,
		rec: ports.AgentSession{
			PID: e.deps.PID, ClientName: client.Name, ClientVersion: client.Version,
			StartedAt: now, LastActivityAt: now,
		},
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.write(ctx, now)
	return p
}

// ToolCalled records a tool call (throttled by PresenceThrottle).
func (p *Presence) ToolCalled(ctx context.Context, tool string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	now := p.now()
	p.rec.LastActivityAt = now
	p.rec.LastTool = tool
	p.rec.LastToolAt = now
	if tool == p.writtenTool && now.Sub(p.lastWrite) < PresenceThrottle {
		return
	}
	p.write(ctx, now)
}

// Close removes the presence record; later calls do nothing.
func (p *Presence) Close(ctx context.Context) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	_ = p.store.Remove(context.WithoutCancel(ctx), p.rec.PID)
}

// write stores the record; the caller holds mu.
func (p *Presence) write(ctx context.Context, now time.Time) {
	if err := p.store.Put(ctx, p.rec); err != nil {
		return
	}
	p.lastWrite = now
	p.writtenTool = p.rec.LastTool
}
