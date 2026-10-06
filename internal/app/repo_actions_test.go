// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

const (
	stashHash0 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	stashHash1 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type stashFixture struct {
	*baseFixture
	applied []bool // restoreIndex of every StashApply call
	dropped []int
}

func newStashFixture(t *testing.T) *stashFixture {
	f := &stashFixture{baseFixture: newBaseFixture(t)}
	f.git.StashListFunc = func(domain.Path) ([]domain.StashEntry, error) {
		return []domain.StashEntry{
			{Index: 0, Ref: "stash@{0}", Hash: stashHash0, Message: "newest"},
			{Index: 1, Ref: "stash@{1}", Hash: stashHash1, Message: "older"},
		}, nil
	}
	f.git.StashApplyFunc = func(_ domain.Path, hash string, restoreIndex bool) (ports.StashApplyResult, error) {
		if hash != stashHash1 {
			t.Fatalf("applied %q, want the verified entry's full hash", hash)
		}
		f.applied = append(f.applied, restoreIndex)
		return ports.StashApplyResult{Outcome: ports.StashApplied}, nil
	}
	f.git.StashDropFunc = func(_ domain.Path, index int) error { f.dropped = append(f.dropped, index); return nil }
	return f
}

func stashInput(f *stashFixture, index int, hash string) app.RepoStashActionInput {
	return app.RepoStashActionInput{RepoRefInput: inspectRef(f.baseFixture, "api"), Index: index, Hash: hash}
}

func TestApplyStash_AppliesTheVerifiedEntryWithItsIndexAndKeepsIt(t *testing.T) {
	f := newStashFixture(t)

	r, err := app.ApplyStash(context.Background(), f.deps(), stashInput(f, 1, "bbbbbbb"))
	if err != nil || r.Err != nil || !r.IndexRestored || r.Dropped || r.Entry.Hash != stashHash1 {
		t.Fatalf("apply = %+v, %v", r, err)
	}
	if !reflect.DeepEqual(f.applied, []bool{true}) || len(f.dropped) != 0 {
		t.Fatalf("applied %v dropped %v", f.applied, f.dropped)
	}
}

func TestApplyStash_RefusesAShiftedOrMissingEntry(t *testing.T) {
	for name, in := range map[string]struct {
		index int
		hash  string
	}{"hash differs": {1, stashHash0}, "index gone": {5, stashHash1}, "no hash": {1, ""}} {
		t.Run(name, func(t *testing.T) {
			f := newStashFixture(t)
			r, err := app.ApplyStash(context.Background(), f.deps(), stashInput(f, in.index, in.hash))
			if err != nil || domain.Code(r.Err) != domain.CodeStashChanged || len(f.applied) != 0 {
				t.Fatalf("apply = %+v, %v; applied %v", r, err, f.applied)
			}
		})
	}
}

func TestApplyStash_RefusalsChangeNothing(t *testing.T) {
	ctx := context.Background()

	f := newStashFixture(t)
	f.git.IntegrationInProgressFunc = func(domain.Path) (domain.UpdateStrategy, error) { return domain.UpdateMerge, nil }
	if r, _ := app.ApplyStash(ctx, f.deps(), stashInput(f, 1, stashHash1)); domain.Code(r.Err) != domain.CodeIntegrationInProgress || len(f.applied) != 0 {
		t.Fatalf("in progress = %+v", r)
	}

	f = newStashFixture(t)
	f.git.ConflictedPathsFunc = func(domain.Path) ([]string, error) { return []string{"old.go"}, nil }
	if r, _ := app.ApplyStash(ctx, f.deps(), stashInput(f, 1, stashHash1)); domain.Code(r.Err) != domain.CodeWorktreeDirty ||
		!slices.Equal(r.Files, []string{"old.go"}) || len(f.applied) != 0 {
		t.Fatalf("unresolved conflicts = %+v", r)
	}

	f = newStashFixture(t)
	f.git.StashUntrackedFilesFunc = func(_ domain.Path, hash string) ([]string, error) {
		return []string{"notes.txt", "gone.txt"}, nil
	}
	if err := f.fs.WriteFile(f.worktree("api").Join("notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, _ := app.ApplyStash(ctx, f.deps(), stashInput(f, 1, stashHash1)); domain.Code(r.Err) != domain.CodeWorktreeDirty ||
		!slices.Equal(r.Files, []string{"notes.txt"}) || len(f.applied) != 0 {
		t.Fatalf("untracked collision = %+v", r)
	}

	f = newStashFixture(t)
	f.git.StashApplyFunc = func(domain.Path, string, bool) (ports.StashApplyResult, error) {
		return ports.StashApplyResult{Outcome: ports.StashOverwriteRefused, Files: []string{"main.go"}}, nil
	}
	if r, _ := app.ApplyStash(ctx, f.deps(), stashInput(f, 1, stashHash1)); domain.Code(r.Err) != domain.CodeWorktreeDirty || !slices.Equal(r.Files, []string{"main.go"}) {
		t.Fatalf("overwrite = %+v", r)
	}
}

func TestApplyStash_AnIndexThatNoLongerAppliesIsRetriedWithoutItAndReported(t *testing.T) {
	f := newStashFixture(t)
	f.git.StashApplyFunc = func(_ domain.Path, _ string, restoreIndex bool) (ports.StashApplyResult, error) {
		f.applied = append(f.applied, restoreIndex)
		if restoreIndex {
			return ports.StashApplyResult{Outcome: ports.StashIndexRefused}, nil
		}
		return ports.StashApplyResult{Outcome: ports.StashApplied}, nil
	}

	r, err := app.ApplyStash(context.Background(), f.deps(), stashInput(f, 1, stashHash1))
	if err != nil || r.Err != nil || r.IndexRestored || !slices.Equal(r.Warnings, []string{app.WarningIndexNotRestored}) {
		t.Fatalf("apply = %+v, %v", r, err)
	}
	if !reflect.DeepEqual(f.applied, []bool{true, false}) {
		t.Fatalf("applied %v, want --index then without", f.applied)
	}
}

func TestPopStash_DropsOnlyAfterACleanApply(t *testing.T) {
	f := newStashFixture(t)
	r, err := app.PopStash(context.Background(), f.deps(), stashInput(f, 1, stashHash1))
	if err != nil || r.Err != nil || !r.Dropped || !reflect.DeepEqual(f.dropped, []int{1}) {
		t.Fatalf("pop = %+v, %v; dropped %v", r, err, f.dropped)
	}
}

func TestPopStash_KeepsTheEntryOnConflictsAndWhenTheIndexWasNotRestored(t *testing.T) {
	f := newStashFixture(t)
	f.git.StashApplyFunc = func(domain.Path, string, bool) (ports.StashApplyResult, error) {
		f.git.ConflictedPathsFunc = func(domain.Path) ([]string, error) { return []string{"main.go"}, nil }
		return ports.StashApplyResult{Outcome: ports.StashStopped}, nil
	}
	r, _ := app.PopStash(context.Background(), f.deps(), stashInput(f, 1, stashHash1))
	if domain.Code(r.Err) != domain.CodeStashConflict || !slices.Equal(r.Conflicts, []string{"main.go"}) || r.Dropped || len(f.dropped) != 0 {
		t.Fatalf("pop conflict = %+v; dropped %v", r, f.dropped)
	}

	f = newStashFixture(t)
	f.git.StashApplyFunc = func(_ domain.Path, _ string, restoreIndex bool) (ports.StashApplyResult, error) {
		if restoreIndex {
			return ports.StashApplyResult{Outcome: ports.StashIndexRefused}, nil
		}
		return ports.StashApplyResult{Outcome: ports.StashApplied}, nil
	}
	r, _ = app.PopStash(context.Background(), f.deps(), stashInput(f, 1, stashHash1))
	if r.Err != nil || r.Dropped || len(f.dropped) != 0 || !slices.Equal(r.Warnings, []string{app.WarningIndexNotRestored}) {
		t.Fatalf("pop without index = %+v; dropped %v", r, f.dropped)
	}
}

func TestPopStash_AStopWithoutConflictsIsAGitFailureAndKeepsTheEntry(t *testing.T) {
	f := newStashFixture(t)
	f.git.StashApplyFunc = func(domain.Path, string, bool) (ports.StashApplyResult, error) {
		return ports.StashApplyResult{Outcome: ports.StashStopped, Files: []string{"notes.txt"}}, nil
	}
	r, _ := app.PopStash(context.Background(), f.deps(), stashInput(f, 1, stashHash1))
	if domain.Code(r.Err) != domain.CodeGitFailed || r.Dropped || !slices.Equal(r.Files, []string{"notes.txt"}) {
		t.Fatalf("pop = %+v", r)
	}
}

func TestDropStash_DropsTheVerifiedEntryOnly(t *testing.T) {
	f := newStashFixture(t)
	e, err := app.DropStash(context.Background(), f.deps(), stashInput(f, 1, stashHash1))
	if err != nil || e.Message != "older" || !reflect.DeepEqual(f.dropped, []int{1}) {
		t.Fatalf("drop = %+v, %v; dropped %v", e, err, f.dropped)
	}
	f = newStashFixture(t)
	if _, err := app.DropStash(context.Background(), f.deps(), stashInput(f, 0, stashHash1)); domain.Code(err) != domain.CodeStashChanged || len(f.dropped) != 0 {
		t.Fatalf("drop of a shifted entry = %v; dropped %v", err, f.dropped)
	}
}

func untrackedFixture(t *testing.T) (*baseFixture, *[][]string) {
	f := newBaseFixture(t)
	status := []domain.PorcelainEntry{
		{X: 'M', Y: ' ', RelPath: "main.go"},
		{X: '?', Y: '?', RelPath: "notes.txt"},
		{X: '?', Y: '?', RelPath: "tmp/out.log"},
		{X: '?', Y: '?', RelPath: "nested/"},
	}
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) { return status, nil }
	cleaned := &[][]string{}
	f.git.CleanUntrackedFunc = func(_ domain.Path, paths []string) error {
		*cleaned = append(*cleaned, append([]string{}, paths...))
		kept := status[:0:0]
		for _, e := range status {
			if !slices.Contains(paths, e.RelPath) {
				kept = append(kept, e)
			}
		}
		status = kept
		return nil
	}
	return f, cleaned
}

func TestDiscardUntracked_RemovesOnlyCurrentlyUntrackedFiles(t *testing.T) {
	f, cleaned := untrackedFixture(t)
	in := app.UntrackedPathsInput{RepoRefInput: inspectRef(f, "api"), Paths: []string{"notes.txt", "tmp/out.log", "notes.txt"}}

	r, err := app.DiscardUntracked(context.Background(), f.deps(), in)
	if err != nil || !slices.Equal(r.Removed, []string{"notes.txt", "tmp/out.log"}) || len(r.Kept) != 0 {
		t.Fatalf("discard = %+v, %v", r, err)
	}
	if !reflect.DeepEqual(*cleaned, [][]string{{"notes.txt", "tmp/out.log"}}) {
		t.Fatalf("cleaned %v", *cleaned)
	}
}

func TestDiscardUntracked_RefusesAnythingElseBeforeRemovingAnything(t *testing.T) {
	for _, bad := range []string{"main.go", "../outside.txt", "/etc/hosts", "nested/", "missing.txt", "tmp", ""} {
		t.Run(bad, func(t *testing.T) {
			f, cleaned := untrackedFixture(t)
			in := app.UntrackedPathsInput{RepoRefInput: inspectRef(f, "api"), Paths: []string{"notes.txt", bad}}
			if _, err := app.DiscardUntracked(context.Background(), f.deps(), in); domain.Code(err) != domain.CodePathNotUntracked {
				t.Fatalf("err = %v, want path_not_untracked", err)
			}
			if len(*cleaned) != 0 {
				t.Fatalf("cleaned %v, want nothing removed", *cleaned)
			}
		})
	}
}

func TestValidateUntracked_ResolvesAbsolutePathsInsideTheWorktree(t *testing.T) {
	f, cleaned := untrackedFixture(t)
	in := app.UntrackedPathsInput{RepoRefInput: inspectRef(f, "api"), Paths: []string{"tmp/out.log"}}

	r, err := app.ValidateUntracked(context.Background(), f.deps(), in)
	if err != nil || r.Worktree != f.worktree("api") || len(r.Paths) != 1 || r.Paths[0].Path != "tmp/out.log" ||
		r.Paths[0].Absolute != f.worktree("api").Join("tmp", "out.log") {
		t.Fatalf("validate = %+v, %v", r, err)
	}
	if len(*cleaned) != 0 {
		t.Fatal("validate removed files")
	}
	in.Paths = []string{"main.go"}
	if _, err := app.ValidateUntracked(context.Background(), f.deps(), in); domain.Code(err) != domain.CodePathNotUntracked {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifiedStash_ShowsTheEntryOnlyWhenItIsStillTheOneSeen(t *testing.T) {
	f := newStashFixture(t)
	f.git.StashShowFunc = func(_ domain.Path, index int, _ bool, _ domain.DiffLimits) (domain.Patch, error) {
		if index != 1 {
			t.Fatalf("shown index %d", index)
		}
		return domain.Patch{Files: []domain.FileDiff{{Path: "a.go"}}}, nil
	}
	d, err := app.VerifiedStash(context.Background(), f.deps(), stashInput(f, 1, stashHash1))
	if err != nil || d.Entry.Message != "older" || len(d.Patch.Files) != 1 {
		t.Fatalf("detail = %+v, %v", d, err)
	}
	if _, err := app.VerifiedStash(context.Background(), f.deps(), stashInput(f, 1, stashHash0)); domain.Code(err) != domain.CodeStashChanged {
		t.Fatalf("err = %v", err)
	}
}
