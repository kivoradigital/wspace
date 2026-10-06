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

func TestCreateContext(t *testing.T) {
	bad := domain.BranchName("bad branch")
	tests := []struct {
		name     string
		in       domain.Context
		existing bool
		wantCode domain.ErrCode
		wantIs   error
	}{
		{name: "valid context is saved", in: domain.Context{Name: "work", WorkspacesRoot: "/w", ProjectsRoot: "/p", IgnorePatterns: []domain.Glob{"vendor"}}},
		{name: "existing name is refused", in: domain.Context{Name: "work", WorkspacesRoot: "/w"}, existing: true, wantCode: domain.CodeContextExists},
		{name: "invalid name", in: domain.Context{Name: "Bad Name", WorkspacesRoot: "/w"}, wantIs: domain.ErrInvalidContextName},
		{name: "relative workspaces root", in: domain.Context{Name: "work", WorkspacesRoot: "w"}, wantIs: domain.ErrInvalidPath},
		{name: "missing workspaces root", in: domain.Context{Name: "work"}, wantIs: domain.ErrInvalidPath},
		{name: "relative projects root", in: domain.Context{Name: "work", WorkspacesRoot: "/w", ProjectsRoot: "p"}, wantIs: domain.ErrInvalidPath},
		{name: "invalid base branch", in: domain.Context{Name: "work", WorkspacesRoot: "/w", Defaults: domain.Options{BaseBranch: &bad}}, wantIs: domain.ErrInvalidBranchName},
		{name: "malformed ignore pattern", in: domain.Context{Name: "work", WorkspacesRoot: "/w", IgnorePatterns: []domain.Glob{"[x"}}, wantIs: app.ErrInvalidIgnorePattern},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := portstest.NewFakeConfigStore()
			if tt.existing {
				store.PutContext(domain.Context{Name: tt.in.Name, WorkspacesRoot: "/old"})
			}
			got, err := app.CreateContext(context.Background(), store, tt.in)
			switch {
			case tt.wantCode != "":
				if domain.Code(err) != tt.wantCode {
					t.Fatalf("error = %v, want code %s", err, tt.wantCode)
				}
				return
			case tt.wantIs != nil:
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("error = %v, want %v", err, tt.wantIs)
				}
				if _, lerr := store.LoadContext(context.Background(), tt.in.Name); lerr == nil && !tt.existing {
					t.Fatal("an invalid context was saved")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateContext() error = %v", err)
			}
			saved, err := store.LoadContext(context.Background(), "work")
			if err != nil || saved.WorkspacesRoot != got.WorkspacesRoot {
				t.Fatalf("saved = %+v, %v; want the created context", saved, err)
			}
		})
	}
}

func TestUpdateContext_RenameMovesActivePointerAndDeletesOldRecord(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/w", Projects: []domain.Project{{Key: "api", SourceDir: "/src/api"}}})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatal(err)
	}

	updated, _, err := app.UpdateContext(context.Background(), store, portstest.NewFakeFS(t), "work", domain.Context{Name: "job", WorkspacesRoot: "/w2"})
	if err != nil {
		t.Fatalf("UpdateContext() error = %v", err)
	}
	if updated.Name != "job" || len(updated.Projects) != 1 {
		t.Fatalf("updated = %+v, want renamed with projects carried over", updated)
	}
	if _, err := store.LoadContext(context.Background(), "work"); domain.Code(err) != domain.CodeContextNotFound {
		t.Fatalf("old record still loadable: %v", err)
	}
	root, _ := store.LoadRoot(context.Background())
	if root.ActiveContext != "job" {
		t.Fatalf("active = %q, want job", root.ActiveContext)
	}
}

func TestUpdateContext_RenameOntoAnExistingContextIsRefused(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/w"})
	store.PutContext(domain.Context{Name: "home", WorkspacesRoot: "/h"})

	_, _, err := app.UpdateContext(context.Background(), store, portstest.NewFakeFS(t), "work", domain.Context{Name: "home", WorkspacesRoot: "/w"})
	if domain.Code(err) != domain.CodeContextExists {
		t.Fatalf("error = %v, want context_exists", err)
	}
	home, _ := store.LoadContext(context.Background(), "home")
	if home.WorkspacesRoot != "/h" {
		t.Fatalf("home was overwritten: %+v", home)
	}
}

func TestUpdateContext_MissingContextIsNotFound(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	_, _, err := app.UpdateContext(context.Background(), store, portstest.NewFakeFS(t), "nope", domain.Context{Name: "nope", WorkspacesRoot: "/w"})
	if domain.Code(err) != domain.CodeContextNotFound {
		t.Fatalf("error = %v, want context_not_found", err)
	}
}
