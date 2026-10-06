// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package agenthost_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/agenthost"
)

// fakeFiles is a fake filesystem for Resolver: native paths of regular
// files, case-insensitive on Windows like NTFS, and whether each is
// executable (Unix only).
type fakeFiles struct {
	goos  string
	files map[string]bool
}

func (f fakeFiles) stat(p string) (bool, bool) {
	for k, exec := range f.files {
		if k == p || (f.goos == "windows" && strings.EqualFold(k, p)) {
			return true, exec
		}
	}
	return false, false
}

func windowsResolver(env map[string]string, files ...string) agenthost.Resolver {
	ff := fakeFiles{goos: "windows", files: map[string]bool{}}
	for _, f := range files {
		ff.files[f] = true
	}
	return agenthost.Resolver{
		GOOS:   "windows",
		Getenv: func(k string) string { return env[k] },
		Stat:   ff.stat,
	}
}

const (
	npmInternalExe = `C:\Program Files\nodejs\node_modules\@anthropic-ai\claude-code\bin\claude.exe`
	npmShim        = `C:\Users\Tester\AppData\Roaming\npm\claude.cmd`
	nativeClaude   = `C:\Users\Tester\.local\bin\claude.exe`
	comSpec        = `C:\Windows\system32\cmd.exe`
)

func TestResolver_WindowsSkipsNodeModulesAndRunsTheShimThroughCmd(t *testing.T) {
	env := map[string]string{
		"PATH":    `C:\Program Files\nodejs\node_modules\@anthropic-ai\claude-code\bin;C:\Windows\system32;C:\Users\Tester\AppData\Roaming\npm`,
		"PATHEXT": ".COM;.EXE;.BAT;.CMD;.VBS;.JS",
		"ComSpec": comSpec,
	}
	r := windowsResolver(env, npmInternalExe, npmShim, comSpec)

	got, ok := r.LookPath("claude")
	if !ok || got != npmShim {
		t.Fatalf("LookPath = %q, %v; want the npm shim %q", got, ok, npmShim)
	}

	inv, err := r.Command("claude", []string{"mcp", "add", "--scope", "user", "wspace", "--", `C:\Users\Tester\AppData\Local\Programs\wspace\wspace.exe`, "mcp", "serve"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Path != comSpec {
		t.Fatalf("Path = %q, want cmd.exe", inv.Path)
	}
	want := comSpec + ` /d /v:off /s /c ""` + npmShim + `" "mcp" "add" "--scope" "user" "wspace" "--" "C:\Users\Tester\AppData\Local\Programs\wspace\wspace.exe" "mcp" "serve""`
	if inv.CmdLine != want {
		t.Fatalf("CmdLine =\n%s\nwant\n%s", inv.CmdLine, want)
	}
}

func TestResolver_WindowsPrefersTheNativeInstallerInPathOrder(t *testing.T) {
	env := map[string]string{
		"PATH":    `C:\Users\Tester\.local\bin;C:\Users\Tester\AppData\Roaming\npm`,
		"PATHEXT": ".COM;.EXE;.BAT;.CMD",
	}
	r := windowsResolver(env, nativeClaude, npmShim)

	inv, err := r.Command("claude", []string{"mcp", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Path != nativeClaude || inv.CmdLine != "" || strings.Join(inv.Args, " ") != "mcp list" {
		t.Fatalf("Command = %+v; want the native exe run directly", inv)
	}
}

func TestResolver_WindowsPathextOrderWithinOneDirectory(t *testing.T) {
	env := map[string]string{"PATH": `C:\tools`, "PATHEXT": ".CMD;.EXE"}
	r := windowsResolver(env, `C:\tools\codex.exe`, `C:\tools\codex.cmd`)
	if got, _ := r.LookPath("codex"); got != `C:\tools\codex.cmd` {
		t.Fatalf("LookPath = %q, want PATHEXT order (.CMD first)", got)
	}
	// Without PATHEXT the cmd.exe default applies: .COM;.EXE;.BAT;.CMD.
	r = windowsResolver(map[string]string{"PATH": `C:\tools`}, `C:\tools\codex.exe`, `C:\tools\codex.cmd`)
	if got, _ := r.LookPath("codex"); got != `C:\tools\codex.exe` {
		t.Fatalf("LookPath (default PATHEXT) = %q", got)
	}
}

func TestResolver_WindowsFallsBackToKnownUserLocations(t *testing.T) {
	env := map[string]string{
		"PATH":        `C:\Windows\system32`,
		"APPDATA":     `C:\Users\Tester\AppData\Roaming`,
		"USERPROFILE": `C:\Users\Tester`,
	}
	r := windowsResolver(env, npmShim)
	if got, ok := r.LookPath("claude"); !ok || got != npmShim {
		t.Fatalf("LookPath = %q, %v; want the npm shim from %%APPDATA%%\\npm", got, ok)
	}
	r = windowsResolver(env, nativeClaude, npmShim)
	if got, _ := r.LookPath("claude"); got != nativeClaude {
		t.Fatalf("LookPath = %q; want the native installer first", got)
	}
	r = windowsResolver(env, `C:\Users\Tester\AppData\Roaming\npm\gemini.cmd`)
	if got, ok := r.LookPath("gemini"); !ok || !strings.HasSuffix(got, `gemini.cmd`) {
		t.Fatalf("LookPath(gemini) = %q, %v", got, ok)
	}
	r = windowsResolver(env)
	if _, ok := r.LookPath("claude"); ok {
		t.Fatal("found claude with nothing installed")
	}
}

func TestResolver_WindowsBatchQuoting(t *testing.T) {
	env := map[string]string{"PATH": `C:\npm`, "SystemRoot": `C:\Windows`}
	r := windowsResolver(env, `C:\npm\gemini.cmd`)

	inv, err := r.Command("gemini", []string{`WSPACE_CONFIG_HOME=C:\Users\A B\cfg\`, "", "a&b|c<d>e^f(g)"})
	if err != nil {
		t.Fatal(err)
	}
	want := `C:\Windows\System32\cmd.exe /d /v:off /s /c ""C:\npm\gemini.cmd" "WSPACE_CONFIG_HOME=C:\Users\A B\cfg\\" "" "a&b|c<d>e^f(g)""`
	if inv.CmdLine != want {
		t.Fatalf("CmdLine =\n%s\nwant\n%s", inv.CmdLine, want)
	}

	for _, bad := range []string{`say "hi"`, "100%", "%PATH%", "line\nbreak", "cr\rx"} {
		if _, err := r.Command("gemini", []string{bad}); !errors.Is(err, agenthost.ErrUnsafeBatchArgument) {
			t.Errorf("Command(%q) err = %v, want ErrUnsafeBatchArgument", bad, err)
		}
	}
}

func TestResolver_UnixWalksPathSkippingNodeModulesAndNonExecutables(t *testing.T) {
	ff := fakeFiles{goos: "linux", files: map[string]bool{
		"/opt/x/node_modules/.bin/claude": true,
		"/usr/local/bin/claude":           false, // not executable
		"/home/t/.local/bin/claude":       true,
	}}
	r := agenthost.Resolver{
		GOOS: "linux",
		Getenv: func(k string) string {
			return map[string]string{"PATH": "/opt/x/node_modules/.bin:/usr/local/bin::/home/t/.local/bin"}[k]
		},
		Stat: ff.stat,
	}
	inv, err := r.Command("claude", []string{"mcp", "add"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Path != "/home/t/.local/bin/claude" || inv.CmdLine != "" {
		t.Fatalf("Command = %+v", inv)
	}
	if _, err := r.Command("codex", nil); !errors.Is(err, agenthost.ErrNotFound) {
		t.Fatalf("Command(missing) err = %v", err)
	}
}
