// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

import "github.com/kivoradigital/wspace/internal/domain"

// Tree copy methods TreeCloner reports.
const (
	// CloneMethodClone: APFS copy-on-write (clonefile(2)), no extra space.
	CloneMethodClone = "clonefile"
	// CloneMethodReflink: Linux copy-on-write (FICLONE), where the
	// filesystem supports it (Btrfs, XFS); other files were copied.
	CloneMethodReflink = "reflink"
	// CloneMethodCopy: a regular byte-for-byte copy.
	CloneMethodCopy = "copy"
)

// TreeCloner copies a directory tree (a node_modules directory) as fast as
// the filesystem allows. Symbolic links are recreated, never followed;
// permission bits (the executable bit) are kept. dst must not exist; on
// failure, whatever was created at dst is removed.
type TreeCloner interface {
	CloneTree(src, dst domain.Path) (method string, err error)
}
