// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func TestRepair_RecreatesMissingWorktreeFromManifest(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	// Seed a second, healthy repo whose worktree directory exists, so
	// Repair must not touch it.
	m, _ := store.LoadManifest(context.Background(), wsRoot)
	m.Workspace.Repos = append(m.Workspace.Repos, domain.RepoEntry{
		Alias: "web", Project: "web", SourceDir: fs.Paths().Home.Join("src", "web"), Branch: "feature-x",
	})
	store.PutManifest(wsRoot, m)
	if err := fs.MkdirAll(wsRoot.Join("web")); err != nil {
		t.Fatalf("seed healthy worktree: %v", err)
	}
	// "api"'s worktree directory (seededWorkspace only created wsRoot
	// itself, not wsRoot/api) is absent, simulating a manually deleted
	// worktree.

	var added []domain.Path
	git.WorktreeAddFunc = func(repo domain.Path, spec ports.WorktreeSpec) error {
		added = append(added, spec.Target)
		return nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	result, err := app.Repair(context.Background(), deps, app.RepairInput{WorkspaceRoot: wsRoot})
	if err != nil {
		t.Fatalf("Repair() unexpected error: %v", err)
	}

	if len(added) != 1 || added[0] != wsRoot.Join("api") {
		t.Fatalf("WorktreeAdd targets = %+v, want exactly [%s]", added, wsRoot.Join("api"))
	}
	if len(result.Recreated) != 1 || result.Recreated[0] != "api" {
		t.Fatalf("result.Recreated = %+v, want [api]", result.Recreated)
	}

	// CRITICAL-2 regression (verify-report.md): Repair must prune the
	// missing repo's stale worktree registration before recreating it —
	// real git refuses WorktreeAdd at a path whose registration still
	// exists after a plain `rm -rf` — and must never prune the healthy
	// "web" repo, which was never missing.
	var prunedRepos []domain.Path
	for _, c := range git.Calls {
		if c.Method == "WorktreePrune" {
			prunedRepos = append(prunedRepos, c.Repo)
		}
	}
	apiSourceDir := fs.Paths().Home.Join("src", "api")
	webSourceDir := fs.Paths().Home.Join("src", "web")
	if len(prunedRepos) != 1 || prunedRepos[0] != apiSourceDir {
		t.Fatalf("WorktreePrune calls = %+v, want exactly one for %q (never for the healthy %q)", prunedRepos, apiSourceDir, webSourceDir)
	}

	var pruneIdx, addIdx = -1, -1
	for i, c := range git.Calls {
		switch {
		case c.Method == "WorktreePrune" && pruneIdx == -1:
			pruneIdx = i
		case c.Method == "WorktreeAdd" && addIdx == -1:
			addIdx = i
		}
	}
	if pruneIdx == -1 || addIdx == -1 || pruneIdx > addIdx {
		t.Fatalf("WorktreePrune must run before WorktreeAdd; calls = %+v", git.Calls)
	}
}
