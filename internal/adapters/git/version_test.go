// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/git"
	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
)

// TestGitAdapter_Version_ParsesAndEnforcesMinimum drives the real installed
// git and asserts its version parses and is at least the 2.20 floor
// (tasks.md 2.6). The "too old" side of the table is exercised via
// TestGitErrors_MapsEachSignal instead, since forcing an old git binary
// into the test environment is not practical.
func TestGitAdapter_Version_ParsesAndEnforcesMinimum(t *testing.T) {
	gitfix.RequireGit(t)

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	v, err := a.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.Major < 2 || (v.Major == 2 && v.Minor < 20) {
		t.Fatalf("expected the installed git to be >= 2.20, got %d.%d.%d", v.Major, v.Minor, v.Patch)
	}
	if domain.Code(err) != "" {
		t.Fatalf("expected no ErrCode for a supported version, got %q", domain.Code(err))
	}
}
