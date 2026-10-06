// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"
	"errors"
	"time"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

var errNegative = errors.New("must not be negative")

// WorkspacesRef selects a context for workspaces.list.
type WorkspacesRef struct {
	Context string `json:"context,omitempty"`
}

// ListWorkspaces returns every workspace in a context with live status. A
// damaged workspace is reported inline (Error set), never hiding the rest.
func (e *Engine) ListWorkspaces(ctx context.Context, p WorkspacesRef) ([]WorkspaceStatus, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return nil, err
	}
	statuses, err := app.List(ctx, e.appDeps(nil), app.ListInput{WorkspacesRoot: c.WorkspacesRoot, Context: c.Name})
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]WorkspaceStatus, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, workspaceStatusToDTO(s))
	}
	return out, nil
}

func workspaceStatusToDTO(s app.WorkspaceStatus) WorkspaceStatus {
	out := WorkspaceStatus{Name: s.Name, Path: string(s.Root), Repos: []RepoStatus{}, Legacy: s.Legacy, OrphanOf: string(s.OrphanOf)}
	if s.Err != nil {
		body := AsError(s.Err).Body()
		out.Error = &body
		return out
	}
	for _, r := range s.Repos {
		rs := repoStatusToDTO(r)
		out.Dirty = out.Dirty || rs.Dirty
		out.Repos = append(out.Repos, rs)
	}
	out.RepoCount = len(out.Repos)
	return out
}

// AdoptedWorkspace is one legacy workspace that now has a wspace manifest.
type AdoptedWorkspace struct {
	Name string `json:"name"`
	Root string `json:"root"`
}

// SkippedWorkspace is one legacy workspace left alone. Reason is a stable
// code; Message is its rendered explanation.
type SkippedWorkspace struct {
	Root    string `json:"root"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// AdoptLegacyResult is workspaces.adoptLegacy's result.
type AdoptLegacyResult struct {
	Adopted []AdoptedWorkspace `json:"adopted"`
	Skipped []SkippedWorkspace `json:"skipped"`
}

// AdoptLegacyWorkspaces gives every workspace the legacy bash tool created
// in a context's WorkspacesRoot a wspace manifest, without touching the
// legacy files. Problems are reported per workspace, never as a failure of
// the whole call.
func (e *Engine) AdoptLegacyWorkspaces(ctx context.Context, p WorkspacesRef) (AdoptLegacyResult, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return AdoptLegacyResult{}, err
	}
	res, err := app.AdoptLegacyWorkspaces(ctx, e.appDeps(nil), app.AdoptLegacyWorkspacesInput{Context: c})
	if err != nil {
		return AdoptLegacyResult{}, wrap(err)
	}
	return adoptionToDTO(res), nil
}

func adoptionToDTO(r app.AdoptLegacyWorkspacesResult) AdoptLegacyResult {
	out := AdoptLegacyResult{Adopted: []AdoptedWorkspace{}, Skipped: []SkippedWorkspace{}}
	for _, a := range r.Adopted {
		out.Adopted = append(out.Adopted, AdoptedWorkspace{Name: a.Name, Root: string(a.Root)})
	}
	for _, s := range r.Skipped {
		out.Skipped = append(out.Skipped, SkippedWorkspace{Root: string(s.Root), Reason: string(s.Reason), Message: s.Message()})
	}
	return out
}

// ClaimWorkspacesParams are workspaces.claim's parameters. Workspaces
// names the workspace folders to claim; empty means every orphaned
// workspace in the context's workspaces root.
type ClaimWorkspacesParams struct {
	Context    string   `json:"context,omitempty"`
	Workspaces []string `json:"workspaces,omitempty"`
}

// ClaimedWorkspace is one orphaned workspace now owned by the claiming
// context; PreviousContext is the removed context it named.
type ClaimedWorkspace struct {
	Name            string `json:"name"`
	Root            string `json:"root"`
	PreviousContext string `json:"previousContext"`
}

// ClaimSkipped is one named workspace that was not claimed. Reason is a
// stable code; Message is its rendered explanation.
type ClaimSkipped struct {
	Name    string `json:"name"`
	Root    string `json:"root,omitempty"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// ClaimWorkspacesResult is workspaces.claim's result.
type ClaimWorkspacesResult struct {
	Claimed []ClaimedWorkspace `json:"claimed"`
	Skipped []ClaimSkipped     `json:"skipped"`
}

// ClaimWorkspaces makes a context the owner of orphaned workspaces (their
// manifest names a context that no longer exists). It never takes a
// workspace from an existing context and rewrites only the owner field;
// problems are reported per workspace. Idempotent.
func (e *Engine) ClaimWorkspaces(ctx context.Context, p ClaimWorkspacesParams) (ClaimWorkspacesResult, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return ClaimWorkspacesResult{}, err
	}
	res, err := app.ClaimWorkspaces(ctx, e.appDeps(nil), app.ClaimWorkspacesInput{Context: c, Workspaces: p.Workspaces})
	if err != nil {
		return ClaimWorkspacesResult{}, wrap(err)
	}
	return claimToDTO(res), nil
}

func claimToDTO(r app.ClaimWorkspacesResult) ClaimWorkspacesResult {
	out := ClaimWorkspacesResult{Claimed: []ClaimedWorkspace{}, Skipped: []ClaimSkipped{}}
	for _, c := range r.Claimed {
		out.Claimed = append(out.Claimed, ClaimedWorkspace{Name: c.Name, Root: string(c.Root), PreviousContext: string(c.PreviousContext)})
	}
	for _, s := range r.Skipped {
		out.Skipped = append(out.Skipped, ClaimSkipped{Name: s.Name, Root: string(s.Root), Reason: string(s.Reason), Message: s.Message()})
	}
	return out
}

// WorkspaceRef names one workspace in a context.
type WorkspaceRef struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
}

// WorkspaceStatus returns one workspace's live per-repo status.
func (e *Engine) WorkspaceStatus(ctx context.Context, p WorkspaceRef) (WorkspaceStatus, error) {
	root, _, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	s, err := app.Status(ctx, e.appDeps(nil), app.StatusInput{WorkspaceRoot: root})
	if err != nil {
		return WorkspaceStatus{}, wrap(err)
	}
	return workspaceStatusToDTO(s), nil
}

// CreateWorkspaceParams are workspaces.create's parameters. Projects
// selects project keys (empty means every project in the context);
// Options overrides resolved options for this workspace only (the
// equivalent of CLI flags).
type CreateWorkspaceParams struct {
	Context  string   `json:"context,omitempty"`
	Name     string   `json:"name"`
	Branch   string   `json:"branch,omitempty"`
	Projects []string `json:"projects,omitempty"`
	Options  *Options `json:"options,omitempty"`
	// CopyNodeModules copies each selected Node project's node_modules
	// from its main clone into the new worktree.
	CopyNodeModules bool `json:"copyNodeModules,omitempty"`
}

// CreateWorkspace creates a workspace, streaming per-repo progress to
// emit. A failure after the first worktree was created is rolled back.
func (e *Engine) CreateWorkspace(ctx context.Context, p CreateWorkspaceParams, emit ProgressFunc) (Workspace, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return Workspace{}, err
	}
	name, err := domain.NewWorkspaceName(p.Name)
	if err != nil {
		return Workspace{}, invalidParam("name", err)
	}
	if p.Branch != "" {
		if _, err := domain.NewBranchName(p.Branch); err != nil {
			return Workspace{}, invalidParam("branch", err)
		}
	}
	flags, err := p.Options.toDomain("options")
	if err != nil {
		return Workspace{}, err
	}
	keys := make([]domain.ProjectKey, 0, len(p.Projects))
	for _, k := range p.Projects {
		if findProjectKey(c, k) {
			keys = append(keys, domain.ProjectKey(k))
			continue
		}
		return Workspace{}, AsError(domain.NewOpError("workspace.create", domain.CodeProjectNotFound, k, "", nil))
	}

	res, err := app.CreateWorkspace(ctx, e.appDeps(emit), app.CreateWorkspaceInput{
		Context: c, Flags: flags, Name: name, Branch: p.Branch, ProjectKeys: keys,
		CopyNodeModules: p.CopyNodeModules,
	})
	if err != nil {
		return Workspace{}, wrap(err)
	}
	ws := res.Workspace
	out := Workspace{
		Name:    ws.Name,
		Path:    string(ws.Root),
		Context: string(ws.Context),
		Branch:  string(ws.Branch),
		Created: ws.Created.UTC().Format(time.RFC3339),
		Repos:   []RepoEntry{},
	}
	for _, r := range ws.Repos {
		out.Repos = append(out.Repos, repoEntryToDTO(r))
	}
	return out, nil
}

func findProjectKey(c domain.Context, key string) bool {
	for _, p := range c.Projects {
		if string(p.Key) == key {
			return true
		}
	}
	return false
}

// DestroyWorkspaceParams are workspaces.destroy's parameters. Without
// Force, any uncommitted tracked change, foreign untracked file or
// unpushed commit makes the call fail with needs_confirmation, listing
// every reason; nothing is removed in that case.
type DestroyWorkspaceParams struct {
	Context        string `json:"context,omitempty"`
	Workspace      string `json:"workspace"`
	Force          bool   `json:"force,omitempty"`
	DeleteBranches bool   `json:"deleteBranches,omitempty"`
}

// PathResult reports the filesystem path an operation acted on.
type PathResult struct {
	Path string `json:"path"`
}

// DestroyWorkspace tears a workspace down.
func (e *Engine) DestroyWorkspace(ctx context.Context, p DestroyWorkspaceParams, emit ProgressFunc) (PathResult, error) {
	root, _, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return PathResult{}, err
	}
	deps := e.appDeps(emit)
	if !p.Force {
		blockers, err := e.TeardownBlockers(ctx, WorkspaceRef{Context: p.Context, Workspace: p.Workspace})
		if err != nil {
			return PathResult{}, err
		}
		if len(blockers) > 0 {
			return PathResult{}, needsConfirmation(messages.EngineDestroyNeedsConfirmation, p.Workspace, blockers)
		}
	}
	if err := app.DestroyWorkspace(ctx, deps, app.DestroyWorkspaceInput{WorkspaceRoot: root, Force: p.Force, DeleteBranches: p.DeleteBranches}); err != nil {
		return PathResult{}, wrap(err)
	}
	return PathResult{Path: string(root)}, nil
}

// TeardownBlockers lists every reason an unforced destroy of the
// workspace would be refused, without changing anything. Empty means an
// unforced destroy would proceed.
func (e *Engine) TeardownBlockers(ctx context.Context, p WorkspaceRef) ([]Blocker, error) {
	return e.teardownBlockers(ctx, p, "")
}

func (e *Engine) teardownBlockers(ctx context.Context, p WorkspaceRef, alias string) ([]Blocker, error) {
	root, _, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return nil, err
	}
	sets, err := app.TeardownBlockers(ctx, e.appDeps(nil), app.TeardownCheckInput{WorkspaceRoot: root, Alias: alias})
	if err != nil {
		return nil, wrap(err)
	}
	out := []Blocker{}
	for _, set := range sets {
		for _, ch := range set.Blocking() {
			b := Blocker{Repo: set.Alias, Path: ch.RelPath, Kind: "tracked_change", Message: messages.T(messages.EngineBlockerTrackedChange, set.Alias, ch.RelPath)}
			if ch.Class == domain.ChangeForeign {
				b.Kind = "foreign_file"
				b.Message = messages.T(messages.EngineBlockerForeignFile, set.Alias, ch.RelPath)
			}
			out = append(out, b)
		}
		if set.Unpushed > 0 {
			out = append(out, Blocker{Repo: set.Alias, Kind: "unpushed_commits", Count: set.Unpushed, Message: messages.T(messages.EngineBlockerUnpushedCommits, set.Alias, set.Unpushed)})
		}
	}
	return out, nil
}

func needsConfirmation(key messages.Key, subject string, blockers []Blocker) *Error {
	e := newError(CodeNeedsConfirmation, key, subject, len(blockers))
	e.Data = map[string]any{"reasons": blockers}
	return e
}

// AddRepoParams are workspaces.addRepo's parameters.
type AddRepoParams struct {
	Context   string   `json:"context,omitempty"`
	Workspace string   `json:"workspace"`
	Project   string   `json:"project"`
	Options   *Options `json:"options,omitempty"`
	// CopyNodeModules: see CreateWorkspaceParams.CopyNodeModules.
	CopyNodeModules bool `json:"copyNodeModules,omitempty"`
}

// AddRepo mounts one more project into an existing workspace (rolled back
// on failure, leaving the workspace as it was).
func (e *Engine) AddRepo(ctx context.Context, p AddRepoParams, emit ProgressFunc) (RepoEntry, error) {
	root, c, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return RepoEntry{}, err
	}
	if !findProjectKey(c, p.Project) {
		return RepoEntry{}, AsError(domain.NewOpError("workspace.add_repo", domain.CodeProjectNotFound, p.Project, "", nil))
	}
	flags, err := p.Options.toDomain("options")
	if err != nil {
		return RepoEntry{}, err
	}
	entry, err := app.AddRepo(ctx, e.appDeps(emit), app.AddRepoInput{WorkspaceRoot: root, Context: c, Flags: flags, ProjectKey: domain.ProjectKey(p.Project), CopyNodeModules: p.CopyNodeModules})
	if err != nil {
		return RepoEntry{}, wrap(err)
	}
	return repoEntryToDTO(entry), nil
}

// RemoveRepoParams are workspaces.removeRepo's parameters; Force has the
// same meaning as for DestroyWorkspaceParams, scoped to one repo.
type RemoveRepoParams struct {
	Context      string `json:"context,omitempty"`
	Workspace    string `json:"workspace"`
	Repo         string `json:"repo"`
	Force        bool   `json:"force,omitempty"`
	DeleteBranch bool   `json:"deleteBranch,omitempty"`
}

// AliasResult reports the repo alias an operation acted on.
type AliasResult struct {
	Alias string `json:"alias"`
}

// RemoveRepo unmounts one repo from a workspace.
func (e *Engine) RemoveRepo(ctx context.Context, p RemoveRepoParams, emit ProgressFunc) (AliasResult, error) {
	root, _, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return AliasResult{}, err
	}
	if !p.Force {
		blockers, err := e.teardownBlockers(ctx, WorkspaceRef{Context: p.Context, Workspace: p.Workspace}, p.Repo)
		if err != nil {
			return AliasResult{}, err
		}
		if len(blockers) > 0 {
			return AliasResult{}, needsConfirmation(messages.EngineRemoveNeedsConfirmation, p.Repo, blockers)
		}
	}
	if err := app.RemoveRepo(ctx, e.appDeps(emit), app.RemoveRepoInput{WorkspaceRoot: root, Alias: p.Repo, Force: p.Force, DeleteBranch: p.DeleteBranch}); err != nil {
		return AliasResult{}, wrap(err)
	}
	return AliasResult{Alias: p.Repo}, nil
}

// RepairResult lists the recreated worktrees.
type RepairResult struct {
	Recreated []string `json:"recreated"`
}

// Repair recreates every worktree the manifest declares but that is
// missing from disk.
func (e *Engine) Repair(ctx context.Context, p WorkspaceRef, emit ProgressFunc) (RepairResult, error) {
	root, _, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return RepairResult{}, err
	}
	res, err := app.Repair(ctx, e.appDeps(emit), app.RepairInput{WorkspaceRoot: root})
	if err != nil {
		return RepairResult{}, wrap(err)
	}
	return RepairResult{Recreated: append([]string{}, res.Recreated...)}, nil
}

// SyncEnvResult lists the env files re-copied (workspace-relative).
type SyncEnvResult struct {
	Copied []string `json:"copied"`
}

// SyncEnv re-copies env files into every repo of a workspace.
func (e *Engine) SyncEnv(ctx context.Context, p WorkspaceRef, emit ProgressFunc) (SyncEnvResult, error) {
	root, _, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return SyncEnvResult{}, err
	}
	res, err := app.SyncEnv(ctx, e.appDeps(emit), app.SyncEnvInput{WorkspaceRoot: root})
	if err != nil {
		return SyncEnvResult{}, wrap(err)
	}
	return SyncEnvResult{Copied: append([]string{}, res.Copied...)}, nil
}

// RepoRef names one repository (by alias) of one workspace.
type RepoRef struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
}

// FileChange is one changed path of a repository (workspaces.repoChanges).
type FileChange struct {
	Path     string `json:"path"`
	OrigPath string `json:"origPath,omitempty"`
	Status   string `json:"status"`
	Staged   bool   `json:"staged"`
	Unstaged bool   `json:"unstaged"`
}

// RepoChanges lists one repository's changed files: modified, added,
// deleted, renamed (with origPath), copied, typechange, untracked and
// conflicted paths, staged and unstaged, in git's order.
func (e *Engine) RepoChanges(ctx context.Context, p RepoRef) ([]FileChange, error) {
	root, _, err := e.workspaceRoot(ctx, p.Context, p.Workspace)
	if err != nil {
		return nil, err
	}
	changes, err := app.RepoChanges(ctx, e.appDeps(nil), root, p.Repo)
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]FileChange, 0, len(changes))
	for _, c := range changes {
		out = append(out, FileChange{Path: c.Path, OrigPath: c.OrigPath, Status: string(c.Status), Staged: c.Staged, Unstaged: c.Unstaged})
	}
	return out, nil
}
