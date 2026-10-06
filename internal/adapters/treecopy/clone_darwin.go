// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build darwin

package treecopy

import (
	"os"

	"golang.org/x/sys/unix"
)

// cloneTree clones the whole directory with one clonefile(2) call (APFS
// clones a directory hierarchy entry by entry, links as links with
// CLONE_NOFOLLOW). Where that is unsupported (another filesystem, another
// volume), a partial result is removed and the tree is copied.
func cloneTree(src, dst string) (string, error) {
	if err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW); err == nil {
		return MethodClone, nil
	}
	_ = os.RemoveAll(dst)
	return MethodCopy, copyTree(src, dst, nil)
}
