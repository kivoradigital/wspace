// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package presencefs is the file-system ports.PresenceStore: one JSON file
// per live `wspace mcp serve` process at <config>/run/mcp/<pid>.json,
// written atomically (temp file + rename) so a reader never sees a partial
// record. Readers validate each PID and delete records (and leftover temp
// files) of processes that are gone, so a killed server never shows as
// connected.
package presencefs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Dir is the presence directory under a wspace config directory.
func Dir(configDir domain.Path) domain.Path {
	return configDir.Join("run").Join("mcp")
}

// Store is the file-system presence store.
type Store struct {
	dir   string
	alive func(pid int) bool
}

var _ ports.PresenceStore = (*Store)(nil)

// New returns a Store under configDir's presence directory, checking
// liveness with ProcessAlive.
func New(configDir domain.Path) *Store {
	return NewAt(filepath.FromSlash(string(Dir(configDir))), ProcessAlive)
}

// NewAt returns a Store over dir with an explicit liveness check (tests).
func NewAt(dir string, alive func(pid int) bool) *Store {
	return &Store{dir: dir, alive: alive}
}

// record is the on-disk shape. Times are RFC 3339 (UTC); lastTool and
// lastToolAt are omitted before the first tool call.
type record struct {
	PID            int        `json:"pid"`
	ClientName     string     `json:"clientName"`
	ClientVersion  string     `json:"clientVersion"`
	StartedAt      time.Time  `json:"startedAt"`
	LastActivityAt time.Time  `json:"lastActivityAt"`
	LastTool       string     `json:"lastTool,omitempty"`
	LastToolAt     *time.Time `json:"lastToolAt,omitempty"`
}

func (s *Store) path(pid int) string {
	return filepath.Join(s.dir, strconv.Itoa(pid)+".json")
}

// Put writes s's record atomically.
func (s *Store) Put(_ context.Context, a ports.AgentSession) error {
	if a.PID <= 0 {
		return fmt.Errorf("presencefs: invalid pid %d", a.PID)
	}
	rec := record{
		PID: a.PID, ClientName: a.ClientName, ClientVersion: a.ClientVersion,
		StartedAt: a.StartedAt.UTC(), LastActivityAt: a.LastActivityAt.UTC(), LastTool: a.LastTool,
	}
	if !a.LastToolAt.IsZero() {
		t := a.LastToolAt.UTC()
		rec.LastToolAt = &t
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("presencefs: encode: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("presencefs: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, "."+strconv.Itoa(a.PID)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("presencefs: %w", err)
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("presencefs: write: %w", errors.Join(werr, cerr))
	}
	if err := os.Rename(tmpName, s.path(a.PID)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("presencefs: %w", err)
	}
	return nil
}

// Remove deletes pid's record.
func (s *Store) Remove(_ context.Context, pid int) error {
	if err := os.Remove(s.path(pid)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("presencefs: %w", err)
	}
	return nil
}

// Live lists the live sessions, oldest first, deleting stale records.
func (s *Store) Live(_ context.Context) ([]ports.AgentSession, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []ports.AgentSession{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("presencefs: %w", err)
	}
	out := []ports.AgentSession{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if pid, ok := tempFilePID(name); ok {
			if !s.alive(pid) {
				_ = os.Remove(filepath.Join(s.dir, name))
			}
			continue
		}
		pid, ok := recordPID(name)
		if !ok {
			continue
		}
		if !s.alive(pid) {
			_ = os.Remove(filepath.Join(s.dir, name))
			continue
		}
		rec, ok := s.read(name)
		if !ok || rec.PID != pid {
			continue
		}
		a := ports.AgentSession{
			PID: rec.PID, ClientName: rec.ClientName, ClientVersion: rec.ClientVersion,
			StartedAt: rec.StartedAt, LastActivityAt: rec.LastActivityAt, LastTool: rec.LastTool,
		}
		if rec.LastToolAt != nil {
			a.LastToolAt = *rec.LastToolAt
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].PID < out[j].PID
	})
	return out, nil
}

func (s *Store) read(name string) (record, bool) {
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return record{}, false
	}
	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		return record{}, false
	}
	return rec, true
}

// recordPID parses "<pid>.json".
func recordPID(name string) (int, bool) {
	base, ok := strings.CutSuffix(name, ".json")
	if !ok {
		return 0, false
	}
	return positiveInt(base)
}

// tempFilePID parses ".<pid>-<random>.tmp".
func tempFilePID(name string) (int, bool) {
	if !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".tmp") {
		return 0, false
	}
	head, _, ok := strings.Cut(name[1:], "-")
	if !ok {
		return 0, false
	}
	return positiveInt(head)
}

func positiveInt(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
