// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func scanFixture(t *testing.T) (*portstest.FakeFS, domain.Path) {
	t.Helper()
	fs := portstest.NewFakeFS(t)
	root := fs.Paths().Home.Join("src")
	for _, d := range []string{"api/.git", "team-a/web/.git", "team-b/web/.git", "vendor/lib/.git", "deep/a/b/c/.git"} {
		if err := fs.MkdirAll(root.Join(d)); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := fs.MkdirAll(root.Join("linked")); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile(root.Join("linked", ".git"), []byte("gitdir: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return fs, root
}

func TestScanProjects_FindsCandidatesWithUniqueSuggestedKeys(t *testing.T) {
	fs, root := scanFixture(t)
	deps := app.Deps{Store: portstest.NewFakeConfigStore(), Git: portstest.NewFakeGit(), FS: fs}

	res, err := app.ScanProjects(context.Background(), deps, app.ScanProjectsInput{
		Roots:    []domain.Path{root},
		MaxDepth: 3,
		Ignore:   []domain.Glob{"vendor"},
		Registered: []domain.Project{
			{Key: "api", SourceDir: root.Join("api")},
		},
	})
	if err != nil {
		t.Fatalf("ScanProjects() error = %v", err)
	}

	want := map[domain.Path]struct {
		key        string
		registered bool
	}{
		root.Join("api"):           {"api", true},
		root.Join("team-a", "web"): {"team-a-web", false},
		root.Join("team-b", "web"): {"team-b-web", false},
	}
	if len(res.Candidates) != len(want) {
		t.Fatalf("candidates = %+v, want %d entries", res.Candidates, len(want))
	}
	for _, c := range res.Candidates {
		w, ok := want[c.Path]
		if !ok {
			t.Fatalf("unexpected candidate %+v", c)
		}
		if c.SuggestedKey != w.key || c.Registered != w.registered || c.Root != root {
			t.Fatalf("candidate %s = %+v, want key=%q registered=%v", c.Path, c, w.key, w.registered)
		}
	}
	if len(res.LinkedWorktrees) != 1 || res.LinkedWorktrees[0] != root.Join("linked") {
		t.Fatalf("LinkedWorktrees = %v, want [%s]", res.LinkedWorktrees, root.Join("linked"))
	}
	if !res.Truncated {
		t.Fatal("Truncated = false, want true: deep/a/b/c lies past MaxDepth 3")
	}
}

func TestScanProjects_RejectsAMalformedIgnorePattern(t *testing.T) {
	fs, root := scanFixture(t)
	deps := app.Deps{Store: portstest.NewFakeConfigStore(), Git: portstest.NewFakeGit(), FS: fs}
	if _, err := app.ScanProjects(context.Background(), deps, app.ScanProjectsInput{Roots: []domain.Path{root}, Ignore: []domain.Glob{"[bad"}}); err == nil {
		t.Fatal("ScanProjects() error = nil, want a malformed-pattern error")
	}
}

// Suggested keys must be registrable together: unique case-insensitively
// (macOS folders "App" and "app" collide), never equal to a key already
// registered in the context (even for a repo outside the scanned root), and
// a path-derived fallback must not collide with another repo's leaf name.
func TestScanProjects_SuggestedKeysAreRegistrableTogether(t *testing.T) {
	fs := portstest.NewFakeFS(t)
	root := fs.Paths().Home.Join("src")
	for _, d := range []string{"ios/App/.git", "android/app/.git", "team-a/web/.git", "team-a-web/.git", "billing/.git", "dotted/my.repo/.git"} {
		if err := fs.MkdirAll(root.Join(d)); err != nil {
			t.Fatal(err)
		}
	}
	deps := app.Deps{Store: portstest.NewFakeConfigStore(), Git: portstest.NewFakeGit(), FS: fs}
	res, err := app.ScanProjects(context.Background(), deps, app.ScanProjectsInput{
		Roots:      []domain.Path{root},
		Registered: []domain.Project{{Key: "billing", SourceDir: "/elsewhere/billing"}},
	})
	if err != nil {
		t.Fatalf("ScanProjects() error = %v", err)
	}
	got := map[string]string{}
	seen := map[string]string{}
	for _, c := range res.Candidates {
		got[c.RelativePath] = c.SuggestedKey
		lower := strings.ToLower(c.SuggestedKey)
		if other, dup := seen[lower]; dup {
			t.Fatalf("suggested key %q for %s collides with %s", c.SuggestedKey, c.RelativePath, other)
		}
		seen[lower] = c.RelativePath
		if _, err := domain.NewProjectKey(c.SuggestedKey); err != nil {
			t.Fatalf("suggested key %q is invalid: %v", c.SuggestedKey, err)
		}
	}
	want := map[string]string{
		"ios/App":        "ios-App",
		"android/app":    "android-app",
		"team-a-web":     "team-a-web",
		"team-a/web":     "web",
		"billing":        "billing-2",
		"dotted/my.repo": "my.repo",
	}
	for rel, key := range want {
		if got[rel] != key {
			t.Errorf("SuggestedKey(%s) = %q, want %q (all: %v)", rel, got[rel], key, got)
		}
	}
}

// Include patterns keep only matching repositories (by folder name or
// root-relative path); an empty list keeps everything.
func TestScanProjects_IncludePatternsFilterCandidates(t *testing.T) {
	fs, root := scanFixture(t)
	deps := app.Deps{Store: portstest.NewFakeConfigStore(), Git: portstest.NewFakeGit(), FS: fs}
	res, err := app.ScanProjects(context.Background(), deps, app.ScanProjectsInput{
		Roots:   []domain.Path{root},
		Include: []domain.Glob{"team-a/*", "api"},
	})
	if err != nil {
		t.Fatalf("ScanProjects() error = %v", err)
	}
	var rels []string
	for _, c := range res.Candidates {
		rels = append(rels, c.RelativePath)
	}
	if strings.Join(rels, ",") != "api,team-a/web" {
		t.Fatalf("candidates = %v, want [api team-a/web]", rels)
	}
	if res.Candidates[1].SuggestedKey != "web" {
		t.Fatalf("SuggestedKey = %q, want web (team-b/web was filtered out, so web is unique)", res.Candidates[1].SuggestedKey)
	}
}

func TestScanProjects_ReportsWhereTheDepthCapStopped(t *testing.T) {
	fs, root := scanFixture(t)
	deps := app.Deps{Store: portstest.NewFakeConfigStore(), Git: portstest.NewFakeGit(), FS: fs}
	res, err := app.ScanProjects(context.Background(), deps, app.ScanProjectsInput{Roots: []domain.Path{root}, MaxDepth: 3, Ignore: []domain.Glob{"vendor"}})
	if err != nil {
		t.Fatalf("ScanProjects() error = %v", err)
	}
	if res.MaxDepth != 3 {
		t.Fatalf("MaxDepth = %d, want 3", res.MaxDepth)
	}
	if len(res.TruncatedDirs) != 1 || res.TruncatedDirs[0] != root.Join("deep", "a", "b") {
		t.Fatalf("TruncatedDirs = %v, want [%s]", res.TruncatedDirs, root.Join("deep", "a", "b"))
	}
}
