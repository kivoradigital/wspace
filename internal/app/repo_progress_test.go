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

// TestLongOperations_ReportStartedAndFinishedPerRepo covers the
// per-repository progress stream every long workspace operation offers a
// progress-aware Reporter (the rpc/mcp surfaces), beyond create/add.
func TestLongOperations_ReportStartedAndFinishedPerRepo(t *testing.T) {
	tests := []struct {
		name string
		op   string
		run  func(deps app.Deps, wsRoot string) error
	}{
		{
			name: "destroy",
			op:   app.OpDestroyWorkspace,
			run: func(deps app.Deps, wsRoot string) error {
				return app.DestroyWorkspace(context.Background(), deps, app.DestroyWorkspaceInput{WorkspaceRoot: domainPath(wsRoot)})
			},
		},
		{
			name: "remove repo",
			op:   app.OpRemoveRepo,
			run: func(deps app.Deps, wsRoot string) error {
				return app.RemoveRepo(context.Background(), deps, app.RemoveRepoInput{WorkspaceRoot: domainPath(wsRoot), Alias: "api"})
			},
		},
		{
			name: "repair",
			op:   app.OpRepair,
			run: func(deps app.Deps, wsRoot string) error {
				_, err := app.Repair(context.Background(), deps, app.RepairInput{WorkspaceRoot: domainPath(wsRoot)})
				return err
			},
		},
		{
			name: "sync env",
			op:   app.OpSyncEnv,
			run: func(deps app.Deps, wsRoot string) error {
				_, err := app.SyncEnv(context.Background(), deps, app.SyncEnvInput{WorkspaceRoot: domainPath(wsRoot)})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := portstest.NewFakeFS(t)
			store := portstest.NewFakeConfigStore()
			reporter := portstest.NewRecordingReporter()
			wsRoot, _ := seededWorkspace(t, fs, store)
			deps := app.Deps{Store: store, Git: portstest.NewFakeGit(), FS: fs, Reporter: reporter}

			if err := tt.run(deps, string(wsRoot)); err != nil {
				t.Fatalf("%s: unexpected error: %v", tt.name, err)
			}
			want := []ports.RepoPhase{ports.RepoStarted, ports.RepoFinished}
			if len(reporter.RepoEvents) != len(want) {
				t.Fatalf("RepoEvents = %+v, want started+finished for api", reporter.RepoEvents)
			}
			for i, ev := range reporter.RepoEvents {
				if ev.Op != tt.op || ev.Repo != "api" || ev.Phase != want[i] {
					t.Fatalf("RepoEvents[%d] = %+v, want {%s api %s}", i, ev, tt.op, want[i])
				}
			}
		})
	}
}

func domainPath(s string) domain.Path { return domain.Path(s) }
