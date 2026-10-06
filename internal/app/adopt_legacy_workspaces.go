// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"sort"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// AdoptLegacyWorkspacesInput parameterizes AdoptLegacyWorkspaces.
type AdoptLegacyWorkspacesInput struct {
	// Context is the context adopted workspaces are recorded under; its
	// WorkspacesRoot is scanned and its Projects resolve each legacy repo.
	Context domain.Context
}

// AdoptedLegacyWorkspace is one legacy workspace that now has a wspace
// manifest.
type AdoptedLegacyWorkspace struct {
	Name  string
	Root  domain.Path
	Repos int
}

// SkippedLegacyWorkspace is one legacy workspace that was left alone, with
// why. Reason is a catalog key whose template takes Detail as its only
// argument.
type SkippedLegacyWorkspace struct {
	Root   domain.Path
	Reason messages.Key
	Detail string
}

// Message renders the skip reason through the catalog.
func (s SkippedLegacyWorkspace) Message() string { return messages.T(s.Reason, s.Detail) }

// AdoptLegacyWorkspacesResult reports every legacy workspace found, adopted
// or skipped. Directories that are not legacy workspaces appear in neither
// list. A legacy workspace that already has a wspace manifest is skipped
// with a reason naming its owner: already adopted by this context, owned by
// another existing context, or orphaned and claimed for this context.
type AdoptLegacyWorkspacesResult struct {
	Adopted []AdoptedLegacyWorkspace
	Skipped []SkippedLegacyWorkspace
}

// AdoptLegacyWorkspaces gives every workspace the legacy bash tool created
// under the context's WorkspacesRoot a wspace manifest, so wspace can list,
// inspect and tear it down. It is non-destructive and idempotent:
//
//   - only directories holding ".ws/workspace.conf" are considered; one
//     that already has ".wspace/workspace.yaml" is only reported (or, when
//     its owner context no longer exists, claimed); the legacy files are only
//     ever read, never changed or removed (the legacy tool may still manage
//     the same workspace);
//   - a workspace whose data cannot be fully resolved is skipped and
//     reported, never adopted with guessed values; one bad workspace never
//     stops the others.
//
// Only a failure to list WorkspacesRoot itself is returned as an error; a
// missing WorkspacesRoot simply has nothing to adopt.
func AdoptLegacyWorkspaces(ctx context.Context, deps Deps, in AdoptLegacyWorkspacesInput) (AdoptLegacyWorkspacesResult, error) {
	var res AdoptLegacyWorkspacesResult
	names, err := deps.FS.ListDirs(in.Context.WorkspacesRoot)
	if err != nil {
		return res, err
	}
	// The registry tells an orphaned manifest (its owner no longer exists)
	// from one another context owns. Unreadable, nothing is ever claimed.
	existing, existingErr := existingContexts(ctx, deps.Store)
	for _, name := range names {
		if strings.HasPrefix(name, ".") {
			continue
		}
		root := in.Context.WorkspacesRoot.Join(name)
		if !isLegacyWorkspace(deps, root) {
			continue
		}
		if m, err := deps.Store.LoadManifest(ctx, root); err == nil {
			if skipped := ownedLegacySkip(ctx, deps, in.Context, existing, existingErr, m, root, name); skipped != nil {
				res.Skipped = append(res.Skipped, *skipped)
			}
			continue
		}
		adopted, skipped, done := adoptOne(ctx, deps, in.Context, name, root)
		switch {
		case skipped != nil:
			res.Skipped = append(res.Skipped, *skipped)
		case done:
			res.Adopted = append(res.Adopted, adopted)
		}
	}
	return res, nil
}

// isLegacyWorkspace reports whether root carries the legacy tool's marker.
func isLegacyWorkspace(deps Deps, root domain.Path) bool {
	ok, err := deps.FS.Exists(root.Join(domain.LegacyWorkspaceConfRel))
	return err == nil && ok
}

// ownedLegacySkip reports a legacy folder that already has a wspace
// manifest: already adopted by this context (or shared by every context),
// owned by another existing context (left alone), or orphaned — its owner
// no longer exists — in which case it is claimed for c, exactly like
// ClaimWorkspaces would.
func ownedLegacySkip(ctx context.Context, deps Deps, c domain.Context, existing map[domain.ContextName]bool, existingErr error, m domain.Manifest, root domain.Path, dir string) *SkippedLegacyWorkspace {
	skip := func(reason messages.Key, detail string) *SkippedLegacyWorkspace {
		return &SkippedLegacyWorkspace{Root: root, Reason: reason, Detail: detail}
	}
	owner := m.Workspace.Context
	if existingErr != nil && owner != "" && owner != c.Name {
		return skip(messages.AdoptSkipOwnedByOther, string(owner))
	}
	switch classifyOwner(owner, c.Name, existing) {
	case ownedBySelf, ownedShared:
		return skip(messages.AdoptSkipAlreadyAdopted, string(c.Name))
	case ownedByOther:
		return skip(messages.AdoptSkipOwnedByOther, string(owner))
	}
	m.Workspace.Root = root
	var claim ClaimWorkspacesResult
	claimOne(ctx, deps, &claim, c.Name, m, dir)
	if len(claim.Claimed) == 0 {
		return skip(messages.AdoptSkipWriteFailed, claim.Skipped[0].Detail)
	}
	return skip(messages.AdoptSkipOrphanClaimed, string(owner))
}

// adoptOne adopts one legacy workspace. done is false (with skipped nil)
// when there is nothing to do: the workspace already has a manifest.
func adoptOne(ctx context.Context, deps Deps, c domain.Context, name string, root domain.Path) (adopted AdoptedLegacyWorkspace, skipped *SkippedLegacyWorkspace, done bool) {
	skip := func(reason messages.Key, detail string) (AdoptedLegacyWorkspace, *SkippedLegacyWorkspace, bool) {
		return AdoptedLegacyWorkspace{}, &SkippedLegacyWorkspace{Root: root, Reason: reason, Detail: detail}, false
	}

	if _, err := deps.Store.LoadManifest(ctx, root); err == nil {
		return AdoptedLegacyWorkspace{}, nil, false
	} else if domain.Code(err) != domain.CodeWorkspaceNotFound {
		return skip(messages.AdoptSkipManifestUnreadable, err.Error())
	}

	// The legacy tool addresses a workspace by its folder name, whatever
	// its manifest's own "name" key says.
	if _, err := domain.NewWorkspaceName(name); err != nil {
		return skip(messages.AdoptSkipInvalidName, name)
	}

	confPath := root.Join(domain.LegacyWorkspaceConfRel)
	data, err := deps.FS.ReadFile(confPath)
	if err != nil {
		return skip(messages.AdoptSkipConfUnreadable, err.Error())
	}
	conf := domain.ParseLegacyWorkspaceConf(data)
	if len(conf.Repos) == 0 {
		return skip(messages.AdoptSkipNoRepos, string(confPath))
	}

	repos := make([]domain.RepoEntry, 0, len(conf.Repos))
	projects := make([]domain.Project, 0, len(conf.Repos))
	for _, lr := range conf.Repos {
		project, ok := resolveLegacyProject(c, conf.ProjectsRoot, lr)
		if !ok {
			return skip(messages.AdoptSkipUnresolvedProject, lr.Project)
		}
		branch := lr.Branch
		if branch == "" {
			branch = conf.Branch
		}
		if branch == "" {
			branch = currentBranchOf(ctx, deps, root.Join(lr.Alias))
		}
		if branch == "" {
			return skip(messages.AdoptSkipBranchUnknown, lr.Alias)
		}
		repos = append(repos, domain.RepoEntry{Alias: lr.Alias, Project: project.Key, SourceDir: project.SourceDir, Branch: branch})
		projects = append(projects, project)
	}

	wsBranch := conf.Branch
	if wsBranch == "" {
		distinct := distinctBranches(repos)
		if len(distinct) != 1 {
			return skip(messages.AdoptSkipBranchAmbiguous, strings.Join(distinct, ", "))
		}
		wsBranch = domain.BranchName(distinct[0])
	}

	opts := domain.Options{BaseBranch: conf.BaseBranch, CopyEnv: conf.CopyEnv}
	if opts.BaseBranch == nil {
		// The legacy tool falls back to its config's base_branch, which the
		// import carried into this context's defaults.
		opts.BaseBranch = c.Defaults.BaseBranch
	}

	recordRepoBases(ctx, deps, c, opts.BaseBranch, projects, repos)

	manifest := domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{
		Name:    name,
		Root:    root,
		Context: c.Name,
		Branch:  wsBranch,
		Created: conf.Created,
		Options: opts,
		Repos:   repos,
	}}
	if err := deps.Store.SaveManifest(ctx, root, manifest); err != nil {
		return skip(messages.AdoptSkipWriteFailed, err.Error())
	}
	return AdoptedLegacyWorkspace{Name: name, Root: root, Repos: len(repos)}, nil, true
}

// recordRepoBases sets RepoEntry.BaseBranch on each repo whose comparison
// base (baseCandidates' precedence, then the remote default branch) exists
// but differs from the workspace base, e.g. a repo that only has master in
// a workspace based on develop. A repo whose base matches the workspace's,
// or cannot be determined at all, records nothing: status resolves it at
// read time. Git failures here never block adoption.
func recordRepoBases(ctx context.Context, deps Deps, c domain.Context, wsBase *domain.BranchName, projects []domain.Project, repos []domain.RepoEntry) {
	resolver := domain.Resolver{Context: &c}
	for i := range repos {
		p := projects[i]
		b, err := resolveRepoBase(ctx, deps, p.SourceDir, resolver.Remote(p.Key).Value, baseCandidates(&p, "", wsBase))
		if err != nil || !b.Found {
			continue
		}
		if wsBase == nil || b.Branch != *wsBase {
			repos[i].BaseBranch = b.Branch
		}
	}
}

// resolveLegacyProject finds the context project a legacy repo line points
// at. The legacy tool locates a clone as "<projects_root>/<project>", so a
// project whose SourceDir is that path (under the legacy manifest's own
// projects_root or the context's) wins; then a project keyed by the legacy
// project name (how the context import registers them); then, as a last
// resort, a project keyed by the alias whose clone folder carries the
// legacy project name. Anything else is unresolved — never guessed.
func resolveLegacyProject(c domain.Context, confRoot domain.Path, lr domain.LegacyWorkspaceRepo) (domain.Project, bool) {
	var dirs []domain.Path
	for _, r := range []domain.Path{confRoot, c.ProjectsRoot} {
		if r != "" {
			dirs = append(dirs, r.Join(lr.Project))
		}
	}
	for _, p := range c.Projects {
		for _, d := range dirs {
			if p.SourceDir == d {
				return p, true
			}
		}
	}
	for _, p := range c.Projects {
		if string(p.Key) == lr.Project {
			return p, true
		}
	}
	for _, p := range c.Projects {
		if string(p.Key) == lr.Alias && p.SourceDir.Base() == lr.Project {
			return p, true
		}
	}
	return domain.Project{}, false
}

// currentBranchOf returns the branch checked out in worktree, or "" when the
// worktree is missing, detached, or git cannot tell.
func currentBranchOf(ctx context.Context, deps Deps, worktree domain.Path) domain.BranchName {
	if ok, err := deps.FS.IsDir(worktree); err != nil || !ok {
		return ""
	}
	b, detached, err := deps.Git.CurrentBranch(ctx, worktree)
	if err != nil || detached {
		return ""
	}
	return b
}

func distinctBranches(repos []domain.RepoEntry) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range repos {
		if !seen[string(r.Branch)] {
			seen[string(r.Branch)] = true
			out = append(out, string(r.Branch))
		}
	}
	sort.Strings(out)
	return out
}

// RenderAdoptSummary renders result for the CLI and the import summary.
func RenderAdoptSummary(result AdoptLegacyWorkspacesResult) []string {
	if len(result.Adopted) == 0 && len(result.Skipped) == 0 {
		return []string{messages.T(messages.AdoptSummaryNone)}
	}
	var lines []string
	if len(result.Adopted) > 0 {
		lines = append(lines, messages.T(messages.AdoptSummaryAdoptedHeader))
		for _, a := range result.Adopted {
			lines = append(lines, messages.T(messages.AdoptSummaryAdoptedRow, a.Name, string(a.Root)))
		}
	}
	if len(result.Skipped) > 0 {
		lines = append(lines, messages.T(messages.AdoptSummarySkippedHeader))
		for _, s := range result.Skipped {
			lines = append(lines, messages.T(messages.AdoptSummarySkippedRow, string(s.Root), s.Message()))
		}
	}
	return lines
}
