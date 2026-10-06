// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// legacyScript returns the path of the legacy bash tool (ws_old/ws at the
// monorepo root), skipping the test when it or bash is unavailable.
func legacyScript(t *testing.T) string {
	t.Helper()
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "..", "..", "ws_old", "ws")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("legacy script not found at %s", script)
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found on PATH")
	}
	return script
}

// runLegacy runs the legacy bash tool hermetically: WS_CONFIG pins its
// configuration file explicitly (it never reads the user config then), and
// HOME/XDG_CONFIG_HOME point at the fixture so nothing outside the
// fixture's temp root can be read or written. It runs from the fixture
// root, so no ws.config above a real directory can be discovered either.
func runLegacy(t *testing.T, fx *Fixture, config, stdin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{legacyScript(t)}, args...)...)
	cmd.Dir = fx.Root
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + fx.Home,
		"XDG_CONFIG_HOME=" + fx.XDG,
		"WS_CONFIG=" + config,
		"NO_COLOR=1",
	}
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("legacy ws %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// hashTree fingerprints every file under dir (relative path + bytes).
func hashTree(t *testing.T, dir string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			entries = append(entries, "d "+rel)
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		entries = append(entries, "f "+rel+" "+hex.EncodeToString(sum[:]))
		return nil
	})
	if err != nil {
		t.Fatalf("hash %s: %v", dir, err)
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}

type listItem struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	ProjectCount int    `json:"project_count"`
	Legacy       bool   `json:"legacy"`
	Error        string `json:"error"`
}

func listJSON(t *testing.T, fx *Fixture) map[string]listItem {
	t.Helper()
	stdout, _ := fx.MustRun("", "list", "--json")
	var items []listItem
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("list --json %q: %v", stdout, err)
	}
	out := map[string]listItem{}
	for _, i := range items {
		out[i.Name] = i
	}
	return out
}

// TestLegacyWorkspaces_ImportAdoptsThemWithoutTouchingTheLegacyFiles builds
// workspaces with the legacy bash tool itself, imports its configuration
// with the real wspace binary, and checks that the workspaces are adopted
// (listed, inspectable, protected by destroy's safety checks) while their
// ".ws/" folders stay byte-identical and the legacy tool keeps working.
func TestLegacyWorkspaces_ImportAdoptsThemWithoutTouchingTheLegacyFiles(t *testing.T) {
	fx := NewFixture(t)

	projectsRoot := filepath.Join(fx.Root, "projects")
	workspacesRoot := filepath.Join(fx.Root, "workspaces")
	for _, d := range []string{projectsRoot, workspacesRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	api := NewGitProject(t, projectsRoot, "Some.Api", "develop")
	NewGitProject(t, projectsRoot, "web", "develop")

	// The legacy configuration, in the exact shape the legacy tool's own
	// write_config emits.
	legacyConfig := filepath.Join(fx.Root, "legacy-config")
	cfg := strings.Join([]string{
		"# ws configuration",
		"workspaces_root = " + workspacesRoot,
		"projects_root = " + projectsRoot,
		"project_prefixes = Some",
		"base_branch = develop",
		"branch_prefix = feature",
		"copy_env_default = no",
		"fetch_before_create = no",
		"env_prune_dirs = .git,node_modules",
		"",
		"[projects]",
		"Some.Api",
		"web",
		"",
	}, "\n")
	if err := os.WriteFile(legacyConfig, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// The legacy tool creates the workspace: "all" selects every managed
	// project in its picker, the empty line confirms.
	runLegacy(t, fx, legacyConfig, "all\n\n", "create", "demo", "--no-env", "--no-fetch", "-y")
	demo := filepath.Join(workspacesRoot, "demo")
	legacyDir := filepath.Join(demo, ".ws")
	if !PathExists(t, filepath.Join(legacyDir, "workspace.conf")) {
		t.Fatalf("legacy tool did not create %s", legacyDir)
	}
	before := hashTree(t, legacyDir)

	// Import the legacy configuration; adoption runs at the end of it.
	stdin := strings.Repeat("\n", 7)
	stdout, stderr := fx.MustRun(stdin, "context", "import", "--from", legacyConfig, "--name", "legacy")
	if !strings.Contains(stdout, "adopted legacy workspaces") || !strings.Contains(stdout, "demo") {
		t.Fatalf("import summary = %q (stderr %q), want demo adopted", stdout, stderr)
	}

	m := ReadManifest(t, demo)
	if m.Workspace.Name != "demo" || m.Workspace.Context != "legacy" || m.Workspace.Branch != "feature/demo" || len(m.Workspace.Repos) != 2 {
		t.Fatalf("adopted manifest = %+v", m.Workspace)
	}
	gotRepos := map[string]string{}
	for _, r := range m.Workspace.Repos {
		gotRepos[r.Alias] = r.Project + "@" + r.Branch
	}
	if gotRepos["someapi"] != "Some.Api@feature/demo" || gotRepos["web"] != "web@feature/demo" {
		t.Fatalf("adopted repos = %v", gotRepos)
	}

	// list and status work on it.
	items := listJSON(t, fx)
	if it := items["demo"]; it.Legacy || it.ProjectCount != 2 || it.Error != "" {
		t.Fatalf("list demo = %+v, want an adopted, healthy workspace", it)
	}
	statusOut, _ := fx.MustRun("", "status", "demo", "--json")
	var status []struct {
		Alias, Branch string
		Dirty         bool
	}
	if err := json.Unmarshal([]byte(statusOut), &status); err != nil || len(status) != 2 {
		t.Fatalf("status --json = %q (%v)", statusOut, err)
	}
	for _, s := range status {
		if s.Branch != "feature/demo" || s.Dirty {
			t.Fatalf("status = %+v, want clean repos on feature/demo", status)
		}
	}

	// Adoption is idempotent: the adopted workspace is only reported as
	// already adopted by this context.
	adoptOut, _ := fx.MustRun("", "adopt-legacy", "--json")
	var again struct {
		Adopted []any                     `json:"adopted"`
		Skipped []struct{ Reason string } `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(adoptOut), &again); err != nil || len(again.Adopted) != 0 || len(again.Skipped) != 1 || again.Skipped[0].Reason != "adopt.skip.already_adopted" {
		t.Fatalf("second adopt-legacy = %q (%v), want demo reported as already adopted", adoptOut, err)
	}

	// A workspace the legacy tool creates later is listed as legacy, then
	// adopted through the rpc method.
	runLegacy(t, fx, legacyConfig, "all\n\n", "create", "later", "--no-env", "--no-fetch", "-y")
	if it := listJSON(t, fx)["later"]; !it.Legacy {
		t.Fatalf("list later = %+v, want legacy: true", it)
	}
	responses, _ := rpcSession(t, fx, `{"id":"adopt","method":"workspaces.adoptLegacy","params":{"context":"legacy"}}`)
	var rpcRes struct {
		Adopted []struct{ Name, Root string } `json:"adopted"`
	}
	if err := json.Unmarshal(mustResult(t, responses, "adopt"), &rpcRes); err != nil || len(rpcRes.Adopted) != 1 || rpcRes.Adopted[0].Name != "later" {
		t.Fatalf("workspaces.adoptLegacy = %+v (%v)", rpcRes, err)
	}

	// The legacy tool still manages the adopted workspaces.
	legacyList := runLegacy(t, fx, legacyConfig, "", "list")
	if !strings.Contains(legacyList, "demo") || !strings.Contains(legacyList, "later") {
		t.Fatalf("legacy ws list = %q, want both workspaces", legacyList)
	}

	// Destroy safety: an uncommitted change blocks an unforced destroy.
	worktree := filepath.Join(demo, "someapi")
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, code := fx.Run("y\n", "destroy", "demo"); code == 0 {
		t.Fatal("unforced destroy of a dirty adopted workspace succeeded, want it blocked")
	}
	if !PathExists(t, worktree) {
		t.Fatal("blocked destroy removed the worktree")
	}

	if after := hashTree(t, legacyDir); after != before {
		t.Fatal(".ws/ changed after import, adoption, list, status and a blocked destroy")
	}

	// A forced destroy tears the adopted workspace down through the
	// recorded source clones, like any wspace workspace.
	fx.MustRun("", "destroy", "demo", "--force")
	if PathExists(t, demo) {
		t.Fatalf("%s still exists after a forced destroy", demo)
	}
	if wt := runGit(t, api.Main, "worktree", "list", "--porcelain"); strings.Contains(wt, filepath.Join("workspaces", "demo")) {
		t.Fatalf("git still lists the destroyed worktree:\n%s", wt)
	}
}
