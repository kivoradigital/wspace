// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package ports declares the driven-side interfaces internal/app is written
// against: GitPort, FileSystemPort, ConfigStore, ReleaseChecker, Prompter and
// Reporter. It contains interfaces and their parameter/result structs only —
// zero implementations (design.md §1, R3).
package ports
