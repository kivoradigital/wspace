// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func TestDoctor_AssertsMinimumGitVersion(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	git.VersionFunc = func() (ports.Version, error) {
		return ports.Version{}, domain.NewOpError("git.version", domain.CodeGitTooOld, "2.10.0", "", nil)
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	result, err := app.Doctor(context.Background(), deps, app.DoctorInput{WorkspacesRoot: wsRoot.Join("..")})
	if err != nil {
		t.Fatalf("Doctor() unexpected error: %v", err)
	}
	if !result.GitTooOld {
		t.Fatal("result.GitTooOld = false, want true")
	}

	found := false
	for _, w := range reporter.Warnings {
		if w.Key == messages.GitVersionTooOld {
			found = true
		}
	}
	if !found {
		t.Fatalf("reporter.Warnings = %+v, want a GitVersionTooOld warning", reporter.Warnings)
	}
}

// TestDoctor_FreshContextWithNoWorkspacesRootYetSucceedsEmpty is
// CRITICAL-3's own regression test (verify-report.md), Doctor's
// counterpart to TestList_FreshContextWithNoWorkspacesRootYetSucceedsEmpty
// in status_list_test.go — see that test's doc comment for the full
// reasoning.
func TestDoctor_FreshContextWithNoWorkspacesRootYetSucceedsEmpty(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}

	workspacesRoot := fs.Paths().Home.Join("workspaces")
	result, err := app.Doctor(context.Background(), deps, app.DoctorInput{WorkspacesRoot: workspacesRoot})
	if err != nil {
		t.Fatalf("Doctor() on a never-created workspaces_root = %v, want nil error", err)
	}
	if result.PrunedWorktrees != 0 {
		t.Fatalf("Doctor() = %+v, want zero pruned worktrees", result)
	}
}

func TestDoctor_PrunesStaleWorktreeRegistrations(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	git.WorktreeListFunc = func(domain.Path) ([]ports.WorktreeRef, error) {
		return []ports.WorktreeRef{{Path: "/gone", Prunable: true}}, nil
	}
	var pruned bool
	git.WorktreePruneFunc = func(domain.Path) error {
		pruned = true
		return nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	if _, err := app.Doctor(context.Background(), deps, app.DoctorInput{WorkspacesRoot: wsRoot.Join("..")}); err != nil {
		t.Fatalf("Doctor() unexpected error: %v", err)
	}
	if !pruned {
		t.Fatal("expected Doctor to prune a stale worktree registration")
	}
}
