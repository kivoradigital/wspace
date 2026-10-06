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

// Fetch runs `git fetch --prune --quiet <remote>` (design.md §7).
func (a *Adapter) Fetch(ctx context.Context, repo domain.Path, remote string) error {
	// `remote get-url` exits 2 when the remote isn't configured: a
	// local-only clone, reported distinctly so callers can carry on.
	exitCode, _, _, err := a.runExpecting(ctx, "git.fetch", repo, []int{0, 2}, "remote", "get-url", "--", remote)
	if err != nil {
		return err
	}
	if exitCode == 2 {
		return domain.NewOpError("git.fetch", domain.CodeRemoteMissing, remote, "no such remote configured", nil)
	}
	_, err = a.run(ctx, "git.fetch", repo, "fetch", "--prune", "--quiet", "--", remote)
	return err
}

// showRefExists reports whether ref exists via `show-ref --verify --quiet`.
// Exit 0 means it exists, exit 1 means it does not; exit >= 2 is a genuine
// error (design.md §7).
func (a *Adapter) showRefExists(ctx context.Context, op string, repo domain.Path, ref string) (bool, error) {
	exitCode, _, _, err := a.runExpecting(ctx, op, repo, []int{0, 1}, "show-ref", "--verify", "--quiet", "--", ref)
	if err != nil {
		return false, err
	}
	return exitCode == 0, nil
}

// ResolveBase prefers origin/<base> over a local branch of the same name,
// falling back to the current HEAD if neither exists, so a stale local
// clone never seeds new work (design.md §7, repository-operations spec
// "Base branch resolution order").
func (a *Adapter) ResolveBase(ctx context.Context, repo domain.Path, remote string, base domain.BranchName) (ports.BaseRef, error) {
	const op = "git.resolve_base"

	remoteRef := "refs/remotes/" + remote + "/" + string(base)
	ok, err := a.showRefExists(ctx, op, repo, remoteRef)
	if err != nil {
		return ports.BaseRef{}, err
	}
	if ok {
		return ports.BaseRef{Ref: remote + "/" + string(base), Remote: true}, nil
	}

	localRef := "refs/heads/" + string(base)
	ok, err = a.showRefExists(ctx, op, repo, localRef)
	if err != nil {
		return ports.BaseRef{}, err
	}
	if ok {
		return ports.BaseRef{Ref: string(base), Remote: false}, nil
	}

	return ports.BaseRef{Ref: "HEAD", Remote: false}, nil
}

// RemoteDefaultBranch reads the symbolic ref refs/remotes/<remote>/HEAD
// (set by `git clone` or `git remote set-head`) via `symbolic-ref --quiet`.
// Exit 1 means it is not recorded: ok is false, never an error.
func (a *Adapter) RemoteDefaultBranch(ctx context.Context, repo domain.Path, remote string) (domain.BranchName, bool, error) {
	exitCode, stdout, _, err := a.runExpecting(ctx, "git.remote_default_branch", repo, []int{0, 1}, "symbolic-ref", "--quiet", "refs/remotes/"+remote+"/HEAD")
	if err != nil {
		return "", false, err
	}
	if exitCode != 0 {
		return "", false, nil
	}
	name := strings.TrimPrefix(strings.TrimSpace(stdout), "refs/remotes/"+remote+"/")
	if name == "" || strings.HasPrefix(name, "refs/") {
		return "", false, nil
	}
	return domain.BranchName(name), true, nil
}

// SyncLocalBase is best-effort convenience: it never returns an error.
// If base is checked out in the main clone (repo itself) and clean, it is
// fast-forwarded there via `merge --ff-only`. If base is not checked out
// anywhere, its ref is updated directly via a refspec fetch. Otherwise (base
// checked out elsewhere, or checked out here but dirty) it is left alone —
// no work is ever lost (design.md §7, repository-operations spec "Base sync
// safety").
func (a *Adapter) SyncLocalBase(ctx context.Context, repo domain.Path, remote string, base domain.BranchName) (bool, error) {
	const op = "git.sync_local_base"

	refs, err := a.WorktreeList(ctx, repo)
	if err != nil {
		return false, nil //nolint:nilerr // best-effort: never surfaces an error
	}

	checkedOutHere := false
	checkedOutElsewhere := false
	for i, wt := range refs {
		if wt.Branch != base {
			continue
		}
		if i == 0 {
			checkedOutHere = true
		} else {
			checkedOutElsewhere = true
		}
	}

	switch {
	case checkedOutHere:
		dirty, statusErr := a.isDirty(ctx, repo)
		if statusErr != nil || dirty {
			return false, nil
		}
		if _, mergeErr := a.run(ctx, op, repo, "merge", "--ff-only", remote+"/"+string(base)); mergeErr != nil {
			return false, nil
		}
		return true, nil

	case checkedOutElsewhere:
		return false, nil

	default:
		refspec := string(base) + ":" + string(base)
		if _, fetchErr := a.run(ctx, op, repo, "fetch", "--quiet", remote, refspec); fetchErr != nil {
			return false, nil
		}
		return true, nil
	}
}

func (a *Adapter) isDirty(ctx context.Context, worktree domain.Path) (bool, error) {
	entries, err := a.Status(ctx, worktree)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

// BranchExists reports whether a local branch exists via `show-ref
// --verify --quiet -- refs/heads/<b>`.
func (a *Adapter) BranchExists(ctx context.Context, repo domain.Path, b domain.BranchName) (bool, error) {
	return a.showRefExists(ctx, "git.branch_exists", repo, "refs/heads/"+string(b))
}

// DeleteBranch runs `branch -d` (or `-D` under force).
func (a *Adapter) DeleteBranch(ctx context.Context, repo domain.Path, b domain.BranchName, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := a.run(ctx, "git.delete_branch", repo, "branch", flag, "--", string(b))
	return err
}

// CurrentBranch runs `rev-parse --abbrev-ref HEAD`; the literal output
// "HEAD" means the worktree is in detached-HEAD state.
func (a *Adapter) CurrentBranch(ctx context.Context, worktree domain.Path) (domain.BranchName, bool, error) {
	out, err := a.run(ctx, "git.current_branch", worktree, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", false, err
	}
	name := strings.TrimSpace(out)
	if name == "HEAD" {
		return "", true, nil
	}
	return domain.BranchName(name), false, nil
}

// AheadBehind runs `rev-list --left-right --count <upstream>...HEAD`, whose
// single tab-separated output line is behind<TAB>ahead (design.md §7,
// named §13 test TestAheadBehind_ParsesLeftRightCount).
func (a *Adapter) AheadBehind(ctx context.Context, worktree domain.Path, upstream string) (ahead, behind int, err error) {
	out, err := a.run(ctx, "git.ahead_behind", worktree, "rev-list", "--left-right", "--count", upstream+"...HEAD")
	if err != nil {
		return 0, 0, err
	}
	behind, ahead, perr := parseLeftRightCount(out)
	if perr != nil {
		return 0, 0, domain.NewOpError("git.ahead_behind", domain.CodeGitFailed, string(worktree), perr.Error(), perr)
	}
	return ahead, behind, nil
}

// UnpushedCount runs `rev-list --count <base>..HEAD`, used when a branch has
// no upstream: a branch that was never pushed is fully "unpushed" against
// its base.
func (a *Adapter) UnpushedCount(ctx context.Context, worktree domain.Path, base string) (int, error) {
	out, err := a.run(ctx, "git.unpushed_count", worktree, "rev-list", "--count", base+"..HEAD")
	if err != nil {
		return 0, err
	}
	n, perr := strconv.Atoi(strings.TrimSpace(out))
	if perr != nil {
		return 0, domain.NewOpError("git.unpushed_count", domain.CodeGitFailed, string(worktree), perr.Error(), perr)
	}
	return n, nil
}

// IsIgnored runs `check-ignore -q -- <rel>`: exit 0 means ignored, exit 1
// means not ignored, exit >= 2 is a genuine error.
func (a *Adapter) IsIgnored(ctx context.Context, worktree domain.Path, rel string) (bool, error) {
	exitCode, _, _, err := a.runExpecting(ctx, "git.is_ignored", worktree, []int{0, 1}, "check-ignore", "-q", "--", rel)
	if err != nil {
		return false, err
	}
	return exitCode == 0, nil
}
