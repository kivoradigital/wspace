// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Call is one recorded FakeGit invocation, in call order. Use cases assert
// pre-flight-before-mutation and similar ordering invariants against Calls
// (design.md §12: "asserts the call sequence on FakeGit").
type Call struct {
	Method string
	Repo   domain.Path
}

// FakeGit is a scriptable ports.GitPort: each method has an optional *Func
// override; when unset, a reasonable zero-friction default is returned. Every
// call is appended to Calls regardless of whether it was scripted.
type FakeGit struct {
	Calls []Call
	FakeGitInspect
	FakeGitWrite

	VersionFunc func() (ports.Version, error)
	// IsMainCloneFunc scripts IsMainClone. The real adapter
	// (internal/adapters/git/adapter.go's IsMainClone) returns exactly
	// three shapes, and a script that does not reproduce the right one for
	// the case it claims to simulate is itself a defect (it can hide a
	// production bug behind a green test — see that method's own doc
	// comment for the full contract):
	//
	//   - (true, nil): dir is a main clone.
	//   - (false, nil): dir has no ".git" entry at all (not a git
	//     repository, or a bare one) — never an error.
	//   - (false, err) where domain.Code(err) == domain.CodeNotAMainClone:
	//     dir is a linked worktree (".git" is a file, not a directory).
	//     Use LinkedWorktreeErr(dir) to construct this shape rather than
	//     hand-rolling it, and never script (false, nil) for this case —
	//     the real adapter never returns that for a linked worktree.
	IsMainCloneFunc func(dir domain.Path) (bool, error)
	FetchFunc       func(repo domain.Path, remote string) error
	ResolveBaseFunc func(repo domain.Path, remote string, base domain.BranchName) (ports.BaseRef, error)
	// RemoteDefaultBranchFunc scripts RemoteDefaultBranch; unset means the
	// remote records no default branch.
	RemoteDefaultBranchFunc func(repo domain.Path, remote string) (domain.BranchName, bool, error)
	SyncLocalBaseFunc       func(repo domain.Path, remote string, base domain.BranchName) (bool, error)
	BranchExistsFunc        func(repo domain.Path, b domain.BranchName) (bool, error)
	DeleteBranchFunc        func(repo domain.Path, b domain.BranchName, force bool) error
	WorktreeAddFunc         func(repo domain.Path, spec ports.WorktreeSpec) error
	WorktreeRemoveFunc      func(repo, worktree domain.Path, force bool) error
	WorktreePruneFunc       func(repo domain.Path) error
	WorktreeListFunc        func(repo domain.Path) ([]ports.WorktreeRef, error)
	CurrentBranchFunc       func(worktree domain.Path) (domain.BranchName, bool, error)
	StatusFunc              func(worktree domain.Path) ([]domain.PorcelainEntry, error)
	AheadBehindFunc         func(worktree domain.Path, upstream string) (ahead, behind int, err error)
	UnpushedCountFunc       func(worktree domain.Path, base string) (int, error)
	IsIgnoredFunc           func(worktree domain.Path, rel string) (bool, error)

	HeadCommitFunc            func(worktree domain.Path) (string, error)
	BehindCountFunc           func(worktree domain.Path, base string) (int, error)
	IntegrateFunc             func(worktree domain.Path, spec ports.IntegrateSpec) error
	IntegrationInProgressFunc func(worktree domain.Path) (domain.UpdateStrategy, error)
	AbortIntegrationFunc      func(worktree domain.Path, s domain.UpdateStrategy) error
	ConflictedPathsFunc       func(worktree domain.Path) ([]string, error)
	StashRefFunc              func(worktree domain.Path) (string, error)
	ResetHardFunc             func(worktree domain.Path, commit string) error
	StashPopFunc              func(worktree domain.Path) error
}

// NewFakeGit constructs an empty FakeGit; script it via its *Func fields.
func NewFakeGit() *FakeGit {
	return &FakeGit{}
}

// LinkedWorktreeErr constructs the exact error shape the real git adapter's
// IsMainClone returns for a linked-worktree directory (its ".git" entry is
// a file, not a directory): domain.CodeNotAMainClone, non-nil. Tests that
// need IsMainCloneFunc to simulate a linked worktree must return this,
// never (false, nil) — see IsMainCloneFunc's own doc comment for why that
// shape cannot happen in the real world for this case.
func LinkedWorktreeErr(dir domain.Path) error {
	return domain.NewOpError("git.is_main_clone", domain.CodeNotAMainClone, string(dir),
		"\".git\" is a file, not a directory: this is a linked worktree, not a main clone", nil)
}

func (f *FakeGit) record(method string, repo domain.Path) {
	f.Calls = append(f.Calls, Call{Method: method, Repo: repo})
}

func (f *FakeGit) Version(_ context.Context) (ports.Version, error) {
	f.record("Version", "")
	if f.VersionFunc != nil {
		return f.VersionFunc()
	}
	return ports.Version{Major: 2, Minor: 43, Patch: 0}, nil
}

func (f *FakeGit) IsMainClone(_ context.Context, dir domain.Path) (bool, error) {
	f.record("IsMainClone", dir)
	if f.IsMainCloneFunc != nil {
		return f.IsMainCloneFunc(dir)
	}
	return true, nil
}

func (f *FakeGit) Fetch(_ context.Context, repo domain.Path, remote string) error {
	f.record("Fetch", repo)
	if f.FetchFunc != nil {
		return f.FetchFunc(repo, remote)
	}
	return nil
}

func (f *FakeGit) ResolveBase(_ context.Context, repo domain.Path, remote string, base domain.BranchName) (ports.BaseRef, error) {
	f.record("ResolveBase", repo)
	if f.ResolveBaseFunc != nil {
		return f.ResolveBaseFunc(repo, remote, base)
	}
	return ports.BaseRef{Ref: remote + "/" + string(base), Remote: true}, nil
}

func (f *FakeGit) RemoteDefaultBranch(_ context.Context, repo domain.Path, remote string) (domain.BranchName, bool, error) {
	f.record("RemoteDefaultBranch", repo)
	if f.RemoteDefaultBranchFunc != nil {
		return f.RemoteDefaultBranchFunc(repo, remote)
	}
	return "", false, nil
}

func (f *FakeGit) SyncLocalBase(_ context.Context, repo domain.Path, remote string, base domain.BranchName) (bool, error) {
	f.record("SyncLocalBase", repo)
	if f.SyncLocalBaseFunc != nil {
		return f.SyncLocalBaseFunc(repo, remote, base)
	}
	return false, nil
}

func (f *FakeGit) BranchExists(_ context.Context, repo domain.Path, b domain.BranchName) (bool, error) {
	f.record("BranchExists", repo)
	if f.BranchExistsFunc != nil {
		return f.BranchExistsFunc(repo, b)
	}
	return false, nil
}

func (f *FakeGit) DeleteBranch(_ context.Context, repo domain.Path, b domain.BranchName, force bool) error {
	f.record("DeleteBranch", repo)
	if f.DeleteBranchFunc != nil {
		return f.DeleteBranchFunc(repo, b, force)
	}
	return nil
}

func (f *FakeGit) WorktreeAdd(_ context.Context, repo domain.Path, spec ports.WorktreeSpec) error {
	f.record("WorktreeAdd", repo)
	if f.WorktreeAddFunc != nil {
		return f.WorktreeAddFunc(repo, spec)
	}
	return nil
}

func (f *FakeGit) WorktreeRemove(_ context.Context, repo, worktree domain.Path, force bool) error {
	f.record("WorktreeRemove", repo)
	if f.WorktreeRemoveFunc != nil {
		return f.WorktreeRemoveFunc(repo, worktree, force)
	}
	return nil
}

func (f *FakeGit) WorktreePrune(_ context.Context, repo domain.Path) error {
	f.record("WorktreePrune", repo)
	if f.WorktreePruneFunc != nil {
		return f.WorktreePruneFunc(repo)
	}
	return nil
}

func (f *FakeGit) WorktreeList(_ context.Context, repo domain.Path) ([]ports.WorktreeRef, error) {
	f.record("WorktreeList", repo)
	if f.WorktreeListFunc != nil {
		return f.WorktreeListFunc(repo)
	}
	return nil, nil
}

func (f *FakeGit) CurrentBranch(_ context.Context, worktree domain.Path) (domain.BranchName, bool, error) {
	f.record("CurrentBranch", worktree)
	if f.CurrentBranchFunc != nil {
		return f.CurrentBranchFunc(worktree)
	}
	return "", false, nil
}

func (f *FakeGit) Status(_ context.Context, worktree domain.Path) ([]domain.PorcelainEntry, error) {
	f.record("Status", worktree)
	if f.StatusFunc != nil {
		return f.StatusFunc(worktree)
	}
	return nil, nil
}

func (f *FakeGit) AheadBehind(_ context.Context, worktree domain.Path, upstream string) (int, int, error) {
	f.record("AheadBehind", worktree)
	if f.AheadBehindFunc != nil {
		return f.AheadBehindFunc(worktree, upstream)
	}
	return 0, 0, nil
}

func (f *FakeGit) UnpushedCount(_ context.Context, worktree domain.Path, base string) (int, error) {
	f.record("UnpushedCount", worktree)
	if f.UnpushedCountFunc != nil {
		return f.UnpushedCountFunc(worktree, base)
	}
	return 0, nil
}

func (f *FakeGit) IsIgnored(_ context.Context, worktree domain.Path, rel string) (bool, error) {
	f.record("IsIgnored", worktree)
	if f.IsIgnoredFunc != nil {
		return f.IsIgnoredFunc(worktree, rel)
	}
	return false, nil
}

func (f *FakeGit) HeadCommit(_ context.Context, worktree domain.Path) (string, error) {
	f.record("HeadCommit", worktree)
	if f.HeadCommitFunc != nil {
		return f.HeadCommitFunc(worktree)
	}
	return "0000000000000000000000000000000000000000", nil
}

func (f *FakeGit) BehindCount(_ context.Context, worktree domain.Path, base string) (int, error) {
	f.record("BehindCount", worktree)
	if f.BehindCountFunc != nil {
		return f.BehindCountFunc(worktree, base)
	}
	return 0, nil
}

func (f *FakeGit) Integrate(_ context.Context, worktree domain.Path, spec ports.IntegrateSpec) error {
	f.record("Integrate", worktree)
	if f.IntegrateFunc != nil {
		return f.IntegrateFunc(worktree, spec)
	}
	return nil
}

func (f *FakeGit) IntegrationInProgress(_ context.Context, worktree domain.Path) (domain.UpdateStrategy, error) {
	f.record("IntegrationInProgress", worktree)
	if f.IntegrationInProgressFunc != nil {
		return f.IntegrationInProgressFunc(worktree)
	}
	return "", nil
}

func (f *FakeGit) AbortIntegration(_ context.Context, worktree domain.Path, s domain.UpdateStrategy) error {
	f.record("AbortIntegration", worktree)
	if f.AbortIntegrationFunc != nil {
		return f.AbortIntegrationFunc(worktree, s)
	}
	return nil
}

func (f *FakeGit) ConflictedPaths(_ context.Context, worktree domain.Path) ([]string, error) {
	f.record("ConflictedPaths", worktree)
	if f.ConflictedPathsFunc != nil {
		return f.ConflictedPathsFunc(worktree)
	}
	return nil, nil
}

func (f *FakeGit) StashRef(_ context.Context, worktree domain.Path) (string, error) {
	f.record("StashRef", worktree)
	if f.StashRefFunc != nil {
		return f.StashRefFunc(worktree)
	}
	return "", nil
}

func (f *FakeGit) ResetHard(_ context.Context, worktree domain.Path, commit string) error {
	f.record("ResetHard", worktree)
	if f.ResetHardFunc != nil {
		return f.ResetHardFunc(worktree, commit)
	}
	return nil
}

func (f *FakeGit) StashPop(_ context.Context, worktree domain.Path) error {
	f.record("StashPop", worktree)
	if f.StashPopFunc != nil {
		return f.StashPopFunc(worktree)
	}
	return nil
}
