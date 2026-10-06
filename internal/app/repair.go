// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// RepairInput parameterizes Repair.
type RepairInput struct {
	WorkspaceRoot domain.Path
}

// RepairResult reports which aliases were recreated.
type RepairResult struct {
	Recreated []string
}

// Repair recreates every worktree the manifest declares but that is
// missing from disk, and leaves healthy worktrees untouched
// (workspace-lifecycle spec: "Repair recreates a missing worktree").
func Repair(ctx context.Context, deps Deps, in RepairInput) (RepairResult, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return RepairResult{}, err
	}

	var recreated []string
	for _, repo := range manifest.Workspace.Repos {
		worktree := in.WorkspaceRoot.Join(repo.Alias)
		exists, err := deps.FS.Exists(worktree)
		if err != nil {
			return RepairResult{}, err
		}
		if exists {
			continue
		}

		// worktree's directory is confirmed missing, but git may still
		// believe it is registered: a plain `rm -rf` (a user's Finder, a
		// shell command, a build script wiping a directory — the exact
		// scenario the spec names, "the worktree directory was deleted
		// manually") never deregisters a worktree the way `git worktree
		// remove` does. Without this prune, WorktreeAdd below fails with
		// "is a missing but already registered worktree", refusing to
		// recreate the very thing Repair exists to recreate.
		//
		// Pruning here cannot touch any *other*, healthy worktree
		// registered against the same repo: `git worktree prune` only
		// ever removes administrative entries for a worktree whose
		// working directory is already gone from disk (git's own
		// documented behavior) — a worktree that still exists on disk is
		// never a candidate for pruning, regardless of what else in the
		// same repo is stale. So this call can only ever clear the exact
		// staleness Exists just detected (and, harmlessly, any other
		// already-stale entry in the same repo), never a live one.
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpRepair, Repo: repo.Alias, Phase: ports.RepoStarted})
		if err := recreateWorktree(ctx, deps, repo, worktree); err != nil {
			reportRepo(deps.Reporter, ports.RepoEvent{Op: OpRepair, Repo: repo.Alias, Phase: ports.RepoFailed, Err: err})
			return RepairResult{}, err
		}
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpRepair, Repo: repo.Alias, Phase: ports.RepoFinished})
		recreated = append(recreated, repo.Alias)
	}

	result := RepairResult{Recreated: recreated}
	deps.Reporter.Result(result)
	return result, nil
}

// recreateWorktree prunes repo's stale registration (see Repair's own
// comment for why that is always safe) and re-adds its worktree on the
// manifest's recorded branch.
func recreateWorktree(ctx context.Context, deps Deps, repo domain.RepoEntry, worktree domain.Path) error {
	if err := deps.Git.WorktreePrune(ctx, repo.SourceDir); err != nil {
		return err
	}
	return deps.Git.WorktreeAdd(ctx, repo.SourceDir, ports.WorktreeSpec{
		Target: worktree,
		Branch: repo.Branch,
	})
}
