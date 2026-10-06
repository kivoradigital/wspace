// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
)

// classifyError maps (git subcommand, exit code, stderr) to a domain.ErrCode
// per the exit-code table in design.md §7. It is the adapter's only
// English-text dependency, which is exactly why the adapter forces
// LC_ALL=C/LANG=C on every subprocess — without that, stderr matching here
// would be locale-dependent and non-deterministic.
func classifyError(gitArgs []string, exitCode int, stderr string) domain.ErrCode {
	sub, sub2 := subcommands(gitArgs)

	switch {
	case strings.Contains(stderr, "is already checked out at"),
		strings.Contains(stderr, "is already used by worktree at"):
		// Design.md §7 documents the marker as "is already checked out at";
		// the installed git (2.54.0) actually emits "is already used by
		// worktree at" for `worktree add`. Both are matched so the mapping
		// holds regardless of git's exact vintage (confirmed empirically —
		// see apply-progress.md phase 2 deviations).
		return domain.CodeBranchCheckedOut
	case sub == "worktree" && sub2 == "add" && strings.Contains(stderr, "already exists"):
		return domain.CodeWorktreeExists
	case sub == "worktree" && sub2 == "remove" && strings.Contains(stderr, "contains modified or untracked files"):
		return domain.CodeWorktreeDirty
	case sub == "worktree" && (strings.Contains(stderr, "is not a working tree") || strings.Contains(stderr, "No such file or directory")):
		return domain.CodeWorktreeMissing
	case (sub == "show-ref" || sub == "rev-list") && (exitCode >= 2 || strings.Contains(stderr, "unknown revision")):
		return domain.CodeRefNotFound
	default:
		return domain.CodeGitFailed
	}
}

// subcommands returns the git subcommand (args[0]) and, when present, its
// first sub-subcommand (args[1]) — e.g. ("worktree", "add") for
// ["worktree", "add", "-b", ...]. Leading `-c <key=value>` pairs are
// skipped.
func subcommands(args []string) (sub, sub2 string) {
	for len(args) > 1 && args[0] == "-c" {
		args = args[2:]
	}
	if len(args) > 0 {
		sub = args[0]
	}
	if len(args) > 1 {
		sub2 = args[1]
	}
	return sub, sub2
}
