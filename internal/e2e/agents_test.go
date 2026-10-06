// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type agentsResult struct {
	Skill struct {
		Name   string `json:"name"`
		Dir    string `json:"dir"`
		Origin string `json:"origin"`
	} `json:"skill"`
	Changes []struct {
		Agent string `json:"agent"`
		Kind  string `json:"kind"`
		Path  string `json:"path"`
	} `json:"changes"`
}

// runBin runs bin with an isolated HOME and a PATH holding no agent CLI,
// so detection depends only on the fake agent directories under home.
func runBin(t *testing.T, bin, home string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config")}
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return out.String(), errBuf.String(), exitErr.ExitCode()
	}
	if err != nil {
		t.Fatalf("run %s %v: %v", bin, args, err)
	}
	return out.String(), errBuf.String(), 0
}

func decodeAgents(t *testing.T, stdout string) agentsResult {
	t.Helper()
	var r agentsResult
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	return r
}

func copyFileT(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		t.Fatal(err)
	}
}

// TestAgents_BundledEngineLinksIntoTheAppBundle runs the real binary from
// inside an app bundle layout: every agent link must point into that
// bundle, the legacy skill is moved aside (not deleted), and uninstall
// removes only the links.
func TestAgents_BundledEngineLinksIntoTheAppBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("app bundles are macOS-only")
	}
	fx := NewFixture(t)
	bundle := filepath.Join(fx.Root, "Apps", "wspace.app")
	bin := filepath.Join(bundle, "Contents", "Helpers", "wspace")
	copyFileT(t, fx.Bin, bin, 0o755)
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	skillSrc := filepath.Join(root, "skills", "wspace-workspaces", "SKILL.md")
	copyFileT(t, skillSrc, filepath.Join(bundle, "Contents", "Resources", "skills", "wspace-workspaces", "SKILL.md"), 0o644)

	legacy := filepath.Join(fx.Home, ".claude", "skills", "ws-workspaces")
	copyFileT(t, skillSrc, filepath.Join(legacy, "SKILL.md"), 0o644)
	for _, d := range []string{".gemini", ".config/opencode"} {
		if err := os.MkdirAll(filepath.Join(fx.Home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	stdout, stderr, code := runBin(t, bin, fx.Home, "agents", "install", "--disable-legacy", "--json")
	if code != 0 {
		t.Fatalf("install exit %d: %s %s", code, stdout, stderr)
	}
	res := decodeAgents(t, stdout)
	realBundle, _ := filepath.EvalSymlinks(bundle)
	wantDir := filepath.Join(realBundle, "Contents", "Resources", "skills", "wspace-workspaces")
	if res.Skill.Origin != "bundle" || res.Skill.Dir != filepath.ToSlash(wantDir) {
		t.Fatalf("skill = %+v, want the bundle dir %s", res.Skill, wantDir)
	}
	for _, d := range []string{".claude/skills", ".gemini/skills", ".config/opencode/skills"} {
		link := filepath.Join(fx.Home, d, "wspace-workspaces")
		target, err := os.Readlink(link)
		if err != nil || target != wantDir {
			t.Fatalf("readlink %s = %q, %v; want %s", link, target, err, wantDir)
		}
		if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
			t.Fatalf("SKILL.md not readable through %s: %v", link, err)
		}
	}
	if _, err := os.Stat(filepath.Join(fx.Home, ".claude", "skills-disabled", "ws-workspaces", "SKILL.md")); err != nil {
		t.Fatalf("legacy skill not moved aside: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(fx.Home, ".cursor")); !os.IsNotExist(err) {
		t.Fatal("an undetected agent must be left alone")
	}

	stdout, stderr, code = runBin(t, bin, fx.Home, "agents", "uninstall", "--json")
	if code != 0 {
		t.Fatalf("uninstall exit %d: %s %s", code, stdout, stderr)
	}
	if _, err := os.Lstat(filepath.Join(fx.Home, ".claude", "skills", "wspace-workspaces")); !os.IsNotExist(err) {
		t.Fatal("uninstall must remove the link")
	}
	if _, err := os.Stat(filepath.Join(bundle, "Contents", "Resources", "skills", "wspace-workspaces", "SKILL.md")); err != nil {
		t.Fatal("uninstall must not touch the bundle")
	}
}

// TestAgents_StandaloneBinaryExtractsItsEmbeddedSkill covers Linux,
// Windows and a CLI installed without the app: the skill comes from the
// binary itself, extracted under the user data directory.
func TestAgents_StandaloneBinaryExtractsItsEmbeddedSkill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need Developer Mode on Windows")
	}
	fx := NewFixture(t)
	if err := os.MkdirAll(filepath.Join(fx.Home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runBin(t, fx.Bin, fx.Home, "agents", "install", "--json")
	if code != 0 {
		t.Fatalf("install exit %d: %s %s", code, stdout, stderr)
	}
	res := decodeAgents(t, stdout)
	extracted := filepath.Join(fx.Home, ".local", "share", "wspace", "skills", "wspace-workspaces")
	if res.Skill.Origin != "embedded" || res.Skill.Dir != filepath.ToSlash(extracted) {
		t.Fatalf("skill = %+v, want %s", res.Skill, extracted)
	}
	got, err := os.ReadFile(filepath.Join(fx.Home, ".claude", "skills", "wspace-workspaces", "SKILL.md"))
	if err != nil || !strings.Contains(string(got), "name: wspace-workspaces") {
		t.Fatalf("SKILL.md through the link = %q, %v", got, err)
	}
}

// TestAgents_ClaudeCodeScopesAreReadWithoutWriting reads user- and
// local-scope registrations from a real ~/.claude.json: status and a dry
// run never modify it.
func TestAgents_ClaudeCodeScopesAreReadWithoutWriting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX paths in the fixture")
	}
	fx := NewFixture(t)
	project := filepath.ToSlash(filepath.Join(fx.Root, "proj"))
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(fx.Home, ".claude.json")
	content := `{"mcpServers":{"wspace":{"command":"/nowhere/wspace"}},"projects":{"` + project + `":{"mcpServers":{"wspace":{"command":"/nowhere/old"}}}}}`
	if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(cfg)

	stdout, stderr, code := runBin(t, fx.Bin, fx.Home, "agents", "status", "--agent", "claude-code", "--json")
	if code != 0 {
		t.Fatalf("status exit %d: %s", code, stderr)
	}
	var st struct {
		Agents []struct {
			MCP           string `json:"mcp"`
			Registrations []struct {
				Scope, Project, Command string
				Stale                   bool
			} `json:"mcp_registrations"`
			Duplicate bool `json:"mcp_duplicate"`
			Stale     bool `json:"mcp_stale"`
		} `json:"agents"`
	}
	if err := json.Unmarshal([]byte(stdout), &st); err != nil {
		t.Fatal(err)
	}
	a := st.Agents[0]
	if a.MCP != "registered" || !a.Duplicate || !a.Stale || len(a.Registrations) != 2 || a.Registrations[1].Project != project {
		t.Fatalf("agent = %+v", a)
	}

	stdout, stderr, code = runBin(t, fx.Bin, fx.Home, "agents", "mcp-clean", "--dry-run", "--json")
	if code != 0 || !strings.Contains(stdout, `"mcp_duplicate_would_remove"`) {
		t.Fatalf("dry run exit %d: %s %s", code, stdout, stderr)
	}
	after, _ := os.Stat(cfg)
	data, _ := os.ReadFile(cfg)
	if string(data) != content || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("status and a dry run must not modify ~/.claude.json")
	}
}
