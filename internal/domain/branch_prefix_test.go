// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestNormalizeBranchPrefix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"/", ""},
		{"feature", "feature/"},
		{"feature/", "feature/"},
		{"feature//", "feature/"},
		{"/feature", "feature/"},
		{" feature ", "feature/"},
		{"team/feature", "team/feature/"},
		{"v2", "v2/"},
		{"alice-", "alice-"},
		{"team_", "team_"},
		{"rel.", "rel."},
		{" alice- ", "alice-"},
		{"/team-", "team-"},
	}
	for _, tt := range tests {
		if got := domain.NormalizeBranchPrefix(tt.in); got != tt.want {
			t.Errorf("NormalizeBranchPrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestJoinBranchPrefix(t *testing.T) {
	tests := []struct{ prefix, name, want string }{
		{"", "test3", "test3"},
		{"feature", "test3", "feature/test3"},
		{"feature/", "test3", "feature/test3"},
		{"feature//", "test3", "feature/test3"},
		{"feature", "/test3", "feature/test3"},
		{"hotfix", "a/b", "hotfix/a/b"},
		{"alice-", "test3", "alice-test3"},
		{"team_", "test3", "team_test3"},
	}
	for _, tt := range tests {
		if got := domain.JoinBranchPrefix(tt.prefix, tt.name); got != tt.want {
			t.Errorf("JoinBranchPrefix(%q, %q) = %q, want %q", tt.prefix, tt.name, got, tt.want)
		}
	}
}
