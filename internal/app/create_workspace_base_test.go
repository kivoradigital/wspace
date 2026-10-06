// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// createBaseFixture creates "ws1" from two projects with a context base of
// develop; present lists, per source dir, the branches that exist there.
func runCreateWithBases(t *testing.T, ctx domain.Context, present map[domain.Path][]domain.BranchName, defaults map[domain.Path]domain.BranchName) (*portstest.FakeGit, *portstest.RecordingReporter, domain.Manifest, map[domain.Path]string) {
	t.Helper()
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()
	reporter := portstest.NewRecordingReporter()

	git.ResolveBaseFunc = func(repo domain.Path, remote string, b domain.BranchName) (ports.BaseRef, error) {
		for _, have := range present[repo] {
			if have == b {
				return ports.BaseRef{Ref: remote + "/" + string(b), Remote: true}, nil
			}
		}
		return ports.BaseRef{Ref: "HEAD"}, nil
	}
	git.RemoteDefaultBranchFunc = func(repo domain.Path, _ string) (domain.BranchName, bool, error) {
		b, ok := defaults[repo]
		return b, ok, nil
	}
	startPoints := map[domain.Path]string{}
	git.WorktreeAddFunc = func(repo domain.Path, spec ports.WorktreeSpec) error {
		startPoints[repo] = spec.StartPoint
		return nil
	}

	deps := app.Deps{Store: store, Git: git, FS: fs, Reporter: reporter}
	ctx.WorkspacesRoot = fs.Paths().Home.Join("workspaces")
	if _, err := app.CreateWorkspace(context.Background(), deps, app.CreateWorkspaceInput{Context: ctx, Branch: "feat", Name: "ws1"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	m, err := store.LoadManifest(context.Background(), ctx.WorkspacesRoot.Join("ws1"))
	if err != nil {
		t.Fatal(err)
	}
	return git, reporter, m, startPoints
}

func baseOf(m domain.Manifest, alias string) domain.BranchName {
	for _, r := range m.Workspace.Repos {
		if r.Alias == alias {
			return r.BaseBranch
		}
	}
	return "<absent>"
}

func TestCreateWorkspace_RepoWithoutTheBaseFallsBackToItsDefaultBranch(t *testing.T) {
	develop := domain.BranchName("develop")
	ctx := domain.Context{Name: "work", Defaults: domain.Options{BaseBranch: &develop}, Projects: []domain.Project{
		{Key: "api", SourceDir: "/src/api"},
		{Key: "hub", SourceDir: "/src/hub"},
	}}

	_, reporter, m, starts := runCreateWithBases(t, ctx,
		map[domain.Path][]domain.BranchName{"/src/api": {"develop"}, "/src/hub": {"master"}},
		map[domain.Path]domain.BranchName{"/src/hub": "master"})

	if starts["/src/api"] != "origin/develop" || starts["/src/hub"] != "origin/master" {
		t.Fatalf("start points = %v, want api from origin/develop and hub from origin/master", starts)
	}
	if baseOf(m, "api") != "" || baseOf(m, "hub") != "master" {
		t.Fatalf("recorded bases api=%q hub=%q, want api unrecorded and hub=master", baseOf(m, "api"), baseOf(m, "hub"))
	}
	if len(reporter.Warnings) != 1 || reporter.Warnings[0].Key != messages.BaseBranchFallback {
		t.Fatalf("warnings = %+v, want one BaseBranchFallback", reporter.Warnings)
	}
}

func TestCreateWorkspace_NoBaseAnywhereStartsFromHEADAndSaysSo(t *testing.T) {
	develop := domain.BranchName("develop")
	ctx := domain.Context{Name: "work", Defaults: domain.Options{BaseBranch: &develop}, Projects: []domain.Project{
		{Key: "hub", SourceDir: "/src/hub"},
	}}

	_, reporter, m, starts := runCreateWithBases(t, ctx, nil, nil)

	if starts["/src/hub"] != "HEAD" {
		t.Fatalf("start point = %q, want HEAD (unchanged fallback)", starts["/src/hub"])
	}
	if baseOf(m, "hub") != "" {
		t.Fatalf("recorded base = %q, want none", baseOf(m, "hub"))
	}
	if len(reporter.Warnings) != 1 || reporter.Warnings[0].Key != messages.BaseBranchMissing {
		t.Fatalf("warnings = %+v, want one BaseBranchMissing", reporter.Warnings)
	}
}

func TestCreateWorkspace_ProjectOriginBranchIsTheBase(t *testing.T) {
	develop := domain.BranchName("develop")
	release := domain.BranchName("release")
	ctx := domain.Context{Name: "work", Defaults: domain.Options{BaseBranch: &develop}, Projects: []domain.Project{
		{Key: "api", SourceDir: "/src/api", OriginBranch: &release},
	}}

	_, reporter, m, starts := runCreateWithBases(t, ctx,
		map[domain.Path][]domain.BranchName{"/src/api": {"develop", "release"}}, nil)

	if starts["/src/api"] != "origin/release" {
		t.Fatalf("start point = %q, want origin/release (the project's origin branch)", starts["/src/api"])
	}
	if baseOf(m, "api") != "release" {
		t.Fatalf("recorded base = %q, want release", baseOf(m, "api"))
	}
	if len(reporter.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", reporter.Warnings)
	}
}
