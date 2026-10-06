// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
)

// WorkspaceStatus is the aggregate a single workspace's per-repo status
// projects into. Both Status and List return it, and both the CLI's
// `--json` surface and the tray's stats cache consume it directly, with no
// reshaping between them (design.md §3: "RepoStatus is the read-only
// projection used by status, --json and the tray").
type WorkspaceStatus struct {
	Name  string
	Root  domain.Path
	Repos []domain.RepoStatus
	// Err is non-nil when this one workspace's status could not be fully
	// collected (e.g. a repo's worktree was removed by hand outside ws).
	// List reports it inline, marked, rather than aborting the whole call
	// over one damaged workspace (phase 6's disclosed defect: a tool
	// whose job is managing many workspaces must not go blind because one
	// of them is broken). Repos is empty whenever Err is non-nil.
	Err error
	// Legacy is true for a directory only the legacy bash tool manages
	// (".ws/workspace.conf" and no wspace manifest yet). Such an entry is
	// listed so it is never silently hidden, but it is not collected:
	// Repos is empty until AdoptLegacyWorkspaces gives it a manifest.
	Legacy bool
	// OrphanOf names the context this workspace's manifest says owns it
	// when that context no longer exists (it was deleted, or renamed by a
	// version that did not move its workspaces). Such a workspace is listed
	// for every context sharing the root, never hidden, and can be claimed
	// (ClaimWorkspaces). Empty otherwise. Set by List only.
	OrphanOf domain.ContextName
}

// StatusInput parameterizes Status.
type StatusInput struct {
	WorkspaceRoot domain.Path
}

// Status reports per-repo branch, ahead/behind counts and dirty state for
// one workspace (repository-operations spec: "Status and change
// classification").
func Status(ctx context.Context, deps Deps, in StatusInput) (WorkspaceStatus, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	return collectWorkspaceStatus(ctx, deps, manifest, loadOwnerContext(ctx, deps, manifest.Workspace.Context, nil))
}

// loadOwnerContext loads the context a manifest names, best-effort: its
// project records only refine each repo's comparison base, so a missing or
// unreadable context (or a manifest naming none) is simply nil. cache, when
// non-nil, memoizes the lookups across one List call.
func loadOwnerContext(ctx context.Context, deps Deps, name domain.ContextName, cache map[domain.ContextName]*domain.Context) *domain.Context {
	if name == "" {
		return nil
	}
	if c, ok := cache[name]; ok {
		return c
	}
	var out *domain.Context
	if c, err := deps.Store.LoadContext(ctx, name); err == nil {
		out = &c
	}
	if cache != nil {
		cache[name] = out
	}
	return out
}

// ListInput parameterizes List.
type ListInput struct {
	WorkspacesRoot domain.Path
	// Context, when set, keeps only the workspaces whose manifest names this
	// context (or names none, as manifests written before contexts were
	// recorded do, or names a context that no longer exists — those are
	// flagged OrphanOf). Several contexts may share one WorkspacesRoot.
	Context domain.ContextName
}

// List reports WorkspaceStatus for every workspace found directly under
// WorkspacesRoot (workspace-lifecycle spec: "Workspace listing and
// repair"). A directory that is not a workspace (no manifest) is silently
// skipped, unless the legacy bash tool created it: that one is reported
// with Legacy set (a legacy manifest names no context, so it belongs to
// every context sharing the root, like a context-less wspace manifest).
//
// A per-workspace collection failure (a corrupt manifest, a repo whose
// worktree was removed by hand, a git call that errors) never aborts the
// whole call: it is reported inline as that one WorkspaceStatus's Err,
// with every other, healthy workspace still collected and returned
// normally (phase 6's disclosed defect — see WorkspaceStatus.Err's own
// doc comment). Only a failure enumerating WorkspacesRoot itself
// (ListDirs) is still fatal to the whole call: there is no list to report
// partial results within.
func List(ctx context.Context, deps Deps, in ListInput) ([]WorkspaceStatus, error) {
	names, err := deps.FS.ListDirs(in.WorkspacesRoot)
	if err != nil {
		return nil, err
	}

	var out []WorkspaceStatus
	contexts := map[domain.ContextName]*domain.Context{}
	var existing map[domain.ContextName]bool
	for _, name := range names {
		root := in.WorkspacesRoot.Join(name)
		manifest, err := deps.Store.LoadManifest(ctx, root)
		if err != nil {
			if domain.Code(err) == domain.CodeWorkspaceNotFound {
				if isLegacyWorkspace(deps, root) {
					out = append(out, WorkspaceStatus{Name: name, Root: root, Legacy: true})
				}
				continue
			}
			out = append(out, WorkspaceStatus{Name: name, Root: root, Err: err})
			continue
		}
		var orphanOf domain.ContextName
		if owner := manifest.Workspace.Context; in.Context != "" && owner != "" && owner != in.Context {
			if existing == nil {
				// Best-effort: when the registry cannot be read, keep the
				// conservative rule and hide every other owner.
				if existing, err = existingContexts(ctx, deps.Store); err != nil {
					existing = nil
					continue
				}
			}
			if classifyOwner(owner, in.Context, existing) != ownedOrphan {
				continue
			}
			orphanOf = owner
		}
		ws, err := collectWorkspaceStatus(ctx, deps, manifest, loadOwnerContext(ctx, deps, manifest.Workspace.Context, contexts))
		if err != nil {
			out = append(out, WorkspaceStatus{Name: manifest.Workspace.Name, Root: manifest.Workspace.Root, Err: err, OrphanOf: orphanOf})
			continue
		}
		ws.OrphanOf = orphanOf
		out = append(out, ws)
	}
	return out, nil
}

// collectWorkspaceStatus builds a WorkspaceStatus from an already-loaded
// manifest, sharing repoStatus with DestroyWorkspace/RemoveRepo's
// repoChangeSet (tasks.md 4a.11). owner is the manifest's context, or nil;
// it supplies each repo's project-level base (see baseCandidates for the
// full precedence). A repo-level failure that is not about the comparison
// base (e.g. its worktree was removed) still fails the whole workspace, so
// the client offers Repair.
func collectWorkspaceStatus(ctx context.Context, deps Deps, manifest domain.Manifest, owner *domain.Context) (WorkspaceStatus, error) {
	ws := manifest.Workspace
	resolver := domain.Resolver{Context: owner, Workspace: &ws}

	repos := make([]domain.RepoStatus, 0, len(ws.Repos))
	for _, repo := range ws.Repos {
		worktree := ws.Root.Join(repo.Alias)
		var project *domain.Project
		if owner != nil {
			project = findProject(owner, repo.Project)
		}
		candidates := baseCandidates(project, repo.BaseBranch, ws.Options.BaseBranch)
		remote := resolver.Remote(repo.Project).Value
		base := func(ctx context.Context) (repoBase, error) {
			return resolveRepoBase(ctx, deps, worktree, remote, candidates)
		}
		rs, err := repoStatus(ctx, deps, repo.Alias, worktree, manifest.EnvCopies, base)
		if err != nil {
			return WorkspaceStatus{}, err
		}
		rs.Project = repo.Project
		repos = append(repos, rs)
	}

	return WorkspaceStatus{Name: ws.Name, Root: ws.Root, Repos: repos}, nil
}
