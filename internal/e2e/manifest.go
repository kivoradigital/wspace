// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// manifestSnapshot mirrors the on-disk shape of "<workspace>/.wspace/workspace.
// yaml" (internal/adapters/configstore/manifest.go's manifestYAML) closely
// enough for this package's assertions. It is a deliberately independent
// copy, not an import of that unexported type: this package's whole point
// is to check what the real binary wrote to real disk, not to trust the
// same code path that wrote it.
type manifestSnapshot struct {
	SchemaVersion int `yaml:"schema_version"`
	Workspace     struct {
		Name    string `yaml:"name"`
		Context string `yaml:"context"`
		Branch  string `yaml:"branch"`
		Repos   []struct {
			Alias     string `yaml:"alias"`
			Project   string `yaml:"project"`
			SourceDir string `yaml:"source_dir"`
			Branch    string `yaml:"branch"`
		} `yaml:"repos"`
	} `yaml:"workspace"`
	EnvCopies []string `yaml:"env_copies"`
}

// ReadManifest reads and parses "<wsRoot>/.wspace/workspace.yaml" directly
// from disk, failing the test if it is missing or malformed.
func ReadManifest(t *testing.T, wsRoot string) manifestSnapshot {
	t.Helper()
	path := filepath.Join(wsRoot, ".wspace", "workspace.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest %s: %v", path, err)
	}
	var m manifestSnapshot
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest %s: %v", path, err)
	}
	return m
}

// contextConfigSnapshot mirrors the on-disk shape of
// "<config>/wspace/contexts/<name>/config.yaml"
// (internal/adapters/configstore/context.go's contextYAML) closely enough
// for this package's assertions — a deliberately independent copy, for
// the same reason manifestSnapshot above is: this package's whole point is
// to check what the real binary actually persisted, never to trust the
// same code path that wrote it.
type contextConfigSnapshot struct {
	SchemaVersion  int      `yaml:"schema_version"`
	WorkspacesRoot string   `yaml:"workspaces_root"`
	ProjectsRoot   string   `yaml:"projects_root"`
	IgnorePatterns []string `yaml:"ignore_patterns"`
	Defaults       struct {
		BaseBranch        string   `yaml:"base_branch"`
		BranchPrefix      string   `yaml:"branch_prefix"`
		CopyEnvDefault    bool     `yaml:"copy_env_default"`
		FetchBeforeCreate bool     `yaml:"fetch_before_create"`
		EnvPruneDirs      []string `yaml:"env_prune_dirs"`
	} `yaml:"defaults"`
	Projects []struct {
		Key         string `yaml:"key"`
		SourceDir   string `yaml:"source_dir"`
		DestBranch  string `yaml:"dest_branch"`
		WorktreeDir string `yaml:"worktree_dir"`
	} `yaml:"projects"`
}

// ReadContextConfig reads and parses "<xdg>/wspace/contexts/<name>/config.yaml"
// directly from disk, failing the test if it is missing or malformed. xdg
// is the fixture's own XDG_CONFIG_HOME (Fixture.XDG), matching exactly
// what the real binary was given as its config root.
func ReadContextConfig(t *testing.T, xdg, name string) contextConfigSnapshot {
	t.Helper()
	path := filepath.Join(xdg, "wspace", "contexts", name, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read context config %s: %v", path, err)
	}
	var c contextConfigSnapshot
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatalf("parse context config %s: %v", path, err)
	}
	return c
}

// PathExists reports whether path exists on disk, failing the test on any
// error other than "does not exist".
func PathExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("stat %s: %v", path, err)
	return false
}
