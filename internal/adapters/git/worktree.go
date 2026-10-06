// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"context"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// WorktreeAdd creates a worktree. When spec.StartPoint is non-empty, a new
// branch is created there (`worktree add -b <branch> -- <target>
// <startpoint>`); otherwise an existing branch is attached (`worktree add --
// <target> <branch>`) (design.md §7).
func (a *Adapter) WorktreeAdd(ctx context.Context, repo domain.Path, spec ports.WorktreeSpec) error {
	var err error
	if spec.StartPoint != "" {
		_, err = a.run(ctx, "git.worktree_add", repo,
			"worktree", "add", "-b", string(spec.Branch), "--", string(spec.Target), spec.StartPoint)
	} else {
		_, err = a.run(ctx, "git.worktree_add", repo,
			"worktree", "add", "--", string(spec.Target), string(spec.Branch))
	}
	return err
}

// WorktreeRemove runs `worktree remove [--force] -- <target>`.
func (a *Adapter) WorktreeRemove(ctx context.Context, repo, worktree domain.Path, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, "--", string(worktree))
	_, err := a.run(ctx, "git.worktree_remove", repo, args...)
	return err
}

// WorktreePrune runs `worktree prune`.
func (a *Adapter) WorktreePrune(ctx context.Context, repo domain.Path) error {
	_, err := a.run(ctx, "git.worktree_prune", repo, "worktree", "prune")
	return err
}

// WorktreeList runs `worktree list --porcelain` and parses its records.
func (a *Adapter) WorktreeList(ctx context.Context, repo domain.Path) ([]ports.WorktreeRef, error) {
	out, err := a.run(ctx, "git.worktree_list", repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreeList(out), nil
}

// parseWorktreeList parses `git worktree list --porcelain` output: records
// are separated by blank lines; recognized keys are "worktree <path>",
// "HEAD <sha>", "branch refs/heads/<name>", and the bare markers "detached"
// and "prunable" (design.md §7).
func parseWorktreeList(out string) []ports.WorktreeRef {
	var refs []ports.WorktreeRef
	var cur *ports.WorktreeRef

	flush := func() {
		if cur != nil {
			refs = append(refs, *cur)
			cur = nil
		}
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			flush()
			continue
		}
		if cur == nil {
			cur = &ports.WorktreeRef{}
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur.Path = domain.Path(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "branch refs/heads/"):
			cur.Branch = domain.BranchName(strings.TrimPrefix(line, "branch refs/heads/"))
		case line == "detached":
			cur.Detached = true
		case strings.HasPrefix(line, "prunable"):
			// Real git may append a reason ("prunable gitdir file points to
			// non-existent location"), even though design.md §7 documents
			// it as a bare marker; matching the prefix keeps both true.
			cur.Prunable = true
		}
		// "HEAD <sha>" and any other key are intentionally ignored: nothing
		// in ports.WorktreeRef needs the raw commit sha.
	}
	flush()

	return refs
}
