// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import "github.com/kivoradigital/wspace/internal/messages"

// countingReporter is a caller-supplied Reporter used to prove Execute does
// not replace one that is already set.
type countingReporter struct {
	results int
}

func (r *countingReporter) Step(messages.Key, ...any) {}
func (r *countingReporter) Info(messages.Key, ...any) {}
func (r *countingReporter) Warn(messages.Key, ...any) {}
func (r *countingReporter) Result(any)                { r.results++ }
