// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/configstore"
	"github.com/kivoradigital/wspace/internal/domain"
)

func mustBranchName(t *testing.T, s string) domain.BranchName {
	t.Helper()
	bn, err := domain.NewBranchName(s)
	if err != nil {
		t.Fatalf("NewBranchName(%q): %v", s, err)
	}
	return bn
}

// TestConfigStore_Context_RoundTrip covers tasks.md 3.7: SaveContext then
// LoadContext returns an equal domain.Context, including project records
// and the pointer-typed Options fields (ADR D5).
func TestConfigStore_Context_RoundTrip(t *testing.T) {
	root := domain.Path(filepath.ToSlash(t.TempDir()))
	store := configstore.New(root)
	ctx := context.Background()

	baseBranch := mustBranchName(t, "main")
	copyEnv := false
	fetchBefore := true

	want := domain.Context{
		Name:           "work",
		WorkspacesRoot: domain.Path("/Users/me/workspaces"),
		ProjectsRoot:   domain.Path("/Users/me/src"),
		Defaults: domain.Options{
			BaseBranch:        &baseBranch,
			CopyEnv:           &copyEnv,
			FetchBeforeCreate: &fetchBefore,
			EnvPruneDirs:      []string{"node_modules", "vendor"},
		},
		IgnorePatterns:      []domain.Glob{"*.tmp", "archive-*"},
		ProjectScanMaxDepth: 8,
		Projects: []domain.Project{
			{
				Key:          "api",
				SourceDir:    domain.Path("/Users/me/src/api"),
				OriginBranch: nil,
				DestBranch:   "",
				WorktreeDir:  "",
			},
			{
				Key:         "web",
				SourceDir:   domain.Path("/Users/me/src/web"),
				WorktreeDir: "apps/{project}",
				Options: domain.Options{
					CopyEnv: &copyEnv,
				},
			},
		},
	}

	if err := store.SaveContext(ctx, want); err != nil {
		t.Fatalf("SaveContext: %v", err)
	}

	got, err := store.LoadContext(ctx, "work")
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}

	if got.Name != want.Name || got.WorkspacesRoot != want.WorkspacesRoot || got.ProjectsRoot != want.ProjectsRoot {
		t.Fatalf("LoadContext top-level fields = %+v, want %+v", got, want)
	}
	if len(got.Projects) != 2 {
		t.Fatalf("LoadContext: got %d projects, want 2", len(got.Projects))
	}
	if *got.Defaults.CopyEnv != false {
		t.Fatalf("LoadContext Defaults.CopyEnv = %v, want pointer to false (ADR D5)", got.Defaults.CopyEnv)
	}
	if got.Projects[1].Options.CopyEnv == nil || *got.Projects[1].Options.CopyEnv != false {
		t.Fatalf("LoadContext Projects[1].Options.CopyEnv = %v, want pointer to false", got.Projects[1].Options.CopyEnv)
	}
	if got.Projects[1].WorktreeDir != "apps/{project}" {
		t.Fatalf("LoadContext Projects[1].WorktreeDir = %q, want %q", got.Projects[1].WorktreeDir, "apps/{project}")
	}
}

// TestConfigStore_ListContexts covers tasks.md 3.7: ListContexts returns
// every saved context name, sorted, and an empty (not error) result before
// any context has ever been saved.
func TestConfigStore_ListContexts(t *testing.T) {
	root := domain.Path(filepath.ToSlash(t.TempDir()))
	store := configstore.New(root)
	ctx := context.Background()

	names, err := store.ListContexts(ctx)
	if err != nil {
		t.Fatalf("ListContexts before any context exists: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("ListContexts before any context exists = %v, want empty", names)
	}

	if err := store.SaveContext(ctx, domain.Context{Name: "oss", WorkspacesRoot: "/tmp/oss"}); err != nil {
		t.Fatalf("SaveContext(oss): %v", err)
	}
	if err := store.SaveContext(ctx, domain.Context{Name: "work", WorkspacesRoot: "/tmp/work"}); err != nil {
		t.Fatalf("SaveContext(work): %v", err)
	}

	names, err = store.ListContexts(ctx)
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	want := []domain.ContextName{"oss", "work"}
	if len(names) != len(want) || names[0] != want[0] || names[1] != want[1] {
		t.Fatalf("ListContexts = %v, want %v (sorted)", names, want)
	}
}

// TestConfigStore_LoadContext_NotFound covers the "context_not_found"
// error code for a name that was never saved (context-management spec
// underpins context lifecycle commands).
func TestConfigStore_LoadContext_NotFound(t *testing.T) {
	root := domain.Path(filepath.ToSlash(t.TempDir()))
	store := configstore.New(root)

	_, err := store.LoadContext(context.Background(), "ghost")
	if domain.Code(err) != domain.CodeContextNotFound {
		t.Fatalf("LoadContext(ghost): Code(err) = %q, want %q (err=%v)", domain.Code(err), domain.CodeContextNotFound, err)
	}
}

// TestConfigStore_DeleteContext covers tasks.md 3.8's DeleteContext.
func TestConfigStore_DeleteContext(t *testing.T) {
	root := domain.Path(filepath.ToSlash(t.TempDir()))
	store := configstore.New(root)
	ctx := context.Background()

	if err := store.SaveContext(ctx, domain.Context{Name: "staging", WorkspacesRoot: "/tmp/staging"}); err != nil {
		t.Fatalf("SaveContext: %v", err)
	}
	if err := store.DeleteContext(ctx, "staging"); err != nil {
		t.Fatalf("DeleteContext: %v", err)
	}
	if _, err := store.LoadContext(ctx, "staging"); domain.Code(err) != domain.CodeContextNotFound {
		t.Fatalf("LoadContext after delete: Code(err) = %q, want %q", domain.Code(err), domain.CodeContextNotFound)
	}
}
