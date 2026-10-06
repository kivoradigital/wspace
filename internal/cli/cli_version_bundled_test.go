// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestCLI_Version_Bundled covers a CLI embedded in a desktop app. The first
// line of `wspace version` stays exactly "wspace version X" in every case,
// because other tools parse it (e.g. `sed -n 's/^wspace version //p'`); the
// bundled note goes on its own second line. `version --check` never calls
// the checker for a bundled build.
func TestCLI_Version_Bundled(t *testing.T) {
	t.Run("plain version, normal build", func(t *testing.T) {
		fx := newFixture(t)
		fx.RT.Version = "0.1.0"

		stdout, stderr, code := run(fx.RT, "", "version")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		if stdout != "wspace version 0.1.0\n" {
			t.Fatalf("stdout = %q, want exactly one line %q", stdout, "wspace version 0.1.0")
		}
		compareGolden(t, "version_plain.golden", stdout)
	})

	t.Run("plain version, bundled build", func(t *testing.T) {
		fx := newFixture(t)
		fx.RT.Version = "0.1.0"
		fx.RT.BundledBy = "Wspace Dev"

		stdout, stderr, code := run(fx.RT, "", "version")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
		if len(lines) != 2 || lines[0] != "wspace version 0.1.0" || lines[1] != "(bundled with Wspace Dev)" {
			t.Fatalf("stdout = %q, want first line %q then %q", stdout, "wspace version 0.1.0", "(bundled with Wspace Dev)")
		}
		compareGolden(t, "version_bundled.golden", stdout)
	})

	t.Run("version --check, bundled build never calls the checker", func(t *testing.T) {
		fx := newFixture(t)
		fx.RT.Version = "0.1.0"
		fx.RT.BundledBy = "Wspace Dev"
		fx.RT.RepoCoordinates = domain.RepoCoordinates{Owner: "acme", Repo: "widget"}
		checker := portstest.NewFakeReleaseChecker()
		checker.Response = domain.ReleaseInfo{Tag: "v9.9.9"}
		fx.RT.CheckDeps = app.CheckForUpdateDeps{Checker: checker}

		stdout, stderr, code := run(fx.RT, "", "version", "--check")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		if checker.Calls != 0 {
			t.Fatalf("checker.Calls = %d, want 0: a bundled CLI must never query the network", checker.Calls)
		}
		if !strings.HasPrefix(stdout, "wspace version 0.1.0\n") {
			t.Fatalf("stdout = %q, want the exact version line first", stdout)
		}
		if !strings.Contains(stdout, "wspace 0.1.0 is bundled with Wspace Dev and updates with it\n") {
			t.Fatalf("stdout = %q, want the bundled update note", stdout)
		}
		compareGolden(t, "version_check_bundled.golden", stdout)
	})
}
