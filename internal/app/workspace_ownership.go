// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Workspace ownership. A manifest's workspace.context names the context
// that owns it. Several contexts may share one WorkspacesRoot, so List
// filters by that owner. Relative to a context C, a manifest is:
//
//   - owned: its owner is C;
//   - shared: it names no owner (written before owners were recorded),
//     so it belongs to every context sharing the root;
//   - foreign: its owner is another context that exists — hidden from C;
//   - orphaned: its owner names a context that no longer exists (renamed
//     by an older version, or deleted). It is shown to every context
//     sharing the root, flagged, and can be claimed.
type ownership int

const (
	ownedBySelf ownership = iota
	ownedShared
	ownedByOther
	ownedOrphan
)

func classifyOwner(owner, self domain.ContextName, existing map[domain.ContextName]bool) ownership {
	switch {
	case owner == self:
		return ownedBySelf
	case owner == "":
		return ownedShared
	case existing[owner]:
		return ownedByOther
	default:
		return ownedOrphan
	}
}

// existingContexts returns the set of registered context names.
func existingContexts(ctx context.Context, store ports.ConfigStore) (map[domain.ContextName]bool, error) {
	names, err := store.ListContexts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.ContextName]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out, nil
}

// setOwner rewrites only workspace.context in the manifest at root and
// saves it atomically through the config store; every other field is kept.
func setOwner(ctx context.Context, store ports.ConfigStore, m domain.Manifest, owner domain.ContextName) error {
	m.Workspace.Context = owner
	return store.SaveManifest(ctx, m.Workspace.Root, m)
}

func workspaceDisplayName(m domain.Manifest, dir string) string {
	if m.Workspace.Name != "" {
		return m.Workspace.Name
	}
	return dir
}

// ReassignedWorkspace is one workspace whose owner was moved to a
// context's new name.
type ReassignedWorkspace struct {
	Name string
	Root domain.Path
}

// ReassignFailure is one workspace whose manifest could not be rewritten
// (or a WorkspacesRoot that could not be scanned, Name empty).
type ReassignFailure struct {
	Name string
	Root domain.Path
	Err  error
}

// WorkspaceReassignment reports what a context rename did to the
// manifests of the workspaces the old name owned.
type WorkspaceReassignment struct {
	Reassigned []ReassignedWorkspace
	Failures   []ReassignFailure
}

// reassignWorkspaces rewrites the owner of every manifest directly under
// each of roots whose owner is from, to to. It runs after the renamed
// context is saved: a failure is reported per workspace and never undoes
// the rename. Manifests that cannot be read are skipped (their owner is
// unknown); legacy ".ws/" files are never touched.
func reassignWorkspaces(ctx context.Context, store ports.ConfigStore, fs ports.FileSystemPort, roots []domain.Path, from, to domain.ContextName) WorkspaceReassignment {
	var out WorkspaceReassignment
	seen := map[domain.Path]bool{}
	for _, wsRoot := range roots {
		if wsRoot == "" || seen[wsRoot] {
			continue
		}
		seen[wsRoot] = true
		dirs, err := fs.ListDirs(wsRoot)
		if err != nil {
			out.Failures = append(out.Failures, ReassignFailure{Root: wsRoot, Err: err})
			continue
		}
		for _, dir := range dirs {
			root := wsRoot.Join(dir)
			m, err := store.LoadManifest(ctx, root)
			if err != nil || m.Workspace.Context != from {
				continue
			}
			m.Workspace.Root = root
			name := workspaceDisplayName(m, dir)
			if err := setOwner(ctx, store, m, to); err != nil {
				out.Failures = append(out.Failures, ReassignFailure{Name: name, Root: root, Err: err})
				continue
			}
			out.Reassigned = append(out.Reassigned, ReassignedWorkspace{Name: name, Root: root})
		}
	}
	return out
}

// ClaimWorkspacesInput parameterizes ClaimWorkspaces.
type ClaimWorkspacesInput struct {
	// Context claims; its WorkspacesRoot is where workspaces are looked up.
	Context domain.Context
	// Workspaces names the folders to claim. Empty means every orphaned
	// workspace under the root (silently ignoring every other one).
	Workspaces []string
}

// ClaimedWorkspace is one orphaned workspace now owned by the claiming
// context.
type ClaimedWorkspace struct {
	Name            string
	Root            domain.Path
	PreviousContext domain.ContextName
}

// SkippedClaim is one named workspace that was not claimed. Reason is a
// catalog key whose template takes Detail as its only argument.
type SkippedClaim struct {
	Name   string
	Root   domain.Path
	Reason messages.Key
	Detail string
}

// Message renders the skip reason through the catalog.
func (s SkippedClaim) Message() string { return messages.T(s.Reason, s.Detail) }

// ClaimWorkspacesResult reports every workspace claimed or skipped.
type ClaimWorkspacesResult struct {
	Claimed []ClaimedWorkspace
	Skipped []SkippedClaim
}

// ClaimWorkspaces makes Context the owner of orphaned workspaces: those
// whose manifest names a context that no longer exists. It never takes a
// workspace from an existing context, never touches a shared (ownerless)
// one, and rewrites only the manifest's owner field. It is idempotent: a
// second run has nothing left to claim. Only a failure to list the
// registered contexts or the WorkspacesRoot is returned as an error.
func ClaimWorkspaces(ctx context.Context, deps Deps, in ClaimWorkspacesInput) (ClaimWorkspacesResult, error) {
	var res ClaimWorkspacesResult
	existing, err := existingContexts(ctx, deps.Store)
	if err != nil {
		return res, err
	}
	wsRoot := in.Context.WorkspacesRoot

	if len(in.Workspaces) == 0 {
		dirs, err := deps.FS.ListDirs(wsRoot)
		if err != nil {
			return res, err
		}
		for _, dir := range dirs {
			root := wsRoot.Join(dir)
			m, err := deps.Store.LoadManifest(ctx, root)
			if err != nil || classifyOwner(m.Workspace.Context, in.Context.Name, existing) != ownedOrphan {
				continue
			}
			m.Workspace.Root = root
			claimOne(ctx, deps, &res, in.Context.Name, m, dir)
		}
		return res, nil
	}

	for _, name := range in.Workspaces {
		skip := func(root domain.Path, reason messages.Key, detail string) {
			res.Skipped = append(res.Skipped, SkippedClaim{Name: name, Root: root, Reason: reason, Detail: detail})
		}
		if _, err := domain.NewWorkspaceName(name); err != nil {
			skip("", messages.ClaimSkipInvalidName, name)
			continue
		}
		root := wsRoot.Join(name)
		m, err := deps.Store.LoadManifest(ctx, root)
		if err != nil {
			if domain.Code(err) == domain.CodeWorkspaceNotFound {
				skip(root, messages.ClaimSkipNotFound, name)
			} else {
				skip(root, messages.ClaimSkipManifestUnreadable, err.Error())
			}
			continue
		}
		m.Workspace.Root = root
		switch classifyOwner(m.Workspace.Context, in.Context.Name, existing) {
		case ownedBySelf:
			skip(root, messages.ClaimSkipAlreadyOwned, string(in.Context.Name))
		case ownedShared:
			skip(root, messages.ClaimSkipShared, string(wsRoot))
		case ownedByOther:
			skip(root, messages.ClaimSkipOwnedByOther, string(m.Workspace.Context))
		case ownedOrphan:
			claimOne(ctx, deps, &res, in.Context.Name, m, name)
		}
	}
	return res, nil
}

func claimOne(ctx context.Context, deps Deps, res *ClaimWorkspacesResult, to domain.ContextName, m domain.Manifest, dir string) {
	name := workspaceDisplayName(m, dir)
	previous := m.Workspace.Context
	if err := setOwner(ctx, deps.Store, m, to); err != nil {
		res.Skipped = append(res.Skipped, SkippedClaim{Name: name, Root: m.Workspace.Root, Reason: messages.ClaimSkipWriteFailed, Detail: err.Error()})
		return
	}
	res.Claimed = append(res.Claimed, ClaimedWorkspace{Name: name, Root: m.Workspace.Root, PreviousContext: previous})
}

// RenderClaimSummary renders result for the CLI and the import summary.
func RenderClaimSummary(result ClaimWorkspacesResult) []string {
	if len(result.Claimed) == 0 && len(result.Skipped) == 0 {
		return []string{messages.T(messages.ClaimSummaryNone)}
	}
	var lines []string
	if len(result.Claimed) > 0 {
		lines = append(lines, messages.T(messages.ClaimSummaryClaimedHeader))
		for _, c := range result.Claimed {
			lines = append(lines, messages.T(messages.ClaimSummaryClaimedRow, c.Name, string(c.Root), string(c.PreviousContext)))
		}
	}
	if len(result.Skipped) > 0 {
		lines = append(lines, messages.T(messages.ClaimSummarySkippedHeader))
		for _, s := range result.Skipped {
			lines = append(lines, messages.T(messages.ClaimSummarySkippedRow, s.Name, s.Message()))
		}
	}
	return lines
}
