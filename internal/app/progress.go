// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import "github.com/kivoradigital/wspace/internal/ports"

// Operation names carried by every ports.RepoEvent this package emits, so
// a progress consumer can tell which long-running use case an event
// belongs to without parsing anything.
const (
	OpCreateWorkspace  = "workspace.create"
	OpAddRepo          = "workspace.add_repo"
	OpDestroyWorkspace = "workspace.destroy"
	OpRemoveRepo       = "workspace.remove_repo"
	OpRepair           = "workspace.repair"
	OpSyncEnv          = "workspace.sync_env"
	OpUpdate           = "workspace.update"
	OpFetch            = "repo.fetch"
	OpPull             = "repo.pull"
	OpPush             = "repo.push"
)

// reportRepo forwards ev to reporter when it implements the optional
// ports.RepoProgressReporter extension, and is a no-op otherwise (a nil
// reporter included), so no existing Reporter has to change.
func reportRepo(reporter ports.Reporter, ev ports.RepoEvent) {
	if p, ok := reporter.(ports.RepoProgressReporter); ok {
		p.RepoProgress(ev)
	}
}
