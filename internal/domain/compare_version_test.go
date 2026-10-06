// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// TestCompareVersion covers tasks.md 5.1 (design.md ADR D11: a hand-written
// "vMAJOR.MINOR.PATCH[-pre]" comparator, no golang.org/x/mod dependency for
// one function).
func TestCompareVersion(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want int
	}{
		{"equal with v prefix", "v1.2.3", "v1.2.3", 0},
		{"equal without v prefix", "1.2.3", "1.2.3", 0},
		{"v prefix does not affect ordering", "1.2.3", "v1.2.3", 0},
		{"patch older", "v1.2.3", "v1.2.4", -1},
		{"patch newer", "v1.2.4", "v1.2.3", 1},
		{"minor beats patch", "v1.9.9", "v1.10.0", -1},
		{"major beats minor and patch", "v1.9.9", "v2.0.0", -1},
		{"major newer", "v2.0.0", "v1.9.9", 1},
		{"pre-release orders before release", "v1.0.0-alpha", "v1.0.0", -1},
		{"release orders after pre-release", "v1.0.0", "v1.0.0-alpha", 1},
		{"pre-release ordering is lexical", "v1.0.0-alpha", "v1.0.0-beta", -1},
		{"pre-release ordering is lexical (reverse)", "v1.0.0-beta", "v1.0.0-alpha", 1},
		{"equal pre-releases", "v1.0.0-rc.1", "v1.0.0-rc.1", 0},
		{"unparseable dev build orders before any real release", "dev", "v1.0.0", -1},
		{"real release orders after an unparseable dev build", "v1.0.0", "dev", 1},
		{"two unparseable versions are equal", "dev", "dev", 0},
		{"missing patch segment is unparseable, orders before a valid version", "v1.2", "v1.2.0", -1},
		{"empty string is unparseable", "", "v0.0.1", -1},
		{"uppercase V prefix is accepted", "V1.2.3", "v1.2.3", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.CompareVersion(tc.a, tc.b); got != tc.want {
				t.Fatalf("CompareVersion(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
