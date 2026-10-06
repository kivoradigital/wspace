// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"reflect"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestSplitChanges_SortsEntriesIntoStagedUnstagedUntrackedAndConflicted(t *testing.T) {
	entries := []domain.PorcelainEntry{
		{X: 'M', Y: 'M', RelPath: "both.go"},
		{X: 'A', Y: ' ', RelPath: "new.go"},
		{X: 'R', Y: ' ', RelPath: "renamed.go", OrigPath: "old.go"},
		{X: ' ', Y: 'D', RelPath: "gone.go"},
		{X: '?', Y: '?', RelPath: "scratch.txt"},
		{X: 'U', Y: 'U', RelPath: "clash.go"},
		{X: '!', Y: '!', RelPath: "ignored.log"},
	}

	got := domain.SplitChanges(entries)

	want := domain.ChangeSets{
		Staged: []domain.ChangeEntry{
			{Path: "both.go", Status: domain.FileModified},
			{Path: "new.go", Status: domain.FileAdded},
			{Path: "renamed.go", OrigPath: "old.go", Status: domain.FileRenamed},
		},
		Unstaged: []domain.ChangeEntry{
			{Path: "both.go", Status: domain.FileModified},
			{Path: "gone.go", Status: domain.FileDeleted},
		},
		Untracked:  []domain.ChangeEntry{{Path: "scratch.txt", Status: domain.FileUntracked}},
		Conflicted: []domain.ChangeEntry{{Path: "clash.go", Status: domain.FileConflicted}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitChanges =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSplitChanges_EmptyIsEmptyListsNotNil(t *testing.T) {
	got := domain.SplitChanges(nil)
	if got.Staged == nil || got.Unstaged == nil || got.Untracked == nil || got.Conflicted == nil {
		t.Fatalf("SplitChanges(nil) = %+v, want empty non-nil lists", got)
	}
}

func TestChangeSets_FindLocatesAPathInTheRequestedSide(t *testing.T) {
	cs := domain.SplitChanges([]domain.PorcelainEntry{
		{X: 'R', Y: 'M', RelPath: "b.go", OrigPath: "a.go"},
		{X: '?', Y: '?', RelPath: "u.txt"},
	})

	if e, ok := cs.Find("b.go", domain.SideStaged); !ok || e.OrigPath != "a.go" {
		t.Fatalf("staged b.go = %+v, %v", e, ok)
	}
	if _, ok := cs.Find("b.go", domain.SideUnstaged); !ok {
		t.Fatal("unstaged b.go not found")
	}
	if _, ok := cs.Find("u.txt", domain.SideUnstaged); !ok {
		t.Fatal("an untracked file is part of the unstaged side")
	}
	if _, ok := cs.Find("u.txt", domain.SideStaged); ok {
		t.Fatal("an untracked file is never staged")
	}
	if _, ok := cs.Find("../etc/passwd", domain.SideUnstaged); ok {
		t.Fatal("a path that is not a change must not be found")
	}
}

func TestParseStashSubject_SplitsBranchAndMessage(t *testing.T) {
	tests := []struct{ in, branch, msg string }{
		{"On main: wip msg", "main", "wip msg"},
		{"WIP on feat/x: 13fa57b fix: thing", "feat/x", "13fa57b fix: thing"},
		{"On (no branch): detached", "(no branch)", "detached"},
		{"custom subject", "", "custom subject"},
	}
	for _, tt := range tests {
		b, m := domain.ParseStashSubject(tt.in)
		if b != tt.branch || m != tt.msg {
			t.Errorf("ParseStashSubject(%q) = %q, %q; want %q, %q", tt.in, b, m, tt.branch, tt.msg)
		}
	}
}

func TestValidCommitHash(t *testing.T) {
	for _, ok := range []string{"abcd", "0123456789abcdef0123456789abcdef01234567", "ABCDEF12"} {
		if !domain.ValidCommitHash(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "abc", "--all", "HEAD", "abcd~1", "g123", "abcd..ef01"} {
		if domain.ValidCommitHash(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
