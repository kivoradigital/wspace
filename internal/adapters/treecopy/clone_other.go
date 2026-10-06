// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build !darwin && !linux

package treecopy

// cloneTree copies the tree: Windows (ReFS block cloning aside) and other
// systems have no copy-on-write call wspace uses.
func cloneTree(src, dst string) (string, error) {
	return MethodCopy, copyTree(src, dst, nil)
}
