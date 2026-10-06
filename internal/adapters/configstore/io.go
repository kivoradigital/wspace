// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package configstore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kivoradigital/wspace/internal/domain"
	"gopkg.in/yaml.v3"
)

// strictDecode decodes data into out with KnownFields(true): an unknown
// field is a loud decode error, never a silently ignored key (design.md §5
// "Decision: YAML").
func strictDecode(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(out)
}

// checkSchemaVersion enforces the schema ceiling and performs the one
// in-memory upgrade step this build knows about (design.md §5): a version
// above currentSchemaVersion is a hard refusal; a version below it is
// upgraded in place with no data loss (there is only one schema version so
// far, so the "upgrade" is a version-number bump with no field changes).
func checkSchemaVersion(op string, v *int) error {
	if *v > currentSchemaVersion {
		return domain.NewOpError(op, domain.CodeSchemaUnsupported, fmt.Sprintf("schema_version=%d", *v), "", nil)
	}
	if *v < currentSchemaVersion {
		*v = currentSchemaVersion
	}
	return nil
}

// atomicWrite writes data to path by writing "<path>.tmp" in the same
// directory, fsyncing it, then renaming it over path (design.md §5: "a
// crashed destroy can never leave a half-written manifest"). Parent
// directories are created as needed.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("configstore: create %q: %w", dir, err)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("configstore: create %q: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("configstore: write %q: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("configstore: sync %q: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("configstore: close %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("configstore: rename %q to %q: %w", tmp, path, err)
	}
	return nil
}
