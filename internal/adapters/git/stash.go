// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"context"
	"strconv"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// StashApply runs `stash apply -q [--index] <hash>`. Applying by commit
// (not stash@{N}) pins the exact entry the caller verified, however the
// list shifts meanwhile. git exits 1 for every refusal and stop; stderr
// tells them apart (LC_ALL=C makes it stable):
//   - "conflicts in index. Try without --index." (nothing changed)
//   - "... would be overwritten by merge:" + TAB-indented paths (nothing changed)
//   - anything else after git started changing the worktree (stopped).
func (a *Adapter) StashApply(ctx context.Context, worktree domain.Path, hash string, restoreIndex bool) (ports.StashApplyResult, error) {
	const op = "git.stash_apply"
	if !domain.ValidCommitHash(hash) {
		return ports.StashApplyResult{}, domain.NewOpError(op, domain.CodeRefNotFound, hash, "not a hexadecimal object name", nil)
	}
	args := []string{"stash", "apply", "-q"}
	if restoreIndex {
		args = append(args, "--index")
	}
	code, _, stderr, err := a.runExpecting(ctx, op, worktree, []int{0, 1}, append(args, hash)...)
	if err != nil {
		return ports.StashApplyResult{}, err
	}
	switch {
	case code == 0:
		return ports.StashApplyResult{Outcome: ports.StashApplied}, nil
	case strings.Contains(stderr, "conflicts in index"):
		return ports.StashApplyResult{Outcome: ports.StashIndexRefused}, nil
	}
	if files := parseOverwrittenFiles(stderr); len(files) > 0 {
		return ports.StashApplyResult{Outcome: ports.StashOverwriteRefused, Files: files}, nil
	}
	return ports.StashApplyResult{Outcome: ports.StashStopped, Files: parseUntrackedNotRestored(stderr)}, nil
}

// parseUntrackedNotRestored lists the paths of git's "<path> already
// exists, no checkout" lines (untracked stash files it could not restore).
func parseUntrackedNotRestored(stderr string) []string {
	var files []string
	for _, line := range strings.Split(stderr, "\n") {
		if p, ok := strings.CutSuffix(line, " already exists, no checkout"); ok && p != "" {
			files = append(files, p)
		}
	}
	return files
}

// StashUntrackedFiles checks for the stash commit's third parent (`rev-parse
// -q --verify <hash>^3`, exit 1: none) and lists its files with `ls-tree
// -r -z --name-only`.
func (a *Adapter) StashUntrackedFiles(ctx context.Context, worktree domain.Path, hash string) ([]string, error) {
	const op = "git.stash_untracked"
	if !domain.ValidCommitHash(hash) {
		return nil, domain.NewOpError(op, domain.CodeRefNotFound, hash, "not a hexadecimal object name", nil)
	}
	code, out, _, err := a.runExpecting(ctx, op, worktree, []int{0, 1}, "rev-parse", "-q", "--verify", hash+"^3")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return []string{}, nil
	}
	tree := strings.TrimSpace(out)
	list, err := a.run(ctx, op, worktree, "ls-tree", "-r", "-z", "--name-only", tree)
	if err != nil {
		return nil, err
	}
	return parseNULList(list), nil
}

// StashDrop runs `stash drop -q stash@{<index>}`.
func (a *Adapter) StashDrop(ctx context.Context, worktree domain.Path, index int) error {
	const op = "git.stash_drop"
	if index < 0 {
		return domain.NewOpError(op, domain.CodeRefNotFound, strconv.Itoa(index), "", nil)
	}
	_, err := a.run(ctx, op, worktree, "stash", "drop", "-q", "stash@{"+strconv.Itoa(index)+"}")
	return err
}

// CleanUntracked runs `clean -f -q -- :(literal)<path>...`. Without -d and
// -x git removes neither directories nor ignored files, and it never
// removes a tracked file. Each path must be a plain worktree-relative file
// path (a directory pathspec would make git recurse into it, so callers
// never pass one).
func (a *Adapter) CleanUntracked(ctx context.Context, worktree domain.Path, paths []string) error {
	const op = "git.clean"
	if len(paths) == 0 {
		return nil
	}
	args := []string{"clean", "-f", "-q", "--"}
	for _, p := range paths {
		if err := safeRelPath(op, p); err != nil {
			return err
		}
		if strings.HasSuffix(p, "/") {
			return domain.NewOpError(op, domain.CodePathNotUntracked, p, "directories are never cleaned", nil)
		}
		args = append(args, literal(p))
	}
	_, err := a.run(ctx, op, worktree, args...)
	return err
}
