// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// SyncEnvInput parameterizes SyncEnv.
type SyncEnvInput struct {
	WorkspaceRoot domain.Path
}

// SyncEnvResult reports every workspace-relative path re-copied.
type SyncEnvResult struct {
	Copied []string
}

// SyncEnv re-runs env file discovery and copy for every mounted repo in an
// existing workspace, without recreating any worktree, and replaces the
// manifest's env-copies list with the fresh result (environment-files
// spec: "Re-sync on demand").
func SyncEnv(ctx context.Context, deps Deps, in SyncEnvInput) (SyncEnvResult, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return SyncEnvResult{}, err
	}

	pruneDirs := manifest.Workspace.Options.EnvPruneDirs

	var allCopied []string
	for _, repo := range manifest.Workspace.Repos {
		worktree := in.WorkspaceRoot.Join(repo.Alias)
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpSyncEnv, Repo: repo.Alias, Phase: ports.RepoStarted})
		copied, err := copyEnvFiles(ctx, deps, repo.SourceDir, worktree, pruneDirs, repo.Alias)
		if err != nil {
			reportRepo(deps.Reporter, ports.RepoEvent{Op: OpSyncEnv, Repo: repo.Alias, Phase: ports.RepoFailed, Err: err})
			return SyncEnvResult{}, err
		}
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpSyncEnv, Repo: repo.Alias, Phase: ports.RepoFinished})
		allCopied = append(allCopied, copied...)
	}

	manifest.EnvCopies = allCopied
	if err := deps.Store.SaveManifest(ctx, in.WorkspaceRoot, manifest); err != nil {
		return SyncEnvResult{}, err
	}

	result := SyncEnvResult{Copied: allCopied}
	deps.Reporter.Result(result)
	return result, nil
}
