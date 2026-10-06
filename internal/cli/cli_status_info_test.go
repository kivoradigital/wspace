// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TestCLI_Status_JSONContract covers tasks.md 4b.9: status --json's schema
// (cli-surface spec: "alias, branch, ahead, behind, dirty" per repo).
func TestCLI_Status_JSONContract(t *testing.T) {
	fx := newFixture(t)

	wsRoot := domain.Path("/fixture/workspaces/ws1")
	fx.Store.PutManifest(wsRoot, domain.Manifest{
		Workspace: domain.Workspace{
			Name: "ws1",
			Root: wsRoot,
			Repos: []domain.RepoEntry{
				{Alias: "svc", SourceDir: "/fixture/src/svc", Branch: "feature-x"},
			},
		},
	})
	fx.Git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feature-x", false, nil }
	fx.Git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: '?', Y: '?', RelPath: "scratch.txt"}}, nil
	}
	fx.Git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 2, 1, nil }

	stdout, stderr, code := run(fx.RT, "", "status", "ws1", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	compareGolden(t, "status.json.golden", stdout)
}

// TestCLI_Info_JSONContract covers tasks.md 4b.9: info --json must emit
// context_name and every resolved option key with its winning Layer (ADR
// D6), asserted here against an explicit non-builtin override so the
// "From" field is meaningfully exercised.
func TestCLI_Info_JSONContract(t *testing.T) {
	fx := newFixture(t)
	base := domain.BranchName("develop")
	fx.Store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: "/fixture/workspaces",
		Defaults:       domain.Options{BaseBranch: &base},
	})

	stdout, stderr, code := run(fx.RT, "", "info", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	compareGolden(t, "info.json.golden", stdout)
}

// TestCLI_Info_HumanOutputShowsLayer covers info's human-readable
// rendering carrying the same provenance ADR D6 requires.
func TestCLI_Info_HumanOutputShowsLayer(t *testing.T) {
	fx := newFixture(t)
	base := domain.BranchName("develop")
	fx.Store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: "/fixture/workspaces",
		Defaults:       domain.Options{BaseBranch: &base},
	})

	stdout, stderr, code := run(fx.RT, "", "info")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	compareGolden(t, "info.golden", stdout)
}

// TestCLI_Status_MissingBase: a repo without the workspace base renders
// with an explanatory note instead of failing the command, and --json
// carries base_branch/base_missing (additive, omitted when unset).
func TestCLI_Status_MissingBase(t *testing.T) {
	fx := newFixture(t)
	develop := domain.BranchName("develop")
	wsRoot := domain.Path("/fixture/workspaces/ws1")
	fx.Store.PutManifest(wsRoot, domain.Manifest{
		Workspace: domain.Workspace{
			Name: "ws1", Root: wsRoot, Options: domain.Options{BaseBranch: &develop},
			Repos: []domain.RepoEntry{{Alias: "hub", SourceDir: "/fixture/src/hub", Branch: "feature-x"}},
		},
	})
	fx.Git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feature-x", false, nil }
	fx.Git.AheadBehindFunc = func(domain.Path, string) (int, int, error) {
		return 0, 0, domain.NewOpError("git.ahead_behind", domain.CodeGitFailed, "", "no upstream", nil)
	}
	fx.Git.ResolveBaseFunc = func(domain.Path, string, domain.BranchName) (ports.BaseRef, error) {
		return ports.BaseRef{Ref: "HEAD"}, nil
	}

	stdout, stderr, code := run(fx.RT, "", "status", "ws1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "base branch develop not found in hub") {
		t.Errorf("stdout = %q, want the missing base explained", stdout)
	}

	stdout, _, code = run(fx.RT, "", "status", "ws1", "--json")
	if code != 0 || !strings.Contains(stdout, `"base_branch": "develop"`) || !strings.Contains(stdout, `"base_missing": true`) {
		t.Errorf("status --json = %q (exit %d), want base_branch and base_missing", stdout, code)
	}
}

// TestCLI_Info_ShowsConfigDir: info (human and --json) names the
// configuration directory in use, so a development checkout running with
// WSPACE_CONFIG_HOME can tell at a glance which configuration it reads.
func TestCLI_Info_ShowsConfigDir(t *testing.T) {
	fx := newFixture(t)
	want := string(fx.FS.Paths().Config)

	stdout, stderr, code := run(fx.RT, "", "info")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "config dir: "+want+"\n") {
		t.Errorf("info = %q, want a %q line", stdout, "config dir: "+want)
	}

	stdout, stderr, code = run(fx.RT, "", "info", "--json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, `"config_dir": "`+want+`"`) {
		t.Errorf("info --json = %q, want config_dir %q", stdout, want)
	}
}
