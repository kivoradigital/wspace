// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package adapters is the parent directory for every driven-adapter
// implementation of the internal/ports interfaces (git, fsstore,
// configstore, githubrelease, presencefs). Adapters never import each other, app, cli or
// gui (design.md §1, R5).
package adapters
