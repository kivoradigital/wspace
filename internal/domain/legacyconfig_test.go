// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestParseLegacyFlatConfig_FullyValid(t *testing.T) {
	src := []byte(`# a legacy flat workspace configuration
workspaces_root = /abs/path/worktrees
projects_root = /abs/path/repos
project_prefixes = Foo,bar
base_branch = develop
branch_prefix = feature
copy_env_default = yes
fetch_before_create = yes
env_prune_dirs = .git,node_modules,bin,obj,dist,build,vendor,.next,target,.venv
[projects]
Some.Project
Another.Project
`)
	got := domain.ParseLegacyFlatConfig(src)

	if got.Context.WorkspacesRoot != "/abs/path/worktrees" {
		t.Errorf("WorkspacesRoot = %q, want /abs/path/worktrees", got.Context.WorkspacesRoot)
	}
	if got.Context.ProjectsRoot != "/abs/path/repos" {
		t.Errorf("ProjectsRoot = %q, want /abs/path/repos", got.Context.ProjectsRoot)
	}
	if got.Context.Defaults.BaseBranch == nil || *got.Context.Defaults.BaseBranch != "develop" {
		t.Errorf("Defaults.BaseBranch = %v, want develop", got.Context.Defaults.BaseBranch)
	}
	if got.Context.Defaults.BranchPrefix == nil || *got.Context.Defaults.BranchPrefix != "feature" {
		t.Errorf("Defaults.BranchPrefix = %v, want feature", got.Context.Defaults.BranchPrefix)
	}
	if got.Context.Defaults.CopyEnv == nil || *got.Context.Defaults.CopyEnv != true {
		t.Errorf("Defaults.CopyEnv = %v, want true", got.Context.Defaults.CopyEnv)
	}
	if got.Context.Defaults.FetchBeforeCreate == nil || *got.Context.Defaults.FetchBeforeCreate != true {
		t.Errorf("Defaults.FetchBeforeCreate = %v, want true", got.Context.Defaults.FetchBeforeCreate)
	}
	wantPrune := []string{".git", "node_modules", "bin", "obj", "dist", "build", "vendor", ".next", "target", ".venv"}
	if len(got.Context.Defaults.EnvPruneDirs) != len(wantPrune) {
		t.Fatalf("Defaults.EnvPruneDirs = %v, want %v", got.Context.Defaults.EnvPruneDirs, wantPrune)
	}
	for i, w := range wantPrune {
		if got.Context.Defaults.EnvPruneDirs[i] != w {
			t.Errorf("EnvPruneDirs[%d] = %q, want %q", i, got.Context.Defaults.EnvPruneDirs[i], w)
		}
	}
	if got.Context.Projects != nil {
		t.Errorf("Context.Projects = %v, want nil (parser never resolves project I/O)", got.Context.Projects)
	}
	wantNames := []string{"Some.Project", "Another.Project"}
	if len(got.ProjectNames) != len(wantNames) {
		t.Fatalf("ProjectNames = %v, want %v", got.ProjectNames, wantNames)
	}
	for i, w := range wantNames {
		if got.ProjectNames[i] != w {
			t.Errorf("ProjectNames[%d] = %q, want %q", i, got.ProjectNames[i], w)
		}
	}

	// project_prefixes is recognized but never has a destination.
	foundNotImported := false
	for _, issue := range got.Issues {
		if issue.Key == "project_prefixes" && issue.Reason == domain.LegacyReasonNotImported {
			foundNotImported = true
		}
	}
	if !foundNotImported {
		t.Errorf("Issues = %+v, want a not-imported issue for project_prefixes", got.Issues)
	}
}

func TestParseLegacyFlatConfig_UnknownKeyReported(t *testing.T) {
	src := []byte("workspaces_root = /abs/root\nsome_future_key = whatever\n[projects]\n")
	got := domain.ParseLegacyFlatConfig(src)

	found := false
	for _, issue := range got.Issues {
		if issue.Key == "some_future_key" && issue.Reason == domain.LegacyReasonUnknownKey {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want an unknown-key issue for some_future_key", got.Issues)
	}
	// An unknown key must never abort the whole parse.
	if got.Context.WorkspacesRoot != "/abs/root" {
		t.Errorf("WorkspacesRoot = %q, want /abs/root despite the unknown key", got.Context.WorkspacesRoot)
	}
}

func TestParseLegacyFlatConfig_MissingProjectsSection(t *testing.T) {
	src := []byte("workspaces_root = /abs/root\n")
	got := domain.ParseLegacyFlatConfig(src)

	if got.ProjectNames != nil {
		t.Errorf("ProjectNames = %v, want nil", got.ProjectNames)
	}
	found := false
	for _, issue := range got.Issues {
		if issue.Reason == domain.LegacyReasonMissingProjectsSection {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want a missing-projects-section issue", got.Issues)
	}
}

func TestParseLegacyFlatConfig_InvalidMappedValueTreatedAsAbsent(t *testing.T) {
	src := []byte("copy_env_default = maybe\nbase_branch = develop\n[projects]\n")
	got := domain.ParseLegacyFlatConfig(src)

	if got.Context.Defaults.CopyEnv != nil {
		t.Errorf("Defaults.CopyEnv = %v, want nil (invalid value never invents a default)", got.Context.Defaults.CopyEnv)
	}
	found := false
	for _, issue := range got.Issues {
		if issue.Key == "copy_env_default" && issue.Reason == domain.LegacyReasonInvalidValue {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want an invalid-value issue for copy_env_default", got.Issues)
	}
	// A sibling valid key must still parse fine.
	if got.Context.Defaults.BaseBranch == nil || *got.Context.Defaults.BaseBranch != "develop" {
		t.Errorf("Defaults.BaseBranch = %v, want develop", got.Context.Defaults.BaseBranch)
	}
}

func TestParseLegacyFlatConfig_YesNoCaseInsensitive(t *testing.T) {
	src := []byte("copy_env_default = YES\nfetch_before_create = No\n[projects]\n")
	got := domain.ParseLegacyFlatConfig(src)

	if got.Context.Defaults.CopyEnv == nil || *got.Context.Defaults.CopyEnv != true {
		t.Errorf("Defaults.CopyEnv = %v, want true (case-insensitive yes)", got.Context.Defaults.CopyEnv)
	}
	if got.Context.Defaults.FetchBeforeCreate == nil || *got.Context.Defaults.FetchBeforeCreate != false {
		t.Errorf("Defaults.FetchBeforeCreate = %v, want false (case-insensitive no)", got.Context.Defaults.FetchBeforeCreate)
	}
}

func TestParseLegacyFlatConfig_CommentsAndBlankLines(t *testing.T) {
	src := []byte("\n# a comment\n\nbase_branch = main\n\n# another\n[projects]\n\n# comment inside projects\nOne\n\nTwo\n")
	got := domain.ParseLegacyFlatConfig(src)

	if got.Context.Defaults.BaseBranch == nil || *got.Context.Defaults.BaseBranch != "main" {
		t.Errorf("Defaults.BaseBranch = %v, want main", got.Context.Defaults.BaseBranch)
	}
	want := []string{"One", "Two"}
	if len(got.ProjectNames) != len(want) {
		t.Fatalf("ProjectNames = %v, want %v", got.ProjectNames, want)
	}
}

func TestParseLegacyFlatConfig_MalformedLineReported(t *testing.T) {
	src := []byte("this line has no equals sign\nbase_branch = main\n[projects]\n")
	got := domain.ParseLegacyFlatConfig(src)

	found := false
	for _, issue := range got.Issues {
		if issue.Reason == domain.LegacyReasonMalformedLine {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want a malformed-line issue", got.Issues)
	}
	if got.Context.Defaults.BaseBranch == nil || *got.Context.Defaults.BaseBranch != "main" {
		t.Errorf("Defaults.BaseBranch = %v, want main despite the malformed line", got.Context.Defaults.BaseBranch)
	}
}

func TestParseLegacyFlatConfig_DuplicateKeyLastWins(t *testing.T) {
	src := []byte("base_branch = main\nbase_branch = develop\n[projects]\n")
	got := domain.ParseLegacyFlatConfig(src)

	if got.Context.Defaults.BaseBranch == nil || *got.Context.Defaults.BaseBranch != "develop" {
		t.Errorf("Defaults.BaseBranch = %v, want develop (last duplicate wins)", got.Context.Defaults.BaseBranch)
	}
}

func TestParseLegacyFlatConfig_InvalidAbsolutePathTreatedAsAbsent(t *testing.T) {
	src := []byte("workspaces_root = relative/not/absolute\n[projects]\n")
	got := domain.ParseLegacyFlatConfig(src)

	if got.Context.WorkspacesRoot != "" {
		t.Errorf("WorkspacesRoot = %q, want empty (invalid path never invents a default)", got.Context.WorkspacesRoot)
	}
	found := false
	for _, issue := range got.Issues {
		if issue.Key == "workspaces_root" && issue.Reason == domain.LegacyReasonInvalidValue {
			found = true
		}
	}
	if !found {
		t.Errorf("Issues = %+v, want an invalid-value issue for workspaces_root", got.Issues)
	}
}
