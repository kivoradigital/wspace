// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// minGitMajor/minGitMinor is the floor design.md §7 requires: 2.20, the
// first version with `worktree list --porcelain` and `worktree remove`.
const (
	minGitMajor = 2
	minGitMinor = 20
)

var versionPattern = regexp.MustCompile(`git version (\d+)\.(\d+)(?:\.(\d+))?`)

// Version runs the one command in the table that never takes `-C <repo>`:
// plain `git --version` (design.md §7).
func (a *Adapter) Version(ctx context.Context) (ports.Version, error) {
	const op = "git.version"

	inv, err := a.exec(ctx, []string{"--version"})
	if err != nil {
		return ports.Version{}, domain.NewOpError(op, classifyExecErr(ctx, err), "", err.Error(), err)
	}
	if inv.exitCode != 0 {
		return ports.Version{}, domain.NewOpError(op, domain.CodeGitFailed, "", inv.stderr, nil)
	}

	m := versionPattern.FindStringSubmatch(inv.stdout)
	if m == nil {
		return ports.Version{}, domain.NewOpError(op, domain.CodeGitFailed, "", "unparsable git --version output: "+inv.stdout, nil)
	}

	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch := 0
	if m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}
	v := ports.Version{Major: major, Minor: minor, Patch: patch}

	if major < minGitMajor || (major == minGitMajor && minor < minGitMinor) {
		subject := fmt.Sprintf("%d.%d.%d", major, minor, patch)
		return v, domain.NewOpError(op, domain.CodeGitTooOld, subject, inv.stdout, nil)
	}
	return v, nil
}
