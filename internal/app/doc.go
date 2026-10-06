// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package app holds the use cases (CreateWorkspace, DestroyWorkspace,
// SwitchContext, and the rest) that internal/cli and internal/gui drive. It
// imports internal/domain, internal/ports and internal/messages only, and
// never an adapter, cli or gui package (design.md §1, R4).
package app
