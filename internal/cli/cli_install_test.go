// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func TestCLI_InstallYesPlacesTheBinaryAndUpdatesPath(t *testing.T) {
	fx := newFixture(t)
	fs := portstest.NewFakeFS(t)
	home := string(fs.Paths().Home)
	fx.RT.InstallDeps = app.InstallDeps{
		FS: fs, GOOS: "linux",
		Getenv:         func(k string) string { return map[string]string{"PATH": "/usr/bin", "SHELL": "/bin/bash"}[k] },
		Executable:     func() (string, error) { return "/tmp/dl/wspace", nil },
		ReadExecutable: func(string) ([]byte, error) { return []byte("bin"), nil },
		SameFile:       func(a, b string) bool { return a == b },
		Rename: func(from, to domain.Path) error {
			data, _ := fs.ReadFile(from)
			_ = fs.WriteFile(to, data, 0o755)
			return fs.RemoveAll(from)
		},
	}

	stdout, stderr, code := run(fx.RT, "", "install", "--yes")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{
		"wspace installed to " + home + "/.local/bin/wspace",
		"added " + home + "/.local/bin to your PATH in " + home + "/.bashrc",
		"shell integration added to " + home + "/.bashrc",
		"open a new terminal to use wspace",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}

	fx.RT.InstallDeps.Reporter = nil // Execute binds a reporter to each run's writers
	stdout, _, code = run(fx.RT, "", "install", "--uninstall")
	if code != 0 || !strings.Contains(stdout, "removed: "+home+"/.local/bin/wspace") || !strings.Contains(stdout, "removed the PATH entry from "+home+"/.bashrc") {
		t.Fatalf("uninstall stdout = %q", stdout)
	}
}

func TestCLI_InstallJSON(t *testing.T) {
	fx := newFixture(t)
	fs := portstest.NewFakeFS(t)
	fx.RT.InstallDeps = app.InstallDeps{
		FS: fs, GOOS: "windows", UserPath: &portstest.FakeUserPath{},
		Getenv: func(k string) string {
			return map[string]string{"PATH": `C:\Windows`, "LOCALAPPDATA": `C:\Users\t\AppData\Local`}[k]
		},
		Executable:     func() (string, error) { return `C:\Users\t\Downloads\wspace.exe`, nil },
		ReadExecutable: func(string) ([]byte, error) { return []byte("bin"), nil },
		SameFile:       func(a, b string) bool { return false },
		Rename: func(from, to domain.Path) error {
			data, _ := fs.ReadFile(from)
			_ = fs.WriteFile(to, data, 0o755)
			return fs.RemoveAll(from)
		},
		Prompter: portstest.NewScriptedPrompter(t), // never asked with --json
	}

	stdout, stderr, code := run(fx.RT, "", "install", "--yes", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var res struct {
		Installed   bool     `json:"installed"`
		Path        string   `json:"path"`
		PathUpdated bool     `json:"path_updated"`
		PathTargets []string `json:"path_targets"`
		NewTerminal bool     `json:"new_terminal"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	if !res.Installed || res.Path != `C:\Users\t\AppData\Local\Programs\wspace\wspace.exe` || !res.PathUpdated || res.PathTargets[0] != `HKCU\Environment\Path` || !res.NewTerminal {
		t.Fatalf("json = %s", stdout)
	}
}
