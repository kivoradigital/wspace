// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func TestTeardownBlockers(t *testing.T) {
	tests := []struct {
		name      string
		alias     string
		status    []domain.PorcelainEntry
		unpushed  int
		wantRepos []string
	}{
		{name: "clean workspace has no blockers"},
		{name: "unpushed commits block", unpushed: 2, wantRepos: []string{"api"}},
		{name: "foreign file blocks", status: []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: "stray.txt"}}, wantRepos: []string{"api"}},
		{name: "own env copy never blocks", status: []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: ".env"}}},
		{name: "scoped to one alias", alias: "api", unpushed: 1, wantRepos: []string{"api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := portstest.NewFakeFS(t)
			store := portstest.NewFakeConfigStore()
			git := portstest.NewFakeGit()
			wsRoot, m := seededWorkspace(t, fs, store)
			m.EnvCopies = []string{"api/.env"}
			store.PutManifest(wsRoot, m)
			git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) { return tt.status, nil }
			git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return tt.unpushed, 0, nil }

			deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: portstest.NewRecordingReporter()}
			blockers, err := app.TeardownBlockers(context.Background(), deps, app.TeardownCheckInput{WorkspaceRoot: wsRoot, Alias: tt.alias})
			if err != nil {
				t.Fatalf("TeardownBlockers() error = %v", err)
			}
			if len(blockers) != len(tt.wantRepos) {
				t.Fatalf("blockers = %+v, want repos %v", blockers, tt.wantRepos)
			}
			for i, b := range blockers {
				if b.Alias != tt.wantRepos[i] {
					t.Fatalf("blockers[%d].Alias = %q, want %q", i, b.Alias, tt.wantRepos[i])
				}
				if b.SafeToRemove() {
					t.Fatalf("blockers[%d] is safe to remove; only unsafe repos belong in the result", i)
				}
			}
		})
	}
}

func TestTeardownBlockers_UnknownAliasIsNotFound(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	wsRoot, _ := seededWorkspace(t, fs, store)
	deps := app.Deps{Store: store, Git: portstest.NewFakeGit(), FS: fs, Reporter: portstest.NewRecordingReporter()}

	_, err := app.TeardownBlockers(context.Background(), deps, app.TeardownCheckInput{WorkspaceRoot: wsRoot, Alias: "nope"})
	if domain.Code(err) != domain.CodeWorkspaceNotFound {
		t.Fatalf("error = %v, want workspace_not_found", err)
	}
}
