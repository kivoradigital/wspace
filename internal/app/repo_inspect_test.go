// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

func inspectRef(f *baseFixture, alias string) app.RepoRefInput {
	return app.RepoRefInput{WorkspaceRoot: f.wsRoot, Alias: alias}
}

func TestRepoBranch_UpstreamBaseAndLastFetch(t *testing.T) {
	f := newBaseFixture(t)
	f.has("api", "develop", "origin/develop")
	fetched := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		return domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin"}, true, nil
	}
	f.git.AheadBehindFunc = func(_ domain.Path, up string) (int, int, error) {
		if up != "@{upstream}" {
			t.Fatalf("ahead/behind against %q", up)
		}
		return 1, 2, nil
	}
	f.git.UnpushedCountFunc = func(_ domain.Path, base string) (int, error) { return 4, nil }
	f.git.BehindCountFunc = func(_ domain.Path, base string) (int, error) { return 5, nil }
	f.git.LastFetchFunc = func(domain.Path) (time.Time, bool, error) { return fetched, true, nil }
	f.git.HeadCommitFunc = func(domain.Path) (string, error) { return "abc123", nil }

	b, err := app.RepoBranch(context.Background(), f.deps(), inspectRef(f, "api"))
	if err != nil {
		t.Fatal(err)
	}
	if b.Branch != "feat" || b.Head != "abc123" || b.Upstream == nil || b.Upstream.Ref != "origin/feat" || b.Upstream.Ahead != 1 || b.Upstream.Behind != 2 {
		t.Fatalf("branch = %+v upstream = %+v", b, b.Upstream)
	}
	if !b.BaseFound || b.BaseBranch != "develop" || b.BaseRef != "origin/develop" || *b.BaseAhead != 4 || *b.BaseBehind != 5 {
		t.Fatalf("base = %+v", b)
	}
	if b.LastFetch == nil || !b.LastFetch.Equal(fetched) {
		t.Fatalf("last fetch = %v", b.LastFetch)
	}
}

func TestRepoBranch_NoUpstreamMissingBaseNeverFetched(t *testing.T) {
	f := newBaseFixture(t)

	b, err := app.RepoBranch(context.Background(), f.deps(), inspectRef(f, "api"))
	if err != nil {
		t.Fatal(err)
	}
	if b.Upstream != nil || b.BaseFound || b.BaseBranch != "develop" || b.BaseAhead != nil || b.BaseBehind != nil || b.LastFetch != nil {
		t.Fatalf("branch = %+v", b)
	}
}

func TestRepoBranch_UnknownAliasIsRepoNotFound(t *testing.T) {
	f := newBaseFixture(t)
	if _, err := app.RepoBranch(context.Background(), f.deps(), inspectRef(f, "ghost")); domain.Code(err) != domain.CodeRepoNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectRepo_CountsChangesAndStashes(t *testing.T) {
	f := newBaseFixture(t)
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: 'M', Y: 'M', RelPath: "a"}, {X: '?', Y: '?', RelPath: "b"}, {X: 'U', Y: 'U', RelPath: "c"}}, nil
	}
	f.git.StashListFunc = func(domain.Path) ([]domain.StashEntry, error) {
		return []domain.StashEntry{{Index: 0}, {Index: 1}}, nil
	}

	got, err := app.InspectRepo(context.Background(), f.deps(), inspectRef(f, "api"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Staged != 1 || got.Unstaged != 1 || got.Untracked != 1 || got.Conflicted != 1 || got.Stashes != 2 || got.Branch.Branch != "feat" {
		t.Fatalf("inspection = %+v", got)
	}
}

func TestRepoDiff_OnlyListedPathsAndTheRightSide(t *testing.T) {
	f := newBaseFixture(t)
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: 'R', Y: ' ', RelPath: "new.go", OrigPath: "old.go"}, {X: '?', Y: '?', RelPath: "u.txt"}}, nil
	}
	var specs []ports.DiffSpec
	f.git.DiffFunc = func(_ domain.Path, s ports.DiffSpec) (domain.Patch, error) {
		specs = append(specs, s)
		return domain.Patch{Files: []domain.FileDiff{{Path: s.Path, Status: domain.FileRenamed}}, Truncated: true}, nil
	}
	ctx := context.Background()

	d, err := app.RepoDiff(ctx, f.deps(), app.RepoDiffInput{RepoRefInput: inspectRef(f, "api"), Path: "new.go", Staged: true})
	if err != nil || d.Path != "new.go" || !d.Truncated {
		t.Fatalf("diff = %+v, %v", d, err)
	}
	if _, err := app.RepoDiff(ctx, f.deps(), app.RepoDiffInput{RepoRefInput: inspectRef(f, "api"), Path: "u.txt"}); err != nil {
		t.Fatal(err)
	}
	want := []ports.DiffSpec{
		{Path: "new.go", OrigPath: "old.go", Staged: true, Limits: domain.DefaultDiffLimits},
		{Path: "u.txt", Untracked: true, Limits: domain.DefaultDiffLimits},
	}
	if !reflect.DeepEqual(specs, want) {
		t.Fatalf("specs = %+v", specs)
	}

	for _, bad := range []app.RepoDiffInput{
		{RepoRefInput: inspectRef(f, "api"), Path: "../../etc/passwd"},
		{RepoRefInput: inspectRef(f, "api"), Path: "u.txt", Staged: true},
		{RepoRefInput: inspectRef(f, "api"), Path: "new.go"},
	} {
		if _, err := app.RepoDiff(ctx, f.deps(), bad); domain.Code(err) != domain.CodePathNotChanged {
			t.Fatalf("diff %+v err = %v, want path_not_changed", bad, err)
		}
	}
}

func TestRepoDiff_ChangeWithoutPatchIsAnEmptyFileDiff(t *testing.T) {
	f := newBaseFixture(t)
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "a.go"}}, nil
	}
	d, err := app.RepoDiff(context.Background(), f.deps(), app.RepoDiffInput{RepoRefInput: inspectRef(f, "api"), Path: "a.go"})
	if err != nil || d.Path != "a.go" || d.Status != domain.FileModified || d.Hunks == nil {
		t.Fatalf("diff = %+v, %v", d, err)
	}
}

func TestRepoCommitLog_RangeFromTheBaseWithPaging(t *testing.T) {
	f := newBaseFixture(t)
	f.has("api", "develop", "origin/develop")
	var gotRange string
	var gotSkip, gotMax int
	f.git.CountCommitsFunc = func(_ domain.Path, r string) (int, error) { return 3, nil }
	f.git.CommitLogFunc = func(_ domain.Path, r string, skip, max int) ([]domain.CommitInfo, error) {
		gotRange, gotSkip, gotMax = r, skip, max
		return []domain.CommitInfo{{Hash: "h2"}, {Hash: "h3"}}, nil
	}

	got, err := app.RepoCommitLog(context.Background(), f.deps(), app.RepoCommitsInput{RepoRefInput: inspectRef(f, "api"), Offset: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if gotRange != "origin/develop..HEAD" || gotSkip != 1 || gotMax != 2 {
		t.Fatalf("log(%q, %d, %d)", gotRange, gotSkip, gotMax)
	}
	if got.Range != "origin/develop..HEAD" || got.Base != "origin/develop" || got.Total != 3 || got.Offset != 1 || len(got.Commits) != 1 || !got.HasMore {
		t.Fatalf("commits = %+v", got)
	}
}

func TestRepoCommitLog_WithoutABaseListsHEADAndCapsTheLimit(t *testing.T) {
	f := newBaseFixture(t)
	var gotRange string
	var gotMax int
	f.git.CommitLogFunc = func(_ domain.Path, r string, _, max int) ([]domain.CommitInfo, error) {
		gotRange, gotMax = r, max
		return nil, nil
	}
	got, err := app.RepoCommitLog(context.Background(), f.deps(), app.RepoCommitsInput{RepoRefInput: inspectRef(f, "api"), Limit: 5000})
	if err != nil || gotRange != "HEAD" || gotMax != app.MaxCommitsPage+1 || got.Base != "" || got.Commits == nil {
		t.Fatalf("commits = %+v (range %q max %d), %v", got, gotRange, gotMax, err)
	}
}

func TestRepoCommitLog_UpstreamRangeListsTheCommitsNotPushed(t *testing.T) {
	f := newBaseFixture(t)
	f.has("api", "develop", "origin/develop")
	f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		return domain.UpstreamInfo{Ref: "origin/feature/x", Remote: "origin"}, true, nil
	}
	var counted, logged string
	f.git.CountCommitsFunc = func(_ domain.Path, r string) (int, error) { counted = r; return 13, nil }
	f.git.CommitLogFunc = func(_ domain.Path, r string, _, _ int) ([]domain.CommitInfo, error) {
		logged = r
		return []domain.CommitInfo{{Hash: "h1"}}, nil
	}

	got, err := app.RepoCommitLog(context.Background(), f.deps(), app.RepoCommitsInput{RepoRefInput: inspectRef(f, "api"), Range: app.CommitRangeUpstream})
	if err != nil {
		t.Fatal(err)
	}
	if counted != "origin/feature/x..HEAD" || logged != "origin/feature/x..HEAD" {
		t.Fatalf("count(%q) log(%q)", counted, logged)
	}
	if got.Range != "origin/feature/x..HEAD" || got.Upstream != "origin/feature/x" || got.Base != "" || got.Total != 13 || len(got.Commits) != 1 {
		t.Fatalf("commits = %+v", got)
	}
}

func TestRepoCommitLog_UpstreamRangeWithoutAnUpstreamIsNoUpstream(t *testing.T) {
	for _, gone := range []bool{false, true} {
		f := newBaseFixture(t)
		f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
			return domain.UpstreamInfo{Ref: "origin/feature/x", Remote: "origin", Gone: true}, gone, nil
		}
		_, err := app.RepoCommitLog(context.Background(), f.deps(), app.RepoCommitsInput{RepoRefInput: inspectRef(f, "api"), Range: app.CommitRangeUpstream})
		if domain.Code(err) != domain.CodeNoUpstream {
			t.Fatalf("gone=%v: err = %v", gone, err)
		}
	}
}

func TestRepoStash_ValidatesTheIndexAndGatesUntrackedOnGitVersion(t *testing.T) {
	f := newBaseFixture(t)
	f.git.StashListFunc = func(domain.Path) ([]domain.StashEntry, error) {
		return []domain.StashEntry{{Index: 0, Ref: "stash@{0}", Message: "wip"}}, nil
	}
	var untracked []bool
	f.git.StashShowFunc = func(_ domain.Path, i int, u bool, _ domain.DiffLimits) (domain.Patch, error) {
		untracked = append(untracked, u)
		return domain.Patch{Files: []domain.FileDiff{}}, nil
	}
	ctx := context.Background()

	got, err := app.RepoStash(ctx, f.deps(), app.RepoStashInput{RepoRefInput: inspectRef(f, "api"), Index: 0})
	if err != nil || got.Entry.Message != "wip" || !got.IncludesUntracked {
		t.Fatalf("stash = %+v, %v", got, err)
	}
	f.git.VersionFunc = func() (ports.Version, error) { return ports.Version{Major: 2, Minor: 31}, nil }
	got, err = app.RepoStash(ctx, f.deps(), app.RepoStashInput{RepoRefInput: inspectRef(f, "api"), Index: 0})
	if err != nil || got.IncludesUntracked {
		t.Fatalf("old git stash = %+v, %v", got, err)
	}
	if !reflect.DeepEqual(untracked, []bool{true, false}) {
		t.Fatalf("include untracked = %v", untracked)
	}
	if _, err := app.RepoStash(ctx, f.deps(), app.RepoStashInput{RepoRefInput: inspectRef(f, "api"), Index: 3}); domain.Code(err) != domain.CodeRefNotFound {
		t.Fatalf("missing stash err = %v", err)
	}
}

// pullFixture: api on feat tracking origin/feat, 2 behind, 0 ahead.
func newPullFixture(t *testing.T) (*baseFixture, *[]string) {
	f := newBaseFixture(t)
	head := "before"
	fetched := &[]string{}
	f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		return domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin"}, true, nil
	}
	f.git.FetchFunc = func(_ domain.Path, remote string) error { *fetched = append(*fetched, remote); return nil }
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 0, 2, nil }
	f.git.HeadCommitFunc = func(domain.Path) (string, error) { return head, nil }
	f.git.MergeFastForwardFunc = func(_ domain.Path, ref string) ([]string, error) {
		if ref != "origin/feat" {
			t.Fatalf("ff ref = %q", ref)
		}
		head = "after"
		return nil, nil
	}
	return f, fetched
}

func TestPullRepo_FastForwardsFromTheUpstream(t *testing.T) {
	f, fetched := newPullFixture(t)
	res, err := app.PullRepo(context.Background(), f.deps(), inspectRef(f, "api"))
	if err != nil || res.Err != nil || res.Upstream != "origin/feat" || res.BeforeHead != "before" || res.AfterHead != "after" || res.CommitsPulled != 2 || res.UpToDate {
		t.Fatalf("pull = %+v, %v", res, err)
	}
	if !reflect.DeepEqual(*fetched, []string{"origin"}) {
		t.Fatalf("fetched %v", *fetched)
	}
}

func TestPullRepo_Refusals(t *testing.T) {
	ctx := context.Background()

	f, _ := newPullFixture(t)
	f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) { return domain.UpstreamInfo{}, false, nil }
	if res, _ := app.PullRepo(ctx, f.deps(), inspectRef(f, "api")); domain.Code(res.Err) != domain.CodeNoUpstream {
		t.Fatalf("no upstream = %+v", res)
	}

	f, _ = newPullFixture(t)
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 1, 2, nil }
	res, _ := app.PullRepo(ctx, f.deps(), inspectRef(f, "api"))
	if domain.Code(res.Err) != domain.CodeDiverged || res.Ahead != 1 || res.Behind != 2 || res.AfterHead != "before" {
		t.Fatalf("diverged = %+v", res)
	}

	f, _ = newPullFixture(t)
	f.git.MergeFastForwardFunc = func(domain.Path, string) ([]string, error) {
		return []string{"x.go"}, domain.NewOpError("git.merge_ff_only", domain.CodeWorktreeDirty, "", "", nil)
	}
	res, _ = app.PullRepo(ctx, f.deps(), inspectRef(f, "api"))
	if domain.Code(res.Err) != domain.CodeWorktreeDirty || !reflect.DeepEqual(res.DirtyFiles, []string{"x.go"}) {
		t.Fatalf("dirty = %+v", res)
	}

	f, _ = newPullFixture(t)
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 0, 0, nil }
	res, _ = app.PullRepo(ctx, f.deps(), inspectRef(f, "api"))
	if res.Err != nil || !res.UpToDate || res.AfterHead != "before" {
		t.Fatalf("up to date = %+v", res)
	}

	f, _ = newPullFixture(t)
	f.git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "", true, nil }
	if res, _ := app.PullRepo(ctx, f.deps(), inspectRef(f, "api")); domain.Code(res.Err) != domain.CodeDetachedHead {
		t.Fatalf("detached = %+v", res)
	}
}

func TestFetchRepo_FetchesTheConfiguredRemoteAndReportsTheBranch(t *testing.T) {
	f, fetched := newPullFixture(t)
	res, err := app.FetchRepo(context.Background(), f.deps(), inspectRef(f, "api"))
	if err != nil || res.Remote != "origin" || res.Branch.Upstream == nil || res.Branch.Upstream.Behind != 2 {
		t.Fatalf("fetch = %+v, %v", res, err)
	}
	if !reflect.DeepEqual(*fetched, []string{"origin"}) {
		t.Fatalf("fetched %v", *fetched)
	}
}
