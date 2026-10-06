// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"
	"errors"
	"time"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
)

// The repository inspector (repos.*): read-only queries over one
// workspace repo, plus a fetch and a fast-forward-only pull.

// UpstreamDTO is a branch's upstream and how far apart they are.
type UpstreamDTO struct {
	Ref    string `json:"ref"`
	Remote string `json:"remote"`
	Gone   bool   `json:"gone"`
	Ahead  int    `json:"ahead"`
	Behind int    `json:"behind"`
}

// BranchInfo is repos.inspect's branch view.
type BranchInfo struct {
	Repo       string       `json:"repo"`
	Branch     string       `json:"branch"`
	Detached   bool         `json:"detached"`
	Head       string       `json:"head"`
	Upstream   *UpstreamDTO `json:"upstream,omitempty"`
	BaseBranch string       `json:"baseBranch,omitempty"`
	BaseRef    string       `json:"baseRef,omitempty"`
	BaseFound  bool         `json:"baseFound"`
	BaseAhead  *int         `json:"baseAhead,omitempty"`
	BaseBehind *int         `json:"baseBehind,omitempty"`
	LastFetch  *time.Time   `json:"lastFetch,omitempty"`
}

// RepoInspection is repos.inspect's result.
type RepoInspection struct {
	Branch     BranchInfo `json:"branch"`
	Staged     int        `json:"staged"`
	Unstaged   int        `json:"unstaged"`
	Untracked  int        `json:"untracked"`
	Conflicted int        `json:"conflicted"`
	Stashes    int        `json:"stashes"`
}

// ChangeEntry is one changed path on one side.
type ChangeEntry struct {
	Path     string `json:"path"`
	OrigPath string `json:"origPath,omitempty"`
	Status   string `json:"status"`
}

// RepoChangeSets is repos.changes' result.
type RepoChangeSets struct {
	Staged     []ChangeEntry `json:"staged"`
	Unstaged   []ChangeEntry `json:"unstaged"`
	Untracked  []ChangeEntry `json:"untracked"`
	Conflicted []ChangeEntry `json:"conflicted"`
}

// DiffHunk is one hunk; each line keeps its prefix (' ', '+', '-', '\').
type DiffHunk struct {
	Header   string   `json:"header"`
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
}

// FileDiff is one file's diff.
type FileDiff struct {
	Path      string     `json:"path"`
	OrigPath  string     `json:"origPath,omitempty"`
	Status    string     `json:"status"`
	Binary    bool       `json:"binary"`
	Truncated bool       `json:"truncated"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Hunks     []DiffHunk `json:"hunks"`
}

// Commit is one commit in a list. Only the author's name is exposed,
// never an e-mail address.
type Commit struct {
	Hash      string    `json:"hash"`
	ShortHash string    `json:"shortHash"`
	Subject   string    `json:"subject"`
	Author    string    `json:"author"`
	Date      time.Time `json:"date"`
}

// RepoCommitsResult is repos.commits' result.
type RepoCommitsResult struct {
	Range string `json:"range"`
	Base  string `json:"base,omitempty"`
	// Upstream is set for range "upstream" (additive, v1).
	Upstream string   `json:"upstream,omitempty"`
	Total    int      `json:"total"`
	Offset   int      `json:"offset"`
	HasMore  bool     `json:"hasMore"`
	Commits  []Commit `json:"commits"`
}

// CommitDetail is repos.commit's result.
type CommitDetail struct {
	Commit
	Body      string     `json:"body"`
	Parents   []string   `json:"parents"`
	Files     []FileDiff `json:"files"`
	Truncated bool       `json:"truncated"`
}

// Stash is one stash entry.
type Stash struct {
	Index   int       `json:"index"`
	Ref     string    `json:"ref"`
	Hash    string    `json:"hash"`
	Branch  string    `json:"branch,omitempty"`
	Message string    `json:"message"`
	Date    time.Time `json:"date"`
}

// StashDetail is repos.stash's result.
type StashDetail struct {
	Stash             Stash      `json:"stash"`
	Files             []FileDiff `json:"files"`
	Truncated         bool       `json:"truncated"`
	IncludesUntracked bool       `json:"includesUntracked"`
}

// RepoFetchResult is repos.fetch's result.
type RepoFetchResult struct {
	Repo   string     `json:"repo"`
	Remote string     `json:"remote"`
	Branch BranchInfo `json:"branch"`
}

// RepoPullResult is repos.pull's result. Error is absent on success; a
// refusal carries data.files (worktree_dirty) or data.ahead/behind
// (diverged).
type RepoPullResult struct {
	Repo          string     `json:"repo"`
	Upstream      string     `json:"upstream"`
	BeforeHead    string     `json:"beforeHead"`
	AfterHead     string     `json:"afterHead"`
	UpToDate      bool       `json:"upToDate"`
	CommitsPulled int        `json:"commitsPulled"`
	Ahead         int        `json:"ahead"`
	Behind        int        `json:"behind"`
	Error         *ErrorBody `json:"error,omitempty"`
}

// RepoDiffParams are repos.diff's parameters.
type RepoDiffParams struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	Staged    bool   `json:"staged,omitempty"`
}

// RepoCommitsParams are repos.commits' parameters.
type RepoCommitsParams struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	// Range is "base" (default: <base>..HEAD) or "upstream"
	// (<upstream>..HEAD, the commits a push would send; conflict
	// no_upstream without a live upstream). Additive, v1.
	Range  string `json:"range,omitempty"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// RepoCommitParams are repos.commit's parameters.
type RepoCommitParams struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	Hash      string `json:"hash"`
}

// RepoStashParams are repos.stash's parameters.
type RepoStashParams struct {
	Context   string `json:"context,omitempty"`
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	Index     int    `json:"index"`
}

func (e *Engine) repoInput(ctx context.Context, contextName, workspace, repo string) (app.RepoRefInput, error) {
	root, _, err := e.workspaceRoot(ctx, contextName, workspace)
	if err != nil {
		return app.RepoRefInput{}, err
	}
	return app.RepoRefInput{WorkspaceRoot: root, Alias: repo}, nil
}

// RepoInspect is the inspector's summary: branch view, change and stash
// counts.
func (e *Engine) RepoInspect(ctx context.Context, p RepoRef) (RepoInspection, error) {
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return RepoInspection{}, err
	}
	r, err := app.InspectRepo(ctx, e.appDeps(nil), in)
	if err != nil {
		return RepoInspection{}, wrap(err)
	}
	return RepoInspection{Branch: branchToDTO(r.Branch), Staged: r.Staged, Unstaged: r.Unstaged, Untracked: r.Untracked, Conflicted: r.Conflicted, Stashes: r.Stashes}, nil
}

// RepoBranch is the branch view alone.
func (e *Engine) RepoBranch(ctx context.Context, p RepoRef) (BranchInfo, error) {
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return BranchInfo{}, err
	}
	b, err := app.RepoBranch(ctx, e.appDeps(nil), in)
	if err != nil {
		return BranchInfo{}, wrap(err)
	}
	return branchToDTO(b), nil
}

// RepoChangeSets lists the repo's staged, unstaged, untracked and
// conflicted paths.
func (e *Engine) RepoChangeSets(ctx context.Context, p RepoRef) (RepoChangeSets, error) {
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return RepoChangeSets{}, err
	}
	cs, err := app.RepoChangeSets(ctx, e.appDeps(nil), in)
	if err != nil {
		return RepoChangeSets{}, wrap(err)
	}
	return RepoChangeSets{Staged: entriesToDTO(cs.Staged), Unstaged: entriesToDTO(cs.Unstaged), Untracked: entriesToDTO(cs.Untracked), Conflicted: entriesToDTO(cs.Conflicted)}, nil
}

// RepoDiff returns one changed path's diff (staged or not).
func (e *Engine) RepoDiff(ctx context.Context, p RepoDiffParams) (FileDiff, error) {
	if p.Path == "" {
		return FileDiff{}, invalidParam("path", errors.New("required"))
	}
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return FileDiff{}, err
	}
	d, err := app.RepoDiff(ctx, e.appDeps(nil), app.RepoDiffInput{RepoRefInput: in, Path: p.Path, Staged: p.Staged})
	if err != nil {
		return FileDiff{}, wrap(err)
	}
	return fileDiffToDTO(d), nil
}

// RepoCommits pages through the branch's commits since its base.
func (e *Engine) RepoCommits(ctx context.Context, p RepoCommitsParams) (RepoCommitsResult, error) {
	if p.Offset < 0 {
		return RepoCommitsResult{}, invalidParam("offset", errors.New("must not be negative"))
	}
	if p.Limit < 0 || p.Limit > app.MaxCommitsPage {
		return RepoCommitsResult{}, invalidParam("limit", errors.New("must be between 0 and 200"))
	}
	commitRange := app.CommitRange(p.Range)
	switch commitRange {
	case "":
		commitRange = app.CommitRangeBase
	case app.CommitRangeBase, app.CommitRangeUpstream:
	default:
		return RepoCommitsResult{}, invalidParam("range", errors.New(`must be "base" or "upstream"`))
	}
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return RepoCommitsResult{}, err
	}
	r, err := app.RepoCommitLog(ctx, e.appDeps(nil), app.RepoCommitsInput{RepoRefInput: in, Range: commitRange, Offset: p.Offset, Limit: p.Limit})
	if err != nil {
		return RepoCommitsResult{}, wrap(err)
	}
	out := RepoCommitsResult{Range: r.Range, Base: r.Base, Upstream: r.Upstream, Total: r.Total, Offset: r.Offset, HasMore: r.HasMore, Commits: make([]Commit, 0, len(r.Commits))}
	for _, c := range r.Commits {
		out.Commits = append(out.Commits, commitToDTO(c))
	}
	return out, nil
}

// RepoCommit returns one commit with its files and diff.
func (e *Engine) RepoCommit(ctx context.Context, p RepoCommitParams) (CommitDetail, error) {
	if !domain.ValidCommitHash(p.Hash) {
		return CommitDetail{}, invalidParam("hash", errors.New("must be a hexadecimal commit hash (4 to 64 digits)"))
	}
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return CommitDetail{}, err
	}
	d, err := app.RepoCommit(ctx, e.appDeps(nil), app.RepoCommitInput{RepoRefInput: in, Hash: p.Hash})
	if err != nil {
		return CommitDetail{}, wrap(err)
	}
	parents := append([]string{}, d.Parents...)
	return CommitDetail{Commit: commitToDTO(d.CommitInfo), Body: d.Body, Parents: parents, Files: filesToDTO(d.Patch.Files), Truncated: d.Patch.Truncated}, nil
}

// RepoStashes lists the stash entries, newest first.
func (e *Engine) RepoStashes(ctx context.Context, p RepoRef) ([]Stash, error) {
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return nil, err
	}
	list, err := app.RepoStashes(ctx, e.appDeps(nil), in)
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]Stash, 0, len(list))
	for _, s := range list {
		out = append(out, stashToDTO(s))
	}
	return out, nil
}

// RepoStash returns one stash entry's content.
func (e *Engine) RepoStash(ctx context.Context, p RepoStashParams) (StashDetail, error) {
	if p.Index < 0 {
		return StashDetail{}, invalidParam("index", errors.New("must not be negative"))
	}
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return StashDetail{}, err
	}
	d, err := app.RepoStash(ctx, e.appDeps(nil), app.RepoStashInput{RepoRefInput: in, Index: p.Index})
	if err != nil {
		return StashDetail{}, wrap(err)
	}
	return StashDetail{Stash: stashToDTO(d.Entry), Files: filesToDTO(d.Patch.Files), Truncated: d.Patch.Truncated, IncludesUntracked: d.IncludesUntracked}, nil
}

// RepoFetch fetches the repo's remote (with prune) and returns the branch
// view after it.
func (e *Engine) RepoFetch(ctx context.Context, p RepoRef, emit ProgressFunc) (RepoFetchResult, error) {
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return RepoFetchResult{}, err
	}
	r, err := app.FetchRepo(ctx, e.appDeps(emit), in)
	if err != nil {
		return RepoFetchResult{}, wrap(err)
	}
	return RepoFetchResult{Repo: r.Alias, Remote: r.Remote, Branch: branchToDTO(r.Branch)}, nil
}

// RepoPull fast-forwards the branch to its upstream; refusals are in the
// result, not an error.
func (e *Engine) RepoPull(ctx context.Context, p RepoRef, emit ProgressFunc) (RepoPullResult, error) {
	in, err := e.repoInput(ctx, p.Context, p.Workspace, p.Repo)
	if err != nil {
		return RepoPullResult{}, err
	}
	r, err := app.PullRepo(ctx, e.appDeps(emit), in)
	if err != nil {
		return RepoPullResult{}, wrap(err)
	}
	out := RepoPullResult{Repo: r.Alias, Upstream: r.Upstream, BeforeHead: r.BeforeHead, AfterHead: r.AfterHead, UpToDate: r.UpToDate,
		CommitsPulled: r.CommitsPulled, Ahead: r.Ahead, Behind: r.Behind}
	if r.Err == nil {
		return out, nil
	}
	body := AsError(r.Err).Body()
	data := map[string]any{}
	for k, v := range body.Data {
		data[k] = v
	}
	switch domain.Code(r.Err) {
	case domain.CodeWorktreeDirty:
		data["files"] = append([]string{}, r.DirtyFiles...)
	case domain.CodeDiverged:
		data["ahead"], data["behind"] = r.Ahead, r.Behind
	}
	body.Data = data
	out.Error = &body
	return out, nil
}

func branchToDTO(b app.BranchInfo) BranchInfo {
	out := BranchInfo{Repo: b.Alias, Branch: string(b.Branch), Detached: b.Detached, Head: b.Head, BaseBranch: string(b.BaseBranch),
		BaseRef: b.BaseRef, BaseFound: b.BaseFound, BaseAhead: b.BaseAhead, BaseBehind: b.BaseBehind}
	if b.Upstream != nil {
		out.Upstream = &UpstreamDTO{Ref: b.Upstream.Ref, Remote: b.Upstream.Remote, Gone: b.Upstream.Gone, Ahead: b.Upstream.Ahead, Behind: b.Upstream.Behind}
	}
	if b.LastFetch != nil {
		t := b.LastFetch.UTC().Truncate(time.Second)
		out.LastFetch = &t
	}
	return out
}

func entriesToDTO(list []domain.ChangeEntry) []ChangeEntry {
	out := make([]ChangeEntry, 0, len(list))
	for _, e := range list {
		out = append(out, ChangeEntry{Path: e.Path, OrigPath: e.OrigPath, Status: string(e.Status)})
	}
	return out
}

func fileDiffToDTO(d domain.FileDiff) FileDiff {
	out := FileDiff{Path: d.Path, OrigPath: d.OrigPath, Status: string(d.Status), Binary: d.Binary, Truncated: d.Truncated,
		Additions: d.Additions, Deletions: d.Deletions, Hunks: make([]DiffHunk, 0, len(d.Hunks))}
	for _, h := range d.Hunks {
		out.Hunks = append(out.Hunks, DiffHunk{Header: h.Header, OldStart: h.OldStart, OldLines: h.OldLines, NewStart: h.NewStart,
			NewLines: h.NewLines, Lines: append([]string{}, h.Lines...)})
	}
	return out
}

func filesToDTO(list []domain.FileDiff) []FileDiff {
	out := make([]FileDiff, 0, len(list))
	for _, d := range list {
		out = append(out, fileDiffToDTO(d))
	}
	return out
}

func commitToDTO(c domain.CommitInfo) Commit {
	return Commit{Hash: c.Hash, ShortHash: c.ShortHash, Subject: c.Subject, Author: c.AuthorName, Date: c.AuthorDate.UTC()}
}

func stashToDTO(s domain.StashEntry) Stash {
	return Stash{Index: s.Index, Ref: s.Ref, Hash: s.Hash, Branch: s.Branch, Message: s.Message, Date: s.Date.UTC()}
}
