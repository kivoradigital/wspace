// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package packaging holds Go tests that assert on the checked-in native
// packaging files (packaging/windows/wspace.iss and friends) structurally,
// without invoking a Windows-only or Linux-only toolchain. The packaging
// files themselves are the source of truth; these tests only read them.
//
// This package matches no rule in internal/archtest's switch: it is a
// small, leaf, stdlib-only package imported by nothing else in the module.
package packaging
