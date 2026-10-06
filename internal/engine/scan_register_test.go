// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/engine"
)

func TestContexts_IncludePatternsPersistAndDriveTheScan(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, d := range []string{"api/.git", "tools/cli/.git", "node_modules/pkg/.git"} {
		if err := f.fs.MkdirAll(f.ctx.ProjectsRoot.Join(d)); err != nil {
			t.Fatal(err)
		}
	}

	include := []string{"api", "tools/*"}
	ignore := []string{"node_modules"}
	updated, err := f.eng.UpdateContext(ctx, engine.UpdateContextParams{Name: "work", IncludePatterns: &include, IgnorePatterns: &ignore})
	if err != nil {
		t.Fatalf("UpdateContext() error = %v", err)
	}
	if len(updated.IncludePatterns) != 2 || updated.IncludePatterns[1] != "tools/*" {
		t.Fatalf("IncludePatterns = %v, want [api tools/*]", updated.IncludePatterns)
	}

	res, err := f.eng.ScanProjects(ctx, engine.ScanProjectsParams{})
	if err != nil {
		t.Fatalf("ScanProjects() error = %v", err)
	}
	var rels []string
	for _, c := range res.Candidates {
		rels = append(rels, c.RelativePath)
	}
	if len(rels) != 2 || rels[0] != "api" || rels[1] != "tools/cli" {
		t.Fatalf("candidates = %v, want [api tools/cli]", rels)
	}

	// An explicit empty include list in the call overrides the context's.
	none := []string{}
	all, err := f.eng.ScanProjects(ctx, engine.ScanProjectsParams{Include: &none})
	if err != nil || len(all.Candidates) != 2 {
		t.Fatalf("ScanProjects(include: []) = %+v, %v; want api and tools/cli (node_modules still ignored)", all.Candidates, err)
	}

	if _, err := f.eng.UpdateContext(ctx, engine.UpdateContextParams{Name: "work", IncludePatterns: &[]string{"[bad"}}); codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("malformed include pattern error = %v, want invalid_params", err)
	}
}

func TestScanProjects_ReportsTheDepthAndWhereItStopped(t *testing.T) {
	f := newFixture(t)
	if err := f.fs.MkdirAll(f.ctx.ProjectsRoot.Join("a", "b", "c", ".git")); err != nil {
		t.Fatal(err)
	}
	res, err := f.eng.ScanProjects(context.Background(), engine.ScanProjectsParams{Depth: 2})
	if err != nil {
		t.Fatalf("ScanProjects() error = %v", err)
	}
	if !res.Truncated || res.MaxDepth != 2 {
		t.Fatalf("Truncated = %v, MaxDepth = %d; want true, 2", res.Truncated, res.MaxDepth)
	}
	if len(res.TruncatedDirs) != 1 || res.TruncatedDirs[0] != string(f.ctx.ProjectsRoot.Join("a", "b")) {
		t.Fatalf("TruncatedDirs = %v, want [<root>/a/b]", res.TruncatedDirs)
	}
}

func TestRegisterProjects_AllOrNothingWithAPerProjectReason(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	root := f.ctx.ProjectsRoot
	for _, d := range []string{"one/.git", "two/.git", "App/.git", "app2/.git"} {
		if err := f.fs.MkdirAll(root.Join(d)); err != nil {
			t.Fatal(err)
		}
	}
	f.git.IsMainCloneFunc = func(dir domain.Path) (bool, error) { return dir != root.Join("app2"), nil }

	res, err := f.eng.RegisterProjects(ctx, engine.RegisterProjectsParams{Projects: []engine.ProjectToRegister{
		{Key: "one", SourceDir: string(root.Join("one"))},
		{Key: "api", SourceDir: string(root.Join("two"))}, // key taken in the context
		{Key: "App", SourceDir: string(root.Join("App"))},
		{Key: "app", SourceDir: string(root.Join("app2"))},        // case-duplicate in the batch, and not a main clone
		{Key: "-bad", SourceDir: string(root.Join("one"))},        // invalid key, and duplicate source
		{Key: "web2", SourceDir: string(root.Join("src", "web"))}, // not absolute? no: already registered source below
		{Key: "web3", SourceDir: string(f.ctx.Projects[1].SourceDir)},
	}})
	if err != nil {
		t.Fatalf("RegisterProjects() error = %v", err)
	}
	if len(res.Registered) != 0 {
		t.Fatalf("Registered = %+v, want none: the batch is all-or-nothing", res.Registered)
	}
	reasons := map[int][]string{}
	for _, p := range res.Problems {
		reasons[p.Index] = append(reasons[p.Index], p.Reason)
	}
	want := map[int]string{1: "key_taken", 3: "key_duplicate", 4: "invalid_key", 6: "source_registered"}
	for i, r := range want {
		if !has(reasons[i], r) {
			t.Errorf("problems for #%d = %v, want to include %q", i, reasons[i], r)
		}
	}
	if has(reasons[0], "key_duplicate") || len(reasons[0]) != 0 {
		t.Errorf("#0 should have no problem, got %v", reasons[0])
	}
	if !has(reasons[3], "not_a_main_clone") {
		t.Errorf("problems for #3 = %v, want not_a_main_clone too", reasons[3])
	}
	list, _ := f.eng.ListProjects(ctx, engine.ProjectsRef{})
	if len(list) != 2 {
		t.Fatalf("projects after a refused batch = %d, want the original 2", len(list))
	}

	ok, err := f.eng.RegisterProjects(ctx, engine.RegisterProjectsParams{Projects: []engine.ProjectToRegister{
		{Key: "one", SourceDir: string(root.Join("one"))},
		{Key: "App", SourceDir: string(root.Join("App"))},
	}})
	if err != nil || len(ok.Problems) != 0 || len(ok.Registered) != 2 {
		t.Fatalf("valid batch = %+v, %v; want 2 registered", ok, err)
	}
	list, _ = f.eng.ListProjects(ctx, engine.ProjectsRef{})
	if len(list) != 4 {
		t.Fatalf("projects after a valid batch = %d, want 4", len(list))
	}
}

func has(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestRepoChanges_ListsEveryChangedPath(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.eng.CreateWorkspace(ctx, engine.CreateWorkspaceParams{Name: "feat", Projects: []string{"api"}}, nil); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{
			{X: 'M', Y: ' ', RelPath: "staged.go"},
			{X: ' ', Y: 'M', RelPath: "dir with space/file.go"},
			{X: 'A', Y: 'M', RelPath: "new.go"},
			{X: ' ', Y: 'D', RelPath: "gone.go"},
			{X: 'R', Y: ' ', RelPath: "renamed.go", OrigPath: "old name.go"},
			{X: '?', Y: '?', RelPath: "notes.txt"},
			{X: 'U', Y: 'U', RelPath: "conflict.go"},
		}, nil
	}
	got, err := f.eng.RepoChanges(ctx, engine.RepoRef{Workspace: "feat", Repo: "api"})
	if err != nil {
		t.Fatalf("RepoChanges() error = %v", err)
	}
	want := []engine.FileChange{
		{Path: "staged.go", Status: "modified", Staged: true},
		{Path: "dir with space/file.go", Status: "modified", Unstaged: true},
		{Path: "new.go", Status: "added", Staged: true, Unstaged: true},
		{Path: "gone.go", Status: "deleted", Unstaged: true},
		{Path: "renamed.go", OrigPath: "old name.go", Status: "renamed", Staged: true},
		{Path: "notes.txt", Status: "untracked"},
		{Path: "conflict.go", Status: "conflicted", Staged: true, Unstaged: true},
	}
	if len(got) != len(want) {
		t.Fatalf("RepoChanges() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RepoChanges()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if _, err := f.eng.RepoChanges(ctx, engine.RepoRef{Workspace: "feat", Repo: "ghost"}); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("unknown repo error = %v, want not_found", err)
	}
}

func TestGetProject_ReturnsOneProjectOrNotFound(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, err := f.eng.GetProject(ctx, engine.ProjectRef{Key: "web"})
	if err != nil || p.Key != "web" || p.SourceDir == "" {
		t.Fatalf("GetProject(web) = %+v, %v", p, err)
	}
	if _, err := f.eng.GetProject(ctx, engine.ProjectRef{Key: "ghost"}); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("GetProject(ghost) error = %v, want not_found", err)
	}
	if _, err := f.eng.GetProject(ctx, engine.ProjectRef{Context: "nope", Key: "web"}); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("GetProject in a missing context error = %v, want not_found", err)
	}
}
