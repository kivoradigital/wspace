// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
)

// TeardownCheckInput parameterizes TeardownBlockers. Alias, when set,
// scopes the check to that one repo (RemoveRepo's view); empty checks
// every repo in the workspace (DestroyWorkspace's view).
type TeardownCheckInput struct {
	WorkspaceRoot domain.Path
	Alias         string
}

// TeardownBlockers runs the exact classification DestroyWorkspace and
// RemoveRepo apply before an unforced teardown (repoChangeSet), but over
// every repo instead of stopping at the first unsafe one, and without
// mutating anything. It returns only the repos that are NOT safe to
// remove, so an empty result means an unforced teardown would proceed.
// Non-interactive callers (rpc, mcp) use it to report every reason a
// teardown needs confirmation in one answer.
func TeardownBlockers(ctx context.Context, deps Deps, in TeardownCheckInput) ([]domain.ChangeSet, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return nil, err
	}

	repos := manifest.Workspace.Repos
	if in.Alias != "" {
		_, repo, found := findRepo(repos, in.Alias)
		if !found {
			return nil, domain.NewOpError("workspace.teardown_check", domain.CodeWorkspaceNotFound, in.Alias, "", nil)
		}
		repos = []domain.RepoEntry{repo}
	}

	var blockers []domain.ChangeSet
	for _, repo := range repos {
		changes, err := repoChangeSet(ctx, deps, repo.Alias, in.WorkspaceRoot.Join(repo.Alias), manifest.EnvCopies, "HEAD")
		if err != nil {
			return nil, err
		}
		if !changes.SafeToRemove() {
			blockers = append(blockers, changes)
		}
	}
	return blockers, nil
}
