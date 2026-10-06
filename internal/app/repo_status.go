// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
)

// repoAheadBehind reports ahead/behind counts against the repo's upstream,
// falling back to UnpushedCount (behind always 0 in that case) when no
// upstream is configured (design.md §7: "UnpushedCount ... used when the
// branch has no upstream").
func repoAheadBehind(ctx context.Context, deps Deps, worktree domain.Path, unpushedBase string) (ahead, behind int, err error) {
	ahead, behind, err = deps.Git.AheadBehind(ctx, worktree, "@{upstream}")
	if err == nil {
		return ahead, behind, nil
	}
	n, uerr := deps.Git.UnpushedCount(ctx, worktree, unpushedBase)
	if uerr != nil {
		return 0, 0, uerr
	}
	return n, 0, nil
}

// repoChangeSet computes one repo's teardown-safety domain.ChangeSet,
// including Unpushed (repository-operations spec: "Status and change
// classification"). DestroyWorkspace and RemoveRepo share this exact call
// (tasks.md 4a.11: "the shared per-repo status/classify loop").
func repoChangeSet(ctx context.Context, deps Deps, alias string, worktree domain.Path, envCopies []string, unpushedBase string) (domain.ChangeSet, error) {
	entries, err := deps.Git.Status(ctx, worktree)
	if err != nil {
		return domain.ChangeSet{}, err
	}
	changes := domain.Classify(alias, entries, envCopies)

	ahead, _, err := repoAheadBehind(ctx, deps, worktree, unpushedBase)
	if err != nil {
		return domain.ChangeSet{}, err
	}
	changes.Unpushed = ahead
	return changes, nil
}

// repoStatus computes the full read-only domain.RepoStatus projection used
// by `status`, `list --json` and the tray (design.md §3: "RepoStatus is the
// read-only projection..."). Ahead/behind come from the upstream when there
// is one; otherwise ahead is counted against the repo's own comparison base,
// resolved lazily by base (so a repo with an upstream costs no extra git
// calls). A base that cannot be resolved is reported on the repo
// (BaseMissing), never as an error: one repo lacking the workspace's base
// branch must not hide the whole workspace.
func repoStatus(ctx context.Context, deps Deps, alias string, worktree domain.Path, envCopies []string, base func(context.Context) (repoBase, error)) (domain.RepoStatus, error) {
	branch, detached, err := deps.Git.CurrentBranch(ctx, worktree)
	if err != nil {
		return domain.RepoStatus{}, err
	}

	entries, err := deps.Git.Status(ctx, worktree)
	if err != nil {
		return domain.RepoStatus{}, err
	}
	changes := domain.Classify(alias, entries, envCopies)

	rs := domain.RepoStatus{Alias: alias, Branch: branch, Detached: detached}
	ahead, behind, upErr := deps.Git.AheadBehind(ctx, worktree, "@{upstream}")
	if upErr != nil {
		ahead, behind = 0, 0
		b, err := base(ctx)
		if err != nil {
			return domain.RepoStatus{}, err
		}
		rs.BaseBranch = b.Branch
		if !b.Found {
			rs.BaseMissing = true
		} else {
			n, err := deps.Git.UnpushedCount(ctx, worktree, b.Ref.Ref)
			switch {
			case err == nil:
				ahead = n
				// Best-effort: an uncountable base only leaves it unknown.
				if nb, berr := deps.Git.BehindCount(ctx, worktree, b.Ref.Ref); berr == nil {
					rs.BaseBehind = &nb
				}
			case domain.Code(err) == domain.CodeRefNotFound:
				rs.BaseMissing = true
			default:
				return domain.RepoStatus{}, err
			}
		}
	}
	changes.Unpushed = ahead

	rs.Ahead = ahead
	rs.Behind = behind
	rs.Dirty = len(changes.Changes) > 0
	rs.Changes = changes
	return rs, nil
}
