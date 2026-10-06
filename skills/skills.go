// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package skills embeds the agent skill shipped with wspace. The files
// under skills/wspace-workspaces are the single source: the Go binary
// embeds them here, and a desktop app that bundles the wspace binary may
// copy the same directory into its bundle's Contents/Resources/skills.
package skills

import (
	"embed"
	"io/fs"
)

// Name is the skill's directory and frontmatter name.
const Name = "wspace-workspaces"

//go:embed all:wspace-workspaces
var files embed.FS

// FS returns the embedded files, rooted so that Name is its only top-level
// directory.
func FS() fs.FS { return files }
