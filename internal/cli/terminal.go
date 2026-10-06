// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import "os"

// isTerminal reports whether f is attached to an interactive terminal,
// using only the standard library (design §14's dependency budget
// authorizes only a small, fixed set of third-party modules).
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
