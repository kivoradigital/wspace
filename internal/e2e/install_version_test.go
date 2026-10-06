// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestVersionCheck_UnbrandedBuildReportsUnavailableAndExitsZero drives the
// real binary end to end for phase 5's update-check surface. The e2e
// binary this package builds (wsBinary, plain `go build ./cmd/ws`, no
// -ldflags) is itself an unbranded build — buildinfo.RepoOwner/RepoName
// are unset — so this exercises the "unbranded build" degradation path
// deterministically, with no dependency on real network reachability:
// exactly the offline/unavailable contract `ws version --check` MUST
// satisfy (update-check spec: "Graceful offline degradation").
func TestVersionCheck_UnbrandedBuildReportsUnavailableAndExitsZero(t *testing.T) {
	fx := NewFixture(t)

	stdout, stderr, code := fx.Run("", "version", "--check")
	if code != 0 {
		t.Fatalf("ws version --check: exit code = %d, want 0; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "unavailable") {
		t.Fatalf("stdout = %q, want it to report the update check as unavailable", stdout)
	}

	// Plain `ws version` (no --check) must never mention the update check
	// at all — design.md §11: "ws version alone never touches the network".
	plainStdout, _, plainCode := fx.Run("", "version")
	if plainCode != 0 {
		t.Fatalf("ws version: exit code = %d, want 0", plainCode)
	}
	if strings.Contains(plainStdout, "unavailable") || strings.Contains(plainStdout, "update") {
		t.Fatalf("plain 'ws version' stdout = %q, want no update-check text at all", plainStdout)
	}
}

// TestInstall_RealBinaryPlacesItselfOnAWritablePathDirectory drives `ws
// install` against a real, writable temporary directory placed first on
// PATH, then `ws install --uninstall` to reverse it — proving the whole
// path end to end against the real filesystem and the real running
// binary, not a fake.
func TestInstall_RealBinaryPlacesItselfOnAWritablePathDirectory(t *testing.T) {
	fx := NewFixture(t)

	// install's own candidate directories are all HOME-relative
	// (~/.local/bin, ~/bin) or the fixed /usr/local/bin — so the writable
	// directory this test puts on PATH must be one of those exact paths,
	// not an arbitrary temp directory, for the real binary to ever choose
	// it.
	localBin := filepath.Join(fx.Home, ".local", "bin")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", localBin, err)
	}
	env := []string{
		"PATH=" + localBin + ":" + os.Getenv("PATH"),
		"SHELL=/bin/bash",
	}

	stdout, stderr, code := fx.RunEnv(env, "y\n", "install")
	if code != 0 {
		t.Fatalf("ws install: exit code = %d, want 0; stdout=%q stderr=%q", code, stdout, stderr)
	}

	installedBin := filepath.Join(localBin, "wspace")
	if !PathExists(t, installedBin) {
		t.Fatalf("%s does not exist after install", installedBin)
	}
	info, err := os.Stat(installedBin)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary mode = %v, want it executable", info.Mode())
	}

	rcPath := bashRC(fx.Home)
	if !PathExists(t, rcPath) {
		t.Fatal(".bashrc was not created despite accepting the shell-integration prompt")
	}
	rcContent, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("read .bashrc: %v", err)
	}
	if !strings.Contains(string(rcContent), "wspace shell-init bash") {
		t.Fatalf(".bashrc = %q, want it to source wspace shell-init bash", rcContent)
	}

	// Running install again, accepting the prompt again, must not duplicate
	// the shell-init block (idempotency).
	if _, _, code := fx.RunEnv(env, "y\n", "install"); code != 0 {
		t.Fatalf("second ws install: exit code = %d, want 0", code)
	}
	rcAfterSecond, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("read .bashrc after second install: %v", err)
	}
	if strings.Count(string(rcAfterSecond), ">>> wspace shell integration >>>") != 1 {
		t.Fatalf(".bashrc after a second accepted install = %q, want the marker exactly once", rcAfterSecond)
	}

	// --uninstall removes both the binary and the rc block.
	uninstallStdout, uninstallStderr, uninstallCode := fx.RunEnv(env, "", "install", "--uninstall")
	if uninstallCode != 0 {
		t.Fatalf("ws install --uninstall: exit code = %d, want 0; stdout=%q stderr=%q", uninstallCode, uninstallStdout, uninstallStderr)
	}
	if PathExists(t, installedBin) {
		t.Fatal("installed binary still exists after --uninstall")
	}
	rcAfterUninstall, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("read .bashrc after uninstall: %v", err)
	}
	if strings.Contains(string(rcAfterUninstall), "ws shell integration") {
		t.Fatalf(".bashrc after --uninstall = %q, want the shell-integration block removed", rcAfterUninstall)
	}
}

// TestInstall_NotOnPathInstallsAndAddsItToPathWithYes drives `wspace
// install --yes` with a PATH that does not list ~/.local/bin (a fresh
// Linux account): the binary is copied there anyway, ~/.bashrc gets the
// PATH block, nothing asks for elevated rights, and --uninstall removes
// both.
func TestInstall_NotOnPathInstallsAndAddsItToPathWithYes(t *testing.T) {
	fx := NewFixture(t)
	env := []string{"PATH=/usr/bin:/bin", "SHELL=/bin/bash"}

	stdout, stderr, code := fx.RunEnv(env, "", "install", "--yes")
	if code != 0 {
		t.Fatalf("wspace install --yes: exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
	}
	localBin := filepath.Join(fx.Home, ".local", "bin")
	if info, err := os.Stat(filepath.Join(localBin, "wspace")); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary = %v, %v", info, err)
	}
	rc, err := os.ReadFile(bashRC(fx.Home))
	if err != nil || !strings.Contains(string(rc), "export PATH='"+localBin+"'") {
		t.Fatalf(".bashrc = %q, %v", rc, err)
	}
	for _, want := range []string{"added " + localBin + " to your PATH", "open a new terminal"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
	if strings.Contains(stdout+stderr, "sudo") {
		t.Fatalf("output mentions sudo: %q", stdout)
	}

	again, _, _ := fx.RunEnv(env, "", "install", "--uninstall")
	if !strings.Contains(again, "removed the PATH entry") {
		t.Fatalf("uninstall stdout = %q", again)
	}
	if rc, _ := os.ReadFile(bashRC(fx.Home)); strings.Contains(string(rc), "wspace") {
		t.Fatalf(".bashrc after uninstall = %q", rc)
	}
}

// bashRC is where install writes for bash: the login file on macOS (whose
// terminals start login shells), ~/.bashrc elsewhere.
func bashRC(home string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".bash_profile")
	}
	return filepath.Join(home, ".bashrc")
}
