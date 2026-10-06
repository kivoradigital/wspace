// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// CheckForUpdateDeps bundles the one port CheckForUpdate needs.
// ports.ReleaseChecker appears nowhere else in internal/app: Deps (used by
// CreateWorkspace, DestroyWorkspace, Status, List, Doctor and the rest of
// phase 4a) has no Checker field, and neither does ExecDeps or any wizard's
// deps bundle. That is what makes "only --check/tray triggers the network
// call" (design.md §11) true by construction, not by convention: nothing
// outside a direct call to CheckForUpdate can ever reach a ReleaseChecker.
type CheckForUpdateDeps struct {
	Checker ports.ReleaseChecker
}

// CheckForUpdateInput carries the two pieces of data CheckForUpdate needs
// that internal/app cannot obtain itself: Coordinates (from
// buildinfo.Coordinates(), which internal/app must not import — R4) and
// CurrentVersion (buildinfo.Version, the same plain string internal/cli's
// Runtime.Version already threads through). Coordinates being the zero
// value is exactly buildinfo.Coordinates()'s ok=false case: an unbranded
// build.
//
// BundledBy (buildinfo.BundledBy) names the desktop app that embeds this
// CLI; when it is non-empty the CLI is updated only with that app.
type CheckForUpdateInput struct {
	Coordinates    domain.RepoCoordinates
	CurrentVersion string
	BundledBy      string
}

// CheckForUpdateResult is the one shape both the CLI's `version --check`
// and (phase 6) the tray's About item render from — design.md §11: "One
// ReleaseChecker, two surfaces". Unavailable, Available and LatestTag are
// mutually exclusive-by-convention (Unavailable=true means the other two
// fields are meaningless), matching domain.ReleaseInfo's own contract one
// layer down. BundledBy non-empty means the CLI is bundled with that app:
// Available and Unavailable are then both false, because there is nothing
// to check — the app owns updates.
type CheckForUpdateResult struct {
	CurrentVersion string
	LatestTag      string
	Available      bool
	Unavailable    bool
	BundledBy      string
}

// CheckForUpdate implements design.md §11's "One ReleaseChecker, two
// surfaces" use case. A bundled build (BundledBy non-empty) never calls
// deps.Checker: the bundling app updates it, so the CLI must not suggest
// an update of its own. An unbranded build (Coordinates is the zero value)
// never calls deps.Checker at all — querying a repository nobody chose
// would be worse than reporting nothing. Otherwise it calls Latest exactly
// once and computes Available itself via domain.CompareVersion, since
// ports.ReleaseChecker's own signature (design.md §4) takes only
// RepoCoordinates and has no notion of "the caller's own running version".
//
// This never returns a non-nil error for a degraded network condition:
// ports.ReleaseChecker.Latest already never does (design.md §11: "Never an
// error to the user"), and CheckForUpdate forwards that contract rather
// than inventing a new failure mode on top of it.
func CheckForUpdate(ctx context.Context, deps CheckForUpdateDeps, in CheckForUpdateInput) (CheckForUpdateResult, error) {
	result := CheckForUpdateResult{CurrentVersion: in.CurrentVersion}

	if in.BundledBy != "" {
		result.BundledBy = in.BundledBy
		return result, nil
	}

	if in.Coordinates == (domain.RepoCoordinates{}) {
		result.Unavailable = true
		return result, nil
	}

	info, err := deps.Checker.Latest(ctx, in.Coordinates)
	if err != nil || info.Unavailable {
		result.Unavailable = true
		return result, nil
	}

	result.LatestTag = info.Tag
	result.Available = domain.CompareVersion(in.CurrentVersion, info.Tag) < 0
	return result, nil
}
