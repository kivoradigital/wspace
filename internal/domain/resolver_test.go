// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func mustBranch(t *testing.T, s string) domain.BranchName {
	t.Helper()
	bn, err := domain.NewBranchName(s)
	if err != nil {
		t.Fatalf("NewBranchName(%q) unexpected error: %v", s, err)
	}
	return bn
}

func mustPath(t *testing.T, s string) domain.Path {
	t.Helper()
	p, err := domain.NewPath(s)
	if err != nil {
		t.Fatalf("NewPath(%q) unexpected error: %v", s, err)
	}
	return p
}

func ptr[T any](v T) *T { return &v }

const testProject domain.ProjectKey = "api"

// buildResolver constructs a Resolver whose layers are populated only up to
// the requested Layer, so each test case exercises exactly one winning
// layer while still exercising the full precedence chain beneath it.
func buildResolver(t *testing.T, layer domain.Layer) domain.Resolver {
	t.Helper()

	project := domain.Project{Key: testProject}
	ctx := &domain.Context{
		Name:     "work",
		Projects: []domain.Project{project},
	}
	var r domain.Resolver

	switch layer {
	case domain.LayerFlag:
		r = domain.Resolver{
			Flags: domain.Options{
				BaseBranch:        ptr(mustBranch(t, "flag-base")),
				CopyEnv:           ptr(false),
				FetchBeforeCreate: ptr(false),
				EnvPruneDirs:      []string{"flag-dir"},
				Remote:            ptr("flag-remote"),
			},
			Context: ctx,
		}
	case domain.LayerProject:
		ctx.Projects[0].Options = domain.Options{
			BaseBranch:        ptr(mustBranch(t, "project-base")),
			CopyEnv:           ptr(false),
			FetchBeforeCreate: ptr(false),
			EnvPruneDirs:      []string{"project-dir"},
			Remote:            ptr("project-remote"),
		}
		r = domain.Resolver{Context: ctx}
	case domain.LayerWorkspace:
		ws := &domain.Workspace{
			Options: domain.Options{
				BaseBranch:        ptr(mustBranch(t, "workspace-base")),
				CopyEnv:           ptr(false),
				FetchBeforeCreate: ptr(false),
				EnvPruneDirs:      []string{"workspace-dir"},
				Remote:            ptr("workspace-remote"),
			},
		}
		r = domain.Resolver{Context: ctx, Workspace: ws}
	case domain.LayerOverlay:
		overlay := &domain.Overlay{
			Options: domain.Options{
				BaseBranch:        ptr(mustBranch(t, "overlay-base")),
				CopyEnv:           ptr(false),
				FetchBeforeCreate: ptr(false),
				EnvPruneDirs:      []string{"overlay-dir"},
				Remote:            ptr("overlay-remote"),
			},
		}
		r = domain.Resolver{Context: ctx, Overlay: overlay}
	case domain.LayerContext:
		ctx.Defaults = domain.Options{
			BaseBranch:        ptr(mustBranch(t, "context-base")),
			CopyEnv:           ptr(false),
			FetchBeforeCreate: ptr(false),
			EnvPruneDirs:      []string{"context-dir"},
			Remote:            ptr("context-remote"),
		}
		r = domain.Resolver{Context: ctx}
	case domain.LayerBuiltin:
		r = domain.Resolver{Context: ctx}
	}
	return r
}

func TestResolver_PrecedenceAcrossAllLayers(t *testing.T) {
	layers := []domain.Layer{
		domain.LayerFlag,
		domain.LayerProject,
		domain.LayerWorkspace,
		domain.LayerOverlay,
		domain.LayerContext,
		domain.LayerBuiltin,
	}

	for _, layer := range layers {
		t.Run(layer.String(), func(t *testing.T) {
			r := buildResolver(t, layer)

			if got := r.BaseBranch(testProject); got.From != layer {
				t.Errorf("BaseBranch().From = %v, want %v (value=%v)", got.From, layer, got.Value)
			}
			if got := r.CopyEnv(testProject); got.From != layer {
				t.Errorf("CopyEnv().From = %v, want %v (value=%v)", got.From, layer, got.Value)
			}
			// FetchBeforeCreate() takes no ProjectKey (design.md §4): it has
			// no per-project layer. buildResolver's "project" fixture only
			// populates the project's Options, so with no per-project layer
			// to consult, no other layer has data either and resolution
			// falls all the way through to the built-in default.
			wantFetchLayer := layer
			if layer == domain.LayerProject {
				wantFetchLayer = domain.LayerBuiltin
			}
			if got := r.FetchBeforeCreate(); got.From != wantFetchLayer {
				t.Errorf("FetchBeforeCreate().From = %v, want %v (value=%v)", got.From, wantFetchLayer, got.Value)
			}
			if got := r.EnvPruneDirs(testProject); got.From != layer {
				t.Errorf("EnvPruneDirs().From = %v, want %v (value=%v)", got.From, layer, got.Value)
			}
			if got := r.Remote(testProject); got.From != layer {
				t.Errorf("Remote().From = %v, want %v (value=%v)", got.From, layer, got.Value)
			}
		})
	}
}

func TestResolver_BuiltinDefaults(t *testing.T) {
	r := domain.Resolver{Context: &domain.Context{Projects: []domain.Project{{Key: testProject}}}}

	if got := r.BaseBranch(testProject); got.Value != domain.BuiltinOptions.BaseBranch {
		t.Errorf("BaseBranch().Value = %v, want built-in %v", got.Value, domain.BuiltinOptions.BaseBranch)
	}
	if got := r.CopyEnv(testProject); got.Value != domain.BuiltinOptions.CopyEnv {
		t.Errorf("CopyEnv().Value = %v, want built-in %v", got.Value, domain.BuiltinOptions.CopyEnv)
	}
	if got := r.Remote(testProject); got.Value != domain.BuiltinOptions.Remote {
		t.Errorf("Remote().Value = %v, want built-in %v", got.Value, domain.BuiltinOptions.Remote)
	}
	if got := r.EnvPruneDirs(testProject); !reflect.DeepEqual(got.Value, domain.BuiltinOptions.EnvPruneDirs) {
		t.Errorf("EnvPruneDirs().Value = %v, want built-in %v", got.Value, domain.BuiltinOptions.EnvPruneDirs)
	}
}

func TestResolver_OverlayProjectScopeBeatsOverlayGeneral(t *testing.T) {
	overlay := &domain.Overlay{
		Options: domain.Options{Remote: ptr("general-remote")},
		Projects: map[domain.ProjectKey]domain.Options{
			testProject: {Remote: ptr("project-scoped-remote")},
		},
	}
	r := domain.Resolver{
		Context: &domain.Context{Projects: []domain.Project{{Key: testProject}}},
		Overlay: overlay,
	}

	got := r.Remote(testProject)
	if got.From != domain.LayerOverlay {
		t.Fatalf("Remote().From = %v, want LayerOverlay", got.From)
	}
	if got.Value != "project-scoped-remote" {
		t.Fatalf("Remote().Value = %q, want %q", got.Value, "project-scoped-remote")
	}
}

func TestResolver_WorktreePath(t *testing.T) {
	root := mustPath(t, "/workspaces/payments-fix")
	v := domain.Vars{Project: "api"}

	t.Run("project template wins", func(t *testing.T) {
		ctx := &domain.Context{Projects: []domain.Project{{Key: testProject, WorktreeDir: "apps/{project}"}}}
		r := domain.Resolver{Context: ctx}

		got, err := r.WorktreePath(testProject, v, root)
		if err != nil {
			t.Fatalf("WorktreePath() unexpected error: %v", err)
		}
		if got.From != domain.LayerProject {
			t.Fatalf("WorktreePath().From = %v, want LayerProject", got.From)
		}
		if string(got.Value) != "/workspaces/payments-fix/apps/api" {
			t.Fatalf("WorktreePath().Value = %q, want %q", got.Value, "/workspaces/payments-fix/apps/api")
		}
	})

	t.Run("builtin default derives alias from project", func(t *testing.T) {
		ctx := &domain.Context{Projects: []domain.Project{{Key: testProject}}}
		r := domain.Resolver{Context: ctx}

		got, err := r.WorktreePath(testProject, v, root)
		if err != nil {
			t.Fatalf("WorktreePath() unexpected error: %v", err)
		}
		if got.From != domain.LayerBuiltin {
			t.Fatalf("WorktreePath().From = %v, want LayerBuiltin", got.From)
		}
		if string(got.Value) != "/workspaces/payments-fix/api" {
			t.Fatalf("WorktreePath().Value = %q, want %q", got.Value, "/workspaces/payments-fix/api")
		}
	})
}

func TestResolver_DestBranch(t *testing.T) {
	v := domain.Vars{}

	t.Run("project dest_branch overrides workspace branch", func(t *testing.T) {
		ctx := &domain.Context{Projects: []domain.Project{{Key: testProject, DestBranch: "hotfix-1"}}}
		ws := &domain.Workspace{Branch: mustBranch(t, "release-1")}
		r := domain.Resolver{Context: ctx, Workspace: ws}

		got, err := r.DestBranch(testProject, v)
		if err != nil {
			t.Fatalf("DestBranch() unexpected error: %v", err)
		}
		if got.From != domain.LayerProject {
			t.Fatalf("DestBranch().From = %v, want LayerProject", got.From)
		}
		if string(got.Value) != "hotfix-1" {
			t.Fatalf("DestBranch().Value = %q, want %q", got.Value, "hotfix-1")
		}
	})

	t.Run("empty project dest_branch inherits workspace branch", func(t *testing.T) {
		ctx := &domain.Context{Projects: []domain.Project{{Key: testProject}}}
		ws := &domain.Workspace{Branch: mustBranch(t, "release-1")}
		r := domain.Resolver{Context: ctx, Workspace: ws}

		got, err := r.DestBranch(testProject, v)
		if err != nil {
			t.Fatalf("DestBranch() unexpected error: %v", err)
		}
		if got.From != domain.LayerWorkspace {
			t.Fatalf("DestBranch().From = %v, want LayerWorkspace", got.From)
		}
		if string(got.Value) != "release-1" {
			t.Fatalf("DestBranch().Value = %q, want %q", got.Value, "release-1")
		}
	})

	t.Run("no workspace yet falls back to the requested branch", func(t *testing.T) {
		ctx := &domain.Context{Projects: []domain.Project{{Key: testProject}}}
		r := domain.Resolver{Context: ctx}

		got, err := r.DestBranch(testProject, domain.Vars{Branch: "new-workspace-branch"})
		if err != nil {
			t.Fatalf("DestBranch() unexpected error: %v", err)
		}
		if string(got.Value) != "new-workspace-branch" {
			t.Fatalf("DestBranch().Value = %q, want %q", got.Value, "new-workspace-branch")
		}
	})

	t.Run("no branch requested and no prefix set defaults to the workspace name", func(t *testing.T) {
		ctx := &domain.Context{Projects: []domain.Project{{Key: testProject}}}
		r := domain.Resolver{Context: ctx}

		got, err := r.DestBranch(testProject, domain.Vars{Workspace: "feature-x"})
		if err != nil {
			t.Fatalf("DestBranch() unexpected error: %v", err)
		}
		if got.From != domain.LayerBuiltin {
			t.Fatalf("DestBranch().From = %v, want LayerBuiltin", got.From)
		}
		if string(got.Value) != "feature-x" {
			t.Fatalf("DestBranch().Value = %q, want %q", got.Value, "feature-x")
		}
	})

	t.Run("no branch requested defaults to the context branch_prefix concatenated with the workspace name", func(t *testing.T) {
		ctx := &domain.Context{
			Projects: []domain.Project{{Key: testProject}},
			Defaults: domain.Options{BranchPrefix: ptr("feature/")},
		}
		r := domain.Resolver{Context: ctx}

		got, err := r.DestBranch(testProject, domain.Vars{Workspace: "payments-fix"})
		if err != nil {
			t.Fatalf("DestBranch() unexpected error: %v", err)
		}
		if got.From != domain.LayerContext {
			t.Fatalf("DestBranch().From = %v, want LayerContext", got.From)
		}
		if string(got.Value) != "feature/payments-fix" {
			t.Fatalf("DestBranch().Value = %q, want %q", got.Value, "feature/payments-fix")
		}
	})

	t.Run("a prefix already ending in a slash is not doubled up", func(t *testing.T) {
		ctx := &domain.Context{
			Projects: []domain.Project{{Key: testProject}},
			Defaults: domain.Options{BranchPrefix: ptr("hotfix/")},
		}
		r := domain.Resolver{Context: ctx}

		got, err := r.DestBranch(testProject, domain.Vars{Workspace: "payments-fix"})
		if err != nil {
			t.Fatalf("DestBranch() unexpected error: %v", err)
		}
		if string(got.Value) != "hotfix/payments-fix" {
			t.Fatalf("DestBranch().Value = %q, want %q (no doubled slash)", got.Value, "hotfix/payments-fix")
		}
	})

	t.Run("a prefix without a trailing slash gets exactly one separator", func(t *testing.T) {
		ctx := &domain.Context{
			Projects: []domain.Project{{Key: testProject}},
			Defaults: domain.Options{BranchPrefix: ptr("feature")},
		}
		r := domain.Resolver{Context: ctx}

		got, err := r.DestBranch(testProject, domain.Vars{Workspace: "test3"})
		if err != nil {
			t.Fatalf("DestBranch() unexpected error: %v", err)
		}
		if string(got.Value) != "feature/test3" {
			t.Fatalf("DestBranch().Value = %q, want %q", got.Value, "feature/test3")
		}
	})

	t.Run("a dest_branch template's {prefix} is the resolved, normalized prefix", func(t *testing.T) {
		ctx := &domain.Context{
			Projects: []domain.Project{{Key: testProject, DestBranch: "{prefix}{workspace}"}},
			Defaults: domain.Options{BranchPrefix: ptr("feature")},
		}
		r := domain.Resolver{Context: ctx}

		got, err := r.DestBranch(testProject, domain.Vars{Workspace: "test3"})
		if err != nil {
			t.Fatalf("DestBranch() unexpected error: %v", err)
		}
		if string(got.Value) != "feature/test3" {
			t.Fatalf("DestBranch().Value = %q, want %q", got.Value, "feature/test3")
		}
	})

	t.Run("a workspace name that would produce an invalid branch name errors", func(t *testing.T) {
		ctx := &domain.Context{Projects: []domain.Project{{Key: testProject}}}
		r := domain.Resolver{Context: ctx}

		_, err := r.DestBranch(testProject, domain.Vars{Workspace: "-bad"})
		if err == nil {
			t.Fatal("DestBranch() error = nil, want an invalid-branch-name error")
		}
		if !errors.Is(err, domain.ErrInvalidBranchName) {
			t.Fatalf("DestBranch() error = %v, want it to wrap domain.ErrInvalidBranchName", err)
		}
	})

	t.Run("an explicit branch flag still wins over any prefix", func(t *testing.T) {
		ctx := &domain.Context{
			Projects: []domain.Project{{Key: testProject}},
			Defaults: domain.Options{BranchPrefix: ptr("feature/")},
		}
		r := domain.Resolver{Context: ctx}

		got, err := r.DestBranch(testProject, domain.Vars{Workspace: "payments-fix", Branch: "explicit-branch"})
		if err != nil {
			t.Fatalf("DestBranch() unexpected error: %v", err)
		}
		if got.From != domain.LayerFlag {
			t.Fatalf("DestBranch().From = %v, want LayerFlag", got.From)
		}
		if string(got.Value) != "explicit-branch" {
			t.Fatalf("DestBranch().Value = %q, want %q", got.Value, "explicit-branch")
		}
	})
}

// TestResolver_BranchPrefix exercises the branch_prefix precedence chain in
// isolation (design.md §6 Chain B), the same shape as every other
// Options-backed key (Remote, BaseBranch, ...). Nothing read this value
// before this defect's fix even though every layer already carries it
// (context-management spec: "branch_prefix" is a required Context field).
func TestResolver_BranchPrefix(t *testing.T) {
	t.Run("builtin default is empty", func(t *testing.T) {
		r := domain.Resolver{}
		got := r.BranchPrefix(testProject)
		if got.From != domain.LayerBuiltin || got.Value != "" {
			t.Fatalf("BranchPrefix() = %+v, want empty builtin", got)
		}
	})

	t.Run("context default wins over builtin", func(t *testing.T) {
		ctx := &domain.Context{Defaults: domain.Options{BranchPrefix: ptr("release/")}}
		r := domain.Resolver{Context: ctx}
		got := r.BranchPrefix(testProject)
		if got.From != domain.LayerContext || got.Value != "release/" {
			t.Fatalf("BranchPrefix() = %+v, want {release/ context}", got)
		}
	})

	t.Run("project override wins over context", func(t *testing.T) {
		ctx := &domain.Context{Projects: []domain.Project{{Key: testProject, Options: domain.Options{BranchPrefix: ptr("proj/")}}}, Defaults: domain.Options{BranchPrefix: ptr("release/")}}
		r := domain.Resolver{Context: ctx}
		got := r.BranchPrefix(testProject)
		if got.From != domain.LayerProject || got.Value != "proj/" {
			t.Fatalf("BranchPrefix() = %+v, want {proj/ project}", got)
		}
	})

	t.Run("flag wins over everything", func(t *testing.T) {
		ctx := &domain.Context{Projects: []domain.Project{{Key: testProject, Options: domain.Options{BranchPrefix: ptr("proj/")}}}}
		r := domain.Resolver{Context: ctx, Flags: domain.Options{BranchPrefix: ptr("flag/")}}
		got := r.BranchPrefix(testProject)
		if got.From != domain.LayerFlag || got.Value != "flag/" {
			t.Fatalf("BranchPrefix() = %+v, want {flag/ flag}", got)
		}
	})
}
