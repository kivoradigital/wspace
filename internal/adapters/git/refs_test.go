// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/git"
	"github.com/kivoradigital/wspace/internal/adapters/git/gitfix"
	"github.com/kivoradigital/wspace/internal/domain"
)

// Fetching a remote the repository doesn't have (a local-only clone) is a
// distinct, recoverable condition, not a generic git failure.
func TestGitAdapter_Fetch_MissingRemoteIsRemoteMissing(t *testing.T) {
	gitfix.RequireGit(t)

	clone := gitfix.NewClone(t, gitfix.NewOrigin(t))
	gitfix.Git(t, clone, "remote", "remove", "origin")

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}
	err = a.Fetch(context.Background(), clone, "origin")
	if domain.Code(err) != domain.CodeRemoteMissing {
		t.Fatalf("Fetch() = %v (code %s), want %s", err, domain.Code(err), domain.CodeRemoteMissing)
	}
}

func TestGitAdapter_ResolveBase_PrefersRemoteOverLocal(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	// Diverge the local "main" from origin/main so the two are distinct
	// refs, then confirm origin/main is still preferred.
	gitfix.Commit(t, clone, "local-only.txt", "local change\n")

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	ref, err := a.ResolveBase(context.Background(), clone, "origin", "main")
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if !ref.Remote || ref.Ref != "origin/main" {
		t.Fatalf("expected {Ref: origin/main, Remote: true}, got %+v", ref)
	}
}

func TestGitAdapter_ResolveBase_FallsBackToHEAD(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	// Neither origin/nonexistent nor a local "nonexistent" branch exists.
	ref, err := a.ResolveBase(context.Background(), clone, "origin", "nonexistent")
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if ref.Remote || ref.Ref != "HEAD" {
		t.Fatalf("expected {Ref: HEAD, Remote: false}, got %+v", ref)
	}
}

func TestGitAdapter_SyncLocalBase_SkipsWhenDirty(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	// clone's "main" (checked out here, in the main clone) becomes dirty.
	gitfix.Dirty(t, clone, "README.md", "dirty, uncommitted\n")

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	synced, err := a.SyncLocalBase(context.Background(), clone, "origin", "main")
	if err != nil {
		t.Fatalf("SyncLocalBase must never return an error (best-effort), got: %v", err)
	}
	if synced {
		t.Fatal("expected SyncLocalBase to skip (false) when the base branch is checked out dirty")
	}
}

func TestGitAdapter_SyncLocalBase_FastForwardsWhenSafe(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	// Advance origin/main past clone's local main via a second clone.
	other := gitfix.NewClone(t, origin)
	gitfix.Commit(t, other, "new-on-origin.txt", "advance origin\n")
	gitfix.Push(t, other, "main")

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	if err := a.Fetch(context.Background(), clone, "origin"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	synced, err := a.SyncLocalBase(context.Background(), clone, "origin", "main")
	if err != nil {
		t.Fatalf("SyncLocalBase must never return an error, got: %v", err)
	}
	if !synced {
		t.Fatal("expected SyncLocalBase to fast-forward when clean and not checked out elsewhere")
	}
}

func TestGitAdapter_RemoteDefaultBranch(t *testing.T) {
	gitfix.RequireGit(t)

	origin := gitfix.NewOrigin(t)
	clone := gitfix.NewClone(t, origin)

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New: %v", err)
	}

	b, ok, err := a.RemoteDefaultBranch(context.Background(), clone, "origin")
	if err != nil || !ok || b != "main" {
		t.Fatalf("RemoteDefaultBranch = (%q, %v, %v), want (main, true, nil) after a clone", b, ok, err)
	}

	if out, err := exec.Command("git", "-C", string(clone), "remote", "set-head", "origin", "-d").CombinedOutput(); err != nil {
		t.Fatalf("remote set-head -d: %v: %s", err, out)
	}
	b, ok, err = a.RemoteDefaultBranch(context.Background(), clone, "origin")
	if err != nil || ok || b != "" {
		t.Fatalf("RemoteDefaultBranch = (%q, %v, %v), want (\"\", false, nil) when origin/HEAD is unset", b, ok, err)
	}
}

// Without git on PATH the adapter still constructs, so commands that don't
// need git (version, help, install) keep working; every git call then fails
// with git_missing instead of the whole program exiting silently.
func TestGitAdapter_WithoutGitConstructsAndReportsGitMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	a, err := git.New()
	if err != nil {
		t.Fatalf("git.New() without git = %v, want an adapter", err)
	}
	if _, err := a.Version(context.Background()); domain.Code(err) != domain.CodeGitMissing {
		t.Fatalf("Version() = %v, want %s", err, domain.CodeGitMissing)
	}
	if err := a.Fetch(context.Background(), domain.Path(t.TempDir()), "origin"); domain.Code(err) != domain.CodeGitMissing {
		t.Fatalf("Fetch() = %v, want %s", err, domain.CodeGitMissing)
	}
}
