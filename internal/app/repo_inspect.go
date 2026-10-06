// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"strconv"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// The repository inspector: read-only queries over one workspace repo,
// plus a fetch and a fast-forward-only pull. Nothing here stages,
// commits, stashes, merges with a merge commit or rebases.

const (
	// inspectTimeout bounds every read-only inspector query.
	inspectTimeout = 30 * time.Second
	// networkTimeout bounds a fetch or pull (network included).
	networkTimeout = 2 * time.Minute
	// DefaultCommitsPage and MaxCommitsPage size RepoCommitLog pages.
	DefaultCommitsPage = 50
	MaxCommitsPage     = 200
)

// stashUntrackedMin is the first git release whose `git stash show`
// accepts --include-untracked (Git 2.32 release notes: "git stash show"
// learned to optionally show untracked part of the stash).
var stashUntrackedMin = ports.Version{Major: 2, Minor: 32}

// RepoRefInput names one repo (by alias) of one workspace.
type RepoRefInput struct {
	WorkspaceRoot domain.Path
	Alias         string
}

// UpstreamState is a branch's upstream and how far apart they are.
type UpstreamState struct {
	Ref    string
	Remote string
	Gone   bool
	Ahead  int
	Behind int
}

// BranchInfo is the inspector's branch view of one repo.
type BranchInfo struct {
	Alias    string
	Branch   domain.BranchName
	Detached bool
	Head     string
	// Upstream is nil when the branch has none.
	Upstream *UpstreamState
	// BaseBranch is the comparison base (same precedence as status);
	// BaseRef the ref counted against when BaseFound. BaseAhead and
	// BaseBehind are nil when unknown.
	BaseBranch domain.BranchName
	BaseRef    string
	BaseFound  bool
	BaseAhead  *int
	BaseBehind *int
	// LastFetch is nil when the repo was never fetched.
	LastFetch *time.Time
}

// RepoInspection is the inspector's summary of one repo.
type RepoInspection struct {
	Branch     BranchInfo
	Staged     int
	Unstaged   int
	Untracked  int
	Conflicted int
	Stashes    int
}

// inspectTarget is a resolved workspace repo.
type inspectTarget struct {
	alias    string
	worktree domain.Path
	remote   string
	base     func(context.Context) (repoBase, error)
}

func resolveInspectTarget(ctx context.Context, deps Deps, in RepoRefInput) (inspectTarget, error) {
	manifest, err := deps.Store.LoadManifest(ctx, in.WorkspaceRoot)
	if err != nil {
		return inspectTarget{}, err
	}
	_, repo, found := findRepo(manifest.Workspace.Repos, in.Alias)
	if !found {
		return inspectTarget{}, domain.NewOpError("repo.inspect", domain.CodeRepoNotFound, in.Alias, "", nil)
	}
	ws := manifest.Workspace
	owner := loadOwnerContext(ctx, deps, ws.Context, nil)
	resolver := domain.Resolver{Context: owner, Workspace: &ws}
	t := inspectTarget{alias: repo.Alias, worktree: ws.Root.Join(repo.Alias), remote: resolver.Remote(repo.Project).Value}
	var project *domain.Project
	if owner != nil {
		project = findProject(owner, repo.Project)
	}
	candidates := baseCandidates(project, repo.BaseBranch, ws.Options.BaseBranch)
	t.base = func(ctx context.Context) (repoBase, error) {
		return resolveRepoBase(ctx, deps, t.worktree, t.remote, candidates)
	}
	return t, nil
}

// RepoBranch reports the current branch, its upstream (ahead/behind), the
// comparison base (ahead/behind, against the last fetch) and when the
// repo was last fetched.
func RepoBranch(ctx context.Context, deps Deps, in RepoRefInput) (BranchInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in)
	if err != nil {
		return BranchInfo{}, err
	}
	return branchInfo(ctx, deps, t)
}

func branchInfo(ctx context.Context, deps Deps, t inspectTarget) (BranchInfo, error) {
	b := BranchInfo{Alias: t.alias}
	var err error
	if b.Branch, b.Detached, err = deps.Git.CurrentBranch(ctx, t.worktree); err != nil {
		return BranchInfo{}, err
	}
	// An unborn branch has no HEAD commit; that is not an error here.
	b.Head, _ = deps.Git.HeadCommit(ctx, t.worktree)

	up, ok, err := deps.Git.Upstream(ctx, t.worktree)
	if err != nil {
		return BranchInfo{}, err
	}
	if ok {
		state := &UpstreamState{Ref: up.Ref, Remote: up.Remote, Gone: up.Gone}
		if !up.Gone {
			// Best-effort: an uncountable upstream only leaves 0/0.
			if ahead, behind, abErr := deps.Git.AheadBehind(ctx, t.worktree, "@{upstream}"); abErr == nil {
				state.Ahead, state.Behind = ahead, behind
			}
		}
		b.Upstream = state
	}

	base, err := t.base(ctx)
	if err != nil {
		return BranchInfo{}, err
	}
	b.BaseBranch, b.BaseFound = base.Branch, base.Found
	if base.Found {
		b.BaseRef = base.Ref.Ref
		if n, cErr := deps.Git.UnpushedCount(ctx, t.worktree, base.Ref.Ref); cErr == nil {
			b.BaseAhead = &n
		} else if domain.Code(cErr) != domain.CodeRefNotFound {
			return BranchInfo{}, cErr
		}
		if n, cErr := deps.Git.BehindCount(ctx, t.worktree, base.Ref.Ref); cErr == nil {
			b.BaseBehind = &n
		}
	}

	if at, ok, ferr := deps.Git.LastFetch(ctx, t.worktree); ferr == nil && ok {
		at := at
		b.LastFetch = &at
	}
	return b, nil
}

// InspectRepo is the inspector's summary: the branch view plus change and
// stash counts.
func InspectRepo(ctx context.Context, deps Deps, in RepoRefInput) (RepoInspection, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in)
	if err != nil {
		return RepoInspection{}, err
	}
	b, err := branchInfo(ctx, deps, t)
	if err != nil {
		return RepoInspection{}, err
	}
	entries, err := deps.Git.Status(ctx, t.worktree)
	if err != nil {
		return RepoInspection{}, err
	}
	cs := domain.SplitChanges(entries)
	stashes, err := deps.Git.StashList(ctx, t.worktree)
	if err != nil {
		return RepoInspection{}, err
	}
	return RepoInspection{Branch: b, Staged: len(cs.Staged), Unstaged: len(cs.Unstaged), Untracked: len(cs.Untracked),
		Conflicted: len(cs.Conflicted), Stashes: len(stashes)}, nil
}

// RepoChangeSets lists the repo's changes split into staged, unstaged,
// untracked and conflicted.
func RepoChangeSets(ctx context.Context, deps Deps, in RepoRefInput) (domain.ChangeSets, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in)
	if err != nil {
		return domain.ChangeSets{}, err
	}
	entries, err := deps.Git.Status(ctx, t.worktree)
	if err != nil {
		return domain.ChangeSets{}, err
	}
	return domain.SplitChanges(entries), nil
}

// RepoDiffInput asks for one changed path's diff, staged or not.
type RepoDiffInput struct {
	RepoRefInput
	Path   string
	Staged bool
}

// RepoDiff returns the diff of one path the repo's status lists on the
// requested side (an untracked file is diffed as all-added). Any other
// path is CodePathNotChanged, so a caller can never read a file that is
// not a change of this worktree.
func RepoDiff(ctx context.Context, deps Deps, in RepoDiffInput) (domain.FileDiff, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return domain.FileDiff{}, err
	}
	entries, err := deps.Git.Status(ctx, t.worktree)
	if err != nil {
		return domain.FileDiff{}, err
	}
	side := domain.SideUnstaged
	if in.Staged {
		side = domain.SideStaged
	}
	entry, ok := domain.SplitChanges(entries).Find(in.Path, side)
	if !ok {
		return domain.FileDiff{}, domain.NewOpError("repo.diff", domain.CodePathNotChanged, in.Path, "", nil)
	}
	spec := ports.DiffSpec{Path: entry.Path, OrigPath: entry.OrigPath, Staged: in.Staged, Untracked: entry.Status == domain.FileUntracked, Limits: domain.DefaultDiffLimits}
	patch, err := deps.Git.Diff(ctx, t.worktree, spec)
	if err != nil {
		return domain.FileDiff{}, err
	}
	if len(patch.Files) == 0 {
		return domain.FileDiff{Path: entry.Path, OrigPath: entry.OrigPath, Status: entry.Status, Hunks: []domain.DiffHunk{}, Truncated: patch.Truncated}, nil
	}
	d := patch.Files[0]
	d.Truncated = d.Truncated || patch.Truncated
	if entry.Status == domain.FileConflicted {
		d.Status = domain.FileConflicted
	}
	return d, nil
}

// CommitRange selects what RepoCommitLog lists.
type CommitRange string

const (
	// CommitRangeBase lists the commits since the comparison base
	// (<base>..HEAD); the default.
	CommitRangeBase CommitRange = "base"
	// CommitRangeUpstream lists the commits the upstream does not have
	// yet (<upstream>..HEAD): what a push would send. A branch without a
	// live upstream fails with CodeNoUpstream.
	CommitRangeUpstream CommitRange = "upstream"
)

// RepoCommitsInput pages through RepoCommitLog. An empty Range is
// CommitRangeBase.
type RepoCommitsInput struct {
	RepoRefInput
	Range  CommitRange
	Offset int
	Limit  int
}

// RepoCommits is one page of the branch's own commits.
type RepoCommits struct {
	// Range is the revision range listed: "<base>..HEAD", or "HEAD" when
	// no base could be resolved (Base is "" then); "<upstream>..HEAD" for
	// CommitRangeUpstream (Upstream set, Base "").
	Range    string
	Base     string
	Upstream string
	Total    int
	Offset   int
	Commits  []domain.CommitInfo
	HasMore  bool
}

// RepoCommitLog lists the commits on the branch since it left its
// comparison base (newest first), Limit (default DefaultCommitsPage, at
// most MaxCommitsPage) at a time from Offset.
func RepoCommitLog(ctx context.Context, deps Deps, in RepoCommitsInput) (RepoCommits, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return RepoCommits{}, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = DefaultCommitsPage
	}
	if limit > MaxCommitsPage {
		limit = MaxCommitsPage
	}
	offset := max(in.Offset, 0)

	out := RepoCommits{Range: "HEAD", Offset: offset}
	if in.Range == CommitRangeUpstream {
		up, ok, err := deps.Git.Upstream(ctx, t.worktree)
		if err != nil {
			return RepoCommits{}, err
		}
		if !ok || up.Gone || up.Ref == "" {
			return RepoCommits{}, domain.NewOpError("repo.commits", domain.CodeNoUpstream, t.alias, "", nil)
		}
		out.Upstream = up.Ref
		out.Range = up.Ref + "..HEAD"
	} else {
		base, err := t.base(ctx)
		if err != nil {
			return RepoCommits{}, err
		}
		if base.Found {
			out.Base = base.Ref.Ref
			out.Range = base.Ref.Ref + "..HEAD"
		}
	}
	if out.Total, err = deps.Git.CountCommits(ctx, t.worktree, out.Range); err != nil {
		return RepoCommits{}, err
	}
	list, err := deps.Git.CommitLog(ctx, t.worktree, out.Range, offset, limit+1)
	if err != nil {
		return RepoCommits{}, err
	}
	if len(list) > limit {
		out.HasMore = true
		list = list[:limit]
	}
	if list == nil {
		list = []domain.CommitInfo{}
	}
	out.Commits = list
	return out, nil
}

// RepoCommitInput names one commit by hash.
type RepoCommitInput struct {
	RepoRefInput
	Hash string
}

// RepoCommit returns one commit with its changed files and diff.
func RepoCommit(ctx context.Context, deps Deps, in RepoCommitInput) (domain.CommitDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return domain.CommitDetail{}, err
	}
	if !domain.ValidCommitHash(in.Hash) {
		return domain.CommitDetail{}, domain.NewOpError("repo.commit", domain.CodeRefNotFound, in.Hash, "", nil)
	}
	return deps.Git.CommitDetail(ctx, t.worktree, in.Hash, domain.DefaultDiffLimits)
}

// RepoStashes lists the repo's stash entries, newest first.
func RepoStashes(ctx context.Context, deps Deps, in RepoRefInput) ([]domain.StashEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in)
	if err != nil {
		return nil, err
	}
	return deps.Git.StashList(ctx, t.worktree)
}

// RepoStashInput names one stash entry by index (stash@{index}).
type RepoStashInput struct {
	RepoRefInput
	Index int
}

// StashDetail is one stash entry with its content.
type StashDetail struct {
	Entry domain.StashEntry
	Patch domain.Patch
	// IncludesUntracked is false when git is older than 2.32, which
	// cannot show a stash's untracked files.
	IncludesUntracked bool
}

// RepoStash returns one stash entry's diff (untracked files included
// when git supports it). An index not in the list is CodeRefNotFound.
func RepoStash(ctx context.Context, deps Deps, in RepoStashInput) (StashDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return StashDetail{}, err
	}
	list, err := deps.Git.StashList(ctx, t.worktree)
	if err != nil {
		return StashDetail{}, err
	}
	var out StashDetail
	found := false
	for _, s := range list {
		if s.Index == in.Index {
			out.Entry, found = s, true
		}
	}
	if !found {
		return StashDetail{}, domain.NewOpError("repo.stash", domain.CodeRefNotFound, "stash@{"+strconv.Itoa(in.Index)+"}", "", nil)
	}
	v, err := deps.Git.Version(ctx)
	if err != nil {
		return StashDetail{}, err
	}
	out.IncludesUntracked = versionAtLeast(v, stashUntrackedMin)
	if out.Patch, err = deps.Git.StashShow(ctx, t.worktree, in.Index, out.IncludesUntracked, domain.DefaultDiffLimits); err != nil {
		return StashDetail{}, err
	}
	return out, nil
}

// RepoFetch is FetchRepo's result: the remote fetched and the branch view
// after the fetch.
type RepoFetch struct {
	Alias  string
	Remote string
	Branch BranchInfo
}

// FetchRepo runs `git fetch --prune` for the repo's configured remote
// (and its upstream's remote when that differs), then reports the branch
// view. It never changes the worktree or any local branch.
func FetchRepo(ctx context.Context, deps Deps, in RepoRefInput) (RepoFetch, error) {
	ctx, cancel := context.WithTimeout(ctx, networkTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in)
	if err != nil {
		return RepoFetch{}, err
	}
	reportRepo(deps.Reporter, ports.RepoEvent{Op: OpFetch, Repo: t.alias, Phase: ports.RepoStarted})
	res, err := fetchRemotes(ctx, deps, t)
	if err != nil {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpFetch, Repo: t.alias, Phase: ports.RepoFailed, Err: err})
		return RepoFetch{}, err
	}
	reportRepo(deps.Reporter, ports.RepoEvent{Op: OpFetch, Repo: t.alias, Phase: ports.RepoFinished})
	return res, nil
}

func fetchRemotes(ctx context.Context, deps Deps, t inspectTarget) (RepoFetch, error) {
	if err := deps.Git.Fetch(ctx, t.worktree, t.remote); err != nil {
		return RepoFetch{}, err
	}
	if up, ok, err := deps.Git.Upstream(ctx, t.worktree); err == nil && ok && up.Remote != "" && up.Remote != "." && up.Remote != t.remote {
		if err := deps.Git.Fetch(ctx, t.worktree, up.Remote); err != nil {
			return RepoFetch{}, err
		}
	}
	b, err := branchInfo(ctx, deps, t)
	if err != nil {
		return RepoFetch{}, err
	}
	return RepoFetch{Alias: t.alias, Remote: t.remote, Branch: b}, nil
}

// RepoPull is one fast-forward pull's outcome. Err is nil on success
// (UpToDate or CommitsPulled > 0) and a *domain.OpError otherwise:
// CodeDetachedHead, CodeIntegrationInProgress, CodeNoUpstream,
// CodeDiverged (Ahead/Behind set), CodeWorktreeDirty (DirtyFiles set) or
// a git failure. A refusal never changes the repo.
type RepoPull struct {
	Alias         string
	Upstream      string
	BeforeHead    string
	AfterHead     string
	UpToDate      bool
	CommitsPulled int
	Ahead         int
	Behind        int
	DirtyFiles    []string
	Err           error
}

// PullRepo fetches the branch's upstream and fast-forwards the branch to
// it. It never creates a merge commit and never rebases: a branch with
// local commits the upstream lacks is refused as CodeDiverged (use the
// update from base, or integrate by hand). Only an unknown workspace or
// alias is returned as an error.
func PullRepo(ctx context.Context, deps Deps, in RepoRefInput) (RepoPull, error) {
	ctx, cancel := context.WithTimeout(ctx, networkTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in)
	if err != nil {
		return RepoPull{}, err
	}
	reportRepo(deps.Reporter, ports.RepoEvent{Op: OpPull, Repo: t.alias, Phase: ports.RepoStarted})
	res := pullOne(ctx, deps, t)
	if res.Err != nil {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpPull, Repo: t.alias, Phase: ports.RepoFailed, Err: res.Err})
	} else {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpPull, Repo: t.alias, Phase: ports.RepoFinished})
	}
	return res, nil
}

func pullErr(code domain.ErrCode, subject string) error {
	return domain.NewOpError("repo.pull", code, subject, "", nil)
}

func pullOne(ctx context.Context, deps Deps, t inspectTarget) RepoPull {
	res := RepoPull{Alias: t.alias}
	fail := func(err error) RepoPull {
		res.Err = err
		if res.AfterHead == "" {
			res.AfterHead = res.BeforeHead
		}
		return res
	}
	_, detached, err := deps.Git.CurrentBranch(ctx, t.worktree)
	if err != nil {
		return fail(err)
	}
	if detached {
		return fail(pullErr(domain.CodeDetachedHead, t.alias))
	}
	if op, err := deps.Git.IntegrationInProgress(ctx, t.worktree); err != nil {
		return fail(err)
	} else if op != "" {
		return fail(pullErr(domain.CodeIntegrationInProgress, t.alias))
	}
	up, ok, err := deps.Git.Upstream(ctx, t.worktree)
	if err != nil {
		return fail(err)
	}
	if !ok || up.Gone {
		res.Upstream = up.Ref
		return fail(pullErr(domain.CodeNoUpstream, t.alias))
	}
	res.Upstream = up.Ref
	if up.Remote != "" && up.Remote != "." {
		if err := deps.Git.Fetch(ctx, t.worktree, up.Remote); err != nil {
			return fail(err)
		}
	}
	if res.BeforeHead, err = deps.Git.HeadCommit(ctx, t.worktree); err != nil {
		return fail(err)
	}
	if res.Ahead, res.Behind, err = deps.Git.AheadBehind(ctx, t.worktree, "@{upstream}"); err != nil {
		return fail(err)
	}
	if res.Behind == 0 {
		res.UpToDate = true
		res.AfterHead = res.BeforeHead
		return res
	}
	if res.Ahead > 0 {
		return fail(pullErr(domain.CodeDiverged, up.Ref))
	}
	files, err := deps.Git.MergeFastForward(ctx, t.worktree, up.Ref)
	if err != nil {
		res.DirtyFiles = files
		res.AfterHead, _ = deps.Git.HeadCommit(ctx, t.worktree)
		return fail(err)
	}
	if res.AfterHead, err = deps.Git.HeadCommit(ctx, t.worktree); err != nil {
		return fail(err)
	}
	res.CommitsPulled = res.Behind
	return res
}
