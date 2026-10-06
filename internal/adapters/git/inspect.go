// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// diffFlags make every inspector diff deterministic and safe to parse:
// no color, fixed a/ b/ prefixes (whatever diff.noprefix or
// diff.mnemonicPrefix say), rename detection, and neither external diff
// drivers nor textconv filters. Both of those run user-configured
// programs, which a read-only query must never do; binary files are
// reported as binary instead.
var diffFlags = append(diffContentFlags, "--src-prefix=a/", "--dst-prefix=b/")

// diffContentFlags are diffFlags without the prefix options.
var diffContentFlags = []string{"--no-color", "--no-ext-diff", "--no-textconv", "-M"}

// defaultPrefixConfig forces the a/ b/ prefixes through configuration
// instead of --src-prefix/--dst-prefix. `git stash show` in git 2.55
// prints garbage bytes in place of prefixes passed as options, so stash
// diffs use this instead; it overrides diff.noprefix and
// diff.mnemonicPrefix from the user's configuration just the same.
var defaultPrefixConfig = []string{"-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false"}

// literal turns a worktree-relative path into a pathspec git matches
// literally (no glob or magic).
func literal(p string) string { return ":(literal)" + p }

// safeRelPath rejects anything but a plain worktree-relative path.
func safeRelPath(op string, p string) error {
	clean := filepath.ToSlash(filepath.Clean(p))
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return domain.NewOpError(op, domain.CodePathNotChanged, p, "path is empty, absolute or escapes the worktree", nil)
	}
	return nil
}

// execCapped runs git like exec but keeps at most maxBytes of stdout:
// past that the process is stopped and truncated is true (its exit
// status is then meaningless and ignored).
func (a *Adapter) execCapped(ctx context.Context, argv []string, maxBytes int) (inv invocation, truncated bool, err error) {
	if a.gitPath == "" {
		return invocation{}, false, errGitMissing()
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, a.gitPath, argv...)
	cmd.Env = buildEnv()
	cmd.WaitDelay = waitDelay
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return invocation{}, false, err
	}
	if err := cmd.Start(); err != nil {
		return invocation{}, false, err
	}
	buf, readErr := io.ReadAll(io.LimitReader(stdout, int64(maxBytes)+1))
	if len(buf) > maxBytes {
		truncated = true
		buf = buf[:maxBytes]
		cancel()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return invocation{}, false, ctx.Err()
	}
	if truncated {
		return invocation{stdout: string(buf), stderr: stderr.String()}, true, nil
	}
	if readErr != nil {
		return invocation{}, false, readErr
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return invocation{stdout: string(buf), stderr: stderr.String(), exitCode: exitErr.ExitCode()}, false, nil
		}
		return invocation{}, false, waitErr
	}
	return invocation{stdout: string(buf), stderr: stderr.String()}, false, nil
}

// runPatch runs a diff-producing command under limits and parses it. Any
// exit code in okExitCodes is success (diff --no-index exits 1 when the
// files differ).
func (a *Adapter) runPatch(ctx context.Context, op string, repo domain.Path, limits domain.DiffLimits, okExitCodes []int, args ...string) (domain.Patch, error) {
	if err := requireAbsRepo(op, repo); err != nil {
		return domain.Patch{}, err
	}
	if limits.MaxBytes <= 0 {
		limits = domain.DefaultDiffLimits
	}
	inv, truncated, err := a.execCapped(ctx, argv(repo, args...), limits.MaxBytes)
	if err != nil {
		return domain.Patch{}, domain.NewOpError(op, classifyExecErr(ctx, err), string(repo), err.Error(), err)
	}
	if !truncated {
		ok := false
		for _, c := range okExitCodes {
			ok = ok || inv.exitCode == c
		}
		if !ok {
			return domain.Patch{}, domain.NewOpError(op, classifyError(args, inv.exitCode, inv.stderr), string(repo), inv.stderr, nil)
		}
	}
	return parsePatch(inv.stdout, truncated, limits), nil
}

// Diff runs `diff --cached` (staged), `diff` (unstaged) or, for an
// untracked file, `diff --no-index -- /dev/null <path>`, always with
// diffFlags and literal pathspecs.
func (a *Adapter) Diff(ctx context.Context, worktree domain.Path, spec ports.DiffSpec) (domain.Patch, error) {
	const op = "git.diff"
	if err := safeRelPath(op, spec.Path); err != nil {
		return domain.Patch{}, err
	}
	if spec.Untracked {
		args := append(append([]string{"diff", "--no-index"}, diffFlags...), "--", "/dev/null", spec.Path)
		return a.runPatch(ctx, op, worktree, spec.Limits, []int{0, 1}, args...)
	}
	args := []string{"diff"}
	if spec.Staged {
		args = append(args, "--cached")
	}
	args = append(append(args, diffFlags...), "--", literal(spec.Path))
	if spec.OrigPath != "" {
		if err := safeRelPath(op, spec.OrigPath); err != nil {
			return domain.Patch{}, err
		}
		args = append(args, literal(spec.OrigPath))
	}
	return a.runPatch(ctx, op, worktree, spec.Limits, []int{0}, args...)
}

// CommitLog runs `log -z --no-show-signature --format=<commitLogFormat>
// --skip=<skip> --max-count=<max> <range>`.
func (a *Adapter) CommitLog(ctx context.Context, worktree domain.Path, revRange string, skip, max int) ([]domain.CommitInfo, error) {
	const op = "git.commit_log"
	if err := rejectOptionLike(op, worktree, revRange); err != nil {
		return nil, err
	}
	out, err := a.run(ctx, op, worktree, "log", "-z", "--no-show-signature", "--no-color", "--format="+commitLogFormat,
		"--skip="+strconv.Itoa(skip), "--max-count="+strconv.Itoa(max), revRange, "--")
	if err != nil {
		return nil, err
	}
	list, perr := parseCommitLog(out)
	if perr != nil {
		return nil, domain.NewOpError(op, domain.CodeGitFailed, string(worktree), perr.Error(), perr)
	}
	return list, nil
}

// CountCommits runs `rev-list --count <range>`.
func (a *Adapter) CountCommits(ctx context.Context, worktree domain.Path, revRange string) (int, error) {
	const op = "git.count_commits"
	if err := rejectOptionLike(op, worktree, revRange); err != nil {
		return 0, err
	}
	out, err := a.run(ctx, op, worktree, "rev-list", "--count", revRange, "--")
	if err != nil {
		return 0, err
	}
	n, perr := strconv.Atoi(strings.TrimSpace(out))
	if perr != nil {
		return 0, domain.NewOpError(op, domain.CodeGitFailed, string(worktree), perr.Error(), perr)
	}
	return n, nil
}

// CommitDetail resolves hash with `rev-parse --verify --quiet
// <hash>^{commit}`, reads it with `show -s --format=...` (author name,
// never the e-mail) and diffs it against its first parent (`diff <p1>
// <hash>`), or with `diff-tree --root` for a root commit.
func (a *Adapter) CommitDetail(ctx context.Context, worktree domain.Path, hash string, limits domain.DiffLimits) (domain.CommitDetail, error) {
	const op = "git.commit_detail"
	if !domain.ValidCommitHash(hash) {
		return domain.CommitDetail{}, domain.NewOpError(op, domain.CodeRefNotFound, hash, "not a hexadecimal object name", nil)
	}
	code, out, _, err := a.runExpecting(ctx, op, worktree, []int{0, 1}, "rev-parse", "--verify", "--quiet", hash+"^{commit}")
	if err != nil {
		return domain.CommitDetail{}, err
	}
	if code != 0 {
		return domain.CommitDetail{}, domain.NewOpError(op, domain.CodeRefNotFound, hash, "", nil)
	}
	full := strings.TrimSpace(out)

	meta, err := a.run(ctx, op, worktree, "show", "-s", "--no-show-signature", "--no-color", "--format=%H%x1f%h%x1f%an%x1f%aI%x1f%P%x1f%B", full, "--")
	if err != nil {
		return domain.CommitDetail{}, err
	}
	f := strings.SplitN(strings.TrimSuffix(meta, "\n"), "\x1f", 6)
	if len(f) != 6 {
		return domain.CommitDetail{}, domain.NewOpError(op, domain.CodeGitFailed, hash, "unexpected show output", nil)
	}
	date, perr := time.Parse(time.RFC3339, f[3])
	if perr != nil {
		return domain.CommitDetail{}, domain.NewOpError(op, domain.CodeGitFailed, hash, perr.Error(), perr)
	}
	d := domain.CommitDetail{
		CommitInfo: domain.CommitInfo{Hash: f[0], ShortHash: f[1], AuthorName: f[2], AuthorDate: date},
		Parents:    strings.Fields(f[4]),
	}
	msg := strings.TrimRight(f[5], "\n")
	d.Subject, d.Body, _ = strings.Cut(msg, "\n")
	d.Body = strings.TrimLeft(d.Body, "\n")
	if d.Parents == nil {
		d.Parents = []string{}
	}

	var args []string
	if len(d.Parents) == 0 {
		args = append(append([]string{"diff-tree", "-p", "-r", "--root", "--no-commit-id"}, diffFlags...), full, "--")
	} else {
		args = append(append([]string{"diff"}, diffFlags...), d.Parents[0], full, "--")
	}
	if d.Patch, err = a.runPatch(ctx, op, worktree, limits, []int{0}, args...); err != nil {
		return domain.CommitDetail{}, err
	}
	return d, nil
}

// StashList runs `stash list -z --format=<stashListFormat>`.
func (a *Adapter) StashList(ctx context.Context, worktree domain.Path) ([]domain.StashEntry, error) {
	const op = "git.stash_list"
	out, err := a.run(ctx, op, worktree, "stash", "list", "-z", "--no-color", "--format="+stashListFormat)
	if err != nil {
		return nil, err
	}
	list, perr := parseStashList(out)
	if perr != nil {
		return nil, domain.NewOpError(op, domain.CodeGitFailed, string(worktree), perr.Error(), perr)
	}
	return list, nil
}

// StashShow runs `<defaultPrefixConfig> stash show -p [--include-untracked]
// <diffContentFlags> stash@{<index>}`.
func (a *Adapter) StashShow(ctx context.Context, worktree domain.Path, index int, includeUntracked bool, limits domain.DiffLimits) (domain.Patch, error) {
	const op = "git.stash_show"
	if index < 0 {
		return domain.Patch{}, domain.NewOpError(op, domain.CodeRefNotFound, strconv.Itoa(index), "", nil)
	}
	args := append(append([]string{}, defaultPrefixConfig...), "stash", "show", "-p")
	if includeUntracked {
		args = append(args, "--include-untracked")
	}
	args = append(append(args, diffContentFlags...), "stash@{"+strconv.Itoa(index)+"}")
	return a.runPatch(ctx, op, worktree, limits, []int{0}, args...)
}

// Upstream runs `symbolic-ref -q HEAD` (exit 1: detached) and then
// `for-each-ref --format=<upstreamFormat> <branch ref>`.
func (a *Adapter) Upstream(ctx context.Context, worktree domain.Path) (domain.UpstreamInfo, bool, error) {
	const op = "git.upstream"
	code, head, _, err := a.runExpecting(ctx, op, worktree, []int{0, 1}, "symbolic-ref", "-q", "HEAD")
	if err != nil || code != 0 {
		return domain.UpstreamInfo{}, false, err
	}
	ref := strings.TrimSpace(head)
	if err := rejectOptionLike(op, worktree, ref); err != nil {
		return domain.UpstreamInfo{}, false, err
	}
	out, err := a.run(ctx, op, worktree, "for-each-ref", "--format="+upstreamFormat, ref)
	if err != nil {
		return domain.UpstreamInfo{}, false, err
	}
	u, ok := parseUpstream(out)
	return u, ok, nil
}

// LastFetch stats FETCH_HEAD in the worktree's git dir and in the common
// (main clone) git dir, located with `rev-parse --git-path FETCH_HEAD
// --git-common-dir`, and returns the newer modification time.
func (a *Adapter) LastFetch(ctx context.Context, worktree domain.Path) (time.Time, bool, error) {
	out, err := a.run(ctx, "git.last_fetch", worktree, "rev-parse", "--git-path", "FETCH_HEAD", "--git-common-dir")
	if err != nil {
		return time.Time{}, false, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	abs := func(p string) string {
		p = strings.TrimSpace(p)
		if !filepath.IsAbs(p) {
			p = filepath.Join(string(worktree), p)
		}
		return p
	}
	var candidates []string
	if len(lines) > 0 && lines[0] != "" {
		candidates = append(candidates, abs(lines[0]))
	}
	if len(lines) > 1 && lines[1] != "" {
		candidates = append(candidates, filepath.Join(abs(lines[1]), "FETCH_HEAD"))
	}
	var newest time.Time
	for _, c := range candidates {
		if info, statErr := os.Stat(c); statErr == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest, !newest.IsZero(), nil
}

// MergeFastForward runs `merge --ff-only <ref>` and maps git's refusals:
// local changes or untracked files that would be overwritten
// (CodeWorktreeDirty, with the paths git listed) and a non-fast-forward
// (CodeDiverged). git changes nothing in either case.
func (a *Adapter) MergeFastForward(ctx context.Context, worktree domain.Path, ref string) ([]string, error) {
	const op = "git.merge_ff_only"
	if err := rejectOptionLike(op, worktree, ref); err != nil {
		return nil, err
	}
	code, _, stderr, err := a.runExpecting(ctx, op, worktree, []int{0, 1, 128}, "merge", "--ff-only", "--no-edit", ref)
	if err != nil {
		return nil, err
	}
	if code == 0 {
		return nil, nil
	}
	if files := parseOverwrittenFiles(stderr); len(files) > 0 {
		return files, domain.NewOpError(op, domain.CodeWorktreeDirty, string(worktree), stderr, nil)
	}
	if strings.Contains(stderr, "Not possible to fast-forward") {
		return nil, domain.NewOpError(op, domain.CodeDiverged, ref, stderr, nil)
	}
	return nil, domain.NewOpError(op, classifyError([]string{"merge"}, code, stderr), string(worktree), stderr, nil)
}
