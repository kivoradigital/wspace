// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// The repository inspector's write actions. Every path reaches git as a
// literal pathspec after a "--", so a file named "*.go" or "-f" is only
// ever that file.

// outputLimit caps the git/hook output kept for a refusal.
const outputLimit = 4000

// literalPaths validates paths and returns them as literal pathspecs.
func literalPaths(op string, paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if err := safeRelPath(op, p); err != nil {
			return nil, err
		}
		out = append(out, literal(p))
	}
	return out, nil
}

// pathCommand runs `<args> -- :(literal)<path>...`; nothing for no paths.
func (a *Adapter) pathCommand(ctx context.Context, op string, worktree domain.Path, paths []string, args ...string) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}
	lit, err := literalPaths(op, paths)
	if err != nil {
		return "", err
	}
	return a.run(ctx, op, worktree, append(append(args, "--"), lit...)...)
}

// Stage runs `add -A -- <paths>`: -A stages deletions as well as
// modifications and new files.
func (a *Adapter) Stage(ctx context.Context, worktree domain.Path, paths []string) error {
	_, err := a.pathCommand(ctx, "git.stage", worktree, paths, "add", "-A")
	return err
}

// Unstage runs `restore --staged -- <paths>`, or, in a repository without
// a commit yet, `rm --cached -q -- <paths>` (there is no HEAD to restore
// from). Neither touches the worktree.
func (a *Adapter) Unstage(ctx context.Context, worktree domain.Path, paths []string, unborn bool) error {
	if unborn {
		_, err := a.pathCommand(ctx, "git.unstage", worktree, paths, "rm", "--cached", "-q")
		return err
	}
	_, err := a.pathCommand(ctx, "git.unstage", worktree, paths, "restore", "--staged")
	return err
}

// UnstagedPatch runs `diff --binary --full-index` (worktree against the
// index) with external diff drivers, textconv and color off, so the
// output is a patch `git apply` can restore exactly.
func (a *Adapter) UnstagedPatch(ctx context.Context, worktree domain.Path, paths []string) (string, error) {
	return a.pathCommand(ctx, "git.unstaged_patch", worktree, paths,
		"diff", "--binary", "--full-index", "--no-ext-diff", "--no-textconv", "--no-color")
}

// RestoreWorktree runs `restore --worktree -- <paths>`: the index is the
// source, so only unstaged changes are discarded.
func (a *Adapter) RestoreWorktree(ctx context.Context, worktree domain.Path, paths []string) error {
	_, err := a.pathCommand(ctx, "git.restore_worktree", worktree, paths, "restore", "--worktree")
	return err
}

// identityMissing reports git's "who are you" refusals.
func identityMissing(stderr string) bool {
	for _, m := range []string{"Please tell me who you are", "unable to auto-detect email address", "auto-detection is disabled", "empty ident name"} {
		if strings.Contains(stderr, m) {
			return true
		}
	}
	return false
}

// IdentityConfigured runs `var GIT_AUTHOR_IDENT` and `var
// GIT_COMMITTER_IDENT`; git refuses them exactly when a commit would be
// refused for a missing identity.
func (a *Adapter) IdentityConfigured(ctx context.Context, worktree domain.Path) (bool, error) {
	const op = "git.identity"
	for _, v := range []string{"GIT_AUTHOR_IDENT", "GIT_COMMITTER_IDENT"} {
		code, _, stderr, err := a.runExpecting(ctx, op, worktree, []int{0, 128}, "var", v)
		if err != nil {
			return false, err
		}
		if code != 0 {
			if identityMissing(stderr) {
				return false, nil
			}
			return false, domain.NewOpError(op, domain.CodeGitFailed, string(worktree), stderr, nil)
		}
	}
	return true, nil
}

// commitHooks are the hooks that can refuse a `git commit`.
var commitHooks = []string{"pre-commit", "prepare-commit-msg", "commit-msg"}

// hasCommitHook reports whether one of commitHooks is installed, located
// with `rev-parse --git-path hooks/<name>` (which honours core.hooksPath).
func (a *Adapter) hasCommitHook(ctx context.Context, worktree domain.Path) bool {
	args := []string{"rev-parse"}
	for _, h := range commitHooks {
		args = append(args, "--git-path", "hooks/"+h)
	}
	out, err := a.run(ctx, "git.hooks", worktree, args...)
	if err != nil {
		return false
	}
	for _, p := range strings.Split(strings.TrimSpace(out), "\n") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(string(worktree), p)
		}
		if hookInstalled(p, runtime.GOOS) {
			return true
		}
	}
	return false
}

// hookInstalled reports whether git would find a hook at p, following
// git's find_hook. On Unix that needs the executable bit. Windows has no
// executable bit: Git for Windows' access(X_OK) only checks that the file
// exists, and it also tries p + ".exe"; Go reports no executable bit for
// any Windows file, so checking it there would hide every hook.
func hookInstalled(p, goos string) bool {
	if goos != "windows" {
		info, err := os.Stat(p)
		return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
	}
	for _, c := range []string{p, p + ".exe"} {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// Commit runs `commit -q -m <message>` (hooks run; never --no-verify,
// never --amend). A refusal by a commit hook is told apart from other
// failures by an installed hook; its output is returned trimmed.
func (a *Adapter) Commit(ctx context.Context, worktree domain.Path, message string) (ports.CommitResult, error) {
	const op = "git.commit"
	if strings.TrimSpace(message) == "" {
		return ports.CommitResult{}, domain.NewOpError(op, domain.CodeGitFailed, string(worktree), "empty commit message", nil)
	}
	code, stdout, stderr, err := a.runExpecting(ctx, op, worktree, []int{0, 1, 128}, "commit", "-q", "-m", message)
	if err != nil {
		return ports.CommitResult{}, err
	}
	switch {
	case code == 0:
		return ports.CommitResult{Outcome: ports.CommitCreated}, nil
	case identityMissing(stderr):
		return ports.CommitResult{Outcome: ports.CommitIdentityMissing, Output: trimOutput(stderr)}, nil
	case a.hasCommitHook(ctx, worktree):
		return ports.CommitResult{Outcome: ports.CommitHookFailed, Output: trimOutput(joinOutput(stdout, stderr))}, nil
	}
	return ports.CommitResult{}, domain.NewOpError(op, domain.CodeGitFailed, string(worktree), stderr, nil)
}

// pushAuthMarkers are git's messages for a remote it could not reach or
// that refused the credentials (GIT_TERMINAL_PROMPT=0 turns a credential
// prompt into a failure).
var pushAuthMarkers = []string{
	"Authentication failed", "Permission denied", "could not read Username", "could not read Password",
	"terminal prompts disabled", "Could not read from remote repository", "Host key verification failed",
	"The requested URL returned error: 401", "The requested URL returned error: 403",
}

// Push runs `push [-u] <remote> <src>:<dst>`. It never passes --force or
// --force-with-lease, and refuses a refspec with a leading "+" (git's
// per-ref force), so a push can only ever fast-forward the remote.
func (a *Adapter) Push(ctx context.Context, worktree domain.Path, spec ports.PushSpec) (ports.PushResult, error) {
	const op = "git.push"
	if err := rejectOptionLike(op, worktree, spec.Remote); err != nil {
		return ports.PushResult{}, err
	}
	src, dst, ok := strings.Cut(spec.Refspec, ":")
	if !ok || !strings.HasPrefix(src, "refs/heads/") || !strings.HasPrefix(dst, "refs/heads/") {
		return ports.PushResult{}, domain.NewOpError(op, domain.CodeRefNotFound, spec.Refspec, "refspec must be refs/heads/<src>:refs/heads/<dst> (never forced)", nil)
	}
	args := []string{"push", "-q"}
	if spec.SetUpstream {
		args = append(args, "-u")
	}
	code, stdout, stderr, err := a.runExpecting(ctx, op, worktree, []int{0, 1, 128}, append(args, spec.Remote, spec.Refspec)...)
	if err != nil {
		return ports.PushResult{}, err
	}
	if code == 0 {
		return ports.PushResult{Outcome: ports.PushDone}, nil
	}
	out := trimOutput(joinOutput(stdout, stderr))
	switch {
	case strings.Contains(stderr, "[rejected]"):
		return ports.PushResult{Outcome: ports.PushRejected, Output: out}, nil
	}
	for _, m := range pushAuthMarkers {
		if strings.Contains(stderr, m) {
			return ports.PushResult{Outcome: ports.PushAuthFailed, Output: out}, nil
		}
	}
	return ports.PushResult{Outcome: ports.PushFailed, Output: out}, nil
}

// StashPush runs `stash push -q [-u] [--keep-index] [-m <message>]`.
func (a *Adapter) StashPush(ctx context.Context, worktree domain.Path, spec ports.StashPushSpec) error {
	args := []string{"stash", "push", "-q"}
	if spec.IncludeUntracked {
		args = append(args, "--include-untracked")
	}
	if spec.KeepIndex {
		args = append(args, "--keep-index")
	}
	if m := strings.TrimSpace(spec.Message); m != "" {
		args = append(args, "-m", m)
	}
	_, err := a.run(ctx, "git.stash_push", worktree, args...)
	return err
}

func joinOutput(stdout, stderr string) string {
	stdout, stderr = strings.TrimSpace(stdout), strings.TrimSpace(stderr)
	switch {
	case stdout == "":
		return stderr
	case stderr == "":
		return stdout
	}
	return stdout + "\n" + stderr
}

// urlCredentials matches the user[:password]@ part of a URL.
var urlCredentials = regexp.MustCompile(`(://)[^/@\s]+@`)

// trimOutput keeps the last outputLimit bytes of git's output (the end
// holds the reason) and masks credentials embedded in URLs.
func trimOutput(s string) string {
	s = urlCredentials.ReplaceAllString(strings.TrimSpace(s), "${1}***@")
	if len(s) > outputLimit {
		start := len(s) - outputLimit
		for start < len(s) && !utf8.RuneStart(s[start]) {
			start++
		}
		s = "…" + s[start:]
	}
	return s
}
