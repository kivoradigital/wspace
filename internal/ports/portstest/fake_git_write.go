// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// FakeGitWrite scripts the repository inspector's write actions; it is
// embedded in FakeGit. Unset funcs succeed (a configured identity, a
// created commit, an accepted push) without changing any state.
type FakeGitWrite struct {
	StageFunc              func(worktree domain.Path, paths []string) error
	UnstageFunc            func(worktree domain.Path, paths []string, unborn bool) error
	UnstagedPatchFunc      func(worktree domain.Path, paths []string) (string, error)
	RestoreWorktreeFunc    func(worktree domain.Path, paths []string) error
	IdentityConfiguredFunc func(worktree domain.Path) (bool, error)
	CommitFunc             func(worktree domain.Path, message string) (ports.CommitResult, error)
	PushFunc               func(worktree domain.Path, spec ports.PushSpec) (ports.PushResult, error)
	StashPushFunc          func(worktree domain.Path, spec ports.StashPushSpec) error
}

func (f *FakeGit) Stage(_ context.Context, worktree domain.Path, paths []string) error {
	f.record("Stage", worktree)
	if f.StageFunc != nil {
		return f.StageFunc(worktree, paths)
	}
	return nil
}

func (f *FakeGit) Unstage(_ context.Context, worktree domain.Path, paths []string, unborn bool) error {
	f.record("Unstage", worktree)
	if f.UnstageFunc != nil {
		return f.UnstageFunc(worktree, paths, unborn)
	}
	return nil
}

func (f *FakeGit) UnstagedPatch(_ context.Context, worktree domain.Path, paths []string) (string, error) {
	f.record("UnstagedPatch", worktree)
	if f.UnstagedPatchFunc != nil {
		return f.UnstagedPatchFunc(worktree, paths)
	}
	return "", nil
}

func (f *FakeGit) RestoreWorktree(_ context.Context, worktree domain.Path, paths []string) error {
	f.record("RestoreWorktree", worktree)
	if f.RestoreWorktreeFunc != nil {
		return f.RestoreWorktreeFunc(worktree, paths)
	}
	return nil
}

func (f *FakeGit) IdentityConfigured(_ context.Context, worktree domain.Path) (bool, error) {
	f.record("IdentityConfigured", worktree)
	if f.IdentityConfiguredFunc != nil {
		return f.IdentityConfiguredFunc(worktree)
	}
	return true, nil
}

func (f *FakeGit) Commit(_ context.Context, worktree domain.Path, message string) (ports.CommitResult, error) {
	f.record("Commit", worktree)
	if f.CommitFunc != nil {
		return f.CommitFunc(worktree, message)
	}
	return ports.CommitResult{Outcome: ports.CommitCreated}, nil
}

func (f *FakeGit) Push(_ context.Context, worktree domain.Path, spec ports.PushSpec) (ports.PushResult, error) {
	f.record("Push", worktree)
	if f.PushFunc != nil {
		return f.PushFunc(worktree, spec)
	}
	return ports.PushResult{Outcome: ports.PushDone}, nil
}

func (f *FakeGit) StashPush(_ context.Context, worktree domain.Path, spec ports.StashPushSpec) error {
	f.record("StashPush", worktree)
	if f.StashPushFunc != nil {
		return f.StashPushFunc(worktree, spec)
	}
	return nil
}
