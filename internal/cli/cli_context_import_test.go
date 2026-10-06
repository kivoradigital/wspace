// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestCLI_ContextImport_NewContext covers "ws context import --from <path>
// --name <name>": a legacy flat configuration file becomes a brand-new
// context, prompted through the exact same field sequence "context
// create" uses.
func TestCLI_ContextImport_NewContext(t *testing.T) {
	fx := newFixture(t)
	if err := fx.FS.WriteFile("/legacy/ws.config", []byte("base_branch = develop\ncopy_env_default = yes\n[projects]\n"), 0o644); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}
	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("/fixture/imported-workspaces"), // workspaces_root (no value in source)
		portstest.TextAnswer(""),                             // projects_root
		portstest.TextAnswer(""),                             // ignore_patterns
		portstest.TextAnswer("develop"),                      // base_branch, prefilled from source
		portstest.ConfirmAnswer(true),                        // copy_env_default, prefilled from source
		portstest.ConfirmAnswer(false),                       // fetch_before_create, built-in default
	)
	fx.RT.ContextWizard = app.ContextWizardDeps{Store: fx.Store, Prompter: prompter}

	stdout, stderr, code := run(fx.RT, "", "context", "import", "--from", "/legacy/ws.config", "--name", "imported")
	if code != 0 {
		t.Fatalf("context import: exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "imported") {
		t.Fatalf("context import stdout = %q, want it to mention the new context name", stdout)
	}
	prompter.CheckUnconsumed()

	persisted, err := fx.Store.LoadContext(context.Background(), "imported")
	if err != nil {
		t.Fatalf("LoadContext(imported): %v", err)
	}
	if persisted.Defaults.BaseBranch == nil || *persisted.Defaults.BaseBranch != "develop" {
		t.Fatalf("Defaults.BaseBranch = %v, want develop", persisted.Defaults.BaseBranch)
	}
}

// TestCLI_ContextImport_ExistingContext_Decline covers "ws context import
// <name> --from <path>": importing into an existing context shows a diff
// and, on decline, writes nothing.
func TestCLI_ContextImport_ExistingContext_Decline(t *testing.T) {
	fx := newFixture(t)
	if err := fx.FS.WriteFile("/legacy/ws.config", []byte("base_branch = develop\n[projects]\n"), 0o644); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}
	prompter := portstest.NewScriptedPrompter(t, portstest.ConfirmAnswer(false))
	fx.RT.ContextWizard = app.ContextWizardDeps{Store: fx.Store, Prompter: prompter}

	stdout, stderr, code := run(fx.RT, "", "context", "import", "work", "--from", "/legacy/ws.config")
	if code != 0 {
		t.Fatalf("context import: exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "cancelled") {
		t.Fatalf("context import stdout = %q, want a cancellation notice", stdout)
	}
	prompter.CheckUnconsumed()
}
