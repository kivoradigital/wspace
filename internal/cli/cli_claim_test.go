// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// seedOwned writes a wspace manifest owned by owner under the fixture
// context's WorkspacesRoot.
func seedOwned(t *testing.T, fx *fixture, name string, owner domain.ContextName) domain.Path {
	t.Helper()
	root := domain.Path("/fixture/workspaces").Join(name)
	fx.Store.PutManifest(root, domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{Name: name, Root: root, Context: owner}})
	if err := fx.FS.MkdirAll(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func ownerOf(t *testing.T, fx *fixture, root domain.Path) domain.ContextName {
	t.Helper()
	m, err := fx.Store.LoadManifest(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return m.Workspace.Context
}

func TestCLI_ListFlagsOrphansAndClaimRecoversThem(t *testing.T) {
	fx := newFixture(t)
	fx.Store.PutContext(domain.Context{Name: "acme", WorkspacesRoot: "/fixture/workspaces"})
	findings := seedOwned(t, fx, "findings", "legacy")
	seedOwned(t, fx, "memoryleak", "legacy")
	theirs := seedOwned(t, fx, "theirs", "acme")

	stdout, stderr, code := run(fx.RT, "", "list")
	if code != 0 || !strings.Contains(stdout, "findings") || !strings.Contains(stdout, "orphaned") || !strings.Contains(stdout, "wspace claim findings") || strings.Contains(stdout, "theirs") {
		t.Fatalf("list = %q (stderr %q, code %d), want orphans flagged and theirs hidden", stdout, stderr, code)
	}
	stdout, _, _ = run(fx.RT, "", "list", "--json")
	if !strings.Contains(stdout, `"orphan_of": "legacy"`) {
		t.Fatalf("list --json = %s, want orphan_of", stdout)
	}

	stdout, stderr, code = run(fx.RT, "", "claim", "findings", "theirs", "--json")
	if code != 0 {
		t.Fatalf("claim --json exit %d, stderr %q", code, stderr)
	}
	var res struct {
		Claimed []struct {
			Name            string `json:"name"`
			Root            string `json:"root"`
			PreviousContext string `json:"previous_context"`
		} `json:"claimed"`
		Skipped []struct{ Name, Root, Reason, Message string } `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("claim --json output %q: %v", stdout, err)
	}
	if len(res.Claimed) != 1 || res.Claimed[0].Name != "findings" || res.Claimed[0].PreviousContext != "legacy" {
		t.Fatalf("claimed = %+v", res.Claimed)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Name != "theirs" || res.Skipped[0].Reason != "claim.skip.owned_by_other" || res.Skipped[0].Message == "" {
		t.Fatalf("skipped = %+v", res.Skipped)
	}
	if ownerOf(t, fx, findings) != "work" || ownerOf(t, fx, theirs) != "acme" {
		t.Fatalf("owners = %s/%s, want work/acme", ownerOf(t, fx, findings), ownerOf(t, fx, theirs))
	}

	stdout, _, code = run(fx.RT, "", "claim")
	if code != 0 || !strings.Contains(stdout, "memoryleak") || !strings.Contains(stdout, "legacy") {
		t.Fatalf("claim = %q (code %d), want memoryleak claimed", stdout, code)
	}
	stdout, _, code = run(fx.RT, "", "claim")
	if code != 0 || !strings.Contains(stdout, "no orphaned workspaces") {
		t.Fatalf("second claim = %q (code %d), want nothing to claim", stdout, code)
	}
}

func TestCLI_Context_EditRenameReportsReassignedWorkspaces(t *testing.T) {
	fx := newFixture(t)
	mine := seedOwned(t, fx, "mine", "work")
	prompter := portstest.NewScriptedPrompter(t,
		portstest.TextAnswer("job"), // context name, renamed
		portstest.TextAnswer("/fixture/workspaces"),
		portstest.TextAnswer(""),
		portstest.TextAnswer(""),
		portstest.TextAnswer("main"),
		portstest.ConfirmAnswer(true),
		portstest.ConfirmAnswer(true),
	)
	fx.RT.ContextWizard = app.ContextWizardDeps{Store: fx.Store, Prompter: prompter}

	stdout, stderr, code := run(fx.RT, "", "context", "edit", "work")
	if code != 0 {
		t.Fatalf("context edit exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "reassigned 1 workspace(s) to context job") {
		t.Fatalf("context edit stdout = %q, want the reassignment reported", stdout)
	}
	if ownerOf(t, fx, mine) != "job" {
		t.Fatalf("mine owner = %s, want job", ownerOf(t, fx, mine))
	}
}
