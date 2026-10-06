// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"path"
	"strings"
)

// Agent skill names. The release app and the CLI install SkillName; the
// development app ("Wspace Dev") ships DevSkillName so its links never
// collide with the installed app's. LegacySkillName is the skill of the
// legacy bash `ws` tool, reported and optionally disabled, never deleted.
const (
	SkillName       = "wspace-workspaces"
	DevSkillName    = "wspace-dev-workspaces"
	LegacySkillName = "ws-workspaces"
)

// bundleHelperSuffix is where a desktop app bundle embeds the engine.
const bundleHelperSuffix = "/Contents/Helpers/"

// AppBundleOf returns the ".app" directory holding exe when exe is the
// engine embedded at "<X>.app/Contents/Helpers/<binary>" (exe is an
// already symlink-resolved, slash-separated absolute path).
func AppBundleOf(exe string) (string, bool) {
	i := strings.LastIndex(exe, bundleHelperSuffix)
	if i <= 0 {
		return "", false
	}
	bundle, rest := exe[:i], exe[i+len(bundleHelperSuffix):]
	if rest == "" || strings.Contains(rest, "/") || !strings.HasSuffix(bundle, ".app") {
		return "", false
	}
	return bundle, true
}

// BundleSkillDir is where an app bundle carries the skill named name.
func BundleSkillDir(bundle, name string) string {
	return bundle + "/Contents/Resources/skills/" + name
}

// BundleSkillNames lists the skill names an app bundle may carry, in the
// order they are looked for: the release name first.
func BundleSkillNames() []string { return []string{SkillName, DevSkillName} }

// MCPServerNameFor derives the MCP server name an agent registers for the
// skill named skillName: "wspace" for the release skill, "wspace-dev" for
// the development one.
func MCPServerNameFor(skillName string) string {
	return strings.TrimSuffix(skillName, "-workspaces")
}

// SourceLocation classifies where a skill source lives.
type SourceLocation string

const (
	// LocationStable is a permanent location an agent link may target.
	LocationStable SourceLocation = "stable"
	// LocationDiskImage is inside a mounted volume (a DMG opened from
	// Finder): the link would break once the image is ejected.
	LocationDiskImage SourceLocation = "disk_image"
	// LocationTranslocated is a randomized App Translocation path macOS
	// uses for a quarantined app run in place: it changes on every launch.
	LocationTranslocated SourceLocation = "translocated"
)

// ClassifySkillSourceLocation reports whether p (slash-separated,
// absolute) is a stable place to link an agent skill to.
func ClassifySkillSourceLocation(p string) SourceLocation {
	switch {
	case strings.Contains(p, "/AppTranslocation/"):
		return LocationTranslocated
	case strings.HasPrefix(p, "/Volumes/"):
		return LocationDiskImage
	default:
		return LocationStable
	}
}

// IsManagedSkillTarget reports whether an existing skill link pointing at
// target was placed by wspace: it targets the skill named name inside any
// wspace app bundle, or the extraction directory dataDir (SkillDataDir)
// of this machine. Anything else (a dotfiles checkout, a source
// repository) belongs to the user.
func IsManagedSkillTarget(target, name, dataDir string) bool {
	t := strings.TrimRight(strings.ReplaceAll(target, `\`, "/"), "/")
	if t == "" {
		return false
	}
	t = path.Clean(t)
	if strings.HasSuffix(t, ".app/Contents/Resources/skills/"+name) {
		return true
	}
	return dataDir != "" && t == path.Clean(dataDir)+"/"+name
}

// SkillDataDir is where a CLI that is not inside an app bundle extracts
// its embedded skill: "$XDG_DATA_HOME/wspace/skills" (default
// "~/.local/share/wspace/skills") on Unix, including macOS, and
// "%LOCALAPPDATA%\wspace\skills" on Windows. Paths are slash-separated.
func SkillDataDir(goos, home string, getenv func(string) string) string {
	if goos == "windows" {
		base := strings.ReplaceAll(getenv("LOCALAPPDATA"), `\`, "/")
		if base == "" {
			base = home + "/AppData/Local"
		}
		return strings.TrimRight(base, "/") + "/wspace/skills"
	}
	base := getenv("XDG_DATA_HOME")
	if base == "" {
		base = home + "/.local/share"
	}
	return strings.TrimRight(base, "/") + "/wspace/skills"
}
