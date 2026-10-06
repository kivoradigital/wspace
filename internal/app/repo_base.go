// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// repoBase is one repo's resolved comparison base.
type repoBase struct {
	// Branch is the base branch found, or, when Found is false, the first
	// configured candidate (the base that was expected), "" if none was.
	Branch domain.BranchName
	// Ref is the git ref to count against or start from ("origin/<b>" or
	// "<b>"); only meaningful when Found.
	Ref   ports.BaseRef
	Found bool
	// FromRemoteDefault is true when Branch came from <remote>/HEAD rather
	// than from a configured candidate.
	FromRemoteDefault bool
}

// baseCandidates lists, in precedence order and without duplicates, the
// configured bases for one repo: the project's origin branch, the
// project's own base_branch option, the base recorded for this repo in the
// manifest, then the workspace-wide base. project may be nil (no context,
// or the project is no longer registered).
func baseCandidates(project *domain.Project, recorded domain.BranchName, workspaceBase *domain.BranchName) []domain.BranchName {
	var raw []domain.BranchName
	if project != nil {
		if project.OriginBranch != nil {
			raw = append(raw, *project.OriginBranch)
		}
		if project.Options.BaseBranch != nil {
			raw = append(raw, *project.Options.BaseBranch)
		}
	}
	raw = append(raw, recorded)
	if workspaceBase != nil {
		raw = append(raw, *workspaceBase)
	}
	out := make([]domain.BranchName, 0, len(raw))
	seen := map[domain.BranchName]bool{}
	for _, b := range raw {
		if b == "" || seen[b] {
			continue
		}
		seen[b] = true
		out = append(out, b)
	}
	return out
}

// resolveRepoBase returns the first candidate that exists in repo, either
// as <remote>/<b> or as a local branch (ports.GitPort.ResolveBase's own
// order), else the remote's default branch (<remote>/HEAD) when it is
// recorded and exists. Nothing found is not an error: Found is false and
// Branch names the first candidate. Only a genuine git failure is
// returned.
func resolveRepoBase(ctx context.Context, deps Deps, repo domain.Path, remote string, candidates []domain.BranchName) (repoBase, error) {
	for _, b := range candidates {
		ref, ok, err := lookupBase(ctx, deps, repo, remote, b)
		if err != nil {
			return repoBase{}, err
		}
		if ok {
			return repoBase{Branch: b, Ref: ref, Found: true}, nil
		}
	}

	miss := repoBase{}
	if len(candidates) > 0 {
		miss.Branch = candidates[0]
	}
	def, ok, err := deps.Git.RemoteDefaultBranch(ctx, repo, remote)
	if err != nil || !ok {
		// The remote default is a best-effort fallback: an unreadable one
		// is the same as an unrecorded one.
		return miss, nil //nolint:nilerr // best-effort fallback
	}
	ref, ok, err := lookupBase(ctx, deps, repo, remote, def)
	if err != nil {
		return repoBase{}, err
	}
	if !ok {
		return miss, nil
	}
	return repoBase{Branch: def, Ref: ref, Found: true, FromRemoteDefault: true}, nil
}

// lookupBase adapts GitPort.ResolveBase, whose "neither exists" answer is
// the HEAD fallback create needs, into a plain found/not-found answer.
func lookupBase(ctx context.Context, deps Deps, repo domain.Path, remote string, b domain.BranchName) (ports.BaseRef, bool, error) {
	ref, err := deps.Git.ResolveBase(ctx, repo, remote, b)
	if err != nil {
		if domain.Code(err) == domain.CodeRefNotFound {
			return ports.BaseRef{}, false, nil
		}
		return ports.BaseRef{}, false, err
	}
	if ref.Ref == "HEAD" && !ref.Remote {
		return ports.BaseRef{}, false, nil
	}
	return ref, true, nil
}
