// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestCLI_Context_CreateListSwitchShow covers the context command group:
// the user-facing entry point to multi-context support (context-management
// spec: "Context lifecycle commands").
func TestCLI_Context_CreateListSwitchShow(t *testing.T) {
	fx := newFixture(t)
	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("oss"),
		portstest.TextAnswer("/fixture/oss-workspaces"),
		portstest.TextAnswer(""),
		portstest.TextAnswer(""), // ignore_patterns -> none
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(false), // decline manual project registration
	)
	fx.RT.ContextWizard = app.ContextWizardDeps{Store: fx.Store, Prompter: prompter}
	fx.RT.ProjectWizard = app.ProjectWizardDeps{Store: fx.Store, FS: fx.FS, Git: fx.Git, Prompter: prompter}

	stdout, stderr, code := run(fx.RT, "", "context", "create")
	if code != 0 {
		t.Fatalf("context create: exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "oss") {
		t.Fatalf("context create stdout = %q, want it to mention the new context", stdout)
	}
	prompter.CheckUnconsumed()

	stdout, stderr, code = run(fx.RT, "", "context", "list")
	if code != 0 {
		t.Fatalf("context list: exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "work") || !strings.Contains(stdout, "oss") {
		t.Fatalf("context list stdout = %q, want both work and oss", stdout)
	}
	if !strings.Contains(stdout, "active") {
		t.Fatalf("context list stdout = %q, want an active marker on work", stdout)
	}

	_, stderr, code = run(fx.RT, "", "context", "switch", "oss")
	if code != 0 {
		t.Fatalf("context switch: exit code = %d, want 0; stderr=%q", code, stderr)
	}

	stdout, stderr, code = run(fx.RT, "", "context", "show")
	if code != 0 {
		t.Fatalf("context show: exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "oss") {
		t.Fatalf("context show stdout = %q, want the now-active oss context", stdout)
	}
}

// TestCLI_Context_EditUpdatesFields covers this change's own authorized
// gap-closure: "ws context edit [name]" must drive
// app.RunEditContextWizard through the exact same ports.Prompter every
// other wizard-backed command already uses, prefilled from the named
// context's persisted values.
func TestCLI_Context_EditUpdatesFields(t *testing.T) {
	fx := newFixture(t)
	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("work"), // context name, unchanged
		portstest.TextAnswer("/fixture/workspaces-renamed"),
		portstest.TextAnswer(""),
		portstest.TextAnswer(""), // ignore_patterns -> none
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(true),
	)
	fx.RT.ContextWizard = app.ContextWizardDeps{Store: fx.Store, Prompter: prompter}

	stdout, stderr, code := run(fx.RT, "", "context", "edit", "work")
	if code != 0 {
		t.Fatalf("context edit: exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "work") {
		t.Fatalf("context edit stdout = %q, want it to mention the edited context", stdout)
	}
	prompter.CheckUnconsumed()

	edited, err := fx.Store.LoadContext(context.Background(), "work")
	if err != nil {
		t.Fatalf("LoadContext(work) after edit: %v", err)
	}
	if edited.WorkspacesRoot != "/fixture/workspaces-renamed" {
		t.Fatalf("WorkspacesRoot = %q, want the edited value", edited.WorkspacesRoot)
	}
}

// TestCLI_Context_RemoveDeletesNonActiveContext covers "ws context remove
// <name>" (this change's own authorized gap-closure addition).
func TestCLI_Context_RemoveDeletesNonActiveContext(t *testing.T) {
	fx := newFixture(t)
	fx.Store.PutContext(domain.Context{Name: "oss", WorkspacesRoot: "/fixture/oss-workspaces"})

	stdout, stderr, code := run(fx.RT, "", "context", "remove", "oss")
	if code != 0 {
		t.Fatalf("context remove: exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "oss") {
		t.Fatalf("context remove stdout = %q, want it to mention the removed context", stdout)
	}

	if _, err := fx.Store.LoadContext(context.Background(), "oss"); err == nil {
		t.Fatal("LoadContext(oss) succeeded after context remove, want it gone")
	}
}

// TestCLI_Context_RemoveRefusesActiveContext covers the safety guard at
// the CLI surface: removing the active context ("work", per newFixture)
// must fail with a non-zero exit code, not silently succeed.
func TestCLI_Context_RemoveRefusesActiveContext(t *testing.T) {
	fx := newFixture(t)

	_, stderr, code := run(fx.RT, "", "context", "remove", "work")
	if code == 0 {
		t.Fatalf("context remove (active): exit code = 0, want non-zero; stderr=%q", stderr)
	}
}

// TestCLI_Context_RmAllowActiveDeletesTheActiveContext covers the opt-in
// that removes the active (here the only) context and clears it; "rm" is
// an alias of "remove".
func TestCLI_Context_RmAllowActiveDeletesTheActiveContext(t *testing.T) {
	fx := newFixture(t)

	_, stderr, code := run(fx.RT, "", "context", "rm", "work", "--allow-active")
	if code != 0 {
		t.Fatalf("context rm --allow-active: exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if _, err := fx.Store.LoadContext(context.Background(), "work"); err == nil {
		t.Fatal("LoadContext(work) succeeded after removal, want it gone")
	}
	root, err := fx.Store.LoadRoot(context.Background())
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if root.ActiveContext != "" {
		t.Fatalf("ActiveContext = %q, want it cleared", root.ActiveContext)
	}
}
