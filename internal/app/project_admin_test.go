// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func projectAdminDeps(t *testing.T) (app.Deps, *portstest.FakeConfigStore, *portstest.FakeGit) {
	t.Helper()
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/w", Projects: []domain.Project{{Key: "api", SourceDir: "/src/api"}}})
	git := portstest.NewFakeGit()
	return app.Deps{Store: store, Git: git, FS: portstest.NewFakeFS(t)}, store, git
}

func TestRegisterProject(t *testing.T) {
	tests := []struct {
		name      string
		project   domain.Project
		notAClone bool
		linked    bool
		wantCode  domain.ErrCode
		wantIs    error
		wantSaved bool
	}{
		{name: "valid project is appended", project: domain.Project{Key: "web", SourceDir: "/src/web", DestBranch: "feat/{workspace}"}, wantSaved: true},
		{name: "duplicate key", project: domain.Project{Key: "api", SourceDir: "/src/other"}, wantCode: domain.CodeProjectExists},
		{name: "duplicate source dir", project: domain.Project{Key: "api2", SourceDir: "/src/api"}, wantCode: domain.CodeProjectExists},
		{name: "invalid key", project: domain.Project{Key: "../x", SourceDir: "/src/x"}, wantIs: domain.ErrInvalidProjectKey},
		{name: "relative source dir", project: domain.Project{Key: "x", SourceDir: "src/x"}, wantIs: domain.ErrInvalidPath},
		{name: "not a git repository", project: domain.Project{Key: "x", SourceDir: "/src/x"}, notAClone: true, wantCode: domain.CodeNotAMainClone},
		{name: "linked worktree", project: domain.Project{Key: "x", SourceDir: "/src/x"}, linked: true, wantCode: domain.CodeNotAMainClone},
		{name: "bad dest template", project: domain.Project{Key: "x", SourceDir: "/src/x", DestBranch: "{nope}"}, wantIs: errAny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, store, git := projectAdminDeps(t)
			git.IsMainCloneFunc = func(dir domain.Path) (bool, error) {
				switch {
				case tt.linked:
					return false, portstest.LinkedWorktreeErr(dir)
				case tt.notAClone:
					return false, nil
				}
				return true, nil
			}
			c, err := app.RegisterProject(context.Background(), deps, "work", tt.project)
			switch {
			case tt.wantCode != "":
				if domain.Code(err) != tt.wantCode {
					t.Fatalf("error = %v, want code %s", err, tt.wantCode)
				}
			case tt.wantIs == errAny:
				if err == nil {
					t.Fatal("error = nil, want a validation error")
				}
			case tt.wantIs != nil:
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("error = %v, want %v", err, tt.wantIs)
				}
			default:
				if err != nil {
					t.Fatalf("RegisterProject() error = %v", err)
				}
			}
			saved, _ := store.LoadContext(context.Background(), "work")
			if tt.wantSaved {
				if len(c.Projects) != 2 || len(saved.Projects) != 2 || saved.Projects[1].Key != tt.project.Key {
					t.Fatalf("saved projects = %+v, want web appended", saved.Projects)
				}
			} else if len(saved.Projects) != 1 {
				t.Fatalf("saved projects = %+v, want unchanged", saved.Projects)
			}
		})
	}
}

func TestUpdateProject_PatchesOnlyGivenFields(t *testing.T) {
	deps, store, _ := projectAdminDeps(t)
	origin := "develop"
	dest := "feat/{workspace}"
	_, err := app.UpdateProject(context.Background(), deps, "work", "api", app.ProjectPatch{OriginBranch: &origin, DestBranch: &dest})
	if err != nil {
		t.Fatalf("UpdateProject() error = %v", err)
	}
	saved, _ := store.LoadContext(context.Background(), "work")
	p := saved.Projects[0]
	if p.SourceDir != "/src/api" || p.OriginBranch == nil || *p.OriginBranch != "develop" || p.DestBranch != "feat/{workspace}" {
		t.Fatalf("project = %+v, want origin/dest patched, source kept", p)
	}

	empty := ""
	if _, err := app.UpdateProject(context.Background(), deps, "work", "api", app.ProjectPatch{OriginBranch: &empty}); err != nil {
		t.Fatalf("clear origin: %v", err)
	}
	saved, _ = store.LoadContext(context.Background(), "work")
	if saved.Projects[0].OriginBranch != nil {
		t.Fatalf("OriginBranch = %v, want cleared by an empty string", *saved.Projects[0].OriginBranch)
	}
}

func TestUpdateProject_UnknownKeyIsNotFound(t *testing.T) {
	deps, _, _ := projectAdminDeps(t)
	if _, err := app.UpdateProject(context.Background(), deps, "work", "nope", app.ProjectPatch{}); domain.Code(err) != domain.CodeProjectNotFound {
		t.Fatalf("error = %v, want project_not_found", err)
	}
}

func TestRemoveProject(t *testing.T) {
	deps, store, _ := projectAdminDeps(t)
	if _, err := app.RemoveProject(context.Background(), deps, "work", "nope"); domain.Code(err) != domain.CodeProjectNotFound {
		t.Fatalf("error = %v, want project_not_found", err)
	}
	if _, err := app.RemoveProject(context.Background(), deps, "work", "api"); err != nil {
		t.Fatalf("RemoveProject() error = %v", err)
	}
	saved, _ := store.LoadContext(context.Background(), "work")
	if len(saved.Projects) != 0 {
		t.Fatalf("projects = %+v, want empty", saved.Projects)
	}
}

// errAny marks a table case that only needs some non-nil error.
var errAny = errors.New("any error")
