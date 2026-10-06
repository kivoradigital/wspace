// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
)

// ReleaseChecker never blocks a command; callers pass a short-deadline
// context (design.md §4, §11).
type ReleaseChecker interface {
	Latest(ctx context.Context, c domain.RepoCoordinates) (domain.ReleaseInfo, error)
}
