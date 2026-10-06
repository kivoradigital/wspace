// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Progress event kinds.
const (
	KindRepo = "repo" // a per-repository transition (Phase/Repo/Op set)
	KindStep = "step" // a step announcement (Key/Message set)
	KindInfo = "info"
	KindWarn = "warn"
)

// ProgressEvent is one streamed progress notification. A "repo" event
// carries Op (e.g. "workspace.create"), Repo (project key or alias) and
// Phase ("started", "finished", "failed", "rolled_back"; Error set on
// "failed"); "step"/"info"/"warn" events carry the catalog Key and its
// rendered Message.
type ProgressEvent struct {
	Kind    string `json:"kind"`
	Op      string `json:"op,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Phase   string `json:"phase,omitempty"`
	Key     string `json:"key,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ProgressFunc receives progress events synchronously, in order, on the
// goroutine running the operation. A nil ProgressFunc discards them.
type ProgressFunc func(ProgressEvent)

// progressReporter adapts a ProgressFunc to ports.Reporter (and its
// optional ports.RepoProgressReporter extension). Result is ignored: the
// engine returns results directly.
type progressReporter struct{ emit ProgressFunc }

var (
	_ ports.Reporter             = progressReporter{}
	_ ports.RepoProgressReporter = progressReporter{}
)

func (r progressReporter) send(ev ProgressEvent) {
	if r.emit != nil {
		r.emit(ev)
	}
}

func (r progressReporter) Step(k messages.Key, args ...any) {
	r.send(ProgressEvent{Kind: KindStep, Key: string(k), Message: messages.T(k, args...)})
}

func (r progressReporter) Info(k messages.Key, args ...any) {
	r.send(ProgressEvent{Kind: KindInfo, Key: string(k), Message: messages.T(k, args...)})
}

func (r progressReporter) Warn(k messages.Key, args ...any) {
	r.send(ProgressEvent{Kind: KindWarn, Key: string(k), Message: messages.T(k, args...)})
}

func (r progressReporter) Result(any) {}

func (r progressReporter) RepoProgress(ev ports.RepoEvent) {
	out := ProgressEvent{Kind: KindRepo, Op: ev.Op, Repo: ev.Repo, Phase: string(ev.Phase)}
	if ev.Err != nil {
		out.Error = AsError(ev.Err).Message
	}
	r.send(out)
}
