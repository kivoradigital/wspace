// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build windows

package presencefs

import "os"

// ProcessAlive is best effort on Windows: os.FindProcess opens a handle to
// the process and fails when no process has that pid. An exited process
// whose handle is still held elsewhere may be reported alive.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
