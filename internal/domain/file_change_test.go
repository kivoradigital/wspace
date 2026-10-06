// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestDescribeChange(t *testing.T) {
	tests := []struct {
		in   domain.PorcelainEntry
		want domain.FileChange
	}{
		{domain.PorcelainEntry{X: 'M', Y: ' ', RelPath: "a"}, domain.FileChange{Path: "a", Status: domain.FileModified, Staged: true}},
		{domain.PorcelainEntry{X: ' ', Y: 'M', RelPath: "a b"}, domain.FileChange{Path: "a b", Status: domain.FileModified, Unstaged: true}},
		{domain.PorcelainEntry{X: 'A', Y: 'M', RelPath: "n"}, domain.FileChange{Path: "n", Status: domain.FileAdded, Staged: true, Unstaged: true}},
		{domain.PorcelainEntry{X: ' ', Y: 'D', RelPath: "d"}, domain.FileChange{Path: "d", Status: domain.FileDeleted, Unstaged: true}},
		{domain.PorcelainEntry{X: 'R', Y: ' ', RelPath: "new", OrigPath: "old"}, domain.FileChange{Path: "new", OrigPath: "old", Status: domain.FileRenamed, Staged: true}},
		{domain.PorcelainEntry{X: ' ', Y: 'T', RelPath: "l"}, domain.FileChange{Path: "l", Status: domain.FileTypeChange, Unstaged: true}},
		{domain.PorcelainEntry{X: '?', Y: '?', RelPath: "u"}, domain.FileChange{Path: "u", Status: domain.FileUntracked}},
		{domain.PorcelainEntry{X: 'U', Y: 'U', RelPath: "c"}, domain.FileChange{Path: "c", Status: domain.FileConflicted, Staged: true, Unstaged: true}},
		{domain.PorcelainEntry{X: 'A', Y: 'A', RelPath: "c2"}, domain.FileChange{Path: "c2", Status: domain.FileConflicted, Staged: true, Unstaged: true}},
	}
	for _, tt := range tests {
		if got := domain.DescribeChange(tt.in); got != tt.want {
			t.Errorf("DescribeChange(%+v) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}
