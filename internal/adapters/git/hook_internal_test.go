// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHookInstalled_FollowsGitsLookupPerOS(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "pre-commit")
	if err := os.WriteFile(plain, []byte("#!/bin/sh\nexit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "commit-msg.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Windows has no executable bit: git runs any file at the hook path,
	// or at the hook path plus ".exe".
	for _, p := range []string{plain, filepath.Join(dir, "commit-msg")} {
		if !hookInstalled(p, "windows") {
			t.Errorf("hookInstalled(%q, windows) = false, want true", p)
		}
	}
	for _, goos := range []string{"windows", "linux"} {
		if hookInstalled(filepath.Join(dir, "missing"), goos) {
			t.Errorf("hookInstalled(missing, %s) = true", goos)
		}
		if hookInstalled(dir, goos) {
			t.Errorf("hookInstalled(a directory, %s) = true", goos)
		}
	}

	// Unix needs the executable bit.
	if hookInstalled(plain, "linux") {
		t.Error("hookInstalled(non-executable file, linux) = true")
	}
	if runtime.GOOS == "windows" {
		return // the executable bit cannot be set here
	}
	if err := os.Chmod(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if !hookInstalled(plain, "linux") {
		t.Error("hookInstalled(executable file, linux) = false")
	}
}
