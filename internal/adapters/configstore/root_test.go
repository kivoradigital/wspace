// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/adapters/configstore"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TestConfigStore_RootConfig_RoundTrip covers tasks.md 3.1: SaveRoot then
// LoadRoot returns an equal domain.RootConfig, and a missing config.yaml
// reports the "not initialized" sentinel cleanly rather than an obscure
// os.PathError (design.md §5 "First run and migration").
func TestConfigStore_RootConfig_RoundTrip(t *testing.T) {
	root := domain.Path(filepath.ToSlash(t.TempDir()))
	store := configstore.New(root)
	ctx := context.Background()

	t.Run("no config.yaml reports not initialized", func(t *testing.T) {
		_, err := store.LoadRoot(ctx)
		if !errors.Is(err, ports.ErrConfigNotInitialized) {
			t.Fatalf("LoadRoot on empty store: err = %v, want ports.ErrConfigNotInitialized", err)
		}
	})

	want := domain.RootConfig{
		SchemaVersion: 1,
		ActiveContext: "work",
		Preferences: domain.Preferences{
			Locale:            "en",
			UpdateCheck:       true,
			UpdateCheckTTL:    24 * time.Hour,
			TrayRefreshPeriod: 60 * time.Second,
		},
	}

	if err := store.SaveRoot(ctx, want); err != nil {
		t.Fatalf("SaveRoot: %v", err)
	}

	got, err := store.LoadRoot(ctx)
	if err != nil {
		t.Fatalf("LoadRoot after SaveRoot: %v", err)
	}
	if got != want {
		t.Fatalf("LoadRoot = %+v, want %+v", got, want)
	}

	t.Run("write is atomic: no leftover .tmp file", func(t *testing.T) {
		if _, err := os.Stat(string(root) + "/config.yaml.tmp"); !os.IsNotExist(err) {
			t.Fatalf("expected no leftover .tmp file, stat err = %v", err)
		}
		if _, err := os.Stat(string(root) + "/config.yaml"); err != nil {
			t.Fatalf("expected config.yaml to exist: %v", err)
		}
	})
}

// TestConfigStore_KnownFieldsRejectsTypo covers tasks.md 3.3: a typo'd field
// name in any config file must fail loudly (strict yaml.v3 KnownFields),
// never be silently ignored.
func TestConfigStore_KnownFieldsRejectsTypo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(
		"schema_version: 1\nactive_context: work\nactiv_context_typo: oops\n",
	), 0o644); err != nil {
		t.Fatalf("write malformed config.yaml: %v", err)
	}

	store := configstore.New(domain.Path(filepath.ToSlash(root)))
	if _, err := store.LoadRoot(context.Background()); err == nil {
		t.Fatal("LoadRoot with an unknown field: err = nil, want a decode error")
	}
}

// TestConfigStore_SchemaVersionRefusal covers tasks.md 3.5: a newer
// schema_version than this build understands is a hard refusal, while an
// older one upgrades in memory (design.md §5).
func TestConfigStore_SchemaVersionRefusal(t *testing.T) {
	t.Run("newer schema hard-refuses", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(
			"schema_version: 99\nactive_context: work\n",
		), 0o644); err != nil {
			t.Fatalf("write config.yaml: %v", err)
		}
		store := configstore.New(domain.Path(filepath.ToSlash(root)))
		_, err := store.LoadRoot(context.Background())
		if domain.Code(err) != domain.CodeSchemaUnsupported {
			t.Fatalf("LoadRoot with schema_version=99: Code(err) = %q, want %q (err=%v)", domain.Code(err), domain.CodeSchemaUnsupported, err)
		}
	})

	t.Run("older schema upgrades in memory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(
			"schema_version: 0\nactive_context: work\n",
		), 0o644); err != nil {
			t.Fatalf("write config.yaml: %v", err)
		}
		store := configstore.New(domain.Path(filepath.ToSlash(root)))
		got, err := store.LoadRoot(context.Background())
		if err != nil {
			t.Fatalf("LoadRoot with schema_version=0: %v", err)
		}
		if got.SchemaVersion != 1 {
			t.Fatalf("LoadRoot with schema_version=0: SchemaVersion = %d, want 1 (in-memory upgrade)", got.SchemaVersion)
		}
	})
}
