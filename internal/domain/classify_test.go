// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestClassify_StagedChangeBlocks(t *testing.T) {
	entries := []domain.PorcelainEntry{
		{X: 'M', Y: ' ', RelPath: "services/api.go"},
	}

	cs := domain.Classify("api", entries, nil)

	blocking := cs.Blocking()
	if len(blocking) != 1 || blocking[0].RelPath != "services/api.go" {
		t.Fatalf("Blocking() = %+v, want one entry for services/api.go", blocking)
	}
	if blocking[0].Class != domain.ChangeTracked {
		t.Fatalf("Class = %v, want ChangeTracked", blocking[0].Class)
	}
	if cs.SafeToRemove() {
		t.Fatalf("SafeToRemove() = true, want false for a staged change")
	}
}

func TestClassify_DeletedTrackedFileBlocks(t *testing.T) {
	entries := []domain.PorcelainEntry{
		{X: ' ', Y: 'D', RelPath: "README.md"},
	}

	cs := domain.Classify("api", entries, nil)

	blocking := cs.Blocking()
	if len(blocking) != 1 || blocking[0].RelPath != "README.md" {
		t.Fatalf("Blocking() = %+v, want one entry for README.md", blocking)
	}
	if blocking[0].Class != domain.ChangeTracked {
		t.Fatalf("Class = %v, want ChangeTracked", blocking[0].Class)
	}
	if cs.SafeToRemove() {
		t.Fatalf("SafeToRemove() = true, want false for a deleted tracked file")
	}
}

func TestClassify_ForeignUntrackedFileBlocks(t *testing.T) {
	entries := []domain.PorcelainEntry{
		{X: '?', Y: '?', RelPath: "scratch.txt"},
	}

	cs := domain.Classify("api", entries, nil)

	blocking := cs.Blocking()
	if len(blocking) != 1 || blocking[0].Class != domain.ChangeForeign {
		t.Fatalf("Blocking() = %+v, want one ChangeForeign entry", blocking)
	}
	if cs.SafeToRemove() {
		t.Fatalf("SafeToRemove() = true, want false for a foreign untracked file")
	}
}

func TestClassify_EnvCopyNeverBlocks(t *testing.T) {
	entries := []domain.PorcelainEntry{
		{X: '?', Y: '?', RelPath: ".env"},
	}

	cs := domain.Classify("api", entries, []string{"api/.env"})

	if len(cs.Blocking()) != 0 {
		t.Fatalf("Blocking() = %+v, want no blocking entries for an env-copy file", cs.Blocking())
	}
	if !cs.SafeToRemove() {
		t.Fatalf("SafeToRemove() = false, want true when only env-copy files are present")
	}
	if cs.Changes[0].Class != domain.ChangeEnvCopy {
		t.Fatalf("Class = %v, want ChangeEnvCopy", cs.Changes[0].Class)
	}
}

func TestChangeSet_SafeToRemove_FalseWhenUnpushed(t *testing.T) {
	cs := domain.ChangeSet{Alias: "api", Unpushed: 2}

	if cs.SafeToRemove() {
		t.Fatalf("SafeToRemove() = true, want false when Unpushed > 0")
	}
}

func TestChangeSet_Blocking_IsInPathOrder(t *testing.T) {
	entries := []domain.PorcelainEntry{
		{X: 'M', Y: ' ', RelPath: "z.txt"},
		{X: 'M', Y: ' ', RelPath: "a.txt"},
	}

	cs := domain.Classify("api", entries, nil)
	blocking := cs.Blocking()

	if len(blocking) != 2 || blocking[0].RelPath != "a.txt" || blocking[1].RelPath != "z.txt" {
		t.Fatalf("Blocking() = %+v, want [a.txt z.txt] in path order", blocking)
	}
}
