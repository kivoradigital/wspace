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

// addableFixture seeds a workspace holding api (by key) and an adopted
// legacy repo whose alias is "legacy-web" but whose source dir is web's
// main clone, written with a trailing slash.
func addableFixture(t *testing.T) (app.Deps, domain.Path, domain.Context) {
	t.Helper()
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	home := fs.Paths().Home
	wsRoot := home.Join("workspaces", "ws1")
	store.PutManifest(wsRoot, domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{
		Name: "ws1", Root: wsRoot, Branch: "feature-x",
		Repos: []domain.RepoEntry{
			{Alias: "api", Project: "api", SourceDir: home.Join("src", "api"), Branch: "feature-x"},
			{Alias: "legacy-web", Project: "legacy-web", SourceDir: domain.Path(string(home.Join("src", "web")) + "/"), Branch: "feature-x"},
			{Alias: "docs", Branch: "feature-x"},
		},
	}})
	if err := fs.MkdirAll(wsRoot); err != nil {
		t.Fatal(err)
	}
	c := domain.Context{Name: "work", Projects: []domain.Project{
		{Key: "api", SourceDir: home.Join("src", "api-renamed")},
		{Key: "web", SourceDir: home.Join("src", "web")},
		{Key: "docs", SourceDir: home.Join("src", "docs")},
		{Key: "cli", SourceDir: home.Join("src", "cli")},
	}}
	return app.Deps{Store: store, Git: portstest.NewFakeGit(), FS: fs, Reporter: portstest.NewRecordingReporter()}, wsRoot, c
}

func TestAddableProjects_ExcludesProjectsAlreadyInTheWorkspace(t *testing.T) {
	deps, wsRoot, c := addableFixture(t)

	got, err := app.AddableProjects(context.Background(), deps, app.AddableProjectsInput{WorkspaceRoot: wsRoot, Context: c})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "cli" {
		t.Fatalf("addable = %+v, want only cli (api by key, web by source dir, docs by alias)", got)
	}
}

func TestAddRepo_RefusesAProjectAlreadyInTheWorkspace(t *testing.T) {
	deps, wsRoot, c := addableFixture(t)
	for _, key := range []domain.ProjectKey{"api", "web", "docs"} {
		_, err := app.AddRepo(context.Background(), deps, app.AddRepoInput{WorkspaceRoot: wsRoot, Context: c, ProjectKey: key})
		if domain.Code(err) != domain.CodeAlreadyInWorkspace {
			t.Errorf("AddRepo(%s) = %v, want already_in_workspace", key, err)
		}
	}
}
