// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build linux

package treecopy

import (
	"os"

	"golang.org/x/sys/unix"
)

// cloneTree copies the tree, cloning each regular file with the FICLONE
// ioctl (a reflink on Btrfs or XFS); a filesystem without it (ext4 answers
// EOPNOTSUPP, another mount EXDEV) gets a regular copy instead. After the
// first refusal no further clone is attempted.
func cloneTree(src, dst string) (string, error) {
	supported := true
	clone := func(in, out *os.File) bool {
		if !supported {
			return false
		}
		if err := unix.IoctlFileClone(int(out.Fd()), int(in.Fd())); err != nil {
			supported = false
			return false
		}
		return true
	}
	n, err := copyTreeCounting(src, dst, clone)
	if err != nil {
		return "", err
	}
	if n > 0 {
		return MethodReflink, nil
	}
	return MethodCopy, nil
}
