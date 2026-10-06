// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest_test

import (
	"context"
	"io/fs"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

var (
	_ ports.FileSystemPort = (*portstest.FakeFS)(nil)
	_ ports.ConfigStore    = (*portstest.FakeConfigStore)(nil)
	_ ports.ReleaseChecker = (*portstest.FakeReleaseChecker)(nil)
	_ ports.Reporter       = (*portstest.RecordingReporter)(nil)
)

func TestFakeFS_WriteThenReadRoundTrips(t *testing.T) {
	ffs := portstest.NewFakeFS(t)

	target := ffs.Paths().Config.Join("config.yaml")
	if err := ffs.WriteFile(target, []byte("hello"), fs.FileMode(0o644)); err != nil {
		t.Fatalf("WriteFile() unexpected error: %v", err)
	}

	got, err := ffs.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() unexpected error: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("ReadFile() = %q, want %q", got, "hello")
	}

	exists, err := ffs.Exists(target)
	if err != nil || !exists {
		t.Fatalf("Exists() = (%v, %v), want (true, nil)", exists, err)
	}
}

func TestFakeFS_PathsAreTempDirBased(t *testing.T) {
	ffs := portstest.NewFakeFS(t)
	p := ffs.Paths()

	if p.Config == "" || p.Cache == "" || p.Home == "" {
		t.Fatalf("Paths() = %+v, want every field set", p)
	}
}

// TestFakeFS_ListDirs_NeverTouchedRootIsEmptyNotError is CRITICAL-3's own
// fake-fidelity regression (verify-report.md): a root nobody ever called
// MkdirAll on (or under) — the state of a brand-new context before its
// first workspace — must report an empty list with no error, exactly like
// the real, fixed fsstore.Adapter.ListDirs does for a directory that does
// not exist yet on disk. This must hold with zero setup beyond
// NewFakeFS(t) — no other directory anywhere under the never-touched root
// may exist for this to pass.
func TestFakeFS_ListDirs_NeverTouchedRootIsEmptyNotError(t *testing.T) {
	ffs := portstest.NewFakeFS(t)
	root := ffs.Paths().Home.Join("workspaces")

	got, err := ffs.ListDirs(root)
	if err != nil {
		t.Fatalf("ListDirs(never-touched root) = %v, want nil error", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListDirs(never-touched root) = %v, want empty", got)
	}
}

// TestFakeFS_MkdirAll_RegistersEveryAncestor covers the same ancestor
// bookkeeping a real os.MkdirAll performs (it creates every missing path
// component, not only the leaf) — without it, ListDirs on a directory's
// parent could not distinguish "genuinely never created" from "created,
// just via a deeper descendant", which is exactly the distinction
// CRITICAL-3 needed.
func TestFakeFS_MkdirAll_RegistersEveryAncestor(t *testing.T) {
	ffs := portstest.NewFakeFS(t)
	root := ffs.Paths().Home.Join("workspaces")

	if err := ffs.MkdirAll(root.Join("ws1")); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	got, err := ffs.ListDirs(root)
	if err != nil {
		t.Fatalf("ListDirs(root) = %v, want nil error (root is an implicit ancestor of a created dir)", err)
	}
	if len(got) != 1 || got[0] != "ws1" {
		t.Fatalf("ListDirs(root) = %v, want [ws1]", got)
	}
}

func TestFakeConfigStore_RootConfigRoundTrip(t *testing.T) {
	cs := portstest.NewFakeConfigStore()
	ctx := context.Background()

	want := domain.RootConfig{SchemaVersion: 1, ActiveContext: "work"}
	if err := cs.SaveRoot(ctx, want); err != nil {
		t.Fatalf("SaveRoot() unexpected error: %v", err)
	}

	got, err := cs.LoadRoot(ctx)
	if err != nil {
		t.Fatalf("LoadRoot() unexpected error: %v", err)
	}
	if got.ActiveContext != want.ActiveContext {
		t.Fatalf("LoadRoot().ActiveContext = %q, want %q", got.ActiveContext, want.ActiveContext)
	}
}

func TestFakeConfigStore_InjectableLoadError(t *testing.T) {
	cs := portstest.NewFakeConfigStore()
	cs.LoadRootErr = context.DeadlineExceeded

	if _, err := cs.LoadRoot(context.Background()); err != context.DeadlineExceeded {
		t.Fatalf("LoadRoot() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestFakeReleaseChecker_FixedResponseAndCallCount(t *testing.T) {
	rc := portstest.NewFakeReleaseChecker()
	rc.Response = domain.ReleaseInfo{Tag: "v1.2.0"}

	got, err := rc.Latest(context.Background(), domain.RepoCoordinates{Owner: "me", Repo: "ws"})
	if err != nil {
		t.Fatalf("Latest() unexpected error: %v", err)
	}
	if got.Tag != "v1.2.0" {
		t.Fatalf("Latest().Tag = %q, want %q", got.Tag, "v1.2.0")
	}
	if rc.Calls != 1 {
		t.Fatalf("Calls = %d, want 1", rc.Calls)
	}
}

func TestRecordingReporter_CapturesStepInfoWarnAndResult(t *testing.T) {
	r := portstest.NewRecordingReporter()

	r.Step(messages.WorkspaceCreated, "payments-fix")
	r.Info(messages.WorkspaceCreated)
	r.Warn(messages.EnvCopyNotIgnored, "api/.env")
	r.Result("done")

	if len(r.Steps) != 1 || r.Steps[0].Key != messages.WorkspaceCreated {
		t.Fatalf("Steps = %+v, want one WorkspaceCreated entry", r.Steps)
	}
	if len(r.Infos) != 1 {
		t.Fatalf("Infos = %+v, want one entry", r.Infos)
	}
	if len(r.Warnings) != 1 || r.Warnings[0].Key != messages.EnvCopyNotIgnored {
		t.Fatalf("Warnings = %+v, want one EnvCopyNotIgnored entry", r.Warnings)
	}
	if len(r.Results) != 1 || r.Results[0] != "done" {
		t.Fatalf("Results = %+v, want [\"done\"]", r.Results)
	}
}

func TestFakeFS_ListFilesListsDirectChildFilesSorted(t *testing.T) {
	ffs := portstest.NewFakeFS(t)
	dir := ffs.Paths().Cache.Join("discarded")
	for _, p := range []domain.Path{dir.Join("b.patch"), dir.Join("a.patch"), dir.Join("sub", "c.patch")} {
		if err := ffs.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ffs.ListFiles(dir)
	if err != nil || len(got) != 2 || got[0] != "a.patch" || got[1] != "b.patch" {
		t.Fatalf("ListFiles = %v, %v", got, err)
	}
}
