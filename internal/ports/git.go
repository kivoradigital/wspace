// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

import (
	"context"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
)

// GitPort is the only subprocess boundary in the product (design.md §7).
type GitPort interface {
	Version(ctx context.Context) (Version, error)
	IsMainClone(ctx context.Context, dir domain.Path) (bool, error)

	Fetch(ctx context.Context, repo domain.Path, remote string) error
	ResolveBase(ctx context.Context, repo domain.Path, remote string, base domain.BranchName) (BaseRef, error)
	// RemoteDefaultBranch reads <remote>/HEAD (the remote's default branch);
	// ok is false when it is not recorded locally.
	RemoteDefaultBranch(ctx context.Context, repo domain.Path, remote string) (branch domain.BranchName, ok bool, err error)
	SyncLocalBase(ctx context.Context, repo domain.Path, remote string, base domain.BranchName) (synced bool, err error)

	BranchExists(ctx context.Context, repo domain.Path, b domain.BranchName) (bool, error)
	DeleteBranch(ctx context.Context, repo domain.Path, b domain.BranchName, force bool) error

	WorktreeAdd(ctx context.Context, repo domain.Path, spec WorktreeSpec) error
	WorktreeRemove(ctx context.Context, repo, worktree domain.Path, force bool) error
	WorktreePrune(ctx context.Context, repo domain.Path) error
	WorktreeList(ctx context.Context, repo domain.Path) ([]WorktreeRef, error)

	CurrentBranch(ctx context.Context, worktree domain.Path) (branch domain.BranchName, detached bool, err error)
	Status(ctx context.Context, worktree domain.Path) ([]domain.PorcelainEntry, error)
	AheadBehind(ctx context.Context, worktree domain.Path, upstream string) (ahead, behind int, err error)
	UnpushedCount(ctx context.Context, worktree domain.Path, base string) (int, error)
	IsIgnored(ctx context.Context, worktree domain.Path, rel string) (bool, error)

	// HeadCommit returns the full object name HEAD points at.
	HeadCommit(ctx context.Context, worktree domain.Path) (string, error)
	// BehindCount counts the commits reachable from base but not from HEAD
	// (base's commits not yet integrated); a missing base is
	// CodeRefNotFound.
	BehindCount(ctx context.Context, worktree domain.Path, base string) (int, error)
	// Integrate merges or rebases spec.Ref into the current branch. Any
	// failure, a conflict included, is returned as an error; callers
	// inspect IntegrationInProgress to tell a stopped operation apart.
	Integrate(ctx context.Context, worktree domain.Path, spec IntegrateSpec) error
	// IntegrationInProgress reports the merge or rebase a worktree is
	// stopped in the middle of, "" when none.
	IntegrationInProgress(ctx context.Context, worktree domain.Path) (domain.UpdateStrategy, error)
	// AbortIntegration aborts the stopped merge or rebase, returning the
	// worktree to its state before it started (re-applying an autostash).
	AbortIntegration(ctx context.Context, worktree domain.Path, s domain.UpdateStrategy) error
	// ConflictedPaths lists the unmerged paths, each once, in git's order.
	ConflictedPaths(ctx context.Context, worktree domain.Path) ([]string, error)
	// StashRef returns the commit refs/stash points at, "" when the stash
	// is empty.
	StashRef(ctx context.Context, worktree domain.Path) (string, error)
	// ResetHard moves the current branch, index and tracked files to
	// commit. Untracked files are left alone.
	ResetHard(ctx context.Context, worktree domain.Path, commit string) error
	// StashPop applies the newest stash entry, index included, and drops
	// it; the entry is kept when it cannot be applied.
	StashPop(ctx context.Context, worktree domain.Path) error

	// The repository inspector's queries (read-only).

	// Diff returns the patch of one changed path: the index against HEAD
	// (Staged), the worktree against the index, or, for an untracked
	// file, its whole content as added. Output beyond limits is cut and
	// marked truncated.
	Diff(ctx context.Context, worktree domain.Path, spec DiffSpec) (domain.Patch, error)
	// CommitLog lists the commits of revRange ("<base>..HEAD" or "HEAD"),
	// newest first, skipping skip and returning at most max.
	CommitLog(ctx context.Context, worktree domain.Path, revRange string, skip, max int) ([]domain.CommitInfo, error)
	// CountCommits counts the commits of revRange.
	CountCommits(ctx context.Context, worktree domain.Path, revRange string) (int, error)
	// CommitDetail returns one commit (hash: a hexadecimal object name)
	// with its diff against its first parent; an unknown hash is
	// CodeRefNotFound.
	CommitDetail(ctx context.Context, worktree domain.Path, hash string, limits domain.DiffLimits) (domain.CommitDetail, error)
	// StashList lists the stash entries, newest first.
	StashList(ctx context.Context, worktree domain.Path) ([]domain.StashEntry, error)
	// StashShow returns the patch of stash@{index}; includeUntracked adds
	// the untracked files it holds (git 2.32+).
	StashShow(ctx context.Context, worktree domain.Path, index int, includeUntracked bool, limits domain.DiffLimits) (domain.Patch, error)
	// Upstream returns the current branch's upstream; ok is false when
	// none is configured (or HEAD is detached).
	Upstream(ctx context.Context, worktree domain.Path) (u domain.UpstreamInfo, ok bool, err error)
	// LastFetch is the modification time of the newest FETCH_HEAD (the
	// worktree's own and the main clone's); ok is false when there is none.
	LastFetch(ctx context.Context, worktree domain.Path) (t time.Time, ok bool, err error)
	// MergeFastForward runs a fast-forward-only merge of ref. When git
	// refuses because local changes or untracked files would be
	// overwritten, the error is CodeWorktreeDirty and files lists them; a
	// non-fast-forward is CodeDiverged. It never creates a merge commit.
	MergeFastForward(ctx context.Context, worktree domain.Path, ref string) (files []string, err error)

	// The repository inspector's stash and clean actions.

	// StashApply applies the stash commit hash (an entry of the stash
	// list) to the worktree, restoring its index when restoreIndex. It
	// never drops the entry. Refusals and stops are reported in the
	// result; err is only a failure to run git or an unexpected exit.
	StashApply(ctx context.Context, worktree domain.Path, hash string, restoreIndex bool) (StashApplyResult, error)
	// StashUntrackedFiles lists the untracked files the stash commit hash
	// holds (none when it was made without --include-untracked).
	StashUntrackedFiles(ctx context.Context, worktree domain.Path, hash string) ([]string, error)
	// StashDrop removes stash@{index} from the stash list.
	StashDrop(ctx context.Context, worktree domain.Path, index int) error
	// CleanUntracked permanently removes the given untracked files
	// (worktree-relative). git itself never removes a tracked or ignored
	// file here; callers pass only paths status lists as untracked.
	CleanUntracked(ctx context.Context, worktree domain.Path, paths []string) error

	// The repository inspector's write actions (stage, discard, commit,
	// push, stash create). Paths are worktree-relative, taken from the
	// worktree's own status; every one is passed as a literal pathspec.

	// Stage adds the paths' worktree state to the index (modifications,
	// deletions and untracked files alike).
	Stage(ctx context.Context, worktree domain.Path, paths []string) error
	// Unstage resets the paths' index entries to HEAD; unborn (no HEAD
	// commit yet) removes them from the index instead. The worktree is
	// never touched.
	Unstage(ctx context.Context, worktree domain.Path, paths []string, unborn bool) error
	// UnstagedPatch is the binary-safe patch of the paths' worktree
	// changes against the index, applicable with `git apply`.
	UnstagedPatch(ctx context.Context, worktree domain.Path, paths []string) (string, error)
	// RestoreWorktree overwrites the paths' worktree files with their
	// index version, discarding unstaged changes. The index is unchanged.
	RestoreWorktree(ctx context.Context, worktree domain.Path, paths []string) error
	// IdentityConfigured reports whether git can build an author and
	// committer identity from the repository's configuration.
	IdentityConfigured(ctx context.Context, worktree domain.Path) (bool, error)
	// Commit records the index as a new commit with message, running the
	// repository's hooks. Refusals are in the result; err is only a
	// failure to run git or an unexpected exit.
	Commit(ctx context.Context, worktree domain.Path, message string) (CommitResult, error)
	// Push pushes spec.Refspec to spec.Remote (never forced).
	Push(ctx context.Context, worktree domain.Path, spec PushSpec) (PushResult, error)
	// StashPush saves the local changes as a new stash entry.
	StashPush(ctx context.Context, worktree domain.Path, spec StashPushSpec) error
}

// CommitOutcome classifies one `git commit`.
type CommitOutcome string

const (
	// CommitCreated: the commit was recorded.
	CommitCreated CommitOutcome = "created"
	// CommitIdentityMissing: git has no author or committer identity.
	CommitIdentityMissing CommitOutcome = "identity_missing"
	// CommitHookFailed: a commit hook (pre-commit, prepare-commit-msg,
	// commit-msg) refused the commit; Output holds what it printed.
	CommitHookFailed CommitOutcome = "hook_failed"
)

// CommitResult is Commit's outcome.
type CommitResult struct {
	Outcome CommitOutcome
	// Output is git's (and the hooks') trimmed output for a refusal.
	Output string
}

// PushSpec parameterizes Push. Refspec is "<src>:<dst>" with full ref
// names (refs/heads/...); SetUpstream records dst as the branch's
// upstream (-u).
type PushSpec struct {
	Remote      string
	Refspec     string
	SetUpstream bool
}

// PushOutcome classifies one `git push`.
type PushOutcome string

const (
	// PushDone: the remote accepted the push.
	PushDone PushOutcome = "pushed"
	// PushRejected: the remote has commits the branch lacks
	// (non-fast-forward); nothing was changed.
	PushRejected PushOutcome = "rejected"
	// PushAuthFailed: the remote could not be reached or refused the
	// credentials.
	PushAuthFailed PushOutcome = "auth_failed"
	// PushFailed: any other refusal (a pre-push hook, a protected branch).
	PushFailed PushOutcome = "failed"
)

// PushResult is Push's outcome.
type PushResult struct {
	Outcome PushOutcome
	// Output is git's trimmed output for a refusal, with credentials in
	// URLs masked.
	Output string
}

// StashPushSpec parameterizes StashPush.
type StashPushSpec struct {
	Message          string
	IncludeUntracked bool
	KeepIndex        bool
}

// StashApplyOutcome classifies one `git stash apply`.
type StashApplyOutcome string

const (
	// StashApplied: the stash was applied cleanly.
	StashApplied StashApplyOutcome = "applied"
	// StashIndexRefused: with restoreIndex, the stashed index no longer
	// applies ("conflicts in index"); git changed nothing.
	StashIndexRefused StashApplyOutcome = "index_refused"
	// StashOverwriteRefused: local changes would be overwritten (Files);
	// git changed nothing.
	StashOverwriteRefused StashApplyOutcome = "overwrite_refused"
	// StashStopped: git stopped after changing the worktree, on merge
	// conflicts or on untracked files it could not restore. The entry is
	// still in the stash list.
	StashStopped StashApplyOutcome = "stopped"
)

// StashApplyResult is StashApply's outcome.
type StashApplyResult struct {
	Outcome StashApplyOutcome
	// Files are the local files git refused to overwrite
	// (StashOverwriteRefused) or the untracked files it could not restore
	// because they already exist (StashStopped).
	Files []string
}

// DiffSpec parameterizes GitPort.Diff. Path (and OrigPath for a rename)
// are worktree-relative paths taken from the worktree's own status.
type DiffSpec struct {
	Path      string
	OrigPath  string
	Staged    bool
	Untracked bool
	Limits    domain.DiffLimits
}

// IntegrateSpec parameterizes GitPort.Integrate.
type IntegrateSpec struct {
	Ref       string // e.g. "origin/develop"
	Strategy  domain.UpdateStrategy
	Autostash bool // stash local changes first and re-apply them after
}

// Version is a parsed `git --version`.
type Version struct{ Major, Minor, Patch int }

// BaseRef is the result of ResolveBase.
type BaseRef struct {
	Ref    string // "origin/main", "main" or "HEAD"
	Remote bool
}

// WorktreeSpec parameterizes WorktreeAdd.
type WorktreeSpec struct {
	Target     domain.Path
	Branch     domain.BranchName
	StartPoint string // non-empty => create the branch here; empty => check out existing
}

// WorktreeRef is one entry from `git worktree list --porcelain`.
type WorktreeRef struct {
	Path     domain.Path
	Branch   domain.BranchName
	Detached bool
	Prunable bool
}
