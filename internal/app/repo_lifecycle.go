// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"path"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// AddRepoInput parameterizes AddRepo: mount one more project's worktree
// into an already-existing workspace.
type AddRepoInput struct {
	WorkspaceRoot domain.Path
	Context       domain.Context
	Overlay       *domain.Overlay
	Flags         domain.Options
	ProjectKey    domain.ProjectKey
	// CopyNodeModules: see CreateWorkspaceInput.CopyNodeModules.
	CopyNodeModules bool
}

// AddRepo validates that in.ProjectKey is not already mounted in the
// target workspace (CodeAlreadyInWorkspace, see inWorkspace), then pre-flights and mutates exactly like
// CreateWorkspace's single-project path before extending the manifest
// (workspace-lifecycle spec's creation safety applies identically to a
// single added repo). A mutate failure is rolled back exactly like
// CreateWorkspace's (the new worktree removed, its branch deleted when this
// call created it); the existing workspace and manifest are left intact.
func AddRepo(ctx context.Context, deps Deps, in AddRepoInput) (domain.RepoEntry, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return domain.RepoEntry{}, err
	}
	project := findProject(&in.Context, in.ProjectKey)
	if project == nil {
		return domain.RepoEntry{}, domain.NewOpError("workspace.add_repo", domain.CodeWorkspaceNotFound, string(in.ProjectKey), "", nil)
	}
	if inWorkspace(manifest.Workspace.Repos, *project) {
		return domain.RepoEntry{}, domain.NewOpError("workspace.add_repo", domain.CodeAlreadyInWorkspace, string(in.ProjectKey), "", nil)
	}

	resolver := domain.Resolver{Flags: in.Flags, Context: &in.Context, Workspace: &manifest.Workspace, Overlay: in.Overlay}
	plan, err := preflightCreate(ctx, deps, resolver, CreateWorkspaceInput{
		Context: in.Context,
		Overlay: in.Overlay,
		Flags:   in.Flags,
		Name:    manifest.Workspace.Name,
		Branch:  string(manifest.Workspace.Branch),

		CopyNodeModules: in.CopyNodeModules,
	}, []domain.Project{*project}, in.WorkspaceRoot)
	if err != nil {
		return domain.RepoEntry{}, err
	}

	// A mutate failure rolls back only the one worktree this call created;
	// the workspace directory and every repo already in it pre-existed
	// this call and are never touched (rolledBack is irrelevant here).
	repos, envCopies, _, err := mutateCreate(ctx, deps, OpAddRepo, plan, in.WorkspaceRoot)
	if err != nil {
		return domain.RepoEntry{}, err
	}

	manifest.Workspace.Repos = append(manifest.Workspace.Repos, repos...)
	manifest.EnvCopies = append(manifest.EnvCopies, envCopies...)
	if err := deps.Store.SaveManifest(ctx, in.WorkspaceRoot, manifest); err != nil {
		return domain.RepoEntry{}, err
	}

	deps.Reporter.Result(repos[0])
	return repos[0], nil
}

// RemoveRepoInput parameterizes RemoveRepo.
type RemoveRepoInput struct {
	WorkspaceRoot domain.Path
	Alias         string
	Force         bool
	DeleteBranch  bool
}

// RemoveRepo applies the same safety classification as DestroyWorkspace,
// scoped to a single repo (workspace-lifecycle spec: "Destructive teardown
// safety").
func RemoveRepo(ctx context.Context, deps Deps, in RemoveRepoInput) error {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return err
	}

	idx, repo, found := findRepo(manifest.Workspace.Repos, in.Alias)
	if !found {
		return domain.NewOpError("workspace.remove_repo", domain.CodeWorkspaceNotFound, in.Alias, "", nil)
	}
	worktree := in.WorkspaceRoot.Join(repo.Alias)

	if !in.Force {
		changes, err := repoChangeSet(ctx, deps, repo.Alias, worktree, manifest.EnvCopies, "HEAD")
		if err != nil {
			return err
		}
		if !changes.SafeToRemove() {
			reportBlocking(deps.Reporter, changes)
			return domain.NewOpError("workspace.remove_repo", domain.CodeUnsafeTeardown, repo.Alias, "", nil)
		}
	}

	// Always force the git-level removal here, never the caller's raw
	// in.Force — see destroy_workspace.go's identical WorktreeRemove call
	// for why: by this point removal is already fully vetted, and real
	// git's own dirty-worktree refusal has no concept of "this untracked
	// file is just an env copy" this tool itself put there.
	reportRepo(deps.Reporter, ports.RepoEvent{Op: OpRemoveRepo, Repo: repo.Alias, Phase: ports.RepoStarted})
	if err := teardownRepo(ctx, deps, repo, worktree, in.DeleteBranch, in.Force); err != nil {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpRemoveRepo, Repo: repo.Alias, Phase: ports.RepoFailed, Err: err})
		return err
	}
	if err := deps.FS.RemoveAll(worktree); err != nil {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpRemoveRepo, Repo: repo.Alias, Phase: ports.RepoFailed, Err: err})
		return err
	}
	reportRepo(deps.Reporter, ports.RepoEvent{Op: OpRemoveRepo, Repo: repo.Alias, Phase: ports.RepoFinished})

	manifest.Workspace.Repos = append(manifest.Workspace.Repos[:idx], manifest.Workspace.Repos[idx+1:]...)
	manifest.EnvCopies = removeEnvCopiesForAlias(manifest.EnvCopies, repo.Alias)
	if err := deps.Store.SaveManifest(ctx, in.WorkspaceRoot, manifest); err != nil {
		return err
	}

	deps.Reporter.Result(struct{ Alias string }{Alias: repo.Alias})
	return nil
}

func findRepo(repos []domain.RepoEntry, alias string) (int, domain.RepoEntry, bool) {
	for i, r := range repos {
		if r.Alias == alias {
			return i, r, true
		}
	}
	return 0, domain.RepoEntry{}, false
}

func removeEnvCopiesForAlias(envCopies []string, alias string) []string {
	prefix := alias + "/"
	out := envCopies[:0]
	for _, e := range envCopies {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			continue
		}
		out = append(out, e)
	}
	return out
}

// AddableProjectsInput names a workspace and its owning context.
type AddableProjectsInput struct {
	WorkspaceRoot domain.Path
	Context       domain.Context
}

// AddableProjects lists the context's projects AddRepo would accept for
// the workspace: every project not already in it (see inWorkspace), in the
// context's order.
func AddableProjects(ctx context.Context, deps Deps, in AddableProjectsInput) ([]domain.Project, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	out := []domain.Project{}
	for _, p := range in.Context.Projects {
		if !inWorkspace(manifest.Workspace.Repos, p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// inWorkspace reports whether project is already one of repos: by project
// key, by main clone (source dir; an adopted legacy workspace may record
// another key for the same clone) or by alias (the worktree directory the
// project would get is taken).
func inWorkspace(repos []domain.RepoEntry, project domain.Project) bool {
	src := path.Clean(string(project.SourceDir))
	for _, r := range repos {
		if r.Project == project.Key || r.Alias == string(project.Key) {
			return true
		}
		if r.SourceDir != "" && project.SourceDir != "" && path.Clean(string(r.SourceDir)) == src {
			return true
		}
	}
	return false
}
