// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kivoradigital/wspace/internal/domain"
	"gopkg.in/yaml.v3"
)

// overlayYAML mirrors "<any project dir>/.ws.yaml" (design.md §5): option
// values only, per ADR D7. Its keys are validated against
// domain.ValidateOverlayScope before this struct is ever decoded into.
type overlayYAML struct {
	SchemaVersion int                    `yaml:"schema_version"`
	Options       optionsYAML            `yaml:"options,omitempty"`
	Projects      map[string]optionsYAML `yaml:"projects,omitempty"`
}

func (dto overlayYAML) toDomain() domain.Overlay {
	projects := make(map[domain.ProjectKey]domain.Options, len(dto.Projects))
	for k, v := range dto.Projects {
		projects[domain.ProjectKey(k)] = v.toDomain()
	}
	return domain.Overlay{
		SchemaVersion: dto.SchemaVersion,
		Options:       dto.Options.toDomain(),
		Projects:      projects,
	}
}

// decodeOverlay validates ADR D7's scope rule against the document's raw
// top-level keys, then strictly decodes it into overlayYAML. Doing the
// scope check first gives CodeOverlayScope for a context/contexts key,
// distinct from a generic "unknown field" typo error a naive strict decode
// alone would produce for either case indiscriminately.
func decodeOverlay(data []byte) (domain.Overlay, error) {
	var raw map[string]yaml.Node
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return domain.Overlay{}, fmt.Errorf("config.load_overlay: %w", err)
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	if err := domain.ValidateOverlayScope(keys); err != nil {
		return domain.Overlay{}, err
	}

	var dto overlayYAML
	if err := strictDecode(data, &dto); err != nil {
		return domain.Overlay{}, fmt.Errorf("config.load_overlay: %w", err)
	}
	if err := checkSchemaVersion("config.load_overlay", &dto.SchemaVersion); err != nil {
		return domain.Overlay{}, err
	}
	return dto.toDomain(), nil
}

// LoadOverlay walks up from "from" looking for a ".ws.yaml" file in each
// ancestor directory (context-management spec: "the nearest local config
// file found by walking up from the current working directory"), and
// returns the first one found. It returns found=false, err=nil when none
// exists anywhere above "from" — an overlay is always optional.
func (a *Adapter) LoadOverlay(_ context.Context, from domain.Path) (domain.Overlay, domain.Path, bool, error) {
	cur := filepath.Clean(filepath.FromSlash(string(from)))
	for {
		candidate := filepath.Join(cur, ".ws.yaml")
		data, err := os.ReadFile(candidate)
		if err == nil {
			overlay, decErr := decodeOverlay(data)
			if decErr != nil {
				return domain.Overlay{}, "", false, decErr
			}
			return overlay, domain.Path(filepath.ToSlash(candidate)), true, nil
		}
		if !os.IsNotExist(err) {
			return domain.Overlay{}, "", false, fmt.Errorf("config.load_overlay: %w", err)
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return domain.Overlay{}, "", false, nil
		}
		cur = parent
	}
}
