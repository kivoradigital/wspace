// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"strings"
	"unicode"
)

// NormalizeBranchPrefix turns a configured branch_prefix into the exact
// text placed before a workspace name: "" when unset or blank, otherwise
// the prefix with surrounding spaces and leading slashes removed. A prefix
// ending in a letter or digit is a branch namespace and gets exactly one
// trailing "/" ("feature", "feature/" and "feature//" all mean
// "feature/"), so a workspace named "test3" never becomes "featuretest3"
// nor "feature//test3". A prefix ending in any other character ("alice-",
// "team_") is a literal and is kept as written, so it yields "alice-test3".
func NormalizeBranchPrefix(prefix string) string {
	p := strings.TrimLeft(strings.TrimSpace(prefix), "/")
	if strings.HasSuffix(p, "/") {
		p = strings.TrimRight(p, "/")
		if p == "" {
			return ""
		}
		return p + "/"
	}
	if p == "" {
		return ""
	}
	last := []rune(p)[len([]rune(p))-1]
	if unicode.IsLetter(last) || unicode.IsDigit(last) {
		return p + "/"
	}
	return p
}

// JoinBranchPrefix composes the default branch for a workspace: the
// normalized prefix followed by name (leading slashes of name dropped).
// An unset prefix yields name unchanged.
func JoinBranchPrefix(prefix, name string) string {
	return NormalizeBranchPrefix(prefix) + strings.TrimLeft(name, "/")
}
