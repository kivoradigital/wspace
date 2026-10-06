// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// DestroyWorkspaceInput parameterizes DestroyWorkspace.
type DestroyWorkspaceInput struct {
	WorkspaceRoot  domain.Path
	Force          bool
	DeleteBranches bool
}

// DestroyWorkspace refuses when the caller's cwd is inside the target
// workspace, classifies every repo's uncommitted changes into
// tracked/foreign/env-copy, blocks on tracked or foreign changes (or any
// unpushed commits) unless forced, archives the manifest before removing
// anything, and prunes worktree registrations after
// (workspace-lifecycle spec: "Destructive teardown safety").
func DestroyWorkspace(ctx context.Context, deps Deps, in DestroyWorkspaceInput) error {
	cwd, err := deps.FS.Cwd()
	if err != nil {
		return err
	}
	if cwdInside(cwd, in.WorkspaceRoot) {
		return domain.NewOpError("workspace.destroy", domain.CodeCwdInsideWorkspace, string(in.WorkspaceRoot), "current working directory is inside the workspace", nil)
	}

	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return err
	}

	if !in.Force {
		for _, repo := range manifest.Workspace.Repos {
			worktree := in.WorkspaceRoot.Join(repo.Alias)
			changes, err := repoChangeSet(ctx, deps, repo.Alias, worktree, manifest.EnvCopies, "HEAD")
			if err != nil {
				return err
			}
			if !changes.SafeToRemove() {
				reportBlocking(deps.Reporter, changes)
				return domain.NewOpError("workspace.destroy", domain.CodeUnsafeTeardown, repo.Alias, "", nil)
			}
		}
	}

	if err := deps.Store.ArchiveManifest(ctx, in.WorkspaceRoot); err != nil {
		return err
	}

	for _, repo := range manifest.Workspace.Repos {
		worktree := in.WorkspaceRoot.Join(repo.Alias)
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpDestroyWorkspace, Repo: repo.Alias, Phase: ports.RepoStarted})
		// Always force the git-level removal here, never the caller's raw
		// in.Force: by this point removal has already been fully vetted,
		// either because the caller asked to override or because the
		// !in.Force loop above already confirmed every repo's remaining
		// untracked content (if any) is this tool's own recorded env
		// copy, never a tracked or foreign change (domain.ChangeSet.
		// SafeToRemove). Real git's own worktree-remove has no concept of
		// "env copy" — it refuses on any untracked file at all unless
		// told --force — so passing the caller's in.Force through here
		// would silently reintroduce exactly the blocking case the
		// classification above just cleared, undoing "destroy ignores
		// its own env copies" at the very last step.
		if err := teardownRepo(ctx, deps, repo, worktree, in.DeleteBranches, in.Force); err != nil {
			reportRepo(deps.Reporter, ports.RepoEvent{Op: OpDestroyWorkspace, Repo: repo.Alias, Phase: ports.RepoFailed, Err: err})
			return err
		}
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpDestroyWorkspace, Repo: repo.Alias, Phase: ports.RepoFinished})
	}

	if err := deps.FS.RemoveAll(in.WorkspaceRoot); err != nil {
		return err
	}

	deps.Reporter.Result(struct {
		WorkspaceRoot domain.Path
	}{WorkspaceRoot: in.WorkspaceRoot})
	return nil
}

// cwdInside reports whether cwd is the workspace root itself or nested
// under it.
func cwdInside(cwd, workspaceRoot domain.Path) bool {
	if cwd == workspaceRoot {
		return true
	}
	return strings.HasPrefix(string(cwd), string(workspaceRoot)+"/")
}

// reportBlocking warns once per blocking change and once for an unpushed
// count, so a caller sees exactly what is stopping teardown
// (workspace-lifecycle spec: "Block on foreign changes without --force").
func reportBlocking(reporter ports.Reporter, changes domain.ChangeSet) {
	for _, change := range changes.Blocking() {
		reporter.Warn(messages.TeardownBlockedChange, changes.Alias, change.RelPath)
	}
	if changes.Unpushed > 0 {
		reporter.Warn(messages.TeardownBlockedUnpushed, changes.Alias, changes.Unpushed)
	}
}

// teardownRepo removes one repo's worktree (always forced at the git
// level — see DestroyWorkspace's own comment for why), prunes its
// registration and, when asked, deletes its branch. DestroyWorkspace and
// RemoveRepo share it.
func teardownRepo(ctx context.Context, deps Deps, repo domain.RepoEntry, worktree domain.Path, deleteBranch, force bool) error {
	if err := deps.Git.WorktreeRemove(ctx, repo.SourceDir, worktree, true); err != nil {
		return err
	}
	if err := deps.Git.WorktreePrune(ctx, repo.SourceDir); err != nil {
		return err
	}
	if deleteBranch {
		if err := deps.Git.DeleteBranch(ctx, repo.SourceDir, repo.Branch, force); err != nil {
			return err
		}
	}
	return nil
}
