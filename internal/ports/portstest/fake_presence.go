// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"context"
	"sort"
	"sync"

	"github.com/kivoradigital/wspace/internal/ports"
)

// FakePresenceStore is an in-memory ports.PresenceStore. Dead lists the
// PIDs whose process is gone: Live skips and deletes their records, like
// the real adapter. Puts counts every Put, so throttling is observable.
type FakePresenceStore struct {
	mu      sync.Mutex
	records map[int]ports.AgentSession
	Dead    map[int]bool
	Puts    int
	PutErr  error
	LiveErr error
}

var _ ports.PresenceStore = (*FakePresenceStore)(nil)

// NewFakePresenceStore returns an empty store.
func NewFakePresenceStore() *FakePresenceStore {
	return &FakePresenceStore{records: map[int]ports.AgentSession{}, Dead: map[int]bool{}}
}

// Put stores s.
func (f *FakePresenceStore) Put(_ context.Context, s ports.AgentSession) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Puts++
	if f.PutErr != nil {
		return f.PutErr
	}
	f.records[s.PID] = s
	return nil
}

// Remove deletes pid's record.
func (f *FakePresenceStore) Remove(_ context.Context, pid int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.records, pid)
	return nil
}

// Live returns live records oldest first, pruning dead ones.
func (f *FakePresenceStore) Live(_ context.Context) ([]ports.AgentSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.LiveErr != nil {
		return nil, f.LiveErr
	}
	out := []ports.AgentSession{}
	for pid, rec := range f.records {
		if f.Dead[pid] {
			delete(f.records, pid)
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].PID < out[j].PID
	})
	return out, nil
}

// Record returns pid's stored record, if any.
func (f *FakePresenceStore) Record(pid int) (ports.AgentSession, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[pid]
	return rec, ok
}
