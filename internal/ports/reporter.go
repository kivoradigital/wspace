// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

import "github.com/kivoradigital/wspace/internal/messages"

// Reporter is the one-way output channel: progress and results, no input.
// The CLI renders human or JSON; the tray maps Result to a state snapshot
// (design.md §4).
type Reporter interface {
	Step(k messages.Key, args ...any)
	Info(k messages.Key, args ...any)
	Warn(k messages.Key, args ...any)
	Result(payload any)
}

// RepoPhase is one per-repository progress transition a long operation
// (create, add, destroy, repair, sync-env) reports through
// RepoProgressReporter.
type RepoPhase string

const (
	// RepoStarted: work on this repository has begun.
	RepoStarted RepoPhase = "started"
	// RepoFinished: work on this repository completed successfully.
	RepoFinished RepoPhase = "finished"
	// RepoFailed: work on this repository failed; RepoEvent.Err says why.
	RepoFailed RepoPhase = "failed"
	// RepoRolledBack: a repository this run had already finished was
	// undone because a later step of the same operation failed.
	RepoRolledBack RepoPhase = "rolled_back"
)

// RepoEvent is one per-repository progress transition. Op names the
// operation (e.g. "workspace.create"); Repo is the project key or alias.
type RepoEvent struct {
	Op    string
	Repo  string
	Phase RepoPhase
	Err   error
}

// RepoProgressReporter is an OPTIONAL extension of Reporter: a Reporter
// that also implements it receives structured per-repository progress
// (the rpc/mcp surfaces stream it to their callers). internal/app checks
// for it with a type assertion, so a Reporter that does not implement it
// (the CLI's HumanReporter, the tray's reporter) keeps working unchanged.
type RepoProgressReporter interface {
	RepoProgress(ev RepoEvent)
}
