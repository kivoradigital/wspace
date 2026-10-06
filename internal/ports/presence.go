// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

import (
	"context"
	"time"
)

// AgentSession is one running `wspace mcp serve` process as recorded in
// its presence record: which agent spawned it and what it did last.
// LastTool and LastToolAt are zero until the first tool call.
type AgentSession struct {
	PID            int
	ClientName     string
	ClientVersion  string
	StartedAt      time.Time
	LastActivityAt time.Time
	LastTool       string
	LastToolAt     time.Time
}

// PresenceStore keeps one presence record per live MCP server process, so
// other processes (a desktop client through `mcp.sessions`) can tell which
// agents are connected. A record belongs to the process whose PID it
// carries.
type PresenceStore interface {
	// Put creates or atomically replaces the record for s.PID.
	Put(ctx context.Context, s AgentSession) error
	// Remove deletes pid's record; a missing record is not an error.
	Remove(ctx context.Context, pid int) error
	// Live returns the records whose process is still running, oldest
	// first. Records of dead processes are ignored and deleted.
	Live(ctx context.Context) ([]AgentSession, error)
}
