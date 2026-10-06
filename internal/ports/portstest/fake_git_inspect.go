// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"context"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// FakeGitInspect scripts the repository inspector's GitPort methods; it
// is embedded in FakeGit. Unset funcs return empty results (no upstream,
// no stash, never fetched).
type FakeGitInspect struct {
	DiffFunc             func(worktree domain.Path, spec ports.DiffSpec) (domain.Patch, error)
	CommitLogFunc        func(worktree domain.Path, revRange string, skip, max int) ([]domain.CommitInfo, error)
	CountCommitsFunc     func(worktree domain.Path, revRange string) (int, error)
	CommitDetailFunc     func(worktree domain.Path, hash string, limits domain.DiffLimits) (domain.CommitDetail, error)
	StashListFunc        func(worktree domain.Path) ([]domain.StashEntry, error)
	StashShowFunc        func(worktree domain.Path, index int, includeUntracked bool, limits domain.DiffLimits) (domain.Patch, error)
	UpstreamFunc         func(worktree domain.Path) (domain.UpstreamInfo, bool, error)
	LastFetchFunc        func(worktree domain.Path) (time.Time, bool, error)
	MergeFastForwardFunc func(worktree domain.Path, ref string) ([]string, error)

	StashApplyFunc          func(worktree domain.Path, hash string, restoreIndex bool) (ports.StashApplyResult, error)
	StashUntrackedFilesFunc func(worktree domain.Path, hash string) ([]string, error)
	StashDropFunc           func(worktree domain.Path, index int) error
	CleanUntrackedFunc      func(worktree domain.Path, paths []string) error
}

func (f *FakeGit) Diff(_ context.Context, worktree domain.Path, spec ports.DiffSpec) (domain.Patch, error) {
	f.record("Diff", worktree)
	if f.DiffFunc != nil {
		return f.DiffFunc(worktree, spec)
	}
	return domain.Patch{Files: []domain.FileDiff{}}, nil
}

func (f *FakeGit) CommitLog(_ context.Context, worktree domain.Path, revRange string, skip, max int) ([]domain.CommitInfo, error) {
	f.record("CommitLog", worktree)
	if f.CommitLogFunc != nil {
		return f.CommitLogFunc(worktree, revRange, skip, max)
	}
	return []domain.CommitInfo{}, nil
}

func (f *FakeGit) CountCommits(_ context.Context, worktree domain.Path, revRange string) (int, error) {
	f.record("CountCommits", worktree)
	if f.CountCommitsFunc != nil {
		return f.CountCommitsFunc(worktree, revRange)
	}
	return 0, nil
}

func (f *FakeGit) CommitDetail(_ context.Context, worktree domain.Path, hash string, limits domain.DiffLimits) (domain.CommitDetail, error) {
	f.record("CommitDetail", worktree)
	if f.CommitDetailFunc != nil {
		return f.CommitDetailFunc(worktree, hash, limits)
	}
	return domain.CommitDetail{}, domain.NewOpError("git.commit_detail", domain.CodeRefNotFound, hash, "", nil)
}

func (f *FakeGit) StashList(_ context.Context, worktree domain.Path) ([]domain.StashEntry, error) {
	f.record("StashList", worktree)
	if f.StashListFunc != nil {
		return f.StashListFunc(worktree)
	}
	return []domain.StashEntry{}, nil
}

func (f *FakeGit) StashShow(_ context.Context, worktree domain.Path, index int, includeUntracked bool, limits domain.DiffLimits) (domain.Patch, error) {
	f.record("StashShow", worktree)
	if f.StashShowFunc != nil {
		return f.StashShowFunc(worktree, index, includeUntracked, limits)
	}
	return domain.Patch{Files: []domain.FileDiff{}}, nil
}

func (f *FakeGit) Upstream(_ context.Context, worktree domain.Path) (domain.UpstreamInfo, bool, error) {
	f.record("Upstream", worktree)
	if f.UpstreamFunc != nil {
		return f.UpstreamFunc(worktree)
	}
	return domain.UpstreamInfo{}, false, nil
}

func (f *FakeGit) LastFetch(_ context.Context, worktree domain.Path) (time.Time, bool, error) {
	f.record("LastFetch", worktree)
	if f.LastFetchFunc != nil {
		return f.LastFetchFunc(worktree)
	}
	return time.Time{}, false, nil
}

func (f *FakeGit) MergeFastForward(_ context.Context, worktree domain.Path, ref string) ([]string, error) {
	f.record("MergeFastForward", worktree)
	if f.MergeFastForwardFunc != nil {
		return f.MergeFastForwardFunc(worktree, ref)
	}
	return nil, nil
}

// StashApply defaults to a clean apply.
func (f *FakeGit) StashApply(_ context.Context, worktree domain.Path, hash string, restoreIndex bool) (ports.StashApplyResult, error) {
	f.record("StashApply", worktree)
	if f.StashApplyFunc != nil {
		return f.StashApplyFunc(worktree, hash, restoreIndex)
	}
	return ports.StashApplyResult{Outcome: ports.StashApplied}, nil
}

func (f *FakeGit) StashUntrackedFiles(_ context.Context, worktree domain.Path, hash string) ([]string, error) {
	f.record("StashUntrackedFiles", worktree)
	if f.StashUntrackedFilesFunc != nil {
		return f.StashUntrackedFilesFunc(worktree, hash)
	}
	return []string{}, nil
}

func (f *FakeGit) StashDrop(_ context.Context, worktree domain.Path, index int) error {
	f.record("StashDrop", worktree)
	if f.StashDropFunc != nil {
		return f.StashDropFunc(worktree, index)
	}
	return nil
}

func (f *FakeGit) CleanUntracked(_ context.Context, worktree domain.Path, paths []string) error {
	f.record("CleanUntracked", worktree)
	if f.CleanUntrackedFunc != nil {
		return f.CleanUntrackedFunc(worktree, paths)
	}
	return nil
}
