// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package fsstore

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// Paths resolves the platform config/cache/home roots (design.md §5).
// WS_CONFIG_HOME, when set, overrides the config root wholesale (not just
// its parent) and is the hook every test uses instead of a real home
// directory. Cache and Home are always resolved normally, independent of
// that override.
func (a *Adapter) Paths() ports.Paths {
	home, _ := os.UserHomeDir()

	config := defaultConfigDir(home)
	if override := os.Getenv("WSPACE_CONFIG_HOME"); override != "" {
		config = override
	}

	return ports.Paths{
		Config: domain.Path(filepath.ToSlash(config)),
		Cache:  domain.Path(filepath.ToSlash(defaultCacheDir(home))),
		Home:   domain.Path(filepath.ToSlash(home)),
	}
}

// defaultConfigDir resolves <config>/ws. Windows uses os.UserConfigDir()
// (%APPDATA%). Every other OS, including macOS, uses XDG_CONFIG_HOME or
// ~/.config: this developer CLI's config is hand-edited and often
// dotfile-managed, and a single Unix code path removes an entire class of
// "the tray and the CLI disagree about where config lives" bug (design.md
// §5).
func defaultConfigDir(home string) string {
	if runtime.GOOS == "windows" {
		base, err := os.UserConfigDir()
		if err != nil {
			base = home
		}
		return filepath.Join(base, "wspace")
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "wspace")
}

// LegacyFlatConfigPath resolves a legacy flat workspace configuration
// tool's own global config file location (this change's import feature).
// It deliberately does not honor WSPACE_CONFIG_HOME — that override
// exists only to isolate *this* application's own config root in tests
// and must never redirect where an unrelated older tool's file is read
// from — and it mirrors defaultConfigDir's own platform split with one
// difference: the directory name is "ws", the legacy tool's own name, not
// "wspace".
func (a *Adapter) LegacyFlatConfigPath() domain.Path {
	home, _ := os.UserHomeDir()

	var dir string
	if runtime.GOOS == "windows" {
		base, err := os.UserConfigDir()
		if err != nil {
			base = home
		}
		dir = filepath.Join(base, "ws")
	} else {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		dir = filepath.Join(base, "ws")
	}

	return domain.Path(filepath.ToSlash(filepath.Join(dir, "config")))
}

// defaultCacheDir resolves <cache>/ws, mirroring defaultConfigDir's
// platform split (design.md §5).
func defaultCacheDir(home string) string {
	if runtime.GOOS == "windows" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = home
		}
		return filepath.Join(base, "wspace")
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "wspace")
}
