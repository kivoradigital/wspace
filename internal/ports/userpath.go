// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

// UserPathStore is the current user's persistent PATH on Windows: the
// "Path" value of HKCU\Environment. Values are raw, unexpanded strings
// (%VAR% references kept), and Expand reports whether the value is a
// REG_EXPAND_SZ, so a rewrite keeps the type it found. The system-wide
// Path (HKLM) is never read or written.
type UserPathStore interface {
	// UserPath returns the raw value; a missing value is "" with Expand
	// true (the type Windows itself creates).
	UserPath() (value string, expand bool, err error)
	// SetUserPath writes value with the given type.
	SetUserPath(value string, expand bool) error
	// BroadcastEnvironmentChange tells running programs (Explorer, and the
	// terminals it starts next) that the environment changed.
	BroadcastEnvironmentChange() error
}
