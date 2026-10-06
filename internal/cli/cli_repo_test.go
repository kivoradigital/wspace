// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/adapters/termprompt"
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

func newRepoCLIFixture(t *testing.T) *updateCLIFixture {
	t.Helper()
	fx := newUpdateCLIFixture(t)
	date := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	fx.Git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		return domain.UpstreamInfo{Ref: "origin/feature-x", Remote: "origin"}, true, nil
	}
	fx.Git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 0, 2, nil }
	fx.Git.UnpushedCountFunc = func(domain.Path, string) (int, error) { return 1, nil }
	fx.Git.HeadCommitFunc = func(domain.Path) (string, error) { return "1111111", nil }
	fx.Git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: 'M', Y: ' ', RelPath: "main.go"}, {X: '?', Y: '?', RelPath: "notes.txt"}}, nil
	}
	fx.Git.CountCommitsFunc = func(domain.Path, string) (int, error) { return 1, nil }
	fx.Git.CommitLogFunc = func(domain.Path, string, int, int) ([]domain.CommitInfo, error) {
		return []domain.CommitInfo{{Hash: "abcdef0123", ShortHash: "abcdef0", Subject: "feat: add login", AuthorName: "Ada Acme", AuthorDate: date}}, nil
	}
	fx.Git.StashListFunc = func(domain.Path) ([]domain.StashEntry, error) {
		return []domain.StashEntry{{Index: 0, Ref: "stash@{0}", Hash: "5555555555555555555555555555555555555555", Branch: "feature-x", Message: "half done", Date: date}}, nil
	}
	fx.Git.DiffFunc = func(_ domain.Path, s ports.DiffSpec) (domain.Patch, error) {
		return domain.Patch{Files: []domain.FileDiff{{Path: s.Path, Status: domain.FileModified, Additions: 1, Deletions: 1,
			Hunks: []domain.DiffHunk{{Header: "@@ -1 +1 @@", OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []string{"-old", "+new"}}}}}}, nil
	}
	return fx
}

func TestCLI_Repo_StatusShowsBranchUpstreamBaseAndChanges(t *testing.T) {
	fx := newRepoCLIFixture(t)

	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "status")

	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, stderr)
	}
	for _, want := range []string{"svc on feature-x", "origin/feature-x: 0 ahead, 2 behind", "origin/develop: 1 ahead, 3 behind", "staged", "main.go", "untracked", "notes.txt", "1 stash"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestCLI_Repo_LogAndStashJSON(t *testing.T) {
	fx := newRepoCLIFixture(t)

	stdout, _, code := run(fx.RT, "", "repo", "ws1", "svc", "log", "--json")
	var log struct {
		Range   string `json:"range"`
		Commits []struct {
			ShortHash string `json:"short_hash"`
			Author    string `json:"author"`
		} `json:"commits"`
	}
	if code != 0 || json.Unmarshal([]byte(stdout), &log) != nil || log.Range != "origin/develop..HEAD" || len(log.Commits) != 1 || log.Commits[0].Author != "Ada Acme" {
		t.Fatalf("log --json (%d) = %s", code, stdout)
	}
	if strings.Contains(stdout, "@") {
		t.Fatalf("log --json exposes an e-mail: %s", stdout)
	}

	stdout, _, code = run(fx.RT, "", "repo", "ws1", "svc", "stash")
	if code != 0 || !strings.Contains(stdout, "stash@{0}") || !strings.Contains(stdout, "half done") {
		t.Fatalf("stash (%d) = %s", code, stdout)
	}
}

func TestCLI_Repo_LogRangeUpstreamListsTheCommitsNotPushed(t *testing.T) {
	fx := newRepoCLIFixture(t)

	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "log", "--range", "upstream", "--json")
	var log struct {
		Range    string `json:"range"`
		Upstream string `json:"upstream"`
	}
	if code != 0 || json.Unmarshal([]byte(stdout), &log) != nil || log.Range != "origin/feature-x..HEAD" || log.Upstream != "origin/feature-x" {
		t.Fatalf("log --range upstream (%d) = %s stderr %q", code, stdout, stderr)
	}
	_, stderr, code = run(fx.RT, "", "repo", "ws1", "svc", "log", "--range", "remote")
	if code == 0 || !strings.Contains(stderr, "--range") {
		t.Fatalf("log --range remote: code %d stderr %q", code, stderr)
	}
}

func TestCLI_Repo_DiffPrintsAUnifiedPatch(t *testing.T) {
	fx := newRepoCLIFixture(t)

	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "diff", "main.go", "--staged")

	if code != 0 || !strings.Contains(stdout, "@@ -1 +1 @@\n-old\n+new") {
		t.Fatalf("diff (%d) = %q stderr %q", code, stdout, stderr)
	}
	_, stderr, code = run(fx.RT, "", "repo", "ws1", "svc", "diff", "../secret")
	if code == 0 || !strings.Contains(stderr, "not a change") {
		t.Fatalf("diff of an unlisted path: code %d stderr %q", code, stderr)
	}
}

func TestCLI_Repo_PullDivergedFails(t *testing.T) {
	fx := newRepoCLIFixture(t)
	fx.Git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 1, 2, nil }

	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "pull")

	if code == 0 || !strings.Contains(stdout+stderr, "diverged") {
		t.Fatalf("pull: code %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestCLI_Repo_UnknownActionIsAUsageError(t *testing.T) {
	fx := newRepoCLIFixture(t)
	if _, _, code := run(fx.RT, "", "repo", "ws1", "svc", "rebase"); code == 0 {
		t.Fatal("unknown action accepted")
	}
}

func calls(fx *updateCLIFixture, method string) int {
	n := 0
	for _, c := range fx.Git.Calls {
		if c.Method == method {
			n++
		}
	}
	return n
}

func TestCLI_Repo_StashApplyAndPop(t *testing.T) {
	fx := newRepoCLIFixture(t)
	var applied []string
	fx.Git.StashApplyFunc = func(_ domain.Path, hash string, _ bool) (ports.StashApplyResult, error) {
		applied = append(applied, hash)
		return ports.StashApplyResult{Outcome: ports.StashApplied}, nil
	}

	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "stash", "apply", "0")
	if code != 0 || !strings.Contains(stdout, "applied stash@{0}") || calls(fx, "StashDrop") != 0 {
		t.Fatalf("apply (%d) = %q stderr %q", code, stdout, stderr)
	}
	stdout, stderr, code = run(fx.RT, "", "repo", "ws1", "svc", "stash", "pop", "0")
	if code != 0 || !strings.Contains(stdout, "applied and removed stash@{0}") || calls(fx, "StashDrop") != 1 {
		t.Fatalf("pop (%d) = %q stderr %q", code, stdout, stderr)
	}
	if len(applied) != 2 || applied[0] != "5555555555555555555555555555555555555555" {
		t.Fatalf("applied %v, want the listed entry's hash", applied)
	}
}

func TestCLI_Repo_StashPopConflictKeepsTheEntryAndFails(t *testing.T) {
	fx := newRepoCLIFixture(t)
	fx.Git.StashApplyFunc = func(domain.Path, string, bool) (ports.StashApplyResult, error) {
		fx.Git.ConflictedPathsFunc = func(domain.Path) ([]string, error) { return []string{"main.go"}, nil }
		return ports.StashApplyResult{Outcome: ports.StashStopped}, nil
	}

	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "stash", "pop", "0")
	if code == 0 || !strings.Contains(stdout+stderr, "conflicts in: main.go") || !strings.Contains(stdout+stderr, "was kept") || calls(fx, "StashDrop") != 0 {
		t.Fatalf("pop conflict (%d) = %q stderr %q", code, stdout, stderr)
	}
}

func TestCLI_Repo_StashDropAsksUnlessYes(t *testing.T) {
	fx := newRepoCLIFixture(t)
	var prompt bytes.Buffer
	fx.RT.Prompter = app.PrompterDeps{Prompter: termprompt.New(strings.NewReader("n\n"), &prompt)}

	if _, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "stash", "drop", "0"); code != 0 || calls(fx, "StashDrop") != 0 {
		t.Fatalf("declined drop: code %d stderr %q, drops %d", code, stderr, calls(fx, "StashDrop"))
	}
	if !strings.Contains(prompt.String(), "half done") {
		t.Fatalf("prompt = %q, want the stash message", prompt.String())
	}
	fx.RT.Prompter = app.PrompterDeps{}
	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "stash", "drop", "0", "--yes")
	if code != 0 || !strings.Contains(stdout, "dropped stash@{0}") || calls(fx, "StashDrop") != 1 {
		t.Fatalf("drop --yes (%d) = %q stderr %q", code, stdout, stderr)
	}
}

func TestCLI_Repo_CleanDeletesOnlyUntrackedFiles(t *testing.T) {
	fx := newRepoCLIFixture(t)
	var cleaned []string
	fx.Git.CleanUntrackedFunc = func(_ domain.Path, paths []string) error { cleaned = append(cleaned, paths...); return nil }

	if _, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "clean", "main.go", "--yes"); code == 0 || !strings.Contains(stderr, "not an untracked file") || len(cleaned) != 0 {
		t.Fatalf("clean of a tracked file: code %d stderr %q cleaned %v", code, stderr, cleaned)
	}
	fx.RT.Prompter = app.PrompterDeps{Prompter: termprompt.New(strings.NewReader("n\n"), &discard{})}
	if _, _, code := run(fx.RT, "", "repo", "ws1", "svc", "clean", "notes.txt"); code != 0 || len(cleaned) != 0 {
		t.Fatalf("declined clean: code %d cleaned %v", code, cleaned)
	}
	if _, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "clean", "notes.txt", "--yes"); code != 0 || len(cleaned) != 1 || cleaned[0] != "notes.txt" {
		t.Fatalf("clean --yes: code %d stderr %q cleaned %v", code, stderr, cleaned)
	}
}

func TestCLI_Repo_StageAndUnstage(t *testing.T) {
	fx := newRepoCLIFixture(t)
	var staged, unstaged []string
	fx.Git.StageFunc = func(_ domain.Path, p []string) error { staged = append(staged, p...); return nil }
	fx.Git.UnstageFunc = func(_ domain.Path, p []string, _ bool) error { unstaged = append(unstaged, p...); return nil }

	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "stage", "notes.txt")
	if code != 0 || !strings.Contains(stdout, "svc: staged 1 path(s)") || len(staged) != 1 || staged[0] != "notes.txt" {
		t.Fatalf("stage (%d) = %q stderr %q staged %v", code, stdout, stderr, staged)
	}
	if _, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "stage", "main.go"); code == 0 || !strings.Contains(stderr, "not a change") {
		t.Fatalf("stage of a staged-only path: code %d stderr %q", code, stderr)
	}
	stdout, stderr, code = run(fx.RT, "", "repo", "ws1", "svc", "unstage", "--all")
	if code != 0 || !strings.Contains(stdout, "svc: unstaged 1 path(s)") || len(unstaged) != 1 || unstaged[0] != "main.go" {
		t.Fatalf("unstage --all (%d) = %q stderr %q unstaged %v", code, stdout, stderr, unstaged)
	}
	if _, _, code := run(fx.RT, "", "repo", "ws1", "svc", "stage"); code == 0 {
		t.Fatal("stage without paths accepted")
	}
}

func TestCLI_Repo_DiscardAsksAndKeepsABackup(t *testing.T) {
	fx := newRepoCLIFixture(t)
	fx.Git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "util.go"}, {X: '?', Y: '?', RelPath: "notes.txt"}}, nil
	}
	fx.Git.UnstagedPatchFunc = func(domain.Path, []string) (string, error) { return "diff --git a/util.go b/util.go\n", nil }
	var restored []string
	fx.Git.RestoreWorktreeFunc = func(_ domain.Path, p []string) error { restored = append(restored, p...); return nil }

	var prompt bytes.Buffer
	fx.RT.Prompter = app.PrompterDeps{Prompter: termprompt.New(strings.NewReader("n\n"), &prompt)}
	if _, _, code := run(fx.RT, "", "repo", "ws1", "svc", "discard", "util.go"); code != 0 || len(restored) != 0 || !strings.Contains(prompt.String(), "util.go (+1 -1)") {
		t.Fatalf("declined discard: code %d restored %v prompt %q", code, restored, prompt.String())
	}
	fx.RT.Prompter = app.PrompterDeps{}
	if _, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "discard", "notes.txt", "--yes"); code == 0 || !strings.Contains(stderr, "untracked") || len(restored) != 0 {
		t.Fatalf("discard of an untracked file: code %d stderr %q", code, stderr)
	}
	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "discard", "util.go", "--yes")
	if code != 0 || !strings.Contains(stdout, "discarded the unstaged changes of 1 file(s)") || !strings.Contains(stdout, "backup patch:") || len(restored) != 1 {
		t.Fatalf("discard --yes (%d) = %q stderr %q", code, stdout, stderr)
	}
}

func TestCLI_Repo_Commit(t *testing.T) {
	fx := newRepoCLIFixture(t)
	var messages []string
	fx.Git.CommitFunc = func(_ domain.Path, m string) (ports.CommitResult, error) {
		messages = append(messages, m)
		return ports.CommitResult{Outcome: ports.CommitCreated}, nil
	}
	if _, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "commit"); code == 0 || !strings.Contains(stderr, "-m <message>") {
		t.Fatalf("commit without -m: code %d stderr %q", code, stderr)
	}
	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "commit", "-m", "feat: add login")
	if code != 0 || !strings.Contains(stdout, "svc: committed abcdef0 feat: add login") || len(messages) != 1 || messages[0] != "feat: add login" {
		t.Fatalf("commit (%d) = %q stderr %q messages %q", code, stdout, stderr, messages)
	}

	fx.Git.IdentityConfiguredFunc = func(domain.Path) (bool, error) { return false, nil }
	stdout, stderr, code = run(fx.RT, "", "repo", "ws1", "svc", "commit", "-m", "x")
	if code == 0 || !strings.Contains(stdout, `git config --global user.email "you@example.com"`) || len(messages) != 1 {
		t.Fatalf("identity missing (%d) = %q stderr %q", code, stdout, stderr)
	}
}

func TestCLI_Repo_PushAndPublish(t *testing.T) {
	fx := newRepoCLIFixture(t)
	var specs []ports.PushSpec
	fx.Git.PushFunc = func(_ domain.Path, s ports.PushSpec) (ports.PushResult, error) {
		specs = append(specs, s)
		return ports.PushResult{Outcome: ports.PushDone}, nil
	}
	fx.Git.AheadBehindFunc = func(domain.Path, string) (int, int, error) { return 2, 0, nil }
	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "push")
	if code != 0 || !strings.Contains(stdout, "svc: pushed 2 commit(s) to origin/feature-x") || len(specs) != 1 || specs[0].SetUpstream {
		t.Fatalf("push (%d) = %q stderr %q specs %+v", code, stdout, stderr, specs)
	}

	fx.Git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) { return domain.UpstreamInfo{}, false, nil }
	stdout, stderr, code = run(fx.RT, "", "repo", "ws1", "svc", "push")
	if code == 0 || !strings.Contains(stdout+stderr, "--set-upstream") || len(specs) != 1 {
		t.Fatalf("push without upstream (%d) = %q stderr %q", code, stdout, stderr)
	}
	stdout, stderr, code = run(fx.RT, "", "repo", "ws1", "svc", "push", "--set-upstream")
	if code != 0 || !strings.Contains(stdout, "published feature-x to origin") || len(specs) != 2 || !specs[1].SetUpstream {
		t.Fatalf("publish (%d) = %q stderr %q specs %+v", code, stdout, stderr, specs)
	}

	fx.Git.UpstreamFunc = func(domain.Path) (domain.UpstreamInfo, bool, error) {
		return domain.UpstreamInfo{Ref: "origin/feature-x", Remote: "origin"}, true, nil
	}
	fx.Git.PushFunc = func(domain.Path, ports.PushSpec) (ports.PushResult, error) {
		return ports.PushResult{Outcome: ports.PushRejected, Output: "! [rejected]"}, nil
	}
	if stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "push"); code == 0 || !strings.Contains(stdout+stderr, "never force-pushes") {
		t.Fatalf("rejected push (%d) = %q stderr %q", code, stdout, stderr)
	}
}

func TestCLI_Repo_StashPush(t *testing.T) {
	fx := newRepoCLIFixture(t)
	ref := ""
	var specs []ports.StashPushSpec
	fx.Git.StashRefFunc = func(domain.Path) (string, error) { return ref, nil }
	fx.Git.StashPushFunc = func(_ domain.Path, s ports.StashPushSpec) error {
		specs = append(specs, s)
		ref = "5555555555555555555555555555555555555555"
		return nil
	}
	stdout, stderr, code := run(fx.RT, "", "repo", "ws1", "svc", "stash", "push", "-m", "half done", "-u", "--keep-index")
	if code != 0 || !strings.Contains(stdout, "svc: saved stash@{0} (half done)") ||
		len(specs) != 1 || specs[0] != (ports.StashPushSpec{Message: "half done", IncludeUntracked: true, KeepIndex: true}) {
		t.Fatalf("stash push (%d) = %q stderr %q specs %+v", code, stdout, stderr, specs)
	}
}
