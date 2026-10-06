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

func TestSyncEnv_RecopiesAndUpdatesEnvCopiesList(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()
	wsRoot, _ := seededWorkspace(t, fs, store)

	src := fs.Paths().Home.Join("src", "api", ".env")
	if err := fs.WriteFile(src, []byte("A=1"), 0o644); err != nil {
		t.Fatalf("seed source env: %v", err)
	}
	git.IsIgnoredFunc = func(domain.Path, string) (bool, error) { return true, nil }

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	result, err := app.SyncEnv(context.Background(), deps, app.SyncEnvInput{WorkspaceRoot: wsRoot})
	if err != nil {
		t.Fatalf("SyncEnv() unexpected error: %v", err)
	}
	if len(result.Copied) != 1 || result.Copied[0] != "api/.env" {
		t.Fatalf("result.Copied = %+v, want [api/.env]", result.Copied)
	}

	dst := wsRoot.Join("api", ".env")
	data, err := fs.ReadFile(dst)
	if err != nil {
		t.Fatalf("expected env file re-copied at %s: %v", dst, err)
	}
	if string(data) != "A=1" {
		t.Fatalf("re-copied env content = %q, want %q", data, "A=1")
	}

	m, err := store.LoadManifest(context.Background(), wsRoot)
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}
	if len(m.EnvCopies) != 1 || m.EnvCopies[0] != "api/.env" {
		t.Fatalf("manifest.EnvCopies = %+v, want [api/.env]", m.EnvCopies)
	}
}
