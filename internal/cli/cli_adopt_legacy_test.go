// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// seedLegacy writes a workspace the legacy bash tool created under the
// fixture context's WorkspacesRoot.
func seedLegacy(t *testing.T, fx *fixture, name, conf string) domain.Path {
	t.Helper()
	root := domain.Path("/fixture/workspaces").Join(name)
	for _, d := range []domain.Path{root.Join(".ws"), root.Join("svc")} {
		if err := fx.FS.MkdirAll(d); err != nil {
			t.Fatal(err)
		}
	}
	if err := fx.FS.WriteFile(root.Join(".ws", "workspace.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCLI_AdoptLegacy_JSONAndHuman(t *testing.T) {
	fx := newFixture(t)
	seedLegacy(t, fx, "old", "branch = feat\n[repos]\nsvc|svc|feat\n")
	seedLegacy(t, fx, "stray", "branch = feat\n[repos]\nsvc|nobody|feat\n")

	// Before adoption, list flags the legacy workspaces instead of hiding them.
	stdout, stderr, code := run(fx.RT, "", "list")
	if code != 0 || !strings.Contains(stdout, "old") || !strings.Contains(stdout, "adopt-legacy") {
		t.Fatalf("list = %q (stderr %q, code %d), want the legacy workspace flagged", stdout, stderr, code)
	}
	stdout, _, _ = run(fx.RT, "", "list", "--json")
	if !strings.Contains(stdout, `"legacy": true`) {
		t.Fatalf("list --json = %s, want a legacy flag", stdout)
	}

	stdout, stderr, code = run(fx.RT, "", "adopt-legacy", "--json")
	if code != 0 {
		t.Fatalf("adopt-legacy --json exit %d, stderr %q", code, stderr)
	}
	var res struct {
		Adopted []struct{ Name, Root string }            `json:"adopted"`
		Skipped []struct{ Root, Reason, Message string } `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("adopt-legacy --json output %q: %v", stdout, err)
	}
	if len(res.Adopted) != 1 || res.Adopted[0].Name != "old" || len(res.Skipped) != 1 || res.Skipped[0].Reason != "adopt.skip.unresolved_project" || res.Skipped[0].Message == "" {
		t.Fatalf("adopt-legacy --json = %+v", res)
	}

	stdout, _, code = run(fx.RT, "", "adopt-legacy")
	if code != 0 || !strings.Contains(stdout, "stray") || strings.Contains(stdout, "adopted legacy") {
		t.Fatalf("second adopt-legacy = %q (code %d), want only the still-skipped workspace", stdout, code)
	}
}
