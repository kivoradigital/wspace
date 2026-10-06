// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestAppBundleOf(t *testing.T) {
	tests := []struct {
		exe        string
		wantBundle string
		wantOK     bool
	}{
		{"/Applications/wspace.app/Contents/Helpers/wspace", "/Applications/wspace.app", true},
		{"/Users/u/Applications/Wspace Dev.app/Contents/Helpers/wspace", "/Users/u/Applications/Wspace Dev.app", true},
		{"/Applications/wspace.app/Contents/MacOS/wspace", "", false},
		{"/usr/local/bin/wspace", "", false},
		{"/home/u/.local/bin/wspace", "", false},
		{"/Contents/Helpers/wspace", "", false},
		{"C:/Program Files/wspace/wspace.exe", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.exe, func(t *testing.T) {
			got, ok := domain.AppBundleOf(tt.exe)
			if got != tt.wantBundle || ok != tt.wantOK {
				t.Fatalf("AppBundleOf(%q) = %q, %v; want %q, %v", tt.exe, got, ok, tt.wantBundle, tt.wantOK)
			}
		})
	}
}

func TestBundleSkillDir(t *testing.T) {
	got := domain.BundleSkillDir("/Applications/wspace.app", domain.SkillName)
	want := "/Applications/wspace.app/Contents/Resources/skills/wspace-workspaces"
	if got != want {
		t.Fatalf("BundleSkillDir = %q, want %q", got, want)
	}
}

func TestBundleSkillNames_PrefersTheReleaseName(t *testing.T) {
	names := domain.BundleSkillNames()
	if len(names) != 2 || names[0] != domain.SkillName || names[1] != domain.DevSkillName {
		t.Fatalf("BundleSkillNames = %v", names)
	}
}

func TestMCPServerNameFor(t *testing.T) {
	if got := domain.MCPServerNameFor(domain.SkillName); got != "wspace" {
		t.Fatalf("release server name = %q", got)
	}
	if got := domain.MCPServerNameFor(domain.DevSkillName); got != "wspace-dev" {
		t.Fatalf("dev server name = %q", got)
	}
}

func TestClassifySkillSourceLocation(t *testing.T) {
	tests := []struct {
		path string
		want domain.SourceLocation
	}{
		{"/Applications/wspace.app", domain.LocationStable},
		{"/Users/u/Applications/wspace.app", domain.LocationStable},
		{"/Volumes/wspace 1.0/wspace.app", domain.LocationDiskImage},
		{"/private/var/folders/xy/abc/T/AppTranslocation/1234-ABCD/d/wspace.app", domain.LocationTranslocated},
		{"/home/u/.local/share/wspace/skills/wspace-workspaces", domain.LocationStable},
	}
	for _, tt := range tests {
		if got := domain.ClassifySkillSourceLocation(tt.path); got != tt.want {
			t.Errorf("ClassifySkillSourceLocation(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// IsManagedSkillTarget decides whether an existing link was placed by
// wspace (so install may refresh it and uninstall may remove it) rather
// than by the user: a skill directory inside any wspace app bundle, or the
// extraction directory of this machine's data dir.
func TestIsManagedSkillTarget(t *testing.T) {
	const dataDir = "/home/u/.local/share/wspace/skills"
	tests := []struct {
		target string
		want   bool
	}{
		{"/Applications/wspace.app/Contents/Resources/skills/wspace-workspaces", true},
		{"/Users/u/Downloads/old/wspace.app/Contents/Resources/skills/wspace-workspaces/", true},
		{"/home/u/.local/share/wspace/skills/wspace-workspaces", true},
		{"/home/u/.local/share/wspace/skills/wspace-workspaces/", true},
		{"/other/share/wspace/skills/wspace-workspaces", false},
		{"/Users/u/dotfiles/skills/wspace-workspaces", false},
		{"/Applications/wspace.app/Contents/Resources/skills/other", false},
		{"/Users/u/src/ws/apps/wspace/skills/wspace-workspaces", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := domain.IsManagedSkillTarget(tt.target, domain.SkillName, dataDir); got != tt.want {
			t.Errorf("IsManagedSkillTarget(%q) = %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestSkillDataDir(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name string
		goos string
		home string
		env  map[string]string
		want string
	}{
		{"xdg", "linux", "/home/u", map[string]string{"XDG_DATA_HOME": "/data"}, "/data/wspace/skills"},
		{"linux default", "linux", "/home/u", nil, "/home/u/.local/share/wspace/skills"},
		{"darwin default", "darwin", "/Users/u", nil, "/Users/u/.local/share/wspace/skills"},
		{"windows", "windows", "C:/Users/u", map[string]string{"LOCALAPPDATA": `C:\Users\u\AppData\Local`}, "C:/Users/u/AppData/Local/wspace/skills"},
		{"windows fallback", "windows", "C:/Users/u", nil, "C:/Users/u/AppData/Local/wspace/skills"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.SkillDataDir(tt.goos, tt.home, env(tt.env)); got != tt.want {
				t.Fatalf("SkillDataDir = %q, want %q", got, tt.want)
			}
		})
	}
}
