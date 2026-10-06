// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package winenv implements ports.UserPathStore: the current user's
// persistent PATH, HKCU\Environment\Path, on Windows. Other systems get a
// store whose every call fails with ErrUnsupported (Install never calls it
// there).
package winenv

import "errors"

// ErrUnsupported is returned on systems without a Windows registry.
var ErrUnsupported = errors.New("the Windows user PATH is not available on this system")
