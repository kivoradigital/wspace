// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package configstore implements ports.ConfigStore against real YAML files
// under a config root directory (design.md §5): root config, per-context
// config, directory-scoped overlays, and per-workspace manifests. It owns
// every on-disk representation decision — schema versioning, strict
// decoding, and atomic writes — so no other package hand-rolls YAML I/O.
package configstore

import (
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// currentSchemaVersion is the schema_version this build writes and the
// ceiling every Load* method enforces (design.md §5: "schema_version >
// current -> hard refusal").
const currentSchemaVersion = 1

// Adapter implements ports.ConfigStore rooted at a single config directory
// (typically ports.Paths.Config, i.e. "<config>/ws"). It performs its own
// file I/O rather than going through ports.FileSystemPort, because atomic
// writes (temp file + rename) are not expressible through that port.
type Adapter struct {
	root domain.Path
	now  func() time.Time // overridable in tests; defaults to time.Now
}

// New constructs an Adapter rooted at root (e.g. ports.Paths.Config).
func New(root domain.Path) *Adapter {
	return &Adapter{root: root, now: time.Now}
}

var _ ports.ConfigStore = (*Adapter)(nil)
