// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"errors"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// The repository inspector's write actions: stage, unstage, discard
// unstaged changes, commit, push and create a stash. Each one re-reads
// the repo's status right before acting and accepts only paths that
// status lists on the side the action works on. None of them ever
// forces a push, skips hooks, amends or rewrites a commit, or touches an
// untracked file.

const (
	// commitTimeout bounds a commit, which runs the repository's hooks
	// (linters, tests).
	commitTimeout = 5 * time.Minute
	// SubjectLimit is the commit subject length past which
	// WarningSubjectTooLong is reported (the commit is still made).
	SubjectLimit = 72
	// WarningSubjectTooLong: the commit's first line exceeds SubjectLimit.
	WarningSubjectTooLong = "subject_too_long"
	// DiscardBackupsKept is how many discard backup patches are kept.
	DiscardBackupsKept = 20
	// discardDir is the backups' directory under the cache root.
	discardDir = "discarded"
)

// ErrEmptyCommitMessage: the commit message is empty or only whitespace.
var ErrEmptyCommitMessage = errors.New("the commit message is empty")

// IdentityCommands are the commands git itself suggests when it has no
// author identity. wspace never runs them; the user does.
var IdentityCommands = []string{
	`git config --global user.name "Your Name"`,
	`git config --global user.email "you@example.com"`,
}

// ChangePathsInput lists changed paths of one repo, exactly as
// RepoChangeSets lists them. All selects every path the action applies
// to instead (Paths is then ignored).
type ChangePathsInput struct {
	RepoRefInput
	Paths []string
	All   bool
}

// StageResult lists the paths StageChanges or UnstageChanges acted on.
type StageResult struct {
	Alias string
	Paths []string
}

// writeTarget resolves the repo and reads its status.
func writeTarget(ctx context.Context, deps Deps, in RepoRefInput) (inspectTarget, domain.ChangeSets, error) {
	t, err := resolveInspectTarget(ctx, deps, in)
	if err != nil {
		return inspectTarget{}, domain.ChangeSets{}, err
	}
	entries, err := deps.Git.Status(ctx, t.worktree)
	if err != nil {
		return inspectTarget{}, domain.ChangeSets{}, err
	}
	return t, domain.SplitChanges(entries), nil
}

// cleanRelPath rejects absolute, escaping and non-canonical spellings and
// directory entries (a nested repository is listed with a trailing "/").
func cleanRelPath(p string) bool {
	return p != "" && !strings.HasSuffix(p, "/") && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../") && !path.IsAbs(p)
}

// pickPaths validates paths against allowed (path → entry), in the
// caller's order, duplicates collapsed. refuse decides the error for a
// path that is not allowed.
func pickPaths(paths []string, allowed map[string]domain.ChangeEntry, refuse func(p string) error) ([]domain.ChangeEntry, error) {
	out := []domain.ChangeEntry{}
	seen := map[string]bool{}
	for _, p := range paths {
		e, ok := allowed[p]
		if !ok || !cleanRelPath(p) {
			return nil, refuse(p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, e)
		}
	}
	return out, nil
}

func entryPaths(entries []domain.ChangeEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func byPath(lists ...[]domain.ChangeEntry) map[string]domain.ChangeEntry {
	m := map[string]domain.ChangeEntry{}
	for _, l := range lists {
		for _, e := range l {
			if cleanRelPath(e.Path) {
				m[e.Path] = e
			}
		}
	}
	return m
}

func notChanged(op string) func(string) error {
	return func(p string) error { return domain.NewOpError(op, domain.CodePathNotChanged, p, "", nil) }
}

// StageChanges stages paths the repo lists as unstaged (modified,
// deleted) or untracked. Anything else (a staged-only or conflicted
// path, an unknown or escaping one) refuses the whole call with
// CodePathNotChanged and changes nothing.
func StageChanges(ctx context.Context, deps Deps, in ChangePathsInput) (StageResult, error) {
	const op = "repo.stage"
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	t, cs, err := writeTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return StageResult{}, err
	}
	allowed := byPath(cs.Unstaged, cs.Untracked)
	paths := in.Paths
	if in.All {
		paths = append(entryPaths(cs.Unstaged), entryPaths(cs.Untracked)...)
		paths = slicesFilter(paths, cleanRelPath)
	}
	picked, err := pickPaths(paths, allowed, notChanged(op))
	if err != nil {
		return StageResult{}, err
	}
	out := entryPaths(picked)
	if err := deps.Git.Stage(ctx, t.worktree, out); err != nil {
		return StageResult{}, err
	}
	return StageResult{Alias: t.alias, Paths: out}, nil
}

// UnstageChanges moves staged paths back out of the index (a staged
// rename's source path included), leaving the worktree as it is. In a
// repo without a commit yet the paths are removed from the index.
func UnstageChanges(ctx context.Context, deps Deps, in ChangePathsInput) (StageResult, error) {
	const op = "repo.unstage"
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	t, cs, err := writeTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return StageResult{}, err
	}
	paths := in.Paths
	if in.All {
		paths = slicesFilter(entryPaths(cs.Staged), cleanRelPath)
	}
	picked, err := pickPaths(paths, byPath(cs.Staged), notChanged(op))
	if err != nil {
		return StageResult{}, err
	}
	args := entryPaths(picked)
	for _, e := range picked {
		if e.OrigPath != "" && cleanRelPath(e.OrigPath) && !contains(args, e.OrigPath) {
			args = append(args, e.OrigPath)
		}
	}
	_, headErr := deps.Git.HeadCommit(ctx, t.worktree)
	if err := deps.Git.Unstage(ctx, t.worktree, args, headErr != nil); err != nil {
		return StageResult{}, err
	}
	return StageResult{Alias: t.alias, Paths: entryPaths(picked)}, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func slicesFilter(list []string, keep func(string) bool) []string {
	out := []string{}
	for _, s := range list {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}

// DiscardFile is one file a discard would restore: its unstaged change
// and line counts.
type DiscardFile struct {
	Path      string
	Status    domain.FileChangeStatus
	Additions int
	Deletions int
	Binary    bool
}

// DiscardPreview is what DiscardChanges would discard.
type DiscardPreview struct {
	Alias string
	Files []DiscardFile
}

// DiscardResult is DiscardChanges' outcome: the paths restored and the
// backup patch holding what was discarded ("" when git produced none).
type DiscardResult struct {
	Alias     string
	Discarded []string
	Backup    domain.Path
}

// validateDiscard accepts only tracked files with unstaged changes. An
// untracked file (or one only marked intent-to-add) is
// CodePathIsUntracked: deleting it is the separate untracked-file action.
// A path whose changes are all staged is CodeStagedOnly (unstage first).
func validateDiscard(cs domain.ChangeSets, paths []string) ([]domain.ChangeEntry, error) {
	const op = "repo.discard"
	allowed := map[string]domain.ChangeEntry{}
	untracked := byPath(cs.Untracked)
	for _, e := range cs.Unstaged {
		if !cleanRelPath(e.Path) {
			continue
		}
		if e.Status == domain.FileAdded {
			untracked[e.Path] = e
			continue
		}
		allowed[e.Path] = e
	}
	staged := byPath(cs.Staged)
	return pickPaths(paths, allowed, func(p string) error {
		switch {
		case !cleanRelPath(p):
			return domain.NewOpError(op, domain.CodePathNotChanged, p, "", nil)
		case untracked[p].Path != "":
			return domain.NewOpError(op, domain.CodePathIsUntracked, p, "", nil)
		case staged[p].Path != "":
			return domain.NewOpError(op, domain.CodeStagedOnly, p, "", nil)
		}
		return domain.NewOpError(op, domain.CodePathNotChanged, p, "", nil)
	})
}

// PreviewDiscard validates paths like DiscardChanges and returns each
// file's unstaged line counts. It changes nothing.
func PreviewDiscard(ctx context.Context, deps Deps, in ChangePathsInput) (DiscardPreview, error) {
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	t, cs, err := writeTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return DiscardPreview{}, err
	}
	picked, err := validateDiscard(cs, in.Paths)
	if err != nil {
		return DiscardPreview{}, err
	}
	p := DiscardPreview{Alias: t.alias, Files: []DiscardFile{}}
	for _, e := range picked {
		f := DiscardFile{Path: e.Path, Status: e.Status}
		patch, err := deps.Git.Diff(ctx, t.worktree, ports.DiffSpec{Path: e.Path, Limits: domain.DefaultDiffLimits})
		if err == nil && len(patch.Files) > 0 {
			f.Additions, f.Deletions, f.Binary = patch.Files[0].Additions, patch.Files[0].Deletions, patch.Files[0].Binary
		}
		p.Files = append(p.Files, f)
	}
	return p, nil
}

// DiscardChanges discards the unstaged changes of tracked files (the
// index, staged changes included, is untouched): it first writes them as
// a patch under <cache>/discarded/ (`git apply` restores them; the newest
// DiscardBackupsKept are kept), then restores the files from the index.
// A backup that cannot be written stops the discard. Callers must have
// the user's confirmation.
func DiscardChanges(ctx context.Context, deps Deps, in ChangePathsInput) (DiscardResult, error) {
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	t, cs, err := writeTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return DiscardResult{}, err
	}
	picked, err := validateDiscard(cs, in.Paths)
	if err != nil {
		return DiscardResult{}, err
	}
	paths := entryPaths(picked)
	patch, err := deps.Git.UnstagedPatch(ctx, t.worktree, paths)
	if err != nil {
		return DiscardResult{}, err
	}
	res := DiscardResult{Alias: t.alias, Discarded: paths}
	if strings.TrimSpace(patch) != "" {
		if res.Backup, err = writeDiscardBackup(deps, t, patch); err != nil {
			return DiscardResult{}, domain.NewOpError("repo.discard", domain.CodeExecFailed, t.alias, err.Error(), err)
		}
	}
	if err := deps.Git.RestoreWorktree(ctx, t.worktree, paths); err != nil {
		return DiscardResult{}, err
	}
	return res, nil
}

func now(deps Deps) time.Time {
	if deps.Now != nil {
		return deps.Now()
	}
	return time.Now()
}

// writeDiscardBackup writes patch to <cache>/discarded/<alias>-<UTC
// timestamp>.patch (mode 0600: it holds source code) with a header saying
// how to restore it, then prunes all but the newest DiscardBackupsKept.
func writeDiscardBackup(deps Deps, t inspectTarget, patch string) (domain.Path, error) {
	dir := deps.FS.Paths().Cache.Join(discardDir)
	if err := deps.FS.MkdirAll(dir); err != nil {
		return "", err
	}
	at := now(deps).UTC()
	stamp := at.Format("20060102T150405.000Z")
	name := t.alias + "-" + stamp + ".patch"
	file := dir.Join(name)
	for i := 2; ; i++ {
		if exists, _ := deps.FS.Exists(file); !exists {
			break
		}
		name = t.alias + "-" + stamp + "-" + strconv.Itoa(i) + ".patch"
		file = dir.Join(name)
	}
	header := "# wspace: unstaged changes discarded from " + t.alias + " (" + string(t.worktree) + ") at " + at.Format(time.RFC3339) + "\n" +
		"# Restore them with: git -C " + string(t.worktree) + " apply " + string(file) + "\n"
	if err := deps.FS.WriteFile(file, []byte(header+patch), 0o600); err != nil {
		return "", err
	}
	pruneDiscardBackups(deps, dir)
	return file, nil
}

// pruneDiscardBackups removes all but the newest DiscardBackupsKept
// patches (newest by modification time, then by name). Best-effort: a
// failure keeps a file, never loses the new backup.
func pruneDiscardBackups(deps Deps, dir domain.Path) {
	files, err := deps.FS.ListFiles(dir)
	if err != nil {
		return
	}
	type backup struct {
		name string
		mod  time.Time
	}
	var list []backup
	for _, f := range files {
		if !strings.HasSuffix(f, ".patch") {
			continue
		}
		mod, _ := deps.FS.ModTime(dir.Join(f))
		list = append(list, backup{f, mod})
	}
	if len(list) <= DiscardBackupsKept {
		return
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].mod.Equal(list[j].mod) {
			return list[i].mod.After(list[j].mod)
		}
		return list[i].name > list[j].name
	})
	for _, b := range list[DiscardBackupsKept:] {
		_ = deps.FS.RemoveAll(dir.Join(b.name))
	}
}

// CommitInput is one commit's message.
type CommitInput struct {
	RepoRefInput
	Message string
}

// CommitReport is CommitChanges' outcome. Err is nil on success (Commit
// set) and a *domain.OpError otherwise: CodeDetachedHead,
// CodeIntegrationInProgress, CodeWorktreeDirty (unresolved conflicts;
// Files), CodeNothingStaged, CodeIdentityMissing (IdentityCommands set),
// CodeHookFailed (Output set) or a git failure. A refusal never commits.
type CommitReport struct {
	Alias            string
	Commit           domain.CommitInfo
	Warnings         []string
	Files            []string
	IdentityCommands []string
	Output           string
	Err              error
}

// normalizeMessage trims surrounding whitespace and each line's trailing
// whitespace.
func normalizeMessage(m string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(m, "\r\n", "\n")), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// CommitChanges commits the staged changes with the repo's configured
// identity, running its hooks. It never sets an identity, never skips
// hooks and never amends. Only an unknown workspace or alias, or an empty
// message (ErrEmptyCommitMessage), is returned as an error.
func CommitChanges(ctx context.Context, deps Deps, in CommitInput) (CommitReport, error) {
	const op = "repo.commit"
	msg := normalizeMessage(in.Message)
	if msg == "" {
		return CommitReport{}, ErrEmptyCommitMessage
	}
	ctx, cancel := context.WithTimeout(ctx, commitTimeout)
	defer cancel()
	t, cs, err := writeTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return CommitReport{}, err
	}
	r := CommitReport{Alias: t.alias, Warnings: []string{}}
	fail := func(err error) (CommitReport, error) { r.Err = err; return r, nil }
	refuse := func(code domain.ErrCode, subject string) (CommitReport, error) {
		return fail(domain.NewOpError(op, code, subject, "", nil))
	}
	if subject, _, _ := strings.Cut(msg, "\n"); utf8.RuneCountInString(subject) > SubjectLimit {
		r.Warnings = append(r.Warnings, WarningSubjectTooLong)
	}

	if _, detached, err := deps.Git.CurrentBranch(ctx, t.worktree); err != nil {
		return fail(err)
	} else if detached {
		return refuse(domain.CodeDetachedHead, t.alias)
	}
	if inProgress, err := deps.Git.IntegrationInProgress(ctx, t.worktree); err != nil {
		return fail(err)
	} else if inProgress != "" {
		return refuse(domain.CodeIntegrationInProgress, t.alias)
	}
	if len(cs.Conflicted) > 0 {
		r.Files = entryPaths(cs.Conflicted)
		return refuse(domain.CodeWorktreeDirty, t.alias)
	}
	if len(cs.Staged) == 0 {
		return refuse(domain.CodeNothingStaged, t.alias)
	}
	if ok, err := deps.Git.IdentityConfigured(ctx, t.worktree); err != nil {
		return fail(err)
	} else if !ok {
		r.IdentityCommands = append([]string{}, IdentityCommands...)
		return refuse(domain.CodeIdentityMissing, t.alias)
	}

	res, err := deps.Git.Commit(ctx, t.worktree, msg)
	if err != nil {
		return fail(err)
	}
	switch res.Outcome {
	case ports.CommitCreated:
	case ports.CommitIdentityMissing:
		r.IdentityCommands = append([]string{}, IdentityCommands...)
		return refuse(domain.CodeIdentityMissing, t.alias)
	default:
		r.Output = res.Output
		return refuse(domain.CodeHookFailed, t.alias)
	}
	log, err := deps.Git.CommitLog(ctx, t.worktree, "HEAD", 0, 1)
	if err != nil {
		return fail(err)
	}
	if len(log) > 0 {
		r.Commit = log[0]
	}
	return r, nil
}

// PushInput asks to push the repo's current branch. SetUpstream allows
// publishing a branch that has no upstream yet.
type PushInput struct {
	RepoRefInput
	SetUpstream bool
}

// PushReport is PushRepo's outcome. Err is nil on success (UpToDate when
// there was nothing to push) and a *domain.OpError otherwise:
// CodeDetachedHead, CodeIntegrationInProgress, CodeNoUpstream (Remote is
// where SetUpstream would publish), CodePushRejected, CodeAuthFailed or a
// git failure (Output holds git's message for the last three). A refusal
// never changes the remote.
type PushReport struct {
	Alias       string
	Branch      string
	Remote      string
	Upstream    string
	SetUpstream bool
	UpToDate    bool
	Pushed      int
	Output      string
	Err         error
}

// PushRepo pushes the current branch to its upstream, or, with
// SetUpstream and no usable upstream (none, gone, a local branch, or a
// remote branch with another name such as the base it was created from),
// publishes it to the context's remote under the same name and records
// that as its upstream. It never forces: a remote with commits the
// branch lacks is refused (CodePushRejected; pull or update first). Only
// an unknown workspace or alias is returned as an error.
func PushRepo(ctx context.Context, deps Deps, in PushInput) (PushReport, error) {
	ctx, cancel := context.WithTimeout(ctx, networkTimeout)
	defer cancel()
	t, err := resolveInspectTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return PushReport{}, err
	}
	reportRepo(deps.Reporter, ports.RepoEvent{Op: OpPush, Repo: t.alias, Phase: ports.RepoStarted})
	r := pushOne(ctx, deps, t, in.SetUpstream)
	if r.Err != nil {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpPush, Repo: t.alias, Phase: ports.RepoFailed, Err: r.Err})
	} else {
		reportRepo(deps.Reporter, ports.RepoEvent{Op: OpPush, Repo: t.alias, Phase: ports.RepoFinished})
	}
	return r, nil
}

func pushOne(ctx context.Context, deps Deps, t inspectTarget, setUpstream bool) PushReport {
	const op = "repo.push"
	r := PushReport{Alias: t.alias, Remote: t.remote}
	fail := func(err error) PushReport { r.Err = err; return r }
	refuse := func(code domain.ErrCode, subject string) PushReport {
		return fail(domain.NewOpError(op, code, subject, "", nil))
	}
	branch, detached, err := deps.Git.CurrentBranch(ctx, t.worktree)
	if err != nil {
		return fail(err)
	}
	if detached || branch == "" {
		return refuse(domain.CodeDetachedHead, t.alias)
	}
	r.Branch = string(branch)
	if inProgress, err := deps.Git.IntegrationInProgress(ctx, t.worktree); err != nil {
		return fail(err)
	} else if inProgress != "" {
		return refuse(domain.CodeIntegrationInProgress, t.alias)
	}
	up, ok, err := deps.Git.Upstream(ctx, t.worktree)
	if err != nil {
		return fail(err)
	}
	src := "refs/heads/" + r.Branch
	spec := ports.PushSpec{Remote: t.remote, Refspec: src + ":" + src, SetUpstream: true}
	if pushableUpstream(up, ok, src) {
		r.Remote, r.Upstream = up.Remote, up.Ref
		spec = ports.PushSpec{Remote: up.Remote, Refspec: src + ":" + src}
		ahead, _, err := deps.Git.AheadBehind(ctx, t.worktree, "@{upstream}")
		if err != nil {
			return fail(err)
		}
		if ahead == 0 {
			r.UpToDate = true
			return r
		}
		r.Pushed = ahead
	} else if !setUpstream {
		return refuse(domain.CodeNoUpstream, t.alias)
	} else {
		r.SetUpstream = true
	}

	res, err := deps.Git.Push(ctx, t.worktree, spec)
	if err != nil {
		return fail(err)
	}
	switch res.Outcome {
	case ports.PushDone:
	case ports.PushRejected:
		r.Output = res.Output
		return refuse(domain.CodePushRejected, r.Upstream)
	case ports.PushAuthFailed:
		r.Output = res.Output
		return refuse(domain.CodeAuthFailed, r.Remote)
	default:
		r.Output = res.Output
		return refuse(domain.CodeGitFailed, r.Remote)
	}
	if r.SetUpstream {
		if up, ok, err := deps.Git.Upstream(ctx, t.worktree); err == nil && ok {
			r.Upstream = up.Ref
		} else {
			r.Upstream = r.Remote + "/" + r.Branch
		}
	}
	return r
}

// pushableUpstream reports whether the branch (src, refs/heads/<name>)
// may be pushed to its upstream: a remote branch that exists and has the
// same name (git's push.default=simple rule). A branch created from
// origin/develop tracks origin/develop; pushing there would put its
// commits on the base branch, so it counts as having no upstream of its
// own and must be published.
func pushableUpstream(up domain.UpstreamInfo, ok bool, src string) bool {
	if !ok || up.Gone || up.Remote == "" || up.Remote == "." {
		return false
	}
	dst := up.RemoteRef
	if dst == "" {
		dst = "refs/heads/" + strings.TrimPrefix(up.Ref, up.Remote+"/")
	}
	return dst == src
}

// StashCreateInput is a new stash entry's options.
type StashCreateInput struct {
	RepoRefInput
	Message          string
	IncludeUntracked bool
	KeepIndex        bool
}

// CreateStash saves the local changes (untracked files too when
// IncludeUntracked; staged changes also kept in place when KeepIndex) as
// a new stash entry and returns it. Nothing to save is
// CodeNothingToStash; unresolved conflicts or a merge/rebase in progress
// are refused, changing nothing.
func CreateStash(ctx context.Context, deps Deps, in StashCreateInput) (domain.StashEntry, error) {
	const op = "repo.stash_create"
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	t, cs, err := writeTarget(ctx, deps, in.RepoRefInput)
	if err != nil {
		return domain.StashEntry{}, err
	}
	if inProgress, err := deps.Git.IntegrationInProgress(ctx, t.worktree); err != nil {
		return domain.StashEntry{}, err
	} else if inProgress != "" {
		return domain.StashEntry{}, domain.NewOpError(op, domain.CodeIntegrationInProgress, t.alias, "", nil)
	}
	if len(cs.Conflicted) > 0 {
		return domain.StashEntry{}, domain.NewOpError(op, domain.CodeWorktreeDirty, t.alias, "", nil)
	}
	if len(cs.Staged)+len(cs.Unstaged) == 0 && (!in.IncludeUntracked || len(cs.Untracked) == 0) {
		return domain.StashEntry{}, domain.NewOpError(op, domain.CodeNothingToStash, t.alias, "", nil)
	}
	before, err := deps.Git.StashRef(ctx, t.worktree)
	if err != nil {
		return domain.StashEntry{}, err
	}
	spec := ports.StashPushSpec{Message: strings.TrimSpace(in.Message), IncludeUntracked: in.IncludeUntracked, KeepIndex: in.KeepIndex}
	if err := deps.Git.StashPush(ctx, t.worktree, spec); err != nil {
		return domain.StashEntry{}, err
	}
	after, err := deps.Git.StashRef(ctx, t.worktree)
	if err != nil {
		return domain.StashEntry{}, err
	}
	if after == "" || after == before {
		return domain.StashEntry{}, domain.NewOpError(op, domain.CodeNothingToStash, t.alias, "", nil)
	}
	list, err := deps.Git.StashList(ctx, t.worktree)
	if err != nil {
		return domain.StashEntry{}, err
	}
	for _, s := range list {
		if s.Hash == after {
			return s, nil
		}
	}
	return domain.StashEntry{Index: 0, Ref: "stash@{0}", Hash: after, Message: spec.Message}, nil
}
