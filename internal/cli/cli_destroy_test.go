// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/termprompt"
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
)

// TestCLI_Destroy_PromptsAndRespectsForce covers tasks.md 4b.11: without
// --force, destroy drives a real TerminalPrompter over scripted stdin
// before tearing down; with --force, no prompt is shown at all.
func TestCLI_Destroy_PromptsAndRespectsForce(t *testing.T) {
	t.Run("declining the prompt leaves the workspace untouched", func(t *testing.T) {
		fx := newFixture(t)
		wsRoot := domain.Path("/fixture/workspaces/ws1")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{Name: "ws1", Root: wsRoot}})
		var promptOut bytes.Buffer
		fx.RT.Prompter = app.PrompterDeps{Prompter: termprompt.New(strings.NewReader("n\n"), &promptOut)}

		_, _, code := run(fx.RT, "", "destroy", "ws1")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (declining is not an error)", code)
		}
		if !strings.Contains(promptOut.String(), "destroy") || !strings.Contains(promptOut.String(), "ws1") ||
			strings.Contains(promptOut.String(), "%") {
			t.Fatalf("prompt output = %q, want the confirmation naming ws1 with no format verbs", promptOut.String())
		}
		if _, err := fx.Store.LoadManifest(context.Background(), wsRoot); err != nil {
			t.Fatalf("manifest should still exist after declining: %v", err)
		}
	})

	t.Run("no input to answer the prompt asks for --force", func(t *testing.T) {
		fx := newFixture(t)
		wsRoot := domain.Path("/fixture/workspaces/ws3")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{Name: "ws3", Root: wsRoot}})
		fx.RT.Prompter = app.PrompterDeps{Prompter: termprompt.New(strings.NewReader(""), &discard{})}

		_, stderr, code := run(fx.RT, "", "destroy", "ws3")
		if code == 0 || strings.Contains(stderr, "unexpected error") || !strings.Contains(stderr, "--force") {
			t.Fatalf("exit %d, stderr %q: want a failure that points to --force", code, stderr)
		}
		if _, err := fx.Store.LoadManifest(context.Background(), wsRoot); err != nil {
			t.Fatalf("manifest should still exist: %v", err)
		}
	})

	t.Run("accepting the prompt tears down the workspace", func(t *testing.T) {
		fx := newFixture(t)
		wsRoot := domain.Path("/fixture/workspaces/ws2")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{Name: "ws2", Root: wsRoot}})
		fx.RT.Prompter = app.PrompterDeps{Prompter: termprompt.New(strings.NewReader("y\n"), &discard{})}

		_, stderr, code := run(fx.RT, "", "destroy", "ws2")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		if _, err := fx.Store.LoadManifest(context.Background(), wsRoot); err == nil {
			t.Fatal("manifest should be gone after accepting destroy")
		}
	})

	t.Run("--force skips the prompt entirely", func(t *testing.T) {
		fx := newFixture(t)
		wsRoot := domain.Path("/fixture/workspaces/ws3")
		fx.Store.PutManifest(wsRoot, domain.Manifest{Workspace: domain.Workspace{Name: "ws3", Root: wsRoot}})
		// No Prompter wired at all: if destroy tried to prompt, this would
		// panic on a nil Prompter, proving --force truly never asks.
		_, stderr, code := run(fx.RT, "", "destroy", "ws3", "--force")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		if _, err := fx.Store.LoadManifest(context.Background(), wsRoot); err == nil {
			t.Fatal("manifest should be gone after --force destroy")
		}
	})
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
