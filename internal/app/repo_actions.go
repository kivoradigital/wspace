// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// The repository inspector's local actions: stash apply / pop / drop and
// the removal of untracked files. Every one re-validates what the caller
// saw (the stash entry's hash, the paths' untracked status) right before
// acting, and none ever resets or discards a tracked change.

// actionTimeout bounds one local action (no network).
const actionTimeout = time.Minute

// WarningIndexNotRestored: the stash's staged changes could not be
// restored as staged (git refused --index), so it was applied without it
// and every change came back unstaged.
const WarningIndexNotRestored = "index_not_restored"

// RepoStashActionInput names the stash entry an action targets: its
// index plus the hash the caller saw for it (full or abbreviated). A
// mismatch means the list shifted since, and the action is refused.
type RepoStashActionInput struct {
	RepoRefInput
	Index int
	Hash  string
}

// StashApplyReport is ApplyStash's and PopStash's outcome. Err is nil on
// success and a *domain.OpError otherwise: CodeStashChanged,
// CodeIntegrationInProgress, CodeWorktreeDirty (Files set; nothing was
// changed), CodeStashConflict (Conflicts set; the worktree holds conflict
// markers and the entry was kept) or a git failure (the entry is kept;
// Files lists untracked files git could not restore, if any).
type StashApplyReport struct {
	Alias string
	Entry domain.StashEntry
	// IndexRestored is true when the stash's staged changes came back
	// staged.
	IndexRestored bool
	// Dropped is true when a pop removed the entry from the list.
	Dropped   bool
	Conflicts []string
	Files     []string
	Warnings  []string
	Err       error
}

// verifyStash finds stash@{index} and checks it still is the entry the
// caller saw (hash prefix, case-insensitive).
func verifyStash(ctx context.Context, deps Deps, t inspectTarget, op string, index int, hash string) (domain.StashEntry, error) {
	list, err := deps.Git.StashList(ctx, t.worktree)
	if err != nil {
		return domain.StashEntry{}, err
	}
	subject := "stash@{" + strconv.Itoa(index) + "}"
	want := strings.ToLower(hash)
	if want == "" || !domain.ValidCommitHash(want) {
		return domain.StashEntry{}, domain.NewOpError(op, domain.CodeStashChanged, subject, "no valid hash given", nil)
	}
	for _, s := range list {
		if s.Index == index {
			if strings.HasPrefix(strings.ToLower(s.Hash), want) {
				return s, nil
			}
			break
		}
	}
	return domain.StashEntry{}, domain.NewOpError(op, domain.CodeStashChanged, subject, "", nil)
}

// ApplyStash applies one stash entry (its index included when possible)
// and keeps it in the list.
func ApplyStash(ctx context.Context, deps Deps, in RepoStashActionInput) (StashApplyReport, error) {
	return applyStash(ctx, deps, in, false)
}

// PopStash applies one stash entry and drops it only when the apply was
// complete: no conflicts and the index restored. Otherwise the entry is
// kept, so nothing the stash holds can be lost.
func PopStash(ctx context.Context, deps Deps, in RepoStashActionInput) (StashApplyReport, error) {
	return applyStash(ctx, deps, in, true)
}

func applyStash(ctx context.Context, deps Deps, in RepoStashActionInput, pop bool) (StashApplyReport, error) {
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return StashApplyReport{}, err
	}
	op := "repo.stash_apply"
	if pop {
		op = "repo.stash_pop"
	}
	r := StashApplyReport{Alias: t.alias}
	fail := func(err error) (StashApplyReport, error) { r.Err = err; return r, nil }
	refuse := func(code domain.ErrCode, subject string) (StashApplyReport, error) {
		return fail(domain.NewOpError(op, code, subject, "", nil))
	}

	if r.Entry, err = verifyStash(ctx, deps, t, op, in.Index, in.Hash); err != nil {
		return fail(err)
	}
	if inProgress, err := deps.Git.IntegrationInProgress(ctx, t.worktree); err != nil {
		return fail(err)
	} else if inProgress != "" {
		return refuse(domain.CodeIntegrationInProgress, t.alias)
	}
	// Conflicts already in the worktree: git refuses to apply over them,
	// and they would be mistaken for this apply's own.
	if conflicted, err := deps.Git.ConflictedPaths(ctx, t.worktree); err != nil {
		return fail(err)
	} else if len(conflicted) > 0 {
		r.Files = conflicted
		return refuse(domain.CodeWorktreeDirty, t.alias)
	}
	// git applies the tracked part before it finds that an untracked file
	// of the stash already exists, so that case is refused up front.
	untracked, err := deps.Git.StashUntrackedFiles(ctx, t.worktree, r.Entry.Hash)
	if err != nil {
		return fail(err)
	}
	for _, p := range untracked {
		if exists, _ := deps.FS.Exists(t.worktree.Join(p)); exists {
			r.Files = append(r.Files, p)
		}
	}
	if len(r.Files) > 0 {
		return refuse(domain.CodeWorktreeDirty, t.alias)
	}

	res, err := deps.Git.StashApply(ctx, t.worktree, r.Entry.Hash, true)
	if err != nil {
		return fail(err)
	}
	r.IndexRestored = true
	if res.Outcome == ports.StashIndexRefused {
		// Nothing changed; applying without --index keeps every change
		// (unstaged) instead of refusing. Reported, never silent.
		r.IndexRestored = false
		r.Warnings = append(r.Warnings, WarningIndexNotRestored)
		if res, err = deps.Git.StashApply(ctx, t.worktree, r.Entry.Hash, false); err != nil {
			return fail(err)
		}
	}
	switch res.Outcome {
	case ports.StashApplied:
	case ports.StashOverwriteRefused:
		r.IndexRestored = false
		r.Files = res.Files
		return refuse(domain.CodeWorktreeDirty, t.alias)
	default:
		r.IndexRestored = false
		r.Files = res.Files
		if r.Conflicts, err = deps.Git.ConflictedPaths(ctx, t.worktree); err != nil {
			return fail(err)
		}
		if len(r.Conflicts) > 0 {
			return refuse(domain.CodeStashConflict, r.Entry.Ref)
		}
		return refuse(domain.CodeGitFailed, r.Entry.Ref)
	}

	if pop && r.IndexRestored {
		// Re-verify: drop works by position, and the list may have moved.
		if _, err := verifyStash(ctx, deps, t, op, in.Index, r.Entry.Hash); err != nil {
			return r, nil
		}
		if err := deps.Git.StashDrop(ctx, t.worktree, in.Index); err != nil {
			return fail(err)
		}
		r.Dropped = true
	}
	return r, nil
}

// VerifiedStash is RepoStash for the entry the caller saw: CodeStashChanged
// when stash@{Index} no longer has that hash. It previews a drop.
func VerifiedStash(ctx context.Context, deps Deps, in RepoStashActionInput) (StashDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return StashDetail{}, err
	}
	if _, err := verifyStash(ctx, deps, t, "repo.stash", in.Index, in.Hash); err != nil {
		return StashDetail{}, err
	}
	return RepoStash(ctx, deps, RepoStashInput{RepoRefInput: in.RepoRefInput, Index: in.Index})
}

// DropStash removes one stash entry after verifying it is the one the
// caller saw (CodeStashChanged otherwise). Its content is lost.
func DropStash(ctx context.Context, deps Deps, in RepoStashActionInput) (domain.StashEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return domain.StashEntry{}, err
	}
	e, err := verifyStash(ctx, deps, t, "repo.stash_drop", in.Index, in.Hash)
	if err != nil {
		return domain.StashEntry{}, err
	}
	if err := deps.Git.StashDrop(ctx, t.worktree, in.Index); err != nil {
		return domain.StashEntry{}, err
	}
	return e, nil
}

// UntrackedPathsInput lists untracked files of one repo (worktree-relative,
// exactly as RepoChangeSets lists them).
type UntrackedPathsInput struct {
	RepoRefInput
	Paths []string
}

// UntrackedPath is one validated untracked file.
type UntrackedPath struct {
	Path     string
	Absolute domain.Path
}

// UntrackedTargets is ValidateUntracked's result.
type UntrackedTargets struct {
	Worktree domain.Path
	Paths    []UntrackedPath
}

// ValidateUntracked checks that every path is an untracked file the repo's
// status lists right now (duplicates collapse) and resolves it inside the
// worktree. Anything else (tracked, ignored, missing, a directory entry,
// absolute or escaping) is CodePathNotUntracked; nothing is changed.
func ValidateUntracked(ctx context.Context, deps Deps, in UntrackedPathsInput) (UntrackedTargets, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return UntrackedTargets{}, err
	}
	return validateUntracked(ctx, deps, t, in.Paths)
}

func validateUntracked(ctx context.Context, deps Deps, t inspectTarget, paths []string) (UntrackedTargets, error) {
	const op = "repo.untracked"
	entries, err := deps.Git.Status(ctx, t.worktree)
	if err != nil {
		return UntrackedTargets{}, err
	}
	untracked := map[string]bool{}
	for _, e := range domain.SplitChanges(entries).Untracked {
		untracked[e.Path] = true
	}
	out := UntrackedTargets{Worktree: t.worktree, Paths: []UntrackedPath{}}
	seen := map[string]bool{}
	for _, p := range paths {
		// A status entry ending in "/" is a nested repository, never a
		// file; the clean form check rejects "a/../b" style spellings.
		if !untracked[p] || strings.HasSuffix(p, "/") || path.Clean(p) != p || strings.HasPrefix(p, "../") || path.IsAbs(p) {
			return UntrackedTargets{}, domain.NewOpError(op, domain.CodePathNotUntracked, p, "", nil)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out.Paths = append(out.Paths, UntrackedPath{Path: p, Absolute: t.worktree.Join(p)})
	}
	return out, nil
}

// DiscardUntrackedResult lists what DiscardUntracked removed and what is
// still there (e.g. recreated meanwhile).
type DiscardUntrackedResult struct {
	Alias   string
	Removed []string
	Kept    []string
}

// DiscardUntracked permanently deletes untracked files after
// ValidateUntracked's checks (all or nothing: one bad path refuses the
// whole call). Callers must have the user's confirmation.
func DiscardUntracked(ctx context.Context, deps Deps, in UntrackedPathsInput) (DiscardUntrackedResult, error) {
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return DiscardUntrackedResult{}, err
	}
	targets, err := validateUntracked(ctx, deps, t, in.Paths)
	if err != nil {
		return DiscardUntrackedResult{}, err
	}
	paths := make([]string, 0, len(targets.Paths))
	for _, p := range targets.Paths {
		paths = append(paths, p.Path)
	}
	if err := deps.Git.CleanUntracked(ctx, t.worktree, paths); err != nil {
		return DiscardUntrackedResult{}, err
	}
	after, err := deps.Git.Status(ctx, t.worktree)
	if err != nil {
		return DiscardUntrackedResult{}, err
	}
	still := map[string]bool{}
	for _, e := range domain.SplitChanges(after).Untracked {
		still[e.Path] = true
	}
	res := DiscardUntrackedResult{Alias: t.alias, Removed: []string{}, Kept: []string{}}
	for _, p := range paths {
		if still[p] {
			res.Kept = append(res.Kept, p)
		} else {
			res.Removed = append(res.Removed, p)
		}
	}
	return res, nil
}
