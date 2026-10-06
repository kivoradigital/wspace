// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// shellRCMarkerBegin/End delimit the shell-init block Install may append to
// a shell rc file; pathRCMarkerBegin/End the block that puts the install
// directory on PATH. A later run (or --uninstall) finds and removes exactly
// these blocks without disturbing anything else the file contains, which
// is also what makes a second run a no-op instead of a duplicate.
const (
	shellRCMarkerBegin = "# >>> wspace shell integration >>>"
	shellRCMarkerEnd   = "# <<< wspace shell integration <<<"
	pathRCMarkerBegin  = "# >>> wspace PATH >>>"
	pathRCMarkerEnd    = "# <<< wspace PATH <<<"
)

// WindowsUserPathTarget names the registry value Install edits on Windows.
const WindowsUserPathTarget = `HKCU\Environment\Path`

// InstallDeps bundles app.Install's dependencies. Every func field beyond
// FS/Prompter/Reporter/UserPath defaults to a real OS-backed
// implementation when nil: GOOS to runtime.GOOS, Getenv to os.Getenv,
// Executable to os.Executable, ReadExecutable to os.ReadFile, IsSymlink to
// os.Lstat, SameFile to os.SameFile and Rename to os.Rename. A test
// overrides them to exercise every OS/PATH combination deterministically,
// without touching the real filesystem, registry or running binary.
type InstallDeps struct {
	FS       ports.FileSystemPort
	Prompter ports.Prompter
	Reporter ports.Reporter
	// UserPath is the Windows user PATH (HKCU\Environment); unused
	// elsewhere, and Windows PATH editing is skipped when it is nil.
	UserPath ports.UserPathStore

	GOOS           string
	Getenv         func(string) string
	Executable     func() (string, error)
	ReadExecutable func(path string) ([]byte, error)
	// IsSymlink reports whether path itself is a symbolic link; a missing
	// path is not one.
	IsSymlink func(path string) (bool, error)
	// SameFile reports whether two paths name the same existing file.
	SameFile func(a, b string) bool
	// Rename moves a file within one directory, replacing to on Unix.
	Rename func(from, to domain.Path) error
}

func (d InstallDeps) resolved() InstallDeps {
	if d.GOOS == "" {
		d.GOOS = runtime.GOOS
	}
	if d.Getenv == nil {
		d.Getenv = os.Getenv
	}
	if d.Executable == nil {
		d.Executable = os.Executable
	}
	if d.ReadExecutable == nil {
		d.ReadExecutable = os.ReadFile
	}
	if d.IsSymlink == nil {
		d.IsSymlink = func(path string) (bool, error) {
			info, err := os.Lstat(path)
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return info.Mode()&fs.ModeSymlink != 0, nil
		}
	}
	if d.SameFile == nil {
		d.SameFile = func(a, b string) bool {
			ia, errA := os.Stat(a)
			ib, errB := os.Stat(b)
			return errA == nil && errB == nil && os.SameFile(ia, ib)
		}
	}
	if d.Rename == nil {
		d.Rename = func(from, to domain.Path) error { return os.Rename(string(from), string(to)) }
	}
	return d
}

// InstallInput parameterizes Install. Uninstall selects the reverse
// operation (`wspace install --uninstall`). Yes answers yes to every
// question (adding the directory to PATH, shell integration) for a
// non-interactive run.
type InstallInput struct {
	Uninstall bool
	Yes       bool
}

// InstallResult is the payload handed to Reporter.Result and rendered by
// internal/cli.
//
//   - Installed/Path/Dir: the binary is at Path inside Dir.
//     AlreadyInstalled means nothing was copied (the running binary is the
//     installed one, or the copy there is identical).
//   - ManualCommand is set instead of Installed when the copy failed.
//   - OnPath: Dir is on this process's PATH. PathConfigured: it is not,
//     but the rc files/registry already add it (a new terminal will have
//     it). PathUpdated: Install added it now, to PathTargets. PathDeclined:
//     the user said no. NewTerminal: only a new terminal sees the change.
//   - ShellRCUpdated/ShellRCPath: the shell-integration block.
//   - Uninstall: Removed lists deleted binaries, PathRemoved the files (or
//     the registry value) a PATH entry was removed from, PendingRemoval a
//     running binary that was renamed because Windows cannot delete it.
type InstallResult struct {
	Uninstall bool

	Installed        bool
	AlreadyInstalled bool
	Path             string
	Dir              string
	ManualCommand    string

	OnPath         bool
	PathConfigured bool
	PathUpdated    bool
	PathDeclined   bool
	PathTargets    []string
	NewTerminal    bool

	ShellRCUpdated bool
	ShellRCPath    string

	Removed        []string
	PathRemoved    []string
	PendingRemoval string
}

// Install implements `wspace install`: it copies the running binary into
// the per-user install directory (~/.local/bin on Linux and macOS,
// %LOCALAPPDATA%\Programs\wspace on Windows), creating it, and makes sure
// that directory is on PATH — asking first (or --yes). On Linux/macOS it
// appends a marked block to the rc file(s) of the user's $SHELL (see
// pathRCFiles); on Windows it appends the directory to the user Path in
// the registry, keeping the value's type and every existing entry (never
// setx, which truncates at 1024 characters), and broadcasts the change.
// It never elevates privileges and never touches the system-wide PATH.
//
// The copy is atomic: the bytes are written to a temporary file in the
// same directory and renamed over the target. A running Windows .exe
// cannot be replaced, but it can be renamed, so an older copy is first
// moved aside to wspace.old.exe, which the next run deletes.
func Install(ctx context.Context, deps InstallDeps, in InstallInput) (InstallResult, error) {
	deps = deps.resolved()
	dir := installDir(deps)
	target := dir.Join(binaryName(deps.GOOS))

	if in.Uninstall {
		return uninstall(deps, dir, target)
	}

	result := InstallResult{Dir: string(dir), Path: string(target)}
	removeLeftovers(deps, dir)

	placed, already, err := placeBinary(deps, dir, target)
	if err != nil {
		return InstallResult{}, err
	}
	if !placed {
		result.Path = ""
		result.ManualCommand = manualInstallCommand(deps, dir, target)
		return finish(deps, result), nil
	}
	result.Installed, result.AlreadyInstalled = true, already

	if err := ensureOnPath(ctx, deps, in, dir, &result); err != nil {
		return InstallResult{}, err
	}
	if in.Yes || deps.Prompter != nil {
		updated, rcPath := maybeAddShellInit(ctx, deps, in.Yes)
		result.ShellRCUpdated = updated
		result.ShellRCPath = string(rcPath)
	}
	return finish(deps, result), nil
}

// finish writes the result's paths in deps.GOOS's native form (what the
// user types and sees: C:\Users\… on Windows) and reports it.
func finish(deps InstallDeps, result InstallResult) InstallResult {
	native := func(p string) string { return nativeDisplay(deps.GOOS, p) }
	result.Path, result.Dir, result.ShellRCPath, result.PendingRemoval = native(result.Path), native(result.Dir), native(result.ShellRCPath), native(result.PendingRemoval)
	for _, list := range [][]string{result.PathTargets, result.Removed, result.PathRemoved} {
		for i, p := range list {
			if p != WindowsUserPathTarget {
				list[i] = native(p)
			}
		}
	}
	if deps.Reporter != nil {
		deps.Reporter.Result(result)
	}
	return result
}

// installDir is the per-user install directory for deps.GOOS.
func installDir(deps InstallDeps) domain.Path {
	if deps.GOOS == "windows" {
		base := slashPath(deps.Getenv("LOCALAPPDATA"))
		if base == "" {
			base = string(deps.FS.Paths().Home.Join("AppData", "Local"))
		}
		return domain.Path(base).Join("Programs", "wspace")
	}
	return deps.FS.Paths().Home.Join(".local", "bin")
}

func asideName(goos string) string {
	if goos == "windows" {
		return "wspace.old.exe"
	}
	return "wspace.old"
}

const tempBinaryName = ".wspace.new"

// removeLeftovers deletes what an earlier run left behind: the copy moved
// aside on Windows (no longer running by now) and an interrupted
// temporary copy. Failures are ignored; the file may still be in use.
func removeLeftovers(deps InstallDeps, dir domain.Path) {
	for _, name := range []string{asideName(deps.GOOS), tempBinaryName} {
		p := dir.Join(name)
		if ok, _ := deps.FS.Exists(p); ok {
			_ = deps.FS.RemoveAll(p)
		}
	}
}

// placeBinary copies the running executable to target. placed is false
// (and err nil) when the directory or file cannot be written, so the
// caller reports a manual command; already is true when nothing had to
// be copied.
func placeBinary(deps InstallDeps, dir, target domain.Path) (placed, already bool, err error) {
	exe, err := deps.Executable()
	if err != nil {
		return false, false, err
	}
	if deps.SameFile(exe, string(target)) {
		return true, true, nil
	}
	// A symlink here was placed by a desktop app that bundles the wspace
	// binary and points into its bundle; replacing it would silently detach
	// the app's own command line tool. Never overwrite it.
	if link, err := deps.IsSymlink(string(target)); err != nil {
		return false, false, err
	} else if link {
		return false, false, domain.NewOpError("install", domain.CodeManagedInstall, string(target),
			"it is a symlink managed by the wspace app; update the app, or remove the link with the app's \"Uninstall Command Line Tool…\" first", nil)
	}
	data, err := deps.ReadExecutable(exe)
	if err != nil {
		return false, false, err
	}
	exists, err := deps.FS.Exists(target)
	if err != nil {
		return false, false, err
	}
	if exists {
		if cur, err := deps.FS.ReadFile(target); err == nil && bytes.Equal(cur, data) {
			return true, true, nil
		}
	}
	if err := deps.FS.MkdirAll(dir); err != nil {
		return false, false, nil
	}
	tmp := dir.Join(tempBinaryName)
	if err := deps.FS.WriteFile(tmp, data, 0o755); err != nil {
		return false, false, nil
	}
	if exists && deps.GOOS == "windows" {
		aside := dir.Join(asideName(deps.GOOS))
		if err := deps.Rename(target, aside); err != nil {
			_ = deps.FS.RemoveAll(tmp)
			return false, false, err
		}
		if err := deps.Rename(tmp, target); err != nil {
			_ = deps.Rename(aside, target)
			_ = deps.FS.RemoveAll(tmp)
			return false, false, err
		}
		return true, false, nil
	}
	if err := deps.Rename(tmp, target); err != nil {
		_ = deps.FS.RemoveAll(tmp)
		return false, false, err
	}
	return true, false, nil
}

// ensureOnPath makes dir reachable through PATH in new terminals, asking
// first unless in.Yes. Without a Prompter (and without Yes) it only
// reports.
func ensureOnPath(ctx context.Context, deps InstallDeps, in InstallInput, dir domain.Path, result *InstallResult) error {
	if containsPath(deps.GOOS, splitPath(deps.Getenv("PATH"), deps.GOOS), dir, deps.Getenv) {
		result.OnPath = true
		return nil
	}
	var (
		missing []domain.Path
		targets []string
		regVal  string
		expand  bool
	)
	if deps.GOOS == "windows" {
		if deps.UserPath == nil {
			return nil
		}
		var err error
		regVal, expand, err = deps.UserPath.UserPath()
		if err != nil {
			return err
		}
		if containsPath("windows", splitPath(regVal, "windows"), dir, deps.Getenv) {
			result.PathConfigured, result.NewTerminal = true, true
			return nil
		}
		targets = []string{WindowsUserPathTarget}
	} else {
		for _, rc := range pathRCFiles(deps) {
			existing, _ := deps.FS.ReadFile(rc)
			if !strings.Contains(string(existing), pathRCMarkerBegin) {
				missing = append(missing, rc)
			}
		}
		if len(missing) == 0 {
			result.PathConfigured, result.NewTerminal = true, true
			return nil
		}
		for _, rc := range missing {
			targets = append(targets, string(rc))
		}
	}

	if !in.Yes {
		if deps.Prompter == nil {
			return nil
		}
		ok, err := deps.Prompter.Confirm(ctx, ports.ConfirmField{
			Field:   ports.Field{Label: messages.InstallPathConfirm, Help: messages.InstallPathConfirmHelp, Args: []any{nativeDisplay(deps.GOOS, string(dir)), strings.Join(targets, ", ")}},
			Default: true,
		})
		if err != nil || !ok {
			result.PathDeclined = true
			return nil
		}
	}

	if deps.GOOS == "windows" {
		entry := nativeDisplay("windows", string(dir))
		updated := entry
		if trimmed := strings.TrimRight(regVal, ";"); trimmed != "" {
			updated = trimmed + ";" + entry
		}
		if err := deps.UserPath.SetUserPath(updated, expand); err != nil {
			return err
		}
		_ = deps.UserPath.BroadcastEnvironmentChange()
	} else {
		block := pathBlock(deps.Getenv("SHELL"), string(dir))
		for _, rc := range missing {
			if err := appendBlock(deps, rc, block); err != nil {
				return err
			}
		}
	}
	result.PathUpdated, result.PathTargets, result.NewTerminal = true, targets, true
	return nil
}

// pathRCFiles is where the PATH block goes for the user's $SHELL:
//
//   - zsh: $ZDOTDIR/.zshrc (default ~/.zshrc), read by every interactive
//     zsh, login or not, on macOS and Linux alike.
//   - fish: $XDG_CONFIG_HOME/fish/config.fish (default ~/.config/fish).
//   - bash: a login bash reads only the first existing of ~/.bash_profile,
//     ~/.bash_login, ~/.profile; a non-login interactive one only
//     ~/.bashrc. Linux terminals start non-login shells, so ~/.bashrc
//     always gets the block, plus that login file. ~/.bash_profile is never
//     created (it would hide the distribution's ~/.profile); with no login
//     file at all, ~/.profile is created. macOS Terminal starts login shells, so there the login
//     file gets it, ~/.bash_profile created when none exists.
//   - anything else: ~/.profile, read by POSIX login shells.
func pathRCFiles(deps InstallDeps) []domain.Path {
	home := deps.FS.Paths().Home
	shell := path.Base(slashPath(deps.Getenv("SHELL")))
	switch {
	case strings.Contains(shell, "zsh"):
		return []domain.Path{zshDir(deps).Join(".zshrc")}
	case strings.Contains(shell, "fish"):
		return []domain.Path{fishConfig(deps)}
	case strings.Contains(shell, "bash"):
		login := domain.Path("")
		for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
			if ok, _ := deps.FS.Exists(home.Join(name)); ok {
				login = home.Join(name)
				break
			}
		}
		if deps.GOOS == "darwin" {
			if login == "" {
				login = home.Join(".bash_profile")
			}
			return []domain.Path{login}
		}
		// With no login file at all (minimal images, containers), a login
		// bash (ssh) reads none of them: create ~/.profile, which hides no
		// other file and is also read by POSIX login shells.
		if login == "" {
			login = home.Join(".profile")
		}
		return []domain.Path{home.Join(".bashrc"), login}
	default:
		return []domain.Path{home.Join(".profile")}
	}
}

func zshDir(deps InstallDeps) domain.Path {
	if z := slashPath(deps.Getenv("ZDOTDIR")); z != "" {
		return domain.Path(z)
	}
	return deps.FS.Paths().Home
}

func fishConfig(deps InstallDeps) domain.Path {
	base := domain.Path(slashPath(deps.Getenv("XDG_CONFIG_HOME")))
	if base == "" {
		base = deps.FS.Paths().Home.Join(".config")
	}
	return base.Join("fish", "config.fish")
}

// allPathRCFiles is every file pathRCFiles can choose, for uninstall: a
// $SHELL change since install must not strand a block.
func allPathRCFiles(deps InstallDeps) []domain.Path {
	home := deps.FS.Paths().Home
	files := []domain.Path{home.Join(".bashrc"), home.Join(".bash_profile"), home.Join(".bash_login"), home.Join(".profile"), home.Join(".zshrc")}
	if z := zshDir(deps).Join(".zshrc"); z != home.Join(".zshrc") {
		files = append(files, z)
	}
	return append(files, fishConfig(deps))
}

// pathBlock is the rc block that prepends dir to PATH: fish_add_path for
// fish (which skips a directory already on PATH; -g keeps it out of the
// universal variables, so removing the block undoes it), and for POSIX
// shells an export guarded against adding dir twice (a login file that
// sources ~/.bashrc runs both blocks).
func pathBlock(shellEnv, dir string) string {
	var line string
	if strings.Contains(path.Base(slashPath(shellEnv)), "fish") {
		line = "fish_add_path -g " + fishQuote(dir)
	} else {
		q := posixQuote(dir)
		line = `case ":$PATH:" in *:` + q + `:*) ;; *) export PATH=` + q + `:"$PATH" ;; esac`
	}
	return pathRCMarkerBegin + "\n" + line + "\n" + pathRCMarkerEnd + "\n"
}

func posixQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func fishQuote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(s) + "'"
}

// appendBlock appends block to rc (creating it and its directory),
// separated from existing content by a newline.
func appendBlock(deps InstallDeps, rc domain.Path, block string) error {
	existing, _ := deps.FS.ReadFile(rc)
	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := deps.FS.MkdirAll(parentDir(rc)); err != nil {
		return err
	}
	return deps.FS.WriteFile(rc, []byte(content+block), 0o644)
}

// uninstall removes the installed binary (never a desktop app's symlink),
// the PATH entry Install added (rc blocks, or the registry entry equal to
// the install directory) and the shell-init block, all idempotently: a
// second call reports nothing removed instead of erroring.
func uninstall(deps InstallDeps, dir, target domain.Path) (InstallResult, error) {
	result := InstallResult{Uninstall: true, Dir: string(dir)}

	exists, err := deps.FS.Exists(target)
	if err != nil {
		return InstallResult{}, err
	}
	link, err := deps.IsSymlink(string(target))
	if err != nil {
		return InstallResult{}, err
	}
	if exists && !link {
		exe, _ := deps.Executable()
		if deps.GOOS == "windows" && exe != "" && deps.SameFile(exe, string(target)) {
			// A running .exe cannot be deleted; renamed aside, the next
			// install deletes it, or the user does once this exits.
			aside := dir.Join(asideName(deps.GOOS))
			_ = deps.FS.RemoveAll(aside)
			if err := deps.Rename(target, aside); err != nil {
				return InstallResult{}, err
			}
			result.PendingRemoval = string(aside)
		} else if err := deps.FS.RemoveAll(target); err != nil {
			return InstallResult{}, err
		}
		result.Removed = append(result.Removed, string(target))
	}

	if deps.GOOS == "windows" {
		if deps.UserPath != nil {
			removed, err := removeWindowsPathEntry(deps, dir)
			if err != nil {
				return InstallResult{}, err
			}
			if removed {
				result.PathRemoved = []string{WindowsUserPathTarget}
			}
		}
	} else {
		for _, rc := range allPathRCFiles(deps) {
			removed, err := removeBlock(deps, rc, pathRCMarkerBegin, pathRCMarkerEnd)
			if err != nil {
				return InstallResult{}, err
			}
			if removed {
				result.PathRemoved = append(result.PathRemoved, string(rc))
			}
		}
	}

	if deps.GOOS != "windows" {
		// Every candidate file, not only $SHELL's: an older version put the
		// block in ~/.bashrc on macOS too.
		for _, rc := range allPathRCFiles(deps) {
			removed, err := removeBlock(deps, rc, shellRCMarkerBegin, shellRCMarkerEnd)
			if err != nil {
				return InstallResult{}, err
			}
			if removed && !result.ShellRCUpdated {
				result.ShellRCUpdated = true
				result.ShellRCPath = string(rc)
			}
		}
	}

	return finish(deps, result), nil
}

// removeWindowsPathEntry drops every user Path entry naming dir and
// rewrites the value (same type) only when one was found.
func removeWindowsPathEntry(deps InstallDeps, dir domain.Path) (bool, error) {
	val, expand, err := deps.UserPath.UserPath()
	if err != nil {
		return false, err
	}
	var kept []string
	removed := false
	for _, e := range strings.Split(val, ";") {
		if e != "" && containsPath("windows", []string{e}, dir, deps.Getenv) {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return false, nil
	}
	if err := deps.UserPath.SetUserPath(strings.Join(kept, ";"), expand); err != nil {
		return false, err
	}
	_ = deps.UserPath.BroadcastEnvironmentChange()
	return true, nil
}

// maybeAddShellInit adds the shell-init block to the user's rc file when
// the user agrees (or yes), unless it is already there.
func maybeAddShellInit(ctx context.Context, deps InstallDeps, yes bool) (bool, domain.Path) {
	rcPath, ok := shellRCPath(deps)
	if !ok {
		return false, ""
	}
	if !yes {
		confirmed, err := deps.Prompter.Confirm(ctx, ports.ConfirmField{
			Field:   ports.Field{Label: messages.InstallShellRCConfirm, Help: messages.InstallShellRCConfirmHelp, Args: []any{string(rcPath)}},
			Default: true,
		})
		if err != nil || !confirmed {
			return false, ""
		}
	}

	existing, _ := deps.FS.ReadFile(rcPath)
	if strings.Contains(string(existing), shellRCMarkerBegin) {
		return false, rcPath
	}
	if err := appendBlock(deps, rcPath, shellInitBlock(deps.Getenv("SHELL"))); err != nil {
		return false, rcPath
	}
	return true, rcPath
}

// removeBlock strips begin..end (inclusive) from rc's content, if
// present. It is a no-op — not an error — when the file or the block does
// not exist.
func removeBlock(deps InstallDeps, rc domain.Path, begin, end string) (bool, error) {
	existing, err := deps.FS.ReadFile(rc)
	if err != nil {
		return false, nil
	}
	content := string(existing)

	start := strings.Index(content, begin)
	if start == -1 {
		return false, nil
	}
	stop := strings.Index(content[start:], end)
	if stop == -1 {
		return false, nil
	}
	stop += start + len(end)
	if stop < len(content) && content[stop] == '\n' {
		stop++
	}

	if err := deps.FS.WriteFile(rc, []byte(content[:start]+content[stop:]), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// shellInitBlock builds the rc-file block for shellEnv (the $SHELL value).
func shellInitBlock(shellEnv string) string {
	var line string
	switch {
	case strings.Contains(shellEnv, "fish"):
		line = "wspace shell-init fish | source"
	default:
		line = `eval "$(wspace shell-init bash)"`
	}
	return shellRCMarkerBegin + "\n" + line + "\n" + shellRCMarkerEnd + "\n"
}

// shellRCPath resolves the rc file the shell-init block goes to. Windows
// has no rc file in scope (PowerShell profiles are not edited), so ok is
// false there.
func shellRCPath(deps InstallDeps) (domain.Path, bool) {
	if deps.GOOS == "windows" {
		return "", false
	}
	switch shell := deps.Getenv("SHELL"); {
	case strings.Contains(shell, "zsh"):
		return zshDir(deps).Join(".zshrc"), true
	case strings.Contains(shell, "fish"):
		return fishConfig(deps), true
	case deps.GOOS == "darwin" && strings.Contains(path.Base(slashPath(shell)), "bash"):
		// macOS Terminal starts login shells, which never read ~/.bashrc.
		return pathRCFiles(deps)[0], true
	default:
		return deps.FS.Paths().Home.Join(".bashrc"), true
	}
}

// parentDir returns p's containing directory, mirroring path.Dir.
func parentDir(p domain.Path) domain.Path {
	return domain.Path(path.Dir(string(p)))
}

func binaryName(goos string) string {
	if goos == "windows" {
		return "wspace.exe"
	}
	return "wspace"
}

// pathListSeparator returns goos's PATH list separator. This is
// deliberately not os.PathListSeparator (a compile-time constant tied to
// the host running the tests), so Windows is testable on any host.
func pathListSeparator(goos string) string {
	if goos == "windows" {
		return ";"
	}
	return ":"
}

func splitPath(pathEnv, goos string) []string {
	if pathEnv == "" {
		return nil
	}
	return strings.Split(pathEnv, pathListSeparator(goos))
}

// containsPath reports whether one of the PATH entries names dir. On
// Windows the comparison ignores case, quotes, the separator style and a
// trailing separator, and expands %VAR% references (registry values keep
// them unexpanded).
func containsPath(goos string, entries []string, dir domain.Path, getenv func(string) string) bool {
	want := strings.TrimRight(string(dir), "/")
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if goos == "windows" {
			e = expandWindowsVars(strings.Trim(e, `"`), getenv)
		}
		got := strings.TrimRight(slashPath(e), "/")
		if got == "" {
			continue
		}
		if got == want || (goos == "windows" && strings.EqualFold(got, want)) {
			return true
		}
	}
	return false
}

// expandWindowsVars replaces %NAME% with getenv(NAME), leaving unknown
// references as they are (cmd.exe's rule).
func expandWindowsVars(s string, getenv func(string) string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			break
		}
		name := s[i+1 : i+1+j]
		if v := getenv(name); name != "" && v != "" {
			b.WriteString(s[:i] + v)
		} else {
			b.WriteString(s[:i+2+j])
		}
		s = s[i+2+j:]
	}
	return b.String() + s
}

// slashPath normalizes a native path the way domain.Path values are.
func slashPath(s string) string {
	return strings.ReplaceAll(s, `\`, "/")
}

// nativeDisplay writes a slash-separated path with goos's separator.
func nativeDisplay(goos, p string) string {
	if goos == "windows" {
		return strings.ReplaceAll(p, "/", `\`)
	}
	return p
}

// manualInstallCommand is the command that does what Install could not:
// PowerShell on Windows, a POSIX shell elsewhere. It never adds sudo; the
// target is the user's own directory.
func manualInstallCommand(deps InstallDeps, dir, target domain.Path) string {
	exe, err := deps.Executable()
	if err != nil {
		exe = "<path to this wspace binary>"
	}
	if deps.GOOS == "windows" {
		ps := func(s string) string { return "'" + strings.ReplaceAll(nativeDisplay("windows", s), "'", "''") + "'" }
		return fmt.Sprintf("New-Item -ItemType Directory -Force -Path %s | Out-Null; Copy-Item %s %s", ps(string(dir)), ps(exe), ps(string(target)))
	}
	return fmt.Sprintf("mkdir -p %s && cp %s %s && chmod +x %s", posixQuote(string(dir)), posixQuote(exe), posixQuote(string(target)), posixQuote(string(target)))
}
