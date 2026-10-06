// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// TestMatchesIgnorePattern covers context-management spec's "Ignore
// pattern excludes a directory from discovery": ignore_patterns is a
// path.Match glob list applied to a bare directory name.
func TestMatchesIgnorePattern(t *testing.T) {
	tests := []struct {
		name     string
		patterns []domain.Glob
		dirName  string
		want     bool
	}{
		{name: "no patterns never matches", patterns: nil, dirName: "vendor", want: false},
		{name: "exact glob matches", patterns: []domain.Glob{"vendor"}, dirName: "vendor", want: true},
		{name: "wildcard glob matches", patterns: []domain.Glob{"archive-*"}, dirName: "archive-2026", want: true},
		{name: "non-matching pattern", patterns: []domain.Glob{"*.tmp"}, dirName: "api", want: false},
		{name: "dotfile glob matches", patterns: []domain.Glob{".*"}, dirName: ".git", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.MatchesIgnorePattern(tt.patterns, tt.dirName)
			if err != nil {
				t.Fatalf("MatchesIgnorePattern: %v", err)
			}
			if got != tt.want {
				t.Fatalf("MatchesIgnorePattern(%v, %q) = %v, want %v", tt.patterns, tt.dirName, got, tt.want)
			}
		})
	}
}
