// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// installDeps builds an InstallDeps against a fresh FakeFS and a fake
// Windows user Path. env carries every environment variable Install reads
// (PATH, SHELL, ZDOTDIR, XDG_CONFIG_HOME, LOCALAPPDATA); "$HOME" inside a
// value is replaced by the fake's home so PATH entries match it exactly.
//
// For a simulated Linux or macOS the fake's home is a fixed POSIX path, not
// t.TempDir(): on a Windows host that would be "C:/...", whose drive colon
// splits a ":"-separated PATH entry in two. The FakeFS is in memory, so the
// path never needs to exist.
func installDeps(t *testing.T, goos, executable string, env map[string]string) (app.InstallDeps, *portstest.FakeFS, *portstest.FakeUserPath) {
	t.Helper()
	var fs *portstest.FakeFS
	if goos == "windows" {
		fs = portstest.NewFakeFS(t)
	} else {
		fs = portstest.NewFakeFS(posixRoot("/home/tester"))
	}
	home := string(fs.Paths().Home)
	userPath := &portstest.FakeUserPath{}
	deps := app.InstallDeps{
		FS:   fs,
		GOOS: goos,
		Getenv: func(k string) string {
			return strings.ReplaceAll(env[k], "$HOME", home)
		},
		Executable:     func() (string, error) { return executable, nil },
		ReadExecutable: func(string) ([]byte, error) { return []byte("new-binary"), nil },
		SameFile:       func(a, b string) bool { return a == b },
		Rename: func(from, to domain.Path) error {
			data, err := fs.ReadFile(from)
			if err != nil {
				return err
			}
			if err := fs.WriteFile(to, data, 0o755); err != nil {
				return err
			}
			return fs.RemoveAll(from)
		},
		UserPath: userPath,
	}
	return deps, fs, userPath
}

// posixRoot is a FakeFS root provider that always returns the same POSIX
// path.
type posixRoot string

func (r posixRoot) TempDir() string { return string(r) }

func read(t *testing.T, fs *portstest.FakeFS, p domain.Path) string {
	t.Helper()
	data, err := fs.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(data)
}

func exists(fs *portstest.FakeFS, p domain.Path) bool {
	ok, _ := fs.Exists(p)
	return ok
}

// --- Linux / macOS -------------------------------------------------------

// A minimal Linux account (e.g. Alpine, containers) has none of bash's login
// files. A login bash (ssh) would then read nothing, so ~/.profile — which
// a POSIX login shell reads and which hides no other file — is created with
// the PATH block, next to ~/.bashrc for interactive shells.
func TestInstall_LinuxBashWithoutLoginFilesCreatesProfile(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin:/bin", "SHELL": "/bin/bash"})
	home := fs.Paths().Home

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	wantTargets := []string{string(home.Join(".bashrc")), string(home.Join(".profile"))}
	if strings.Join(got.PathTargets, ",") != strings.Join(wantTargets, ",") {
		t.Fatalf("PathTargets = %v, want %v", got.PathTargets, wantTargets)
	}
	if !strings.Contains(read(t, fs, home.Join(".profile")), "wspace PATH") {
		t.Fatal(".profile lacks the PATH block")
	}
	if exists(fs, home.Join(".bash_profile")) {
		t.Fatal(".bash_profile created")
	}
}

// A fresh Linux account: ~/.local/bin does not exist and is not on PATH.
// Install creates it, copies itself there, and (--yes) adds it to PATH in
// ~/.bashrc and in the login file bash reads (the first existing of
// ~/.bash_profile, ~/.bash_login, ~/.profile), never creating one.
func TestInstall_LinuxBashCreatesTheDirAndAddsItToPath(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin:/bin", "SHELL": "/bin/bash"})
	home := fs.Paths().Home
	if err := fs.WriteFile(home.Join(".profile"), []byte("# distro profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	target := home.Join(".local", "bin", "wspace")
	if !got.Installed || got.Path != string(target) || got.Dir != string(home.Join(".local", "bin")) {
		t.Fatalf("Install() = %+v", got)
	}
	if read(t, fs, target) != "new-binary" {
		t.Fatal("binary not copied")
	}
	if !got.PathUpdated || got.OnPath || !got.NewTerminal {
		t.Fatalf("PATH outcome = %+v", got)
	}
	wantTargets := []string{string(home.Join(".bashrc")), string(home.Join(".profile"))}
	if strings.Join(got.PathTargets, ",") != strings.Join(wantTargets, ",") {
		t.Fatalf("PathTargets = %v, want %v", got.PathTargets, wantTargets)
	}
	bashrc := read(t, fs, home.Join(".bashrc"))
	if !strings.Contains(bashrc, "# >>> wspace PATH >>>") || !strings.Contains(bashrc, `export PATH='`+string(home)+`/.local/bin':"$PATH"`) {
		t.Fatalf(".bashrc = %q", bashrc)
	}
	if profile := read(t, fs, home.Join(".profile")); !strings.HasPrefix(profile, "# distro profile\n") || !strings.Contains(profile, "wspace PATH") {
		t.Fatalf(".profile = %q", profile)
	}
	if exists(fs, home.Join(".bash_profile")) {
		t.Fatal(".bash_profile created; bash would then skip ~/.profile")
	}
	if exists(fs, home.Join(".local", "bin", ".wspace.new")) {
		t.Fatal("temporary copy left behind")
	}
}

func TestInstall_PathPromptDeclinedLeavesRCFilesAlone(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin", "SHELL": "/bin/bash"})
	deps.Prompter = portstest.NewScriptedPrompter(t, portstest.ConfirmAnswer(false), portstest.ConfirmAnswer(false))

	got, err := app.Install(context.Background(), deps, app.InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Installed || got.PathUpdated || !got.PathDeclined {
		t.Fatalf("Install() = %+v", got)
	}
	if exists(fs, fs.Paths().Home.Join(".bashrc")) {
		t.Fatal(".bashrc written after the user declined")
	}
}

func TestInstall_WithoutPrompterOrYesNeverEditsPath(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin", "SHELL": "/bin/bash"})

	got, err := app.Install(context.Background(), deps, app.InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Installed || got.PathUpdated || got.OnPath {
		t.Fatalf("Install() = %+v", got)
	}
	if exists(fs, fs.Paths().Home.Join(".bashrc")) {
		t.Fatal(".bashrc written without consent")
	}
}

// macOS Terminal opens login shells, which read only the login file:
// with none present, ~/.bash_profile is created.
func TestInstall_DarwinBashUsesTheLoginFile(t *testing.T) {
	deps, fs, _ := installDeps(t, "darwin", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin", "SHELL": "/bin/bash"})

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	home := fs.Paths().Home
	if strings.Join(got.PathTargets, ",") != string(home.Join(".bash_profile")) {
		t.Fatalf("PathTargets = %v", got.PathTargets)
	}
}

func TestInstall_ZshUsesZshrcUnderZdotdir(t *testing.T) {
	deps, fs, _ := installDeps(t, "darwin", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin", "SHELL": "/bin/zsh", "ZDOTDIR": "$HOME/.config/zsh"})

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	want := fs.Paths().Home.Join(".config", "zsh", ".zshrc")
	if strings.Join(got.PathTargets, ",") != string(want) || !strings.Contains(read(t, fs, want), "export PATH=") {
		t.Fatalf("PathTargets = %v", got.PathTargets)
	}
}

func TestInstall_FishUsesFishAddPath(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin", "SHELL": "/usr/bin/fish"})

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	home := fs.Paths().Home
	cfg := home.Join(".config", "fish", "config.fish")
	if strings.Join(got.PathTargets, ",") != string(cfg) {
		t.Fatalf("PathTargets = %v", got.PathTargets)
	}
	if !strings.Contains(read(t, fs, cfg), "fish_add_path -g '"+string(home)+"/.local/bin'") {
		t.Fatalf("config.fish = %q", read(t, fs, cfg))
	}
}

func TestInstall_AlreadyOnPathAsksNothingAboutPath(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "$HOME/.local/bin/:/usr/bin", "SHELL": "/bin/bash"})
	// Only the shell-integration question is asked.
	deps.Prompter = portstest.NewScriptedPrompter(t, portstest.ConfirmAnswer(false))

	got, err := app.Install(context.Background(), deps, app.InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Installed || !got.OnPath || got.PathUpdated || got.NewTerminal {
		t.Fatalf("Install() = %+v", got)
	}
	if exists(fs, fs.Paths().Home.Join(".bashrc")) {
		t.Fatal(".bashrc written although the directory is on PATH")
	}
}

func TestInstall_SecondRunDuplicatesNothing(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin", "SHELL": "/bin/bash"})
	if _, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true}); err != nil {
		t.Fatal(err)
	}
	first := read(t, fs, fs.Paths().Home.Join(".bashrc"))

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if second := read(t, fs, fs.Paths().Home.Join(".bashrc")); second != first {
		t.Fatalf(".bashrc changed on a second run:\n%s\n---\n%s", first, second)
	}
	if got.PathUpdated || !got.PathConfigured {
		t.Fatalf("second Install() = %+v, want PATH reported as already configured", got)
	}
	if strings.Count(first, ">>> wspace PATH >>>") != 1 || strings.Count(first, ">>> wspace shell integration >>>") != 1 {
		t.Fatalf(".bashrc = %q", first)
	}
}

// The running binary is the installed one (wspace install run from
// ~/.local/bin): nothing is copied.
func TestInstall_RunningTheInstalledCopyReportsAlreadyInstalled(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "", map[string]string{"PATH": "$HOME/.local/bin"})
	target := fs.Paths().Home.Join(".local", "bin", "wspace")
	deps.Executable = func() (string, error) { return string(target), nil }

	got, err := app.Install(context.Background(), deps, app.InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Installed || !got.AlreadyInstalled || len(fs.Writes) != 0 {
		t.Fatalf("Install() = %+v, writes %v", got, fs.Writes)
	}
}

// A desktop app that bundles the wspace binary may install
// ~/.local/bin/wspace as a symlink into its own bundle; writing through it
// would overwrite the app's engine.
func TestInstall_RefusesToReplaceAnAppManagedSymlink(t *testing.T) {
	deps, fs, _ := installDeps(t, "darwin", "/tmp/dl/wspace", map[string]string{"PATH": "$HOME/.local/bin"})
	link := fs.Paths().Home.Join(".local", "bin", "wspace")
	if err := fs.WriteFile(link, []byte("bundled-engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	deps.IsSymlink = func(p string) (bool, error) { return p == string(link), nil }

	_, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if domain.Code(err) != domain.CodeManagedInstall {
		t.Fatalf("Install() error = %v, want code %s", err, domain.CodeManagedInstall)
	}
	if read(t, fs, link) != "bundled-engine" {
		t.Fatal("link target overwritten")
	}
}

func TestInstall_UnwritableTargetReportsAManualCommand(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin"})
	fs.DenyWrite = func(domain.Path) bool { return true }

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Installed || !strings.Contains(got.ManualCommand, "/tmp/dl/wspace") || strings.Contains(got.ManualCommand, "sudo") {
		t.Fatalf("Install() = %+v", got)
	}
}

func TestInstall_ShellIntegrationIsOfferedAfterPath(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin", "SHELL": "/bin/bash"})
	deps.Prompter = portstest.NewScriptedPrompter(t, portstest.ConfirmAnswer(true), portstest.ConfirmAnswer(true))

	got, err := app.Install(context.Background(), deps, app.InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	bashrc := read(t, fs, fs.Paths().Home.Join(".bashrc"))
	if !got.ShellRCUpdated || strings.Index(bashrc, "wspace PATH") > strings.Index(bashrc, "wspace shell integration") {
		t.Fatalf("Install() = %+v, .bashrc = %q (PATH must come first)", got, bashrc)
	}
}

func TestInstall_UninstallRemovesOnlyWhatInstallAdded(t *testing.T) {
	deps, fs, _ := installDeps(t, "linux", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin", "SHELL": "/bin/bash"})
	home := fs.Paths().Home
	if err := fs.WriteFile(home.Join(".profile"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	installed, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	got, err := app.Install(context.Background(), deps, app.InstallInput{Uninstall: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Removed) != 1 || got.Removed[0] != installed.Path || exists(fs, domain.Path(installed.Path)) {
		t.Fatalf("Removed = %v", got.Removed)
	}
	if len(got.PathRemoved) != 2 {
		t.Fatalf("PathRemoved = %v", got.PathRemoved)
	}
	if p := read(t, fs, home.Join(".profile")); p != "keep me\n" {
		t.Fatalf(".profile = %q", p)
	}
	if b := read(t, fs, home.Join(".bashrc")); strings.Contains(b, "wspace") {
		t.Fatalf(".bashrc = %q", b)
	}

	again, err := app.Install(context.Background(), deps, app.InstallInput{Uninstall: true})
	if err != nil || len(again.Removed) != 0 || len(again.PathRemoved) != 0 {
		t.Fatalf("second uninstall = %+v, %v", again, err)
	}
}

// --- Windows -------------------------------------------------------------

const winLocal = `C:\Users\dev\AppData\Local`

func windowsDeps(t *testing.T, path string) (app.InstallDeps, *portstest.FakeFS, *portstest.FakeUserPath) {
	t.Helper()
	return installDeps(t, "windows", `C:\Users\dev\Downloads\wspace.exe`, map[string]string{
		"PATH": path, "LOCALAPPDATA": winLocal, "USERPROFILE": `C:\Users\dev`,
	})
}

func TestInstall_WindowsAddsTheDirToTheUserPathInTheRegistry(t *testing.T) {
	deps, fs, reg := windowsDeps(t, `C:\Windows\System32`)
	long := strings.Repeat(`C:\a-very-long-tool-directory;`, 40) + `%USERPROFILE%\bin`
	reg.Value, reg.Expand = long, true

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	const target = "C:/Users/dev/AppData/Local/Programs/wspace/wspace.exe"
	if !got.Installed || got.Path != `C:\Users\dev\AppData\Local\Programs\wspace\wspace.exe` || read(t, fs, target) != "new-binary" {
		t.Fatalf("Install() = %+v", got)
	}
	want := long + `;C:\Users\dev\AppData\Local\Programs\wspace`
	if reg.Value != want || !reg.Expand {
		t.Fatalf("user Path = %q (expand %v), want the old value kept whole plus the dir", reg.Value, reg.Expand)
	}
	if reg.Broadcasts != 1 || !got.PathUpdated || !got.NewTerminal || strings.Join(got.PathTargets, ",") != `HKCU\Environment\Path` {
		t.Fatalf("Install() = %+v, broadcasts %d", got, reg.Broadcasts)
	}

	again, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Value != want || reg.Writes != 1 || again.PathUpdated || !again.PathConfigured {
		t.Fatalf("second run changed the user Path: %q writes %d, %+v", reg.Value, reg.Writes, again)
	}
}

func TestInstall_WindowsCreatesTheUserPathWhenMissing(t *testing.T) {
	deps, _, reg := windowsDeps(t, `C:\Windows\System32`)

	if _, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if reg.Value != `C:\Users\dev\AppData\Local\Programs\wspace` || !reg.Expand {
		t.Fatalf("user Path = %q expand %v", reg.Value, reg.Expand)
	}
}

// An entry spelled with a variable and another case counts as present.
func TestInstall_WindowsRecognizesAnExpandableEntry(t *testing.T) {
	deps, _, reg := windowsDeps(t, `C:\Windows\System32`)
	reg.Value, reg.Expand = `%LOCALAPPDATA%\programs\WSPACE\`, true

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Writes != 0 || !got.PathConfigured {
		t.Fatalf("Install() = %+v, writes %d", got, reg.Writes)
	}
}

// A running .exe cannot be overwritten on Windows, but it can be renamed:
// the older copy is moved aside and removed on the next run.
func TestInstall_WindowsReplacesAnOlderCopyByRenamingItAside(t *testing.T) {
	deps, fs, _ := windowsDeps(t, `C:\Users\dev\AppData\Local\Programs\wspace`)
	dir := domain.Path("C:/Users/dev/AppData/Local/Programs/wspace")
	if err := fs.MkdirAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile(dir.Join("wspace.exe"), []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := app.Install(context.Background(), deps, app.InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Installed || got.AlreadyInstalled || read(t, fs, dir.Join("wspace.exe")) != "new-binary" {
		t.Fatalf("Install() = %+v", got)
	}
	if read(t, fs, dir.Join("wspace.old.exe")) != "old-binary" {
		t.Fatal("older copy not moved aside")
	}
	if !got.OnPath {
		t.Fatalf("OnPath = false for a dir on PATH: %+v", got)
	}

	if _, err := app.Install(context.Background(), deps, app.InstallInput{}); err != nil {
		t.Fatal(err)
	}
	if exists(fs, dir.Join("wspace.old.exe")) {
		t.Fatal("the moved-aside copy survived the next run")
	}
}

func TestInstall_WindowsRunningTheInstalledCopy(t *testing.T) {
	deps, fs, _ := windowsDeps(t, `C:\Users\dev\AppData\Local\Programs\wspace`)
	deps.Executable = func() (string, error) { return `C:\Users\dev\AppData\Local\Programs\wspace\wspace.exe`, nil }
	deps.SameFile = func(a, b string) bool {
		return strings.EqualFold(strings.ReplaceAll(a, `\`, "/"), strings.ReplaceAll(b, `\`, "/"))
	}

	got, err := app.Install(context.Background(), deps, app.InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.AlreadyInstalled || len(fs.Writes) != 0 {
		t.Fatalf("Install() = %+v, writes %v", got, fs.Writes)
	}
}

func TestInstall_WindowsUninstallRemovesOnlyItsPathEntry(t *testing.T) {
	deps, fs, reg := windowsDeps(t, `C:\Windows\System32`)
	reg.Value, reg.Expand = `C:\tools;%USERPROFILE%\bin`, true
	installed, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}

	got, err := app.Install(context.Background(), deps, app.InstallInput{Uninstall: true})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Value != `C:\tools;%USERPROFILE%\bin` || !reg.Expand || reg.Broadcasts != 2 {
		t.Fatalf("user Path = %q expand %v broadcasts %d", reg.Value, reg.Expand, reg.Broadcasts)
	}
	if exists(fs, "C:/Users/dev/AppData/Local/Programs/wspace/wspace.exe") || !installed.Installed || len(got.Removed) != 1 || strings.Join(got.PathRemoved, ",") != `HKCU\Environment\Path` {
		t.Fatalf("uninstall = %+v", got)
	}
}

// macOS bash reads only the login file, so shell integration goes there
// too, after the PATH block.
func TestInstall_DarwinBashShellIntegrationUsesTheLoginFile(t *testing.T) {
	deps, fs, _ := installDeps(t, "darwin", "/tmp/dl/wspace", map[string]string{"PATH": "/usr/bin", "SHELL": "/bin/bash"})

	got, err := app.Install(context.Background(), deps, app.InstallInput{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	profile := fs.Paths().Home.Join(".bash_profile")
	if got.ShellRCPath != string(profile) || exists(fs, fs.Paths().Home.Join(".bashrc")) {
		t.Fatalf("ShellRCPath = %q", got.ShellRCPath)
	}
	if c := read(t, fs, profile); strings.Index(c, "wspace PATH") > strings.Index(c, "shell integration") {
		t.Fatalf(".bash_profile = %q", c)
	}
}
