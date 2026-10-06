// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// mergeAutostashMin is the first git release whose `git merge` accepts
// --autostash (Git 2.27 release notes: "git merge" learns the
// "--autostash" option). `git rebase --autostash` predates the supported
// minimum (2.20), so it needs no gate.
var mergeAutostashMin = ports.Version{Major: 2, Minor: 27}

// UpdateRepoInput parameterizes UpdateRepo.
type UpdateRepoInput struct {
	WorkspaceRoot domain.Path
	Alias         string
	Strategy      domain.UpdateStrategy
	// Autostash stashes uncommitted tracked changes before integrating and
	// re-applies them after; without it a dirty worktree is refused.
	Autostash bool
}

// UpdateWorkspaceInput parameterizes UpdateWorkspace.
type UpdateWorkspaceInput struct {
	WorkspaceRoot domain.Path
	Strategy      domain.UpdateStrategy
	Autostash     bool
}

// RepoUpdate is one repository's update outcome. Err is nil on success
// (UpToDate or CommitsIntegrated > 0) and a *domain.OpError otherwise:
// CodeDetachedHead, CodeIntegrationInProgress, CodeBaseMissing,
// CodeWorktreeDirty (DirtyFiles set), CodeGitTooOld, CodeUpdateConflict
// (Conflicts set, Restored says whether the pre-update state was verified)
// or a git failure.
type RepoUpdate struct {
	Alias    string
	Strategy domain.UpdateStrategy
	// Base is the ref integrated ("origin/develop"); when the base is
	// missing it names the branch that was expected.
	Base              string
	BeforeHead        string
	AfterHead         string
	UpToDate          bool
	CommitsIntegrated int
	Conflicts         []string
	DirtyFiles        []string
	// Restored is true after a conflict when the repo was verified to be
	// back at its previous HEAD and status, with no operation in progress.
	Restored bool
	Err      error
}

// UpdateRepo brings one workspace repository up to date with its
// comparison base: it fetches the remote, resolves the base with the same
// per-repo precedence status uses (see baseCandidates), and merges or
// rebases it into the current branch. It never leaves the repository in
// the middle of a merge or rebase: a conflict is aborted and reported.
// Only a workspace or alias that cannot be found is returned as an error;
// every per-repository outcome is in the result.
func UpdateRepo(ctx context.Context, deps Deps, in UpdateRepoInput) (RepoUpdate, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return RepoUpdate{}, err
	}
	_, repo, found := findRepo(manifest.Workspace.Repos, in.Alias)
	if !found {
		return RepoUpdate{}, domain.NewOpError("workspace.update", domain.CodeRepoNotFound, in.Alias, "", nil)
	}
	owner := loadOwnerContext(ctx, deps, manifest.Workspace.Context, nil)
	res := updateOne(ctx, deps, manifest, owner, repo, in.Strategy, in.Autostash)
	deps.Reporter.Result(res)
	return res, nil
}

// UpdateWorkspace runs UpdateRepo's per-repository update for every repo
// of the workspace, sequentially and in manifest order. One repository's
// refusal or failure never stops the others.
func UpdateWorkspace(ctx context.Context, deps Deps, in UpdateWorkspaceInput) ([]RepoUpdate, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	owner := loadOwnerContext(ctx, deps, manifest.Workspace.Context, nil)
	out := make([]RepoUpdate, 0, len(manifest.Workspace.Repos))
	for _, repo := range manifest.Workspace.Repos {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out = append(out, updateOne(ctx, deps, manifest, owner, repo, in.Strategy, in.Autostash))
	}
	deps.Reporter.Result(out)
	return out, nil
}

func updateOne(ctx context.Context, deps Deps, manifest domain.Manifest, owner *domain.Context, repo domain.RepoEntry, s domain.UpdateStrategy, autostash bool) RepoUpdate {
	reportRepo(deps.Reporter, ports.RepoEvent{Op: OpUpdate, Repo: repo.Alias, Phase: ports.RepoStarted})
	res := integrateBase(ctx, deps, manifest, owner, repo, s, autostash)
	if res.Err != nil {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpUpdate, Repo: repo.Alias, Phase: ports.RepoFailed, Err: res.Err})
	} else {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpUpdate, Repo: repo.Alias, Phase: ports.RepoFinished})
	}
	return res
}

func updateErr(code domain.ErrCode, subject string) error {
	return domain.NewOpError("workspace.update", code, subject, "", nil)
}

// integrateBase is the per-repository update. Every refusal happens
// before anything is changed; the only mutation is the single Integrate
// call, undone on a conflict.
func integrateBase(ctx context.Context, deps Deps, manifest domain.Manifest, owner *domain.Context, repo domain.RepoEntry, s domain.UpdateStrategy, autostash bool) RepoUpdate {
	ws := manifest.Workspace
	worktree := ws.Root.Join(repo.Alias)
	res := RepoUpdate{Alias: repo.Alias, Strategy: s}
	fail := func(err error) RepoUpdate {
		res.Err = err
		if res.AfterHead == "" {
			res.AfterHead = res.BeforeHead
		}
		return res
	}

	_, detached, err := deps.Git.CurrentBranch(ctx, worktree)
	if err != nil {
		return fail(err)
	}
	if detached {
		return fail(updateErr(domain.CodeDetachedHead, repo.Alias))
	}
	if op, err := deps.Git.IntegrationInProgress(ctx, worktree); err != nil {
		return fail(err)
	} else if op != "" {
		return fail(updateErr(domain.CodeIntegrationInProgress, repo.Alias))
	}

	resolver := domain.Resolver{Context: owner, Workspace: &ws}
	remote := resolver.Remote(repo.Project).Value
	if err := deps.Git.Fetch(ctx, worktree, remote); err != nil {
		return fail(err)
	}
	var project *domain.Project
	if owner != nil {
		project = findProject(owner, repo.Project)
	}
	base, err := resolveRepoBase(ctx, deps, worktree, remote, baseCandidates(project, repo.BaseBranch, ws.Options.BaseBranch))
	if err != nil {
		return fail(err)
	}
	if !base.Found {
		res.Base = string(base.Branch)
		return fail(updateErr(domain.CodeBaseMissing, string(base.Branch)))
	}
	res.Base = base.Ref.Ref

	if res.BeforeHead, err = deps.Git.HeadCommit(ctx, worktree); err != nil {
		return fail(err)
	}
	behind, err := deps.Git.BehindCount(ctx, worktree, res.Base)
	if err != nil {
		return fail(err)
	}
	if behind == 0 {
		res.UpToDate = true
		res.AfterHead = res.BeforeHead
		return res
	}

	entries, err := deps.Git.Status(ctx, worktree)
	if err != nil {
		return fail(err)
	}
	dirty := domain.DirtyForUpdate(entries)
	if len(dirty) > 0 && !autostash {
		res.DirtyFiles = dirty
		return fail(updateErr(domain.CodeWorktreeDirty, repo.Alias))
	}
	stashing := len(dirty) > 0
	stashBefore := ""
	if stashing {
		if s == domain.UpdateMerge {
			v, err := deps.Git.Version(ctx)
			if err != nil {
				return fail(err)
			}
			if !versionAtLeast(v, mergeAutostashMin) {
				return fail(updateErr(domain.CodeGitTooOld, "2.27 (merge --autostash)"))
			}
		}
		if stashBefore, err = deps.Git.StashRef(ctx, worktree); err != nil {
			return fail(err)
		}
	}

	snapshot := preUpdate{head: res.BeforeHead, entries: entries}
	if err := deps.Git.Integrate(ctx, worktree, ports.IntegrateSpec{Ref: res.Base, Strategy: s, Autostash: stashing}); err != nil {
		return fail(abortStopped(ctx, deps, worktree, snapshot, &res, err))
	}
	if stashing {
		// git re-applies the autostash after integrating; when that
		// conflicts it exits 0, leaves conflict markers and keeps the
		// stash entry. Undo the whole update in that case.
		paths, err := deps.Git.ConflictedPaths(ctx, worktree)
		if err != nil {
			return fail(err)
		}
		if len(paths) > 0 {
			res.Conflicts = paths
			if ref, err := deps.Git.StashRef(ctx, worktree); err == nil && ref != "" && ref != stashBefore {
				if deps.Git.ResetHard(ctx, worktree, snapshot.head) == nil && deps.Git.StashPop(ctx, worktree) == nil {
					res.Restored = isRestored(ctx, deps, worktree, snapshot)
				}
			}
			res.AfterHead, _ = deps.Git.HeadCommit(ctx, worktree)
			return fail(updateErr(domain.CodeUpdateConflict, repo.Alias))
		}
	}

	if res.AfterHead, err = deps.Git.HeadCommit(ctx, worktree); err != nil {
		return fail(err)
	}
	res.CommitsIntegrated = behind
	return res
}

// preUpdate is what a repository must return to when an update is undone.
type preUpdate struct {
	head    string
	entries []domain.PorcelainEntry
}

// abortStopped handles a failed Integrate. When git stopped mid-merge or
// mid-rebase, the conflicting paths are collected, the operation is
// aborted and the restore verified; the result is CodeUpdateConflict.
// A failure that left nothing in progress is returned unchanged.
func abortStopped(ctx context.Context, deps Deps, worktree domain.Path, snap preUpdate, res *RepoUpdate, cause error) error {
	op, err := deps.Git.IntegrationInProgress(ctx, worktree)
	if err != nil || op == "" {
		res.AfterHead, _ = deps.Git.HeadCommit(ctx, worktree)
		return cause
	}
	paths, _ := deps.Git.ConflictedPaths(ctx, worktree)
	res.Conflicts = paths
	if deps.Git.AbortIntegration(ctx, worktree, op) == nil {
		res.Restored = isRestored(ctx, deps, worktree, snap)
	}
	res.AfterHead, _ = deps.Git.HeadCommit(ctx, worktree)
	return domain.NewOpError("workspace.update", domain.CodeUpdateConflict, res.Alias, cause.Error(), cause)
}

// isRestored verifies a repository is exactly as it was before the update:
// nothing in progress, the same HEAD and the same status entries.
func isRestored(ctx context.Context, deps Deps, worktree domain.Path, snap preUpdate) bool {
	if op, err := deps.Git.IntegrationInProgress(ctx, worktree); err != nil || op != "" {
		return false
	}
	if head, err := deps.Git.HeadCommit(ctx, worktree); err != nil || head != snap.head {
		return false
	}
	entries, err := deps.Git.Status(ctx, worktree)
	if err != nil || len(entries) != len(snap.entries) {
		return false
	}
	for i := range entries {
		if entries[i] != snap.entries[i] {
			return false
		}
	}
	return true
}

func versionAtLeast(v, min ports.Version) bool {
	if v.Major != min.Major {
		return v.Major > min.Major
	}
	if v.Minor != min.Minor {
		return v.Minor > min.Minor
	}
	return v.Patch >= min.Patch
}
