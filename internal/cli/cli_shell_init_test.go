// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import "testing"

// TestShellInit_EmitsPOSIXAndFishFunctions covers tasks.md 4b.17
// (shell-integration spec: "shell-init emits shell-specific functions").
func TestShellInit_EmitsPOSIXAndFishFunctions(t *testing.T) {
	fx := newFixture(t)

	for _, tc := range []struct {
		shell  string
		golden string
	}{
		{"bash", "shell_init_bash.golden"},
		{"zsh", "shell_init_bash.golden"},
		{"sh", "shell_init_bash.golden"},
		{"fish", "shell_init_fish.golden"},
	} {
		stdout, stderr, code := run(fx.RT, "", "shell-init", tc.shell)
		if code != 0 {
			t.Fatalf("shell-init %s: exit code = %d, want 0; stderr=%q", tc.shell, code, stderr)
		}
		compareGolden(t, tc.golden, stdout)
	}
}

// TestShellInit_UnknownShellIsRejected covers the usage-error path.
func TestShellInit_UnknownShellIsRejected(t *testing.T) {
	fx := newFixture(t)
	_, stderr, code := run(fx.RT, "", "shell-init", "powershell")
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero for an unsupported shell")
	}
	if stderr == "" {
		t.Fatal("stderr = empty, want a usage message")
	}
}
