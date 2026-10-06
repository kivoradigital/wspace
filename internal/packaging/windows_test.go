// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package packaging_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPackagingManifest_WindowsInstallerScriptReferencesPathEntry covers
// tasks.md 8.4 (packaging-distribution spec: "Installer completes without
// manual PATH edit"): packaging/windows/wspace.iss must actually register the
// install directory on PATH, not merely place binaries there and leave a
// manual step, and must install the CLI (wspace.exe) only: Linux and
// Windows ship the CLI alone (there is no tray GUI). Asserted
// structurally by reading the real, checked-in script — writing an Inno
// Setup parser for one file is not worth it, and actually compiling the
// installer requires Inno Setup's own Windows-only iscc.exe (this phase's
// documented "manual only" / CI-only verification, not a unit test's
// job).
func TestPackagingManifest_WindowsInstallerScriptReferencesPathEntry(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "packaging", "windows", "wspace.iss"))
	if err != nil {
		t.Fatalf("read wspace.iss: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"// --- PATH environment registration ---",
		"EnvAddPath",
		"EnvRemovePath",
		"wspace.exe",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("wspace.iss does not contain %q, want the PATH-registration section and the CLI binary listed", want)
		}
	}
	if strings.Contains(content, "wspace-tray") {
		t.Error("wspace.iss still references wspace-tray; the tray was removed and the installer must package the CLI only")
	}
}

// moduleRoot locates the module root by walking up from this file's own
// directory until go.mod is found.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("packaging: cannot determine caller for module root discovery")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("packaging: go.mod not found above internal/packaging")
		}
		dir = parent
	}
}
