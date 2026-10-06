// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build !windows

package presencefs

import (
	"errors"
	"syscall"
)

// ProcessAlive reports whether pid names a running process: kill(pid, 0)
// succeeds, or fails only with EPERM (the process exists but belongs to
// another user).
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
