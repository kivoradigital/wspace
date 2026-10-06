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

// TestCLI_VersionCheck_JSONAndHuman covers tasks.md 5.16. The cli-surface
// spec's own "--json contract on read-only commands" requirement scopes
// --json to exactly {info, list, status} and states the converse as a MUST
// too ("Non-JSON commands remain human-output only" — any other command
// given --json MUST be rejected). version is not one of the three, so this
// test's "JSON" half proves --check does not quietly carve out an
// exception to that MUST; its "Human" half proves the three outcomes
// version --check can render (up to date, available, unavailable) each
// go through the catalog correctly.
func TestCLI_VersionCheck_JSONAndHuman(t *testing.T) {
	t.Run("json flag is rejected on version, same as any other unsupported command", func(t *testing.T) {
		fx := newFixture(t)
		_, stderr, code := run(fx.RT, "", "version", "--check", "--json")
		if code == 0 {
			t.Fatal("exit code = 0, want non-zero: --json is not supported on version")
		}
		if !strings.Contains(stderr, "--json") {
			t.Fatalf("stderr = %q, want it to name --json as unsupported", stderr)
		}
	})

	t.Run("up to date", func(t *testing.T) {
		fx := newFixture(t)
		fx.RT.Version = "v1.0.0"
		fx.RT.RepoCoordinates = domain.RepoCoordinates{Owner: "acme", Repo: "widget"}
		checker := portstest.NewFakeReleaseChecker()
		checker.Response = domain.ReleaseInfo{Tag: "v1.0.0"}
		fx.RT.CheckDeps = app.CheckForUpdateDeps{Checker: checker}

		stdout, stderr, code := run(fx.RT, "", "version", "--check")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		compareGolden(t, "version_check_up_to_date.golden", stdout)
	})

	t.Run("update available", func(t *testing.T) {
		fx := newFixture(t)
		fx.RT.Version = "v1.0.0"
		fx.RT.RepoCoordinates = domain.RepoCoordinates{Owner: "acme", Repo: "widget"}
		checker := portstest.NewFakeReleaseChecker()
		checker.Response = domain.ReleaseInfo{Tag: "v2.0.0", URL: "https://example.invalid/v2.0.0"}
		fx.RT.CheckDeps = app.CheckForUpdateDeps{Checker: checker}

		stdout, stderr, code := run(fx.RT, "", "version", "--check")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		compareGolden(t, "version_check_available.golden", stdout)
	})

	t.Run("unavailable (unbranded build) still exits 0", func(t *testing.T) {
		fx := newFixture(t)
		fx.RT.Version = "v1.0.0"
		// RepoCoordinates left at its zero value: an unbranded build.
		checker := portstest.NewFakeReleaseChecker()
		checker.Response = domain.ReleaseInfo{Tag: "v9.9.9"}
		fx.RT.CheckDeps = app.CheckForUpdateDeps{Checker: checker}

		stdout, stderr, code := run(fx.RT, "", "version", "--check")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0: an unavailable check is not a command failure; stderr=%q", code, stderr)
		}
		if checker.Calls != 0 {
			t.Fatalf("checker.Calls = %d, want 0: an unbranded build must never query the network", checker.Calls)
		}
		compareGolden(t, "version_check_unavailable.golden", stdout)
	})

	t.Run("plain version never touches the checker", func(t *testing.T) {
		fx := newFixture(t)
		checker := portstest.NewFakeReleaseChecker()
		fx.RT.CheckDeps = app.CheckForUpdateDeps{Checker: checker}

		_, stderr, code := run(fx.RT, "", "version")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		if checker.Calls != 0 {
			t.Fatalf("checker.Calls = %d, want 0: plain 'version' (no --check) must never trigger a network call", checker.Calls)
		}
	})
}
