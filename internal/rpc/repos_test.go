// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package rpc_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TestRepos_Golden pins the repos.* wire shapes (repository inspector).
func TestRepos_Golden(t *testing.T) {
	f := newFixture(t)
	f.git.ResolveBaseFunc = func(_ domain.Path, remote string, b domain.BranchName) (ports.BaseRef, error) {
		return ports.BaseRef{Ref: remote + "/" + string(b), Remote: true}, nil
	}
	f.git.RemoteDefaultBranchFunc = func(domain.Path, string) (domain.BranchName, bool, error) { return "develop", true, nil }
	f.git.HeadCommitFunc = func(domain.Path) (string, error) { return "1111111", nil }
	f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		return domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin"}, true, nil
	}
	ahead := 0
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return ahead, 2, nil }
	f.git.UnpushedCountFunc = func(domain.Path, string) (int, error) { return 3, nil }
	f.git.BehindCountFunc = func(domain.Path, string) (int, error) { return 1, nil }
	f.git.LastFetchFunc = func(domain.Path) (time.Time, bool, error) {
		return time.Date(2026, 10, 3, 9, 30, 0, 0, time.UTC), true, nil
	}
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: 'M', Y: 'M', RelPath: "main.go"}, {X: '?', Y: '?', RelPath: "notes.txt"}}, nil
	}
	f.git.DiffFunc = func(_ domain.Path, s ports.DiffSpec) (domain.Patch, error) {
		return domain.Patch{Files: []domain.FileDiff{{Path: s.Path, Status: domain.FileModified, Additions: 1, Deletions: 1,
			Hunks: []domain.DiffHunk{{Header: "@@ -1 +1 @@", OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []string{"-old", "+new"}}}}}}, nil
	}
	date := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	f.git.CountCommitsFunc = func(domain.Path, string) (int, error) { return 1, nil }
	f.git.CommitLogFunc = func(domain.Path, string, int, int) ([]domain.CommitInfo, error) {
		return []domain.CommitInfo{{Hash: "abcdef0123456789abcdef0123456789abcdef01", ShortHash: "abcdef0", Subject: "feat: add login", AuthorName: "Ada Acme", AuthorDate: date}}, nil
	}
	f.git.CommitDetailFunc = func(_ domain.Path, hash string, _ domain.DiffLimits) (domain.CommitDetail, error) {
		return domain.CommitDetail{CommitInfo: domain.CommitInfo{Hash: "abcdef0123456789abcdef0123456789abcdef01", ShortHash: "abcdef0", Subject: "feat: add login", AuthorName: "Ada Acme", AuthorDate: date},
			Body: "Adds the login form.", Parents: []string{"9999999999999999999999999999999999999999"},
			Patch: domain.Patch{Files: []domain.FileDiff{{Path: "login.go", Status: domain.FileAdded, Additions: 1, Hunks: []domain.DiffHunk{{Header: "@@ -0,0 +1 @@", NewStart: 1, NewLines: 1, Lines: []string{"+package login"}}}}}}}, nil
	}
	f.git.StashListFunc = func(domain.Path) ([]domain.StashEntry, error) {
		return []domain.StashEntry{{Index: 0, Ref: "stash@{0}", Hash: "5555555555555555555555555555555555555555", Branch: "feat", Message: "half done", Date: date}}, nil
	}
	f.git.StashShowFunc = func(domain.Path, int, bool, domain.DiffLimits) (domain.Patch, error) {
		return domain.Patch{Files: []domain.FileDiff{{Path: "scratch.txt", Status: domain.FileAdded, Binary: true, Hunks: []domain.DiffHunk{}}}}, nil
	}

	lines := f.serve(t, `{"id":"1","method":"repos.inspect","params":{"workspace":"feat","repo":"api"}}`+"\n"+
		`{"id":"2","method":"repos.changes","params":{"workspace":"feat","repo":"api"}}`+"\n"+
		`{"id":"3","method":"repos.diff","params":{"workspace":"feat","repo":"api","path":"main.go","staged":true}}`+"\n"+
		`{"id":"4","method":"repos.commits","params":{"workspace":"feat","repo":"api","limit":20}}`+"\n"+
		`{"id":"5","method":"repos.commit","params":{"workspace":"feat","repo":"api","hash":"abcdef0"}}`+"\n"+
		`{"id":"6","method":"repos.stashes","params":{"workspace":"feat","repo":"api"}}`+"\n"+
		`{"id":"7","method":"repos.stash","params":{"workspace":"feat","repo":"api","index":0}}`+"\n"+
		`{"id":"8","method":"repos.fetch","params":{"workspace":"feat","repo":"api"}}`+"\n"+
		`{"id":"9","method":"repos.pull","params":{"workspace":"feat","repo":"api"}}`+"\n")
	ahead = 1
	lines = append(lines, f.serve(t, `{"id":"10","method":"repos.pull","params":{"workspace":"feat","repo":"api"}}`+"\n"+
		`{"id":"11","method":"repos.diff","params":{"workspace":"feat","repo":"api","path":"../secret"}}`+"\n")...)
	compareGolden(t, "repos.golden", lines)
}

// TestRepoActions_Golden pins the stash apply/pop/drop, untracked
// validate/discard and addable-projects wire shapes.
func TestRepoActions_Golden(t *testing.T) {
	f := newFixture(t)
	date := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	const h0, h1 = "5555555555555555555555555555555555555555", "6666666666666666666666666666666666666666"
	f.git.StashListFunc = func(domain.Path) ([]domain.StashEntry, error) {
		return []domain.StashEntry{
			{Index: 0, Ref: "stash@{0}", Hash: h0, Branch: "feat", Message: "half done", Date: date},
			{Index: 1, Ref: "stash@{1}", Hash: h1, Branch: "feat", Message: "older", Date: date},
		}, nil
	}
	f.git.StashShowFunc = func(domain.Path, int, bool, domain.DiffLimits) (domain.Patch, error) {
		return domain.Patch{Files: []domain.FileDiff{{Path: "a.go", Status: domain.FileModified}, {Path: "b.go", Status: domain.FileAdded}}}, nil
	}
	outcome := ports.StashApplyResult{Outcome: ports.StashApplied}
	conflicts := []string{}
	f.git.StashApplyFunc = func(_ domain.Path, _ string, restoreIndex bool) (ports.StashApplyResult, error) {
		if outcome.Outcome == ports.StashIndexRefused && !restoreIndex {
			return ports.StashApplyResult{Outcome: ports.StashApplied}, nil
		}
		if outcome.Outcome == ports.StashStopped {
			conflicts = []string{"main.go"}
		}
		return outcome, nil
	}
	f.git.ConflictedPathsFunc = func(domain.Path) ([]string, error) { return conflicts, nil }
	status := []domain.PorcelainEntry{{X: 'M', Y: ' ', RelPath: "main.go"}, {X: '?', Y: '?', RelPath: "notes.txt"}}
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) { return status, nil }
	f.git.CleanUntrackedFunc = func(domain.Path, []string) error { status = status[:1]; return nil }

	lines := f.serve(t, `{"id":"1","method":"repos.stashApply","params":{"workspace":"feat","repo":"api","index":1,"hash":"6666666"}}`+"\n"+
		`{"id":"2","method":"repos.stashPop","params":{"workspace":"feat","repo":"api","index":0,"hash":"`+h0+`"}}`+"\n"+
		`{"id":"3","method":"repos.stashApply","params":{"workspace":"feat","repo":"api","index":0,"hash":"`+h1+`"}}`+"\n"+
		`{"id":"4","method":"repos.stashDrop","params":{"workspace":"feat","repo":"api","index":1,"hash":"`+h1+`"}}`+"\n"+
		`{"id":"5","method":"repos.stashDrop","params":{"workspace":"feat","repo":"api","index":1,"hash":"`+h1+`","confirm":true}}`+"\n"+
		`{"id":"6","method":"repos.stashApply","params":{"workspace":"feat","repo":"api","index":1}}`+"\n"+
		`{"id":"7","method":"repos.validateUntracked","params":{"workspace":"feat","repo":"api","paths":["notes.txt"]}}`+"\n"+
		`{"id":"8","method":"repos.validateUntracked","params":{"workspace":"feat","repo":"api","paths":["main.go"]}}`+"\n"+
		`{"id":"9","method":"repos.discardUntracked","params":{"workspace":"feat","repo":"api","paths":["notes.txt"]}}`+"\n"+
		`{"id":"10","method":"repos.discardUntracked","params":{"workspace":"feat","repo":"api","paths":["notes.txt"],"confirm":true}}`+"\n"+
		`{"id":"11","method":"workspaces.addableProjects","params":{"workspace":"feat"}}`+"\n")
	outcome = ports.StashApplyResult{Outcome: ports.StashStopped}
	lines = append(lines, f.serve(t, `{"id":"12","method":"repos.stashPop","params":{"workspace":"feat","repo":"api","index":1,"hash":"`+h1+`"}}`+"\n")...)
	outcome, conflicts = ports.StashApplyResult{Outcome: ports.StashIndexRefused}, []string{}
	lines = append(lines, f.serve(t, `{"id":"13","method":"repos.stashPop","params":{"workspace":"feat","repo":"api","index":1,"hash":"`+h1+`"}}`+"\n")...)
	outcome = ports.StashApplyResult{Outcome: ports.StashOverwriteRefused, Files: []string{"main.go"}}
	lines = append(lines, f.serve(t, `{"id":"14","method":"repos.stashApply","params":{"workspace":"feat","repo":"api","index":1,"hash":"`+h1+`"}}`+"\n")...)
	compareGolden(t, "repo_actions.golden", lines)
}

var backupPattern = regexp.MustCompile(`"backup":"[^"]*/discarded/api-\d{8}T\d{6}\.\d{3}Z\.patch"`)

// TestRepoWrite_Golden pins the stage/unstage, discard, commit, push and
// stash-create wire shapes (repos.commitChanges, not repos.commit, which
// reads one commit).
func TestRepoWrite_Golden(t *testing.T) {
	f := newFixture(t)
	date := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	status := []domain.PorcelainEntry{{X: 'M', Y: 'M', RelPath: "main.go"}, {X: ' ', Y: 'M', RelPath: "util.go"}, {X: '?', Y: '?', RelPath: "notes.txt"}}
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) { return status, nil }
	f.git.DiffFunc = func(_ domain.Path, s ports.DiffSpec) (domain.Patch, error) {
		return domain.Patch{Files: []domain.FileDiff{{Path: s.Path, Status: domain.FileModified, Additions: 3, Deletions: 1}}}, nil
	}
	f.git.UnstagedPatchFunc = func(domain.Path, []string) (string, error) { return "diff --git a/util.go b/util.go\n", nil }
	f.git.CommitLogFunc = func(domain.Path, string, int, int) ([]domain.CommitInfo, error) {
		return []domain.CommitInfo{{Hash: "abcdef0123456789abcdef0123456789abcdef01", ShortHash: "abcdef0", Subject: "feat: login", AuthorName: "Ada Acme", AuthorDate: date}}, nil
	}
	upstream := false
	f.git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		if !upstream {
			return domain.UpstreamInfo{}, false, nil
		}
		return domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin", RemoteRef: "refs/heads/feat"}, true, nil
	}
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 1, 0, nil }
	pushed := ports.PushResult{Outcome: ports.PushDone}
	f.git.PushFunc = func(domain.Path, ports.PushSpec) (ports.PushResult, error) { upstream = true; return pushed, nil }
	identity := true
	f.git.IdentityConfiguredFunc = func(domain.Path) (bool, error) { return identity, nil }
	stashRef := ""
	f.git.StashRefFunc = func(domain.Path) (string, error) { return stashRef, nil }
	f.git.StashPushFunc = func(domain.Path, ports.StashPushSpec) error {
		stashRef = "5555555555555555555555555555555555555555"
		return nil
	}
	f.git.StashListFunc = func(domain.Path) ([]domain.StashEntry, error) {
		return []domain.StashEntry{{Index: 0, Ref: "stash@{0}", Hash: "5555555555555555555555555555555555555555", Branch: "feat", Message: "wip", Date: date}}, nil
	}

	ws := `"workspace":"feat","repo":"api"`
	lines := f.serve(t, `{"id":"1","method":"repos.stage","params":{`+ws+`,"paths":["util.go","notes.txt"]}}`+"\n"+
		`{"id":"2","method":"repos.stage","params":{`+ws+`,"paths":["missing.go"]}}`+"\n"+
		`{"id":"3","method":"repos.unstage","params":{`+ws+`,"all":true}}`+"\n"+
		`{"id":"4","method":"repos.discard","params":{`+ws+`,"paths":["util.go"]}}`+"\n"+
		`{"id":"5","method":"repos.discard","params":{`+ws+`,"paths":["notes.txt"],"confirm":true}}`+"\n"+
		`{"id":"6","method":"repos.discard","params":{`+ws+`,"paths":["util.go"],"confirm":true}}`+"\n"+
		`{"id":"7","method":"repos.commitChanges","params":{`+ws+`,"message":"feat: login"}}`+"\n"+
		`{"id":"8","method":"repos.commitChanges","params":{`+ws+`,"message":" "}}`+"\n"+
		`{"id":"9","method":"repos.commitChanges","params":{`+ws+`,"message":"x","amend":true}}`+"\n"+
		`{"id":"10","method":"repos.push","params":{`+ws+`}}`+"\n"+
		`{"id":"11","method":"repos.push","params":{`+ws+`,"setUpstream":true}}`+"\n"+
		`{"id":"12","method":"repos.stashCreate","params":{`+ws+`,"message":"wip","includeUntracked":true}}`+"\n")
	identity = false
	pushed = ports.PushResult{Outcome: ports.PushRejected, Output: " ! [rejected]        feat -> feat (fetch first)"}
	lines = append(lines, f.serve(t, `{"id":"13","method":"repos.commitChanges","params":{`+ws+`,"message":"feat: login"}}`+"\n"+
		`{"id":"14","method":"repos.push","params":{`+ws+`}}`+"\n"+
		`{"id":"15","method":"repos.stashCreate","params":{`+ws+`}}`+"\n")...)
	for i, l := range lines {
		lines[i] = backupPattern.ReplaceAllString(l, `"backup":"<CACHE>/discarded/api-<STAMP>.patch"`)
	}
	compareGolden(t, "repo_write.golden", lines)
}
