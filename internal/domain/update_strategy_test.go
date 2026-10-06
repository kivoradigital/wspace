// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"errors"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestParseUpdateStrategy(t *testing.T) {
	tests := []struct {
		in   string
		want domain.UpdateStrategy
	}{
		{"", domain.UpdateMerge},
		{"merge", domain.UpdateMerge},
		{"rebase", domain.UpdateRebase},
	}
	for _, tt := range tests {
		got, err := domain.ParseUpdateStrategy(tt.in)
		if err != nil || got != tt.want {
			t.Fatalf("ParseUpdateStrategy(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, bad := range []string{"Merge", "squash", " rebase"} {
		if _, err := domain.ParseUpdateStrategy(bad); !errors.Is(err, domain.ErrInvalidUpdateStrategy) {
			t.Fatalf("ParseUpdateStrategy(%q) error = %v, want ErrInvalidUpdateStrategy", bad, err)
		}
	}
}

func TestDirtyForUpdate_IgnoresUntrackedFiles(t *testing.T) {
	entries := []domain.PorcelainEntry{
		{X: ' ', Y: 'M', RelPath: "a.go"},
		{X: '?', Y: '?', RelPath: "notes.txt"},
		{X: 'A', Y: ' ', RelPath: "b.go"},
		{X: 'R', Y: ' ', RelPath: "c.go", OrigPath: "old.go"},
		{X: 'U', Y: 'U', RelPath: "d.go"},
	}
	got := domain.DirtyForUpdate(entries)
	want := []string{"a.go", "b.go", "c.go", "d.go"}
	if len(got) != len(want) {
		t.Fatalf("DirtyForUpdate = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DirtyForUpdate = %v, want %v", got, want)
		}
	}
	if got := domain.DirtyForUpdate([]domain.PorcelainEntry{{X: '?', Y: '?', RelPath: "x"}}); len(got) != 0 {
		t.Fatalf("untracked-only = %v, want none", got)
	}
}
