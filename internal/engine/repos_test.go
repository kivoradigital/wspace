// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/ports"
)

func newInspectFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	if _, err := f.eng.CreateWorkspace(context.Background(), engine.CreateWorkspaceParams{Name: "feat", Projects: []string{"api"}}, nil); err != nil {
		t.Fatal(err)
	}
	f.git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feat", false, nil }
	f.git.HeadCommitFunc = func(domain.Path) (string, error) { return "1111111111111111111111111111111111111111", nil }
	return f
}

func TestRepoInspect_SummaryJSONShape(t *testing.T) {
	f := newInspectFixture(t)
	f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		return domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin"}, true, nil
	}
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 2, 1, nil }
	f.git.LastFetchFunc = func(domain.Path) (time.Time, bool, error) {
		return time.Date(2026, 10, 3, 12, 0, 0, 0, time.FixedZone("x", 7200)), true, nil
	}
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: 'M', Y: ' ', RelPath: "a"}, {X: '?', Y: '?', RelPath: "b"}}, nil
	}

	got, err := f.eng.RepoInspect(context.Background(), engine.RepoRef{Workspace: "feat", Repo: "api"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	for _, want := range []string{`"repo":"api"`, `"branch":"feat"`, `"upstream":{"ref":"origin/feat","remote":"origin","gone":false,"ahead":2,"behind":1}`,
		`"lastFetch":"2026-10-03T10:00:00Z"`, `"staged":1`, `"untracked":1`, `"stashes":0`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("json %s lacks %s", b, want)
		}
	}
}

func TestRepoDiff_ParamsAndNotChangedPath(t *testing.T) {
	f := newInspectFixture(t)
	ctx := context.Background()
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "a.go"}}, nil
	}
	f.git.DiffFunc = func(_ domain.Path, s ports.DiffSpec) (domain.Patch, error) {
		return domain.Patch{Files: []domain.FileDiff{{Path: s.Path, Status: domain.FileModified, Additions: 1,
			Hunks: []domain.DiffHunk{{Header: "@@ -1 +1,2 @@", OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 2, Lines: []string{" a", "+b"}}}}}}, nil
	}

	d, err := f.eng.RepoDiff(ctx, engine.RepoDiffParams{Workspace: "feat", Repo: "api", Path: "a.go"})
	if err != nil || d.Path != "a.go" || d.Additions != 1 || len(d.Hunks) != 1 || d.Hunks[0].Lines[1] != "+b" {
		t.Fatalf("diff = %+v, %v", d, err)
	}
	if _, err := f.eng.RepoDiff(ctx, engine.RepoDiffParams{Workspace: "feat", Repo: "api", Path: "nope.go"}); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("not changed err = %v", err)
	}
	if _, err := f.eng.RepoDiff(ctx, engine.RepoDiffParams{Workspace: "feat", Repo: "api"}); codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("empty path err = %v", err)
	}
}

func TestRepoCommits_ValidatesPagingAndHash(t *testing.T) {
	f := newInspectFixture(t)
	ctx := context.Background()
	if _, err := f.eng.RepoCommits(ctx, engine.RepoCommitsParams{Workspace: "feat", Repo: "api", Offset: -1}); codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("negative offset err = %v", err)
	}
	bad, err := f.eng.RepoCommits(ctx, engine.RepoCommitsParams{Workspace: "feat", Repo: "api", Range: "remote"})
	if codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("unknown range = %+v, %v", bad, err)
	}
	if _, err := f.eng.RepoCommit(ctx, engine.RepoCommitParams{Workspace: "feat", Repo: "api", Hash: "HEAD~1"}); codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("revision expression err = %v", err)
	}
	if _, err := f.eng.RepoStash(ctx, engine.RepoStashParams{Workspace: "feat", Repo: "api", Index: -1}); codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("negative index err = %v", err)
	}
}

func TestRepoCommit_NeverCarriesAnEmailField(t *testing.T) {
	f := newInspectFixture(t)
	f.git.CommitDetailFunc = func(_ domain.Path, hash string, _ domain.DiffLimits) (domain.CommitDetail, error) {
		return domain.CommitDetail{CommitInfo: domain.CommitInfo{Hash: hash, ShortHash: hash[:7], Subject: "s", AuthorName: "Ada Acme"}, Parents: []string{}}, nil
	}
	got, err := f.eng.RepoCommit(context.Background(), engine.RepoCommitParams{Workspace: "feat", Repo: "api", Hash: "abcdef1234"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(strings.ToLower(string(b)), "email") || !strings.Contains(string(b), `"author":"Ada Acme"`) || !strings.Contains(string(b), `"files":[]`) {
		t.Fatalf("json = %s", b)
	}
}

func TestRepoPull_DivergedIsAResultErrorWithCounts(t *testing.T) {
	f := newInspectFixture(t)
	f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		return domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin"}, true, nil
	}
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 1, 3, nil }

	res, err := f.eng.RepoPull(context.Background(), engine.RepoRef{Workspace: "feat", Repo: "api"}, nil)
	if err != nil || res.Error == nil || res.Error.Code != engine.CodeConflict || res.Error.Data["domainCode"] != "diverged" ||
		res.Error.Data["ahead"] != 1 || res.Error.Data["behind"] != 3 || res.Ahead != 1 || res.Behind != 3 {
		t.Fatalf("pull = %+v, %v", res, err)
	}
}
