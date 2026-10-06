// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// writeFixture scripts one worktree's status and records every write
// action the use cases run.
type writeFixture struct {
	*baseFixture
	status   []domain.PorcelainEntry
	staged   [][]string
	unstaged [][]string
	unborn   []bool
	restored [][]string
}

func newWriteFixture(t *testing.T, status ...domain.PorcelainEntry) *writeFixture {
	f := &writeFixture{baseFixture: newBaseFixture(t), status: status}
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) { return f.status, nil }
	f.git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feat", false, nil }
	f.git.StageFunc = func(_ domain.Path, p []string) error { f.staged = append(f.staged, p); return nil }
	f.git.UnstageFunc = func(_ domain.Path, p []string, unborn bool) error {
		f.unstaged = append(f.unstaged, p)
		f.unborn = append(f.unborn, unborn)
		return nil
	}
	f.git.RestoreWorktreeFunc = func(_ domain.Path, p []string) error { f.restored = append(f.restored, p); return nil }
	return f
}

func (f *writeFixture) paths(paths ...string) app.ChangePathsInput {
	return app.ChangePathsInput{RepoRefInput: inspectRef(f.baseFixture, "api"), Paths: paths}
}

func entry(x, y byte, path string) domain.PorcelainEntry {
	return domain.PorcelainEntry{X: x, Y: y, RelPath: path}
}

func TestStageChanges_StagesUnstagedAndUntrackedPathsOnly(t *testing.T) {
	f := newWriteFixture(t, entry(' ', 'M', "main.go"), entry(' ', 'D', "gone.go"), entry('?', '?', "new.go"),
		entry('M', ' ', "staged.go"), entry('U', 'U', "conflict.go"), entry('?', '?', "nested/"))

	r, err := app.StageChanges(context.Background(), f.deps(), f.paths("main.go", "gone.go", "new.go", "main.go"))
	if err != nil || !slices.Equal(r.Paths, []string{"main.go", "gone.go", "new.go"}) || r.Alias != "api" {
		t.Fatalf("stage = %+v, %v", r, err)
	}
	if !reflect.DeepEqual(f.staged, [][]string{{"main.go", "gone.go", "new.go"}}) {
		t.Fatalf("staged %v", f.staged)
	}
	for _, bad := range []string{"staged.go", "conflict.go", "nested/", "missing.go", "../x", ""} {
		f.staged = nil
		if _, err := app.StageChanges(context.Background(), f.deps(), f.paths("main.go", bad)); domain.Code(err) != domain.CodePathNotChanged || f.staged != nil {
			t.Fatalf("stage %q = %v; staged %v", bad, err, f.staged)
		}
	}
}

func TestStageChanges_AllStagesEveryUnstagedAndUntrackedPath(t *testing.T) {
	f := newWriteFixture(t, entry(' ', 'M', "main.go"), entry('?', '?', "new.go"), entry('M', ' ', "staged.go"), entry('?', '?', "nested/"))
	in := f.paths()
	in.All = true
	r, err := app.StageChanges(context.Background(), f.deps(), in)
	if err != nil || !slices.Equal(r.Paths, []string{"main.go", "new.go"}) {
		t.Fatalf("stage all = %+v, %v", r, err)
	}
}

func TestUnstageChanges_UnstagesStagedPathsWithRenameSources(t *testing.T) {
	f := newWriteFixture(t, entry('M', 'M', "main.go"), domain.PorcelainEntry{X: 'R', Y: ' ', RelPath: "new.go", OrigPath: "old.go"}, entry(' ', 'M', "only-unstaged.go"))

	r, err := app.UnstageChanges(context.Background(), f.deps(), f.paths("main.go", "new.go"))
	if err != nil || !slices.Equal(r.Paths, []string{"main.go", "new.go"}) {
		t.Fatalf("unstage = %+v, %v", r, err)
	}
	if !reflect.DeepEqual(f.unstaged, [][]string{{"main.go", "new.go", "old.go"}}) || !slices.Equal(f.unborn, []bool{false}) {
		t.Fatalf("unstaged %v unborn %v", f.unstaged, f.unborn)
	}
	if _, err := app.UnstageChanges(context.Background(), f.deps(), f.paths("only-unstaged.go")); domain.Code(err) != domain.CodePathNotChanged {
		t.Fatalf("unstage of an unstaged-only path = %v", err)
	}
}

func TestUnstageChanges_InARepoWithoutCommitsRemovesFromTheIndex(t *testing.T) {
	f := newWriteFixture(t, entry('A', ' ', "first.go"))
	f.git.HeadCommitFunc = func(domain.Path) (string, error) {
		return "", domain.NewOpError("git.head_commit", domain.CodeGitFailed, "", "", nil)
	}
	if _, err := app.UnstageChanges(context.Background(), f.deps(), f.paths("first.go")); err != nil || !slices.Equal(f.unborn, []bool{true}) {
		t.Fatalf("unstage unborn = %v; unborn %v", err, f.unborn)
	}
}

func discardFixture(t *testing.T) *writeFixture {
	f := newWriteFixture(t, entry(' ', 'M', "main.go"), entry('M', 'M', "both.go"), entry(' ', 'D', "gone.go"),
		entry('M', ' ', "staged.go"), entry('?', '?', "new.go"), entry('U', 'U', "conflict.go"), entry(' ', 'A', "intent.go"))
	f.git.DiffFunc = func(_ domain.Path, s ports.DiffSpec) (domain.Patch, error) {
		if s.Staged || s.Untracked {
			t.Fatalf("preview diffed %+v, want the unstaged side", s)
		}
		return domain.Patch{Files: []domain.FileDiff{{Path: s.Path, Status: domain.FileModified, Additions: 2, Deletions: 1}}}, nil
	}
	f.git.UnstagedPatchFunc = func(_ domain.Path, p []string) (string, error) {
		return "diff --git a/" + p[0] + " b/" + p[0] + "\n", nil
	}
	return f
}

func TestPreviewDiscard_ListsFilesWithLineCounts(t *testing.T) {
	f := discardFixture(t)
	p, err := app.PreviewDiscard(context.Background(), f.deps(), f.paths("main.go", "both.go"))
	if err != nil || len(p.Files) != 2 || p.Files[0].Path != "main.go" || p.Files[0].Additions != 2 || p.Files[1].Deletions != 1 {
		t.Fatalf("preview = %+v, %v", p, err)
	}
	if len(f.restored) != 0 {
		t.Fatal("preview discarded changes")
	}
}

func TestDiscardChanges_RefusesUntrackedStagedOnlyAndOtherPaths(t *testing.T) {
	for bad, want := range map[string]domain.ErrCode{
		"new.go":      domain.CodePathIsUntracked,
		"intent.go":   domain.CodePathIsUntracked,
		"staged.go":   domain.CodeStagedOnly,
		"conflict.go": domain.CodePathNotChanged,
		"missing.go":  domain.CodePathNotChanged,
		"../x":        domain.CodePathNotChanged,
	} {
		t.Run(bad, func(t *testing.T) {
			f := discardFixture(t)
			if _, err := app.DiscardChanges(context.Background(), f.deps(), f.paths("main.go", bad)); domain.Code(err) != want {
				t.Fatalf("discard %q = %v, want %s", bad, err, want)
			}
			if len(f.restored) != 0 || len(f.fs.Writes) != 0 {
				t.Fatalf("refused discard changed something: restored %v writes %v", f.restored, f.fs.Writes)
			}
		})
	}
}

func TestDiscardChanges_WritesABackupPatchBeforeRestoring(t *testing.T) {
	f := discardFixture(t)
	var order []string
	f.git.UnstagedPatchFunc = func(_ domain.Path, p []string) (string, error) {
		order = append(order, "patch")
		return "diff --git a/main.go b/main.go\n-old\n+new\n", nil
	}
	f.git.RestoreWorktreeFunc = func(_ domain.Path, p []string) error {
		order = append(order, "restore")
		f.restored = append(f.restored, p)
		return nil
	}
	deps := f.deps()
	deps.Now = func() time.Time { return time.Date(2026, 10, 4, 9, 30, 15, 0, time.UTC) }

	r, err := app.DiscardChanges(context.Background(), deps, f.paths("main.go", "gone.go", "both.go"))
	if err != nil || !slices.Equal(r.Discarded, []string{"main.go", "gone.go", "both.go"}) {
		t.Fatalf("discard = %+v, %v", r, err)
	}
	if !slices.Equal(order, []string{"patch", "restore"}) || !reflect.DeepEqual(f.restored, [][]string{{"main.go", "gone.go", "both.go"}}) {
		t.Fatalf("order %v restored %v", order, f.restored)
	}
	want := f.fs.Paths().Cache.Join("discarded", "api-20261004T093015.000Z.patch")
	if r.Backup != want {
		t.Fatalf("backup = %q, want %q", r.Backup, want)
	}
	data, err := f.fs.ReadFile(want)
	if err != nil || !strings.Contains(string(data), "+new") || !strings.Contains(string(data), "git -C "+string(f.worktree("api"))+" apply") {
		t.Fatalf("backup content = %q, %v", data, err)
	}
}

func TestDiscardChanges_ABackupThatCannotBeWrittenStopsTheDiscard(t *testing.T) {
	f := discardFixture(t)
	f.fs.DenyWrite = func(domain.Path) bool { return true }
	if _, err := app.DiscardChanges(context.Background(), f.deps(), f.paths("main.go")); err == nil || len(f.restored) != 0 {
		t.Fatalf("discard without backup = %v; restored %v", err, f.restored)
	}
}

func TestDiscardChanges_KeepsOnlyTheNewestBackups(t *testing.T) {
	f := discardFixture(t)
	dir := f.fs.Paths().Cache.Join("discarded")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Every file gets the same mod time, as on a coarse clock: the new
	// "api-" backup must still outrank the older "web-" ones.
	f.fs.Now = func() time.Time { return base }
	for i := 0; i < app.DiscardBackupsKept+3; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		if err := f.fs.WriteFile(dir.Join("web-"+ts.Format("20060102T150405.000Z")+".patch"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	deps := f.deps()
	deps.Now = func() time.Time { return base.Add(time.Hour) }
	if _, err := app.DiscardChanges(context.Background(), deps, f.paths("main.go")); err != nil {
		t.Fatal(err)
	}
	files, _ := f.fs.ListFiles(dir)
	if len(files) != app.DiscardBackupsKept || slices.Contains(files, "web-20260101T000000.000Z.patch") || !slices.Contains(files, "api-20260101T010000.000Z.patch") {
		t.Fatalf("kept %d backups: %v", len(files), files)
	}
}

func commitFixture(t *testing.T, status ...domain.PorcelainEntry) (*writeFixture, *[]string) {
	if len(status) == 0 {
		status = []domain.PorcelainEntry{entry('M', ' ', "main.go")}
	}
	f := newWriteFixture(t, status...)
	messages := &[]string{}
	f.git.CommitFunc = func(_ domain.Path, m string) (ports.CommitResult, error) {
		*messages = append(*messages, m)
		return ports.CommitResult{Outcome: ports.CommitCreated}, nil
	}
	f.git.CommitLogFunc = func(_ domain.Path, rev string, skip, max int) ([]domain.CommitInfo, error) {
		if rev != "HEAD" || max != 1 {
			t.Fatalf("log %q %d", rev, max)
		}
		return []domain.CommitInfo{{Hash: "abcdef0123456789abcdef0123456789abcdef01", ShortHash: "abcdef0", Subject: "feat: login"}}, nil
	}
	return f, messages
}

func commitIn(f *writeFixture, msg string) app.CommitInput {
	return app.CommitInput{RepoRefInput: inspectRef(f.baseFixture, "api"), Message: msg}
}

func TestCommitChanges_CommitsTheIndexAndReturnsTheNewCommit(t *testing.T) {
	f, messages := commitFixture(t)
	r, err := app.CommitChanges(context.Background(), f.deps(), commitIn(f, "  feat: login\n\nbody  \n"))
	if err != nil || r.Err != nil || r.Commit.ShortHash != "abcdef0" || len(r.Warnings) != 0 {
		t.Fatalf("commit = %+v, %v", r, err)
	}
	if !slices.Equal(*messages, []string{"feat: login\n\nbody"}) {
		t.Fatalf("messages %q", *messages)
	}
}

func TestCommitChanges_ALongSubjectIsAWarningNotAnError(t *testing.T) {
	f, messages := commitFixture(t)
	r, _ := app.CommitChanges(context.Background(), f.deps(), commitIn(f, strings.Repeat("x", 73)))
	if r.Err != nil || !slices.Equal(r.Warnings, []string{app.WarningSubjectTooLong}) || len(*messages) != 1 {
		t.Fatalf("commit = %+v", r)
	}
}

func TestCommitChanges_Refusals(t *testing.T) {
	ctx := context.Background()
	f, messages := commitFixture(t, entry(' ', 'M', "main.go"), entry('?', '?', "new.go"))
	if r, _ := app.CommitChanges(ctx, f.deps(), commitIn(f, "msg")); domain.Code(r.Err) != domain.CodeNothingStaged {
		t.Fatalf("nothing staged = %+v", r)
	}
	if _, err := app.CommitChanges(ctx, f.deps(), commitIn(f, " \n\t")); !errors.Is(err, app.ErrEmptyCommitMessage) {
		t.Fatalf("empty message = %v", err)
	}

	f, _ = commitFixture(t)
	f.git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "", true, nil }
	if r, _ := app.CommitChanges(ctx, f.deps(), commitIn(f, "msg")); domain.Code(r.Err) != domain.CodeDetachedHead {
		t.Fatalf("detached = %+v", r)
	}

	f, _ = commitFixture(t)
	f.git.IntegrationInProgressFunc = func(domain.Path) (domain.UpdateStrategy, error) { return domain.UpdateRebase, nil }
	if r, _ := app.CommitChanges(ctx, f.deps(), commitIn(f, "msg")); domain.Code(r.Err) != domain.CodeIntegrationInProgress {
		t.Fatalf("rebase in progress = %+v", r)
	}

	f, _ = commitFixture(t)
	f.git.IdentityConfiguredFunc = func(domain.Path) (bool, error) { return false, nil }
	r, _ := app.CommitChanges(ctx, f.deps(), commitIn(f, "msg"))
	if domain.Code(r.Err) != domain.CodeIdentityMissing || len(r.IdentityCommands) != 2 || !strings.HasPrefix(r.IdentityCommands[0], "git config --global user.name") {
		t.Fatalf("identity missing = %+v", r)
	}
	if len(*messages) != 0 || calls(f, "Commit") != 0 {
		t.Fatal("a refused commit ran git commit")
	}

	f, _ = commitFixture(t)
	f.git.CommitFunc = func(domain.Path, string) (ports.CommitResult, error) {
		return ports.CommitResult{Outcome: ports.CommitHookFailed, Output: "lint failed"}, nil
	}
	if r, _ := app.CommitChanges(ctx, f.deps(), commitIn(f, "msg")); domain.Code(r.Err) != domain.CodeHookFailed || r.Output != "lint failed" {
		t.Fatalf("hook failed = %+v", r)
	}
}

func calls(f *writeFixture, method string) int {
	n := 0
	for _, c := range f.git.Calls {
		if c.Method == method {
			n++
		}
	}
	return n
}

func pushFixture(t *testing.T) (*writeFixture, *[]ports.PushSpec) {
	f := newWriteFixture(t)
	specs := &[]ports.PushSpec{}
	f.git.PushFunc = func(_ domain.Path, s ports.PushSpec) (ports.PushResult, error) {
		*specs = append(*specs, s)
		return ports.PushResult{Outcome: ports.PushDone}, nil
	}
	f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		return domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin", RemoteRef: "refs/heads/feat"}, true, nil
	}
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 2, 0, nil }
	return f, specs
}

func pushIn(f *writeFixture, setUpstream bool) app.PushInput {
	return app.PushInput{RepoRefInput: inspectRef(f.baseFixture, "api"), SetUpstream: setUpstream}
}

func TestPushRepo_PushesTheBranchToItsUpstream(t *testing.T) {
	f, specs := pushFixture(t)
	r, err := app.PushRepo(context.Background(), f.deps(), pushIn(f, false))
	if err != nil || r.Err != nil || r.Pushed != 2 || r.Upstream != "origin/feat" || r.Remote != "origin" || r.Branch != "feat" {
		t.Fatalf("push = %+v, %v", r, err)
	}
	if !reflect.DeepEqual(*specs, []ports.PushSpec{{Remote: "origin", Refspec: "refs/heads/feat:refs/heads/feat"}}) {
		t.Fatalf("specs %+v", *specs)
	}
}

func TestPushRepo_NothingToPushIsUpToDate(t *testing.T) {
	f, specs := pushFixture(t)
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 0, 3, nil }
	r, _ := app.PushRepo(context.Background(), f.deps(), pushIn(f, false))
	if r.Err != nil || !r.UpToDate || len(*specs) != 0 {
		t.Fatalf("push = %+v; specs %+v", r, *specs)
	}
}

func TestPushRepo_WithoutUpstreamPublishesOnlyWhenAsked(t *testing.T) {
	for name, up := range map[string]func(domain.Path) (domain.UpstreamInfo, bool, error){
		"none": func(domain.Path) (domain.UpstreamInfo, bool, error) { return domain.UpstreamInfo{}, false, nil },
		"gone": func(domain.Path) (domain.UpstreamInfo, bool, error) {
			return domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin", Gone: true}, true, nil
		},
		"local": func(domain.Path) (domain.UpstreamInfo, bool, error) {
			return domain.UpstreamInfo{Ref: "main", Remote: "."}, true, nil
		},
		// A branch created from origin/develop tracks it; pushing there
		// would land feature commits on the base branch.
		"other name": func(domain.Path) (domain.UpstreamInfo, bool, error) {
			return domain.UpstreamInfo{Ref: "origin/develop", Remote: "origin", RemoteRef: "refs/heads/develop"}, true, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, specs := pushFixture(t)
			f.git.UpstreamFunc = up
			r, _ := app.PushRepo(context.Background(), f.deps(), pushIn(f, false))
			if domain.Code(r.Err) != domain.CodeNoUpstream || r.Remote != "origin" || len(*specs) != 0 {
				t.Fatalf("push without upstream = %+v", r)
			}
			r, _ = app.PushRepo(context.Background(), f.deps(), pushIn(f, true))
			if r.Err != nil || !r.SetUpstream || !reflect.DeepEqual(*specs, []ports.PushSpec{{Remote: "origin", Refspec: "refs/heads/feat:refs/heads/feat", SetUpstream: true}}) {
				t.Fatalf("publish = %+v; specs %+v", r, *specs)
			}
		})
	}
}

func TestPushRepo_RefusalsAndFailures(t *testing.T) {
	ctx := context.Background()
	f, specs := pushFixture(t)
	f.git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "", true, nil }
	if r, _ := app.PushRepo(ctx, f.deps(), pushIn(f, true)); domain.Code(r.Err) != domain.CodeDetachedHead || len(*specs) != 0 {
		t.Fatalf("detached = %+v", r)
	}
	for outcome, want := range map[ports.PushOutcome]domain.ErrCode{
		ports.PushRejected:   domain.CodePushRejected,
		ports.PushAuthFailed: domain.CodeAuthFailed,
		ports.PushFailed:     domain.CodeGitFailed,
	} {
		f, _ := pushFixture(t)
		f.git.PushFunc = func(domain.Path, ports.PushSpec) (ports.PushResult, error) {
			return ports.PushResult{Outcome: outcome, Output: "git said no"}, nil
		}
		if r, _ := app.PushRepo(ctx, f.deps(), pushIn(f, false)); domain.Code(r.Err) != want || r.Output != "git said no" {
			t.Fatalf("%s = %+v", outcome, r)
		}
	}
}

func stashCreateIn(f *writeFixture, msg string, untracked, keepIndex bool) app.StashCreateInput {
	return app.StashCreateInput{RepoRefInput: inspectRef(f.baseFixture, "api"), Message: msg, IncludeUntracked: untracked, KeepIndex: keepIndex}
}

func TestCreateStash_SavesChangesAndReturnsTheNewEntry(t *testing.T) {
	f := newWriteFixture(t, entry(' ', 'M', "main.go"), entry('?', '?', "new.go"))
	stashRef := ""
	var pushed []ports.StashPushSpec
	f.git.StashRefFunc = func(domain.Path) (string, error) { return stashRef, nil }
	f.git.StashPushFunc = func(_ domain.Path, s ports.StashPushSpec) error {
		pushed = append(pushed, s)
		stashRef = stashHash0
		return nil
	}
	f.git.StashListFunc = func(domain.Path) ([]domain.StashEntry, error) {
		return []domain.StashEntry{{Index: 0, Ref: "stash@{0}", Hash: stashHash0, Message: "wip"}}, nil
	}
	e, err := app.CreateStash(context.Background(), f.deps(), stashCreateIn(f, " wip ", true, true))
	if err != nil || e.Hash != stashHash0 || !reflect.DeepEqual(pushed, []ports.StashPushSpec{{Message: "wip", IncludeUntracked: true, KeepIndex: true}}) {
		t.Fatalf("stash = %+v, %v; pushed %+v", e, err, pushed)
	}
}

func TestCreateStash_NothingToStash(t *testing.T) {
	f := newWriteFixture(t, entry('?', '?', "new.go"))
	if _, err := app.CreateStash(context.Background(), f.deps(), stashCreateIn(f, "", false, false)); domain.Code(err) != domain.CodeNothingToStash || calls(f, "StashPush") != 0 {
		t.Fatalf("only untracked without -u = %v", err)
	}
	// git saved nothing (refs/stash unchanged): also nothing_to_stash.
	f = newWriteFixture(t, entry(' ', 'M', "main.go"))
	if _, err := app.CreateStash(context.Background(), f.deps(), stashCreateIn(f, "", false, false)); domain.Code(err) != domain.CodeNothingToStash {
		t.Fatalf("unchanged stash ref = %v", err)
	}
	f = newWriteFixture(t, entry('U', 'U', "conflict.go"), entry(' ', 'M', "main.go"))
	if _, err := app.CreateStash(context.Background(), f.deps(), stashCreateIn(f, "", false, false)); domain.Code(err) != domain.CodeWorktreeDirty || calls(f, "StashPush") != 0 {
		t.Fatalf("conflicts = %v", err)
	}
}
