// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// ReportEntry is one captured Step/Info/Warn call: the key and its
// arguments, never rendered text — assertions match by key (design.md §12).
type ReportEntry struct {
	Key  messages.Key
	Args []any
}

// RecordingReporter is a ports.Reporter (and ports.RepoProgressReporter)
// that captures every call for assertion instead of rendering anything.
type RecordingReporter struct {
	Steps      []ReportEntry
	Infos      []ReportEntry
	Warnings   []ReportEntry
	Results    []any
	RepoEvents []ports.RepoEvent
}

// NewRecordingReporter constructs an empty RecordingReporter.
func NewRecordingReporter() *RecordingReporter {
	return &RecordingReporter{}
}

func (r *RecordingReporter) Step(k messages.Key, args ...any) {
	r.Steps = append(r.Steps, ReportEntry{Key: k, Args: args})
}

func (r *RecordingReporter) Info(k messages.Key, args ...any) {
	r.Infos = append(r.Infos, ReportEntry{Key: k, Args: args})
}

func (r *RecordingReporter) Warn(k messages.Key, args ...any) {
	r.Warnings = append(r.Warnings, ReportEntry{Key: k, Args: args})
}

func (r *RecordingReporter) Result(payload any) {
	r.Results = append(r.Results, payload)
}

// RepoProgress records one per-repository progress event.
func (r *RecordingReporter) RepoProgress(ev ports.RepoEvent) {
	r.RepoEvents = append(r.RepoEvents, ev)
}
