// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// rejectOptionLike refuses a revision that git would parse as an option
// (design.md §13: ref argument injection). Refs this package receives are
// resolved by the engine itself, so this is a last line of defense.
func rejectOptionLike(op string, repo domain.Path, rev string) error {
	if rev == "" || strings.HasPrefix(rev, "-") {
		return domain.NewOpError(op, domain.CodeRefNotFound, rev, "revision is empty or looks like an option", nil)
	}
	return nil
}

// HeadCommit runs `rev-parse --verify HEAD`.
func (a *Adapter) HeadCommit(ctx context.Context, worktree domain.Path) (string, error) {
	out, err := a.run(ctx, "git.head_commit", worktree, "rev-parse", "--verify", "HEAD")
	return strings.TrimSpace(out), err
}

// BehindCount runs `rev-list --count HEAD..<base>`.
func (a *Adapter) BehindCount(ctx context.Context, worktree domain.Path, base string) (int, error) {
	const op = "git.behind_count"
	if err := rejectOptionLike(op, worktree, base); err != nil {
		return 0, err
	}
	out, err := a.run(ctx, op, worktree, "rev-list", "--count", "HEAD.."+base)
	if err != nil {
		return 0, err
	}
	n, perr := strconv.Atoi(strings.TrimSpace(out))
	if perr != nil {
		return 0, domain.NewOpError(op, domain.CodeGitFailed, string(worktree), perr.Error(), perr)
	}
	return n, nil
}

// Integrate runs `merge --no-edit [--autostash] <ref>` or `rebase
// [--autostash] <ref>`. --no-edit keeps a merge commit from ever waiting
// for an editor; a fast-forward happens whenever possible.
func (a *Adapter) Integrate(ctx context.Context, worktree domain.Path, spec ports.IntegrateSpec) error {
	const op = "git.integrate"
	if err := rejectOptionLike(op, worktree, spec.Ref); err != nil {
		return err
	}
	var args []string
	switch spec.Strategy {
	case domain.UpdateRebase:
		args = []string{"rebase"}
	default:
		args = []string{"merge", "--no-edit"}
	}
	if spec.Autostash {
		args = append(args, "--autostash")
	}
	_, err := a.run(ctx, op, worktree, append(args, spec.Ref)...)
	return err
}

// IntegrationInProgress checks MERGE_HEAD (`rev-parse -q --verify`, exit
// 1 when absent) and the rebase state directories git keeps in the
// worktree's own git dir (`rev-parse --git-path`).
func (a *Adapter) IntegrationInProgress(ctx context.Context, worktree domain.Path) (domain.UpdateStrategy, error) {
	const op = "git.integration_in_progress"
	code, _, _, err := a.runExpecting(ctx, op, worktree, []int{0, 1}, "rev-parse", "-q", "--verify", "MERGE_HEAD")
	if err != nil {
		return "", err
	}
	if code == 0 {
		return domain.UpdateMerge, nil
	}
	out, err := a.run(ctx, op, worktree, "rev-parse", "--git-path", "rebase-merge", "--git-path", "rebase-apply")
	if err != nil {
		return "", err
	}
	for _, p := range strings.Split(strings.TrimSpace(out), "\n") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(string(worktree), p)
		}
		if _, statErr := os.Stat(p); statErr == nil {
			return domain.UpdateRebase, nil
		}
	}
	return "", nil
}

// AbortIntegration runs `merge --abort` or `rebase --abort`.
func (a *Adapter) AbortIntegration(ctx context.Context, worktree domain.Path, s domain.UpdateStrategy) error {
	sub := "merge"
	if s == domain.UpdateRebase {
		sub = "rebase"
	}
	_, err := a.run(ctx, "git.abort_integration", worktree, sub, "--abort")
	return err
}

// ConflictedPaths runs `diff --name-only --diff-filter=U -z`; -z keeps
// names with spaces, quotes or newlines intact.
func (a *Adapter) ConflictedPaths(ctx context.Context, worktree domain.Path) ([]string, error) {
	out, err := a.run(ctx, "git.conflicted_paths", worktree, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil, err
	}
	return parseNULList(out), nil
}

// parseNULList splits NUL-terminated names, dropping empties and repeats.
func parseNULList(raw string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range strings.Split(raw, "\x00") {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// StashRef runs `rev-parse -q --verify refs/stash` (exit 1: empty stash).
func (a *Adapter) StashRef(ctx context.Context, worktree domain.Path) (string, error) {
	code, out, _, err := a.runExpecting(ctx, "git.stash_ref", worktree, []int{0, 1}, "rev-parse", "-q", "--verify", "refs/stash")
	if err != nil || code != 0 {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ResetHard runs `reset -q --hard <commit>`.
func (a *Adapter) ResetHard(ctx context.Context, worktree domain.Path, commit string) error {
	const op = "git.reset_hard"
	if err := rejectOptionLike(op, worktree, commit); err != nil {
		return err
	}
	_, err := a.run(ctx, op, worktree, "reset", "-q", "--hard", commit)
	return err
}

// StashPop runs `stash pop -q --index`.
func (a *Adapter) StashPop(ctx context.Context, worktree domain.Path) error {
	_, err := a.run(ctx, "git.stash_pop", worktree, "stash", "pop", "-q", "--index")
	return err
}
