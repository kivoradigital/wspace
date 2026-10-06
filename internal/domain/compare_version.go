// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"strconv"
	"strings"
)

// CompareVersion compares two "vMAJOR.MINOR.PATCH[-pre]" version strings (an
// optional leading "v"/"V" is stripped before parsing) and returns -1, 0 or
// 1 the way strings.Compare does: negative when a orders before b, zero when
// equal, positive when a orders after b (design.md §11, ADR D11: hand-
// written, no golang.org/x/mod dependency for one function).
//
// A string that does not parse as exactly three dot-separated non-negative
// integers (optionally followed by "-<pre-release>") is treated as
// unparseable and orders before every parseable version — this is exactly
// buildinfo's own "dev" placeholder's intended ordering: an unreleased
// local build is always older than any real tagged release. Two
// unparseable strings compare equal to each other.
func CompareVersion(a, b string) int {
	va, oka := parseVersion(a)
	vb, okb := parseVersion(b)

	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}

	if c := compareInt(va.major, vb.major); c != 0 {
		return c
	}
	if c := compareInt(va.minor, vb.minor); c != 0 {
		return c
	}
	if c := compareInt(va.patch, vb.patch); c != 0 {
		return c
	}

	// Equal MAJOR.MINOR.PATCH: per semver, a release with no pre-release
	// outranks any pre-release of the same core version (1.0.0 > 1.0.0-alpha).
	switch {
	case va.pre == "" && vb.pre == "":
		return 0
	case va.pre == "":
		return 1
	case vb.pre == "":
		return -1
	default:
		return strings.Compare(va.pre, vb.pre)
	}
}

type parsedVersion struct {
	major, minor, patch int
	pre                 string
}

// parseVersion parses s as "[vV]MAJOR.MINOR.PATCH[-pre]". ok is false when s
// does not have exactly three non-negative integer components.
func parseVersion(s string) (parsedVersion, bool) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	core, pre, _ := strings.Cut(s, "-")

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return parsedVersion{}, false
	}

	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return parsedVersion{}, false
		}
		nums[i] = n
	}
	return parsedVersion{major: nums[0], minor: nums[1], patch: nums[2], pre: pre}, true
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
