// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package agenthost

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"
)

// ErrNotFound is returned by Resolver.Command when the program is not
// installed where the user's shell would find it.
var ErrNotFound = errors.New("not found on PATH")

// ErrUnsafeBatchArgument is returned by Resolver.Command for an argument
// that cannot be passed through cmd.exe to a batch file without being
// reinterpreted: a double quote, a percent sign (variable expansion cannot
// be escaped on a cmd.exe command line) or a line break.
var ErrUnsafeBatchArgument = errors.New("argument cannot be passed safely to a batch file")

// Resolver finds agent CLIs (claude, codex, gemini) the way the user's
// shell does, independently of the host it runs on: GOOS, Getenv and Stat
// are injected so Windows resolution is tested on any machine.
//
// On Windows it follows cmd.exe: PATH in order and, within each directory,
// the PATHEXT extensions in order (".COM;.EXE;.BAT;.CMD" when unset).
// Directories inside a node_modules tree are never used: an npm package's
// internal binary (node_modules\@anthropic-ai\claude-code\bin\claude.exe)
// is what its shim runs, not what the user types. When PATH has no match
// (a process started before the CLI was installed), the per-user install
// locations are tried: %USERPROFILE%\.local\bin\<name>.exe (Claude Code's
// native installer) and %APPDATA%\npm\<name>.cmd (npm's default global
// prefix). On other systems it walks PATH for an executable regular file,
// again skipping node_modules directories. Relative PATH entries are
// ignored on every system (os/exec's ErrDot rule).
type Resolver struct {
	GOOS   string
	Getenv func(string) string
	// Stat reports whether the native path p is a regular file (following
	// symlinks) and, on Unix, whether it is executable.
	Stat func(p string) (isFile, executable bool)
}

// Invocation is a resolved program ready to start. Path is the program
// to execute and Args its arguments; for a Windows batch file Path is
// cmd.exe and CmdLine the complete, already quoted command line (Go's
// SysProcAttr.CmdLine), because cmd.exe does not parse arguments the way
// os/exec quotes them. Script is the resolved name (the .cmd itself).
type Invocation struct {
	Path    string
	Args    []string
	CmdLine string
	Script  string
}

// NewResolver returns the Resolver of the running system.
func NewResolver() Resolver {
	return Resolver{GOOS: runtime.GOOS, Getenv: os.Getenv, Stat: statFile}
}

func statFile(p string) (bool, bool) {
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return false, false
	}
	return true, info.Mode()&0o111 != 0
}

var _ fs.FileMode // keep io/fs for statFile's mode bits documentation

// LookPath returns the program the user's shell runs for name. A name
// that already is a path is used as given when the file exists.
func (r Resolver) LookPath(name string) (string, bool) {
	if strings.Contains(name, "/") || (r.GOOS == "windows" && strings.ContainsAny(name, `\:`)) {
		if isFile, _ := r.Stat(name); isFile {
			return name, true
		}
		return "", false
	}
	if r.GOOS == "windows" {
		return r.lookWindows(name)
	}
	return r.lookUnix(name)
}

// Command resolves name and builds how to start it with args.
func (r Resolver) Command(name string, args []string) (Invocation, error) {
	p, ok := r.LookPath(name)
	if !ok {
		return Invocation{}, fmt.Errorf("%s: %w", name, ErrNotFound)
	}
	if r.GOOS != "windows" || !isBatch(p) {
		return Invocation{Path: p, Args: args, Script: p}, nil
	}
	line, err := batchCommandLine(r.comSpec(), p, args)
	if err != nil {
		return Invocation{}, err
	}
	return Invocation{Path: r.comSpec(), CmdLine: line, Script: p}, nil
}

func (r Resolver) lookUnix(name string) (string, bool) {
	if strings.Contains(name, "/") {
		return "", false
	}
	for _, dir := range strings.Split(r.Getenv("PATH"), ":") {
		if !strings.HasPrefix(dir, "/") || inNodeModules(dir, "/") {
			continue
		}
		p := strings.TrimRight(dir, "/") + "/" + name
		if isFile, exec := r.Stat(p); isFile && exec {
			return p, true
		}
	}
	return "", false
}

func (r Resolver) lookWindows(name string) (string, bool) {
	if strings.ContainsAny(name, `\/:`) {
		return "", false
	}
	exts := r.pathExt()
	for _, dir := range strings.Split(r.Getenv("PATH"), ";") {
		dir = strings.Trim(strings.TrimSpace(dir), `"`)
		if !isWindowsAbs(dir) || inNodeModules(dir, `\`) {
			continue
		}
		if p, ok := r.findInDir(strings.TrimRight(dir, `\/`), name, exts); ok {
			return p, true
		}
	}
	var fallbacks []string
	if home := r.Getenv("USERPROFILE"); home != "" {
		fallbacks = append(fallbacks, home+`\.local\bin\`+name+".exe")
	}
	if appData := r.Getenv("APPDATA"); appData != "" {
		fallbacks = append(fallbacks, appData+`\npm\`+name+".cmd")
	}
	for _, p := range fallbacks {
		if isFile, _ := r.Stat(p); isFile {
			return p, true
		}
	}
	return "", false
}

// findInDir tries name as given when it already carries a PATHEXT
// extension, then name plus each extension in order.
func (r Resolver) findInDir(dir, name string, exts []string) (string, bool) {
	base := dir + `\` + name
	lower := strings.ToLower(name)
	for _, e := range exts {
		if strings.HasSuffix(lower, e) {
			if isFile, _ := r.Stat(base); isFile {
				return base, true
			}
			break
		}
	}
	for _, e := range exts {
		if isFile, _ := r.Stat(base + e); isFile {
			return base + e, true
		}
	}
	return "", false
}

// pathExt is PATHEXT's extensions, lowercased, in order.
func (r Resolver) pathExt() []string {
	var exts []string
	for _, e := range strings.Split(strings.ToLower(r.Getenv("PATHEXT")), ";") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		exts = append(exts, e)
	}
	if len(exts) == 0 {
		exts = []string{".com", ".exe", ".bat", ".cmd"}
	}
	return exts
}

// comSpec is cmd.exe's path: %ComSpec%, else %SystemRoot%\System32.
func (r Resolver) comSpec() string {
	if c := r.Getenv("ComSpec"); c != "" {
		return c
	}
	root := r.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return root + `\System32\cmd.exe`
}

func isWindowsAbs(p string) bool {
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

func inNodeModules(dir, sep string) bool {
	for _, part := range strings.FieldsFunc(dir, func(c rune) bool { return string(c) == sep || c == '/' }) {
		if strings.EqualFold(part, "node_modules") {
			return true
		}
	}
	return false
}

func isBatch(p string) bool {
	l := strings.ToLower(p)
	return strings.HasSuffix(l, ".cmd") || strings.HasSuffix(l, ".bat")
}

// batchCommandLine builds `cmd.exe /d /v:off /s /c ""<script>" "<arg>"…"`.
// /d skips AutoRun commands, /v:off disables delayed (!var!) expansion
// and /s makes cmd.exe strip exactly the outer pair of quotes. Every
// argument is quoted, so cmd.exe's metacharacters (& | < > ^ ( )) stay
// literal, and quoted the way CommandLineToArgvW reads it (a trailing
// backslash run doubled) for the program the script finally starts.
// What cmd.exe cannot carry safely inside quotes — '"', '%', line breaks
// — is refused rather than escaped (Go's documented approach for batch
// files: build the line yourself in SysProcAttr.CmdLine).
func batchCommandLine(comSpec, script string, args []string) (string, error) {
	parts := make([]string, 0, len(args)+1)
	for _, a := range append([]string{script}, args...) {
		if strings.ContainsAny(a, "\"%\r\n") {
			return "", fmt.Errorf("%q: %w", a, ErrUnsafeBatchArgument)
		}
		trailing := len(a) - len(strings.TrimRight(a, `\`))
		parts = append(parts, `"`+a+strings.Repeat(`\`, trailing)+`"`)
	}
	return comSpec + ` /d /v:off /s /c "` + strings.Join(parts, " ") + `"`, nil
}
