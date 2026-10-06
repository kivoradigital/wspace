// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package git shells out to the system `git` binary. It is the only
// subprocess boundary in the product (design.md §7, ADR D3). Every
// invocation uses a fixed `-C <repo>` prefix (never `cd`), and an explicit
// environment allowlist rather than the inherited process environment (ADR
// D13): inherited GIT_DIR/GIT_WORK_TREE would otherwise silently retarget
// every call.
package git

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Adapter implements ports.GitPort by shelling out to a resolved git binary.
type Adapter struct {
	gitPath string
}

// New resolves the git binary once via exec.LookPath and returns an Adapter
// bound to it. Without git on PATH it still returns an Adapter, so commands
// that never touch git (version, help, install) keep working; every git
// call then fails with git_missing.
func New() (*Adapter, error) {
	p, err := exec.LookPath("git")
	if err != nil {
		return &Adapter{}, nil
	}
	return &Adapter{gitPath: p}, nil
}

// errGitMissing is returned by every git call when New found no git.
func errGitMissing() error {
	return domain.NewOpError("git.lookup", domain.CodeGitMissing, "", "git was not found on PATH", nil)
}

var _ ports.GitPort = (*Adapter)(nil)

// allowlistedEnvVars are the only variables carried over from the calling
// process's environment into every git subprocess (design.md §7).
var allowlistedEnvVars = []string{
	"PATH",
	"HOME",
	"USERPROFILE",
	"SSH_AUTH_SOCK",
	"SSH_ASKPASS",
	"GIT_SSH_COMMAND",
	"SystemRoot",
}

// buildEnv constructs the subprocess environment from the allowlist plus a
// fixed set of forced values. It never reads os.Environ(), which is what
// keeps GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE and every other GIT_* out of
// the child process regardless of what the caller's shell exports.
func buildEnv() []string {
	env := make([]string, 0, len(allowlistedEnvVars)+5)
	for _, k := range allowlistedEnvVars {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return append(env,
		"LC_ALL=C",
		"LANG=C",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_ADVICE=0",
	)
}

// invocation is the raw result of running the git binary once: it never
// classifies exit codes into domain errors, because several commands (e.g.
// show-ref, check-ignore) use exit codes to carry a yes/no answer rather
// than success/failure.
type invocation struct {
	stdout   string
	stderr   string
	exitCode int
}

// exec runs the git binary with argv, using the allowlisted environment.
// The returned error is non-nil only for a genuine failure to run the
// command (a cancelled/expired context, or the process failing to start);
// a non-zero exit from a git command that did run is reported through
// invocation.exitCode, not through err.
func (a *Adapter) exec(ctx context.Context, argv []string) (invocation, error) {
	if a.gitPath == "" {
		return invocation{}, errGitMissing()
	}
	cmd := exec.CommandContext(ctx, a.gitPath, argv...)
	cmd.Env = buildEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return invocation{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitErr.ExitCode()}, nil
		}
		if ctx.Err() != nil {
			return invocation{}, ctx.Err()
		}
		return invocation{}, runErr
	}
	return invocation{stdout: stdout.String(), stderr: stderr.String(), exitCode: 0}, nil
}

// argv builds the fixed invocation shape for every command that operates on
// a repository: `-C <repo> --no-pager -c core.quotepath=false -c
// advice.detachedHead=false <args...>` (design.md §7). repo may be empty for
// commands that take no repository (only Version does).
func argv(repo domain.Path, args ...string) []string {
	out := make([]string, 0, len(args)+6)
	if repo != "" {
		out = append(out, "-C", string(repo))
	}
	out = append(out, "--no-pager", "-c", "core.quotepath=false", "-c", "advice.detachedHead=false")
	return append(out, args...)
}

// requireAbsRepo re-validates repo as an absolute, non-escaping path right
// before it reaches a subprocess. domain.Path carries no enforcement of its
// own once constructed, so every port boundary that receives one from a
// caller must not trust it blindly (design.md §13: "ref/path argument
// injection into git").
func requireAbsRepo(op string, repo domain.Path) error {
	if _, err := domain.NewPath(string(repo)); err != nil {
		return domain.NewOpError(op, domain.CodeGitFailed, string(repo), err.Error(), err)
	}
	return nil
}

// IsMainClone reports whether dir is a main clone rather than a linked
// worktree. It stats "<dir>/.git": a directory means a main clone; a file
// means a linked worktree, which is rejected (design.md §7, §13).
//
// It returns three distinct shapes callers must not conflate (this is the
// exact contract a scripted portstest.FakeGit must reproduce faithfully —
// see FakeGit.IsMainCloneFunc's own doc comment):
//
//   - (true, nil): dir is a main clone.
//   - (false, nil): dir has no ".git" entry at all — it is not a git
//     repository (an ordinary folder) or it is a bare repository. Neither
//     case is the "linked worktree" this method exists to reject, and
//     neither is an operational failure: a caller scanning a directory
//     tree full of ordinary, non-repo folders must be able to skip this
//     one candidate without treating it as an error (project-configuration
//     spec: "Discovery scan as wizard pre-fill only" — scanning must never
//     be the only way to register a project, which an error here would
//     defeat by aborting the whole scan).
//   - (false, err) with domain.Code(err) == domain.CodeNotAMainClone: dir's
//     ".git" is a file, not a directory — a genuine linked worktree, which
//     must be rejected, but only as this one candidate, not as a reason to
//     abort whatever operation is scanning multiple directories.
//   - (false, err) with any other code: a genuine operational failure (a
//     stat error that is not "does not exist", or a failed
//     "rev-parse --git-dir"), which callers may treat as fatal.
func (a *Adapter) IsMainClone(ctx context.Context, dir domain.Path) (bool, error) {
	const op = "git.is_main_clone"
	if err := requireAbsRepo(op, dir); err != nil {
		return false, err
	}

	gitEntry := dir.Join(".git")
	info, statErr := os.Stat(string(gitEntry))
	if statErr != nil {
		if os.IsNotExist(statErr) {
			// No ".git" entry at all: not a git repository, or a bare one.
			// Not an error — just "not a main clone".
			return false, nil
		}
		return false, domain.NewOpError(op, domain.CodeGitFailed, string(dir), statErr.Error(), statErr)
	}
	if !info.IsDir() {
		return false, domain.NewOpError(op, domain.CodeNotAMainClone, string(dir),
			"\".git\" is a file, not a directory: this is a linked worktree, not a main clone", nil)
	}

	if _, err := a.run(ctx, op, dir, "rev-parse", "--git-dir"); err != nil {
		return false, err
	}
	return true, nil
}
