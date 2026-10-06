// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"errors"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Deps bundles the four ports shared by nearly every phase-4a use case
// (CreateWorkspace, DestroyWorkspace, AddRepo, RemoveRepo, Repair, SyncEnv,
// Status, List, Doctor). Phase 3's handoff note asked phase 4a to decide
// whether internal/app needs a composition struct now that it carries far
// more than three use cases — this is that decision: a plain data bundle,
// not a service object with behavior, so "one use case = one file = one
// testable unit" (design.md §1 package inventory) and R4 (app imports only
// domain/ports/messages) are unaffected. Use cases that need only one or
// two ports (SwitchContext, Jump) keep taking them directly, exactly as
// phase 3 left them; ProjectWizardDeps keeps its own shape since it also
// carries a Prompter, which most of phase 4a's use cases do not need.
type Deps struct {
	Store    ports.ConfigStore
	Git      ports.GitPort
	FS       ports.FileSystemPort
	Reporter ports.Reporter
	// Trees copies node_modules into new worktrees when a create/add asks
	// for it; nil skips that copy with a warning.
	Trees ports.TreeCloner
	// Now stamps what a use case records (a discard backup's name);
	// nil means time.Now.
	Now func() time.Time
}

// warnOpError reports err through reporter under key, passing exactly one
// render argument (the *domain.OpError's Subject, or the raw error text
// when err is not an OpError) so internal/app never constructs user-facing
// text itself and every catalog template driven by this helper needs
// exactly one placeholder (phase 4b's message catalog). This is the one
// shared error-to-Reporter.Warn mapping every phase-4a use case funnels a
// non-fatal git/config failure through (tasks.md 4a.29).
func warnOpError(reporter ports.Reporter, key messages.Key, err error) {
	var opErr *domain.OpError
	if errors.As(err, &opErr) {
		reporter.Warn(key, opErr.Subject)
		return
	}
	reporter.Warn(key, err.Error())
}
