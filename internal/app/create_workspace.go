// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// CreateWorkspaceInput parameterizes CreateWorkspace. ProjectKeys is the
// caller's selection (e.g. the wizard/CLI's chosen subset); an empty slice
// selects every project in Context.
type CreateWorkspaceInput struct {
	Context     domain.Context
	Overlay     *domain.Overlay
	Flags       domain.Options
	Name        string
	Branch      string
	ProjectKeys []domain.ProjectKey
	// CopyNodeModules copies each Node project's node_modules from its
	// main clone into the new worktree (see copyNodeModules).
	CopyNodeModules bool
}

// CreateWorkspaceResult is the payload handed to Reporter.Result and
// returned to the caller.
type CreateWorkspaceResult struct {
	Workspace domain.Workspace
}

// createPlanItem is one project's fully pre-flighted plan: everything
// needed to mutate is already resolved and validated before any mutation
// runs (workspace-lifecycle spec: "Worktree failure aborts manifest write";
// design.md §8.1 sequence diagram, PRE-FLIGHT before MUTATE).
type createPlanItem struct {
	project domain.Project
	remote  string
	baseRef ports.BaseRef
	// recordedBase is the repo's own base for the manifest: set only when
	// it differs from the workspace-wide base (domain.RepoEntry.BaseBranch).
	recordedBase domain.BranchName
	destBranch   domain.BranchName
	target       domain.Path
	copyEnv      bool
	envPruneDirs []string
	fetch        bool
	nodeModules  bool
}

// CreateWorkspace validates every selected project before touching any of
// them, then creates one worktree per project, copies env files, and
// writes the manifest only once every worktree succeeded
// (workspace-lifecycle spec: "Workspace creation").
//
// Rollback semantics: a MUTATE-phase failure (after pre-flight already
// passed for every project) undoes exactly what this call created, in
// reverse order — each worktree it added is removed, each branch it
// created (one that did not exist before this call) is deleted, and the
// workspace directory itself, which pre-flight proved did not exist, is
// removed. Nothing that existed before the call is touched: a branch that
// already existed is kept, and nothing outside the new workspace
// directory is removed. The manifest is never written in this case
// (workspace-lifecycle spec: "Worktree failure aborts manifest write").
// A rollback step that itself fails is reported through Reporter.Warn and
// the remaining steps still run; the workspace directory is then left in
// place for the user to inspect. The original mutate error is always the
// one returned.
func CreateWorkspace(ctx context.Context, deps Deps, in CreateWorkspaceInput) (CreateWorkspaceResult, error) {
	wsRoot := in.Context.WorkspacesRoot.Join(in.Name)

	projects, err := selectProjects(in.Context, in.ProjectKeys)
	if err != nil {
		return CreateWorkspaceResult{}, err
	}

	exists, err := deps.FS.Exists(wsRoot)
	if err != nil {
		return CreateWorkspaceResult{}, err
	}
	if exists {
		return CreateWorkspaceResult{}, domain.NewOpError("workspace.create", domain.CodeWorkspaceExists, in.Name, "", nil)
	}

	resolver := domain.Resolver{Flags: in.Flags, Context: &in.Context, Overlay: in.Overlay}

	// The workspace-level branch is resolved with an empty ProjectKey (the
	// same trick freezeOptions uses): no project ever matches "", so
	// DestBranch always falls through to the explicit --branch flag or, in
	// its absence, BranchPrefix(p)+name (defect: this used to be a raw
	// domain.BranchName(in.Branch) cast that skipped NewBranchName's
	// validation entirely and produced an empty/invalid BranchName whenever
	// --branch was omitted).
	wsBranch, err := resolver.DestBranch("", domain.Vars{Workspace: in.Name, Branch: in.Branch})
	if err != nil {
		return CreateWorkspaceResult{}, err
	}

	plan, err := preflightCreate(ctx, deps, resolver, in, projects, wsRoot)
	if err != nil {
		return CreateWorkspaceResult{}, err
	}

	repos, envCopies, rolledBack, err := mutateCreate(ctx, deps, OpCreateWorkspace, plan, wsRoot)
	if err != nil {
		if rolledBack {
			if rmErr := deps.FS.RemoveAll(wsRoot); rmErr != nil {
				warnOpError(deps.Reporter, messages.RollbackStepFailed, rmErr)
			}
		}
		return CreateWorkspaceResult{}, err
	}

	ws := domain.Workspace{
		Name:    in.Name,
		Root:    wsRoot,
		Context: in.Context.Name,
		Branch:  wsBranch.Value,
		Created: time.Now().UTC(),
		Options: freezeOptions(resolver),
		Repos:   repos,
	}
	manifest := domain.Manifest{SchemaVersion: 1, Workspace: ws, EnvCopies: envCopies}
	if err := deps.Store.SaveManifest(ctx, wsRoot, manifest); err != nil {
		return CreateWorkspaceResult{}, err
	}

	result := CreateWorkspaceResult{Workspace: ws}
	deps.Reporter.Result(result)
	return result, nil
}

// selectProjects filters ctx.Projects by keys; an empty keys selects every
// project.
func selectProjects(ctx domain.Context, keys []domain.ProjectKey) ([]domain.Project, error) {
	if len(keys) == 0 {
		return ctx.Projects, nil
	}
	var out []domain.Project
	for _, k := range keys {
		p := findProject(&ctx, k)
		if p == nil {
			return nil, fmt.Errorf("workspace.create: project %q not found in context %q", k, ctx.Name)
		}
		out = append(out, *p)
	}
	return out, nil
}

// findProject is exported-in-package-only sugar over the same lookup
// domain.Resolver uses internally; kept here since domain does not expose
// its unexported helper of the same name.
func findProject(ctx *domain.Context, key domain.ProjectKey) *domain.Project {
	for i := range ctx.Projects {
		if ctx.Projects[i].Key == key {
			return &ctx.Projects[i]
		}
	}
	return nil
}

// preflightCreate validates every project — main-clone status, resolvable
// base, and target branch freedom — before mutateCreate touches any of
// them (design.md §8.1: "Pre-flight validates every project before
// touching any of them").
func preflightCreate(ctx context.Context, deps Deps, resolver domain.Resolver, in CreateWorkspaceInput, projects []domain.Project, wsRoot domain.Path) ([]createPlanItem, error) {
	plan := make([]createPlanItem, 0, len(projects))
	for _, p := range projects {
		vars := domain.Vars{Workspace: in.Name, Project: string(p.Key), Branch: in.Branch}

		ok, err := deps.Git.IsMainClone(ctx, p.SourceDir)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, domain.NewOpError("workspace.create", domain.CodeNotAMainClone, string(p.SourceDir), "", nil)
		}

		remote := resolver.Remote(p.Key).Value
		baseRef, recordedBase, err := resolveCreateBase(ctx, deps, resolver, p, remote)
		if err != nil {
			return nil, err
		}

		destResolved, err := resolver.DestBranch(p.Key, vars)
		if err != nil {
			return nil, err
		}

		refs, err := deps.Git.WorktreeList(ctx, p.SourceDir)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			if ref.Branch == destResolved.Value {
				return nil, domain.NewOpError("workspace.create", domain.CodeBranchCheckedOut, string(destResolved.Value), string(ref.Path), nil)
			}
		}

		targetResolved, err := resolver.WorktreePath(p.Key, vars, wsRoot)
		if err != nil {
			return nil, err
		}

		plan = append(plan, createPlanItem{
			project:      p,
			remote:       remote,
			baseRef:      baseRef,
			recordedBase: recordedBase,
			destBranch:   destResolved.Value,
			target:       targetResolved.Value,
			copyEnv:      resolver.CopyEnv(p.Key).Value,
			envPruneDirs: resolver.EnvPruneDirs(p.Key).Value,
			fetch:        resolver.FetchBeforeCreate().Value,
			nodeModules:  in.CopyNodeModules,
		})
	}
	return plan, nil
}

// resolveCreateBase picks the start point for one project's new worktree.
// Candidates, in order: an explicit base_branch flag (it overrides
// everything for this workspace); otherwise the project's origin branch,
// then the resolved base_branch chain (project option, overlay, context,
// built-in). The first candidate found as <remote>/<b> or <b> wins; when
// none exists the remote's default branch (<remote>/HEAD) is used, with a
// warning; when that is unknown too, the clone's current HEAD is used, as
// before, again with a warning. It also returns the base to record in the
// manifest: the branch used when it differs from the workspace-wide base.
func resolveCreateBase(ctx context.Context, deps Deps, resolver domain.Resolver, p domain.Project, remote string) (ports.BaseRef, domain.BranchName, error) {
	var candidates []domain.BranchName
	if resolver.Flags.BaseBranch != nil {
		candidates = []domain.BranchName{*resolver.Flags.BaseBranch}
	} else {
		resolved := resolver.BaseBranch(p.Key).Value
		candidates = baseCandidates(&domain.Project{OriginBranch: p.OriginBranch}, "", &resolved)
	}
	b, err := resolveRepoBase(ctx, deps, p.SourceDir, remote, candidates)
	if err != nil {
		return ports.BaseRef{}, "", err
	}
	if !b.Found {
		deps.Reporter.Warn(messages.BaseBranchMissing, string(p.Key), string(b.Branch))
		return ports.BaseRef{Ref: "HEAD"}, "", nil
	}
	if b.Branch != candidates[0] {
		deps.Reporter.Warn(messages.BaseBranchFallback, string(p.Key), string(candidates[0]), string(b.Branch))
	}
	var recorded domain.BranchName
	if b.Branch != resolver.BaseBranch("").Value {
		recorded = b.Branch
	}
	return b.Ref, recorded, nil
}

// createdRepo is one worktree mutateCreate itself created, remembered so a
// later failure in the same call can undo exactly it and nothing else.
type createdRepo struct {
	key           string
	sourceDir     domain.Path
	target        domain.Path
	branch        domain.BranchName
	branchCreated bool
}

// mutateCreate performs the create-time side effects for every
// pre-flighted plan item, in order: fetch, best-effort local-base sync,
// worktree creation, then env-file copy with a per-file gitignore-coverage
// warning (design.md §8.1 MUTATE; environment-files spec: "Gitignore
// coverage warning"). Each item is announced through Reporter.Step and,
// for a progress-aware Reporter, as a started/finished/failed RepoEvent
// tagged with op.
//
// On failure it rolls back every worktree it created (rollbackCreated)
// before returning the original error; rolledBack reports whether every
// rollback step succeeded, so the caller can decide whether removing its
// own enclosing directory is safe.
func mutateCreate(ctx context.Context, deps Deps, op string, plan []createPlanItem, wsRoot domain.Path) (repos []domain.RepoEntry, envCopies []string, rolledBack bool, err error) {
	if err := deps.FS.MkdirAll(wsRoot); err != nil {
		return nil, nil, true, err
	}

	var created []createdRepo
	fail := func(key string, cause error) ([]domain.RepoEntry, []string, bool, error) {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: op, Repo: key, Phase: ports.RepoFailed, Err: cause})
		return nil, nil, rollbackCreated(ctx, deps, op, created), cause
	}

	for _, item := range plan {
		key := string(item.project.Key)
		deps.Reporter.Step(messages.WorkspaceCreateProject, key)
		reportRepo(deps.Reporter, ports.RepoEvent{Op: op, Repo: key, Phase: ports.RepoStarted})

		if item.fetch {
			if err := deps.Git.Fetch(ctx, item.project.SourceDir, item.remote); err != nil {
				if domain.Code(err) != domain.CodeRemoteMissing {
					return fail(key, err)
				}
				// A local-only clone: carry on from its local base.
				deps.Reporter.Warn(messages.FetchSkippedNoRemote, key, item.remote)
			}
		}

		_, _ = deps.Git.SyncLocalBase(ctx, item.project.SourceDir, item.remote, domain.BranchName(baseBranchOf(item)))

		branchExists, err := deps.Git.BranchExists(ctx, item.project.SourceDir, item.destBranch)
		if err != nil {
			return fail(key, err)
		}
		spec := ports.WorktreeSpec{Target: item.target, Branch: item.destBranch}
		if !branchExists {
			spec.StartPoint = item.baseRef.Ref
		}
		if err := deps.Git.WorktreeAdd(ctx, item.project.SourceDir, spec); err != nil {
			return fail(key, err)
		}
		created = append(created, createdRepo{
			key:           key,
			sourceDir:     item.project.SourceDir,
			target:        item.target,
			branch:        item.destBranch,
			branchCreated: !branchExists,
		})

		if item.copyEnv {
			copied, err := copyEnvFiles(ctx, deps, item.project.SourceDir, item.target, item.envPruneDirs, key)
			if err != nil {
				return fail(key, err)
			}
			envCopies = append(envCopies, copied...)
		}

		if item.nodeModules {
			copyNodeModules(ctx, deps, item.project.SourceDir, item.target, key)
		}

		repos = append(repos, domain.RepoEntry{
			Alias:      key,
			Project:    item.project.Key,
			SourceDir:  item.project.SourceDir,
			Branch:     item.destBranch,
			BaseBranch: item.recordedBase,
		})
		reportRepo(deps.Reporter, ports.RepoEvent{Op: op, Repo: key, Phase: ports.RepoFinished})
	}

	return repos, envCopies, false, nil
}

// rollbackCreated undoes every worktree in created, newest first: remove
// the worktree (forced — it holds nothing but what this run just put
// there), prune its registration, delete its branch only when this run
// created it, and remove whatever is left of its directory. A failing step
// is reported through Reporter.Warn and never stops the remaining steps.
// It reports whether every step succeeded.
func rollbackCreated(ctx context.Context, deps Deps, op string, created []createdRepo) bool {
	clean := true
	warn := func(err error) {
		clean = false
		warnOpError(deps.Reporter, messages.RollbackStepFailed, err)
	}
	for i := len(created) - 1; i >= 0; i-- {
		c := created[i]
		if err := deps.Git.WorktreeRemove(ctx, c.sourceDir, c.target, true); err != nil {
			warn(err)
		}
		if err := deps.Git.WorktreePrune(ctx, c.sourceDir); err != nil {
			warn(err)
		}
		if c.branchCreated {
			if err := deps.Git.DeleteBranch(ctx, c.sourceDir, c.branch, true); err != nil {
				warn(err)
			}
		}
		if err := deps.FS.RemoveAll(c.target); err != nil {
			warn(err)
		}
		reportRepo(deps.Reporter, ports.RepoEvent{Op: op, Repo: c.key, Phase: ports.RepoRolledBack})
	}
	return clean
}

// baseBranchOf recovers the plain base branch name SyncLocalBase needs from
// the resolved BaseRef ("origin/main" or "main" -> "main").
func baseBranchOf(item createPlanItem) string {
	ref := item.baseRef.Ref
	if item.baseRef.Remote {
		if idx := lastSlash(ref); idx >= 0 {
			return ref[idx+1:]
		}
	}
	return ref
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// copyEnvFiles discovers and copies every env file from sourceDir into
// target, warning per file whose destination is not gitignore-covered
// (environment-files spec: "Env file copy preserves relative path",
// "Gitignore coverage warning"). Returned paths are workspace-relative
// ("<alias>/<path>"), matching domain.Manifest.EnvCopies's documented
// shape.
func copyEnvFiles(ctx context.Context, deps Deps, sourceDir, target domain.Path, pruneDirs []string, alias string) ([]string, error) {
	files, err := deps.FS.FindEnvFiles(sourceDir, pruneDirs)
	if err != nil {
		return nil, err
	}

	copied := make([]string, 0, len(files))
	for _, rel := range files {
		src := sourceDir.Join(rel)
		dst := target.Join(rel)
		if err := deps.FS.CopyFile(src, dst); err != nil {
			return nil, err
		}
		copied = append(copied, alias+"/"+rel)

		ignored, err := deps.Git.IsIgnored(ctx, target, rel)
		if err != nil {
			warnOpError(deps.Reporter, messages.OpWarning, err)
			continue
		}
		if !ignored {
			deps.Reporter.Warn(messages.EnvCopyNotIgnored, alias, rel)
		}
	}
	return copied, nil
}

// freezeOptions resolves the workspace-wide option values at creation time
// (domain.Workspace.Options doc: "the values frozen at creation time").
// Passing an empty ProjectKey means the per-project layer never matches,
// which is exactly the workspace-level (not per-project) freeze this needs.
func freezeOptions(r domain.Resolver) domain.Options {
	base := r.BaseBranch("").Value
	copyEnv := r.CopyEnv("").Value
	fetch := r.FetchBeforeCreate().Value
	remote := r.Remote("").Value
	prune := r.EnvPruneDirs("").Value
	return domain.Options{
		BaseBranch:        &base,
		CopyEnv:           &copyEnv,
		FetchBeforeCreate: &fetch,
		Remote:            &remote,
		EnvPruneDirs:      prune,
	}
}
