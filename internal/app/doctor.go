// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// DoctorInput parameterizes Doctor.
type DoctorInput struct {
	WorkspacesRoot domain.Path
}

// DoctorResult summarizes Doctor's read-only findings.
type DoctorResult struct {
	GitTooOld       bool
	PrunedWorktrees int
}

// Doctor reports diagnostics for every workspace under WorkspacesRoot and
// prunes stale worktree registrations discovered during the check, without
// any other mutation (workspace-lifecycle spec: "Doctor diagnostics").
func Doctor(ctx context.Context, deps Deps, in DoctorInput) (DoctorResult, error) {
	var result DoctorResult

	if _, err := deps.Git.Version(ctx); err != nil {
		switch domain.Code(err) {
		case domain.CodeGitMissing:
			result.GitTooOld = true
			deps.Reporter.Warn(messages.GitMissing)
		case domain.CodeGitTooOld:
			result.GitTooOld = true
			warnOpError(deps.Reporter, messages.GitVersionTooOld, err)
		default:
			return DoctorResult{}, err
		}
	}

	if in.WorkspacesRoot == "" {
		// No context yet: only the git check applies.
		return result, nil
	}
	names, err := deps.FS.ListDirs(in.WorkspacesRoot)
	if err != nil {
		return DoctorResult{}, err
	}

	for _, name := range names {
		root := in.WorkspacesRoot.Join(name)
		manifest, err := deps.Store.LoadManifest(ctx, root)
		if err != nil {
			if domain.Code(err) == domain.CodeWorkspaceNotFound {
				continue
			}
			return DoctorResult{}, err
		}

		for _, repo := range manifest.Workspace.Repos {
			refs, err := deps.Git.WorktreeList(ctx, repo.SourceDir)
			if err != nil {
				return DoctorResult{}, err
			}

			anyPrunable := false
			for _, ref := range refs {
				if !ref.Prunable {
					continue
				}
				anyPrunable = true
				result.PrunedWorktrees++
				deps.Reporter.Warn(messages.WorktreeRegistrationPruned, string(repo.SourceDir), string(ref.Path))
			}
			if anyPrunable {
				if err := deps.Git.WorktreePrune(ctx, repo.SourceDir); err != nil {
					return DoctorResult{}, err
				}
			}
		}
	}

	deps.Reporter.Result(result)
	return result, nil
}
