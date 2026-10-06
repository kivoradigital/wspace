// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/configstore"
	"github.com/kivoradigital/wspace/internal/domain"
)

// TestOverlay_RejectsContextScopedFields covers tasks.md 3.9: a .ws.yaml
// containing "context:" or "contexts:" is rejected at parse time with
// CodeOverlayScope (ADR D7), never silently accepted or treated as a
// context switch.
func TestOverlay_RejectsContextScopedFields(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{name: "bare context key", yaml: "schema_version: 1\ncontext: oss\n"},
		{name: "contexts key", yaml: "schema_version: 1\ncontexts:\n  oss: {}\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(projectDir, ".ws.yaml"), []byte(tt.yaml), 0o644); err != nil {
				t.Fatalf("write .ws.yaml: %v", err)
			}

			store := configstore.New(domain.Path(filepath.ToSlash(t.TempDir())))
			_, _, _, err := store.LoadOverlay(context.Background(), domain.Path(filepath.ToSlash(projectDir)))
			if domain.Code(err) != domain.CodeOverlayScope {
				t.Fatalf("LoadOverlay: Code(err) = %q, want %q (err=%v)", domain.Code(err), domain.CodeOverlayScope, err)
			}
		})
	}
}

// TestConfigStore_LoadOverlay_WalksUpAndOverridesOptionsOnly covers the
// positive path: a valid overlay is found by walking up from a nested
// directory and carries option values only (context-management spec:
// "Local config overrides one option").
func TestConfigStore_LoadOverlay_WalksUpAndOverridesOptionsOnly(t *testing.T) {
	projectDir := t.TempDir()
	nested := filepath.Join(projectDir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	overlayYAML := "schema_version: 1\noptions:\n  fetch_before_create: false\nprojects:\n  api:\n    base_branch: release/2026.09\n"
	if err := os.WriteFile(filepath.Join(projectDir, ".ws.yaml"), []byte(overlayYAML), 0o644); err != nil {
		t.Fatalf("write .ws.yaml: %v", err)
	}

	store := configstore.New(domain.Path(filepath.ToSlash(t.TempDir())))
	overlay, foundAt, found, err := store.LoadOverlay(context.Background(), domain.Path(filepath.ToSlash(nested)))
	if err != nil {
		t.Fatalf("LoadOverlay: %v", err)
	}
	if !found {
		t.Fatal("LoadOverlay: found = false, want true (walking up should find the overlay)")
	}
	wantPath := domain.Path(filepath.ToSlash(filepath.Join(projectDir, ".ws.yaml")))
	if foundAt != wantPath {
		t.Fatalf("LoadOverlay: foundAt = %q, want %q", foundAt, wantPath)
	}
	if overlay.Options.FetchBeforeCreate == nil || *overlay.Options.FetchBeforeCreate != false {
		t.Fatalf("LoadOverlay: Options.FetchBeforeCreate = %v, want pointer to false", overlay.Options.FetchBeforeCreate)
	}
	apiOpts, ok := overlay.Projects["api"]
	if !ok || apiOpts.BaseBranch == nil || *apiOpts.BaseBranch != domain.BranchName("release/2026.09") {
		t.Fatalf("LoadOverlay: Projects[api] = %+v, want BaseBranch=release/2026.09", apiOpts)
	}
}

// TestConfigStore_LoadOverlay_NoneFound covers the case where no .ws.yaml
// exists anywhere above the given directory: found is false, err is nil
// (an overlay is optional).
func TestConfigStore_LoadOverlay_NoneFound(t *testing.T) {
	dir := t.TempDir()
	store := configstore.New(domain.Path(filepath.ToSlash(t.TempDir())))
	_, _, found, err := store.LoadOverlay(context.Background(), domain.Path(filepath.ToSlash(dir)))
	if err != nil {
		t.Fatalf("LoadOverlay with no overlay present: %v", err)
	}
	if found {
		t.Fatal("LoadOverlay with no overlay present: found = true, want false")
	}
}
