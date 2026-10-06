// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/adapters/configstore"
	"github.com/kivoradigital/wspace/internal/domain"
)

func sampleManifest() domain.Manifest {
	return domain.Manifest{
		SchemaVersion: 1,
		Workspace: domain.Workspace{
			Name:    "payments-fix",
			Context: "work",
			Branch:  "feature/payments-fix",
			Created: time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC),
			Repos: []domain.RepoEntry{
				{Alias: "api", Project: "api", SourceDir: "/Users/me/src/api", Branch: "feature/payments-fix"},
			},
		},
		EnvCopies: []string{"api/.env"},
	}
}

// TestConfigStore_Manifest_RoundTrip covers tasks.md 3.11: SaveManifest
// then LoadManifest returns an equal domain.Manifest, with Workspace.Root
// set to the caller-supplied wsRoot (not persisted in the YAML itself).
func TestConfigStore_Manifest_RoundTrip(t *testing.T) {
	wsRoot := domain.Path(filepath.ToSlash(t.TempDir()))
	store := configstore.New(domain.Path(filepath.ToSlash(t.TempDir())))
	ctx := context.Background()

	want := sampleManifest()
	if err := store.SaveManifest(ctx, wsRoot, want); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	got, err := store.LoadManifest(ctx, wsRoot)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if got.Workspace.Root != wsRoot {
		t.Fatalf("LoadManifest: Workspace.Root = %q, want %q (derived from wsRoot, not YAML)", got.Workspace.Root, wsRoot)
	}
	if got.Workspace.Name != want.Workspace.Name || got.Workspace.Branch != want.Workspace.Branch {
		t.Fatalf("LoadManifest = %+v, want %+v", got, want)
	}
	if len(got.Workspace.Repos) != 1 || got.Workspace.Repos[0].Alias != "api" {
		t.Fatalf("LoadManifest Repos = %+v, want one alias=api entry", got.Workspace.Repos)
	}
	if len(got.EnvCopies) != 1 || got.EnvCopies[0] != "api/.env" {
		t.Fatalf("LoadManifest EnvCopies = %v, want [api/.env]", got.EnvCopies)
	}
}

// TestConfigStore_LoadManifest_NotFound covers reading a workspace root
// that was never created.
func TestConfigStore_LoadManifest_NotFound(t *testing.T) {
	store := configstore.New(domain.Path(filepath.ToSlash(t.TempDir())))
	_, err := store.LoadManifest(context.Background(), domain.Path(filepath.ToSlash(t.TempDir())))
	if domain.Code(err) != domain.CodeWorkspaceNotFound {
		t.Fatalf("LoadManifest: Code(err) = %q, want %q (err=%v)", domain.Code(err), domain.CodeWorkspaceNotFound, err)
	}
}

var archiveFilePattern = regexp.MustCompile(`^payments-fix-\d{8}T\d{6}Z\.yaml$`)

// TestConfigStore_ArchiveManifest_WritesTimestampedFile covers tasks.md
// 3.11: destroy archives the manifest under "<workspaces_root>/.ws-archive/"
// as a timestamped file (design.md §5 tree).
func TestConfigStore_ArchiveManifest_WritesTimestampedFile(t *testing.T) {
	workspacesRoot := t.TempDir()
	wsRoot := domain.Path(filepath.ToSlash(filepath.Join(workspacesRoot, "payments-fix")))
	store := configstore.New(domain.Path(filepath.ToSlash(t.TempDir())))
	ctx := context.Background()

	if err := store.SaveManifest(ctx, wsRoot, sampleManifest()); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	if err := store.ArchiveManifest(ctx, wsRoot); err != nil {
		t.Fatalf("ArchiveManifest: %v", err)
	}

	archiveDir := filepath.Join(workspacesRoot, ".ws-archive")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("archive dir has %d entries, want 1: %v", len(entries), entries)
	}
	if !archiveFilePattern.MatchString(entries[0].Name()) {
		t.Fatalf("archive file name = %q, want to match %s", entries[0].Name(), archiveFilePattern)
	}
}

// TestConfigStore_FindWorkspaceRoot covers tasks.md 3.12's
// FindWorkspaceRoot: walking up from any directory inside a workspace
// finds the root containing ".wspace/workspace.yaml".
func TestConfigStore_FindWorkspaceRoot(t *testing.T) {
	wsRoot := t.TempDir()
	nested := filepath.Join(wsRoot, "api", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	store := configstore.New(domain.Path(filepath.ToSlash(t.TempDir())))
	ctx := context.Background()
	if err := store.SaveManifest(ctx, domain.Path(filepath.ToSlash(wsRoot)), sampleManifest()); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	found, ok, err := store.FindWorkspaceRoot(ctx, domain.Path(filepath.ToSlash(nested)))
	if err != nil {
		t.Fatalf("FindWorkspaceRoot: %v", err)
	}
	if !ok {
		t.Fatal("FindWorkspaceRoot: ok = false, want true")
	}
	want := domain.Path(filepath.ToSlash(wsRoot))
	if found != want {
		t.Fatalf("FindWorkspaceRoot = %q, want %q", found, want)
	}

	t.Run("outside any workspace", func(t *testing.T) {
		outside := t.TempDir()
		_, ok, err := store.FindWorkspaceRoot(ctx, domain.Path(filepath.ToSlash(outside)))
		if err != nil {
			t.Fatalf("FindWorkspaceRoot outside: %v", err)
		}
		if ok {
			t.Fatal("FindWorkspaceRoot outside any workspace: ok = true, want false")
		}
	})
}

// TestConfigStore_Manifest_PerRepoBaseBranch: a repo's own comparison base
// round-trips, and an unrecorded one is not written at all (so manifests
// that need no per-repo base stay byte-identical to before).
func TestConfigStore_Manifest_PerRepoBaseBranch(t *testing.T) {
	wsRoot := domain.Path(filepath.ToSlash(t.TempDir()))
	store := configstore.New(domain.Path(filepath.ToSlash(t.TempDir())))
	ctx := context.Background()

	m := sampleManifest()
	m.Workspace.Repos = append(m.Workspace.Repos, domain.RepoEntry{Alias: "hub", Project: "hub", SourceDir: "/Users/me/src/hub", Branch: "feature/payments-fix", BaseBranch: "master"})
	if err := store.SaveManifest(ctx, wsRoot, m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	got, err := store.LoadManifest(ctx, wsRoot)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if got.Workspace.Repos[0].BaseBranch != "" || got.Workspace.Repos[1].BaseBranch != "master" {
		t.Fatalf("Repos = %+v, want api unrecorded and hub=master", got.Workspace.Repos)
	}
	data, err := os.ReadFile(filepath.Join(string(wsRoot), ".wspace", "workspace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(regexp.MustCompile(`(?m)^\s+base_branch: master$`).FindAll(data, -1)); n != 1 {
		t.Fatalf("manifest has %d per-repo base_branch lines, want exactly 1:\n%s", n, data)
	}
}
