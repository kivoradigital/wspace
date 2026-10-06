// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/ports"
)

func TestAsError_MapsUpdateCodes(t *testing.T) {
	tests := map[domain.ErrCode]engine.Code{
		domain.CodeDetachedHead:          engine.CodeConflict,
		domain.CodeIntegrationInProgress: engine.CodeConflict,
		domain.CodeUpdateConflict:        engine.CodeConflict,
		domain.CodeBaseMissing:           engine.CodeNotFound,
	}
	for code, want := range tests {
		if got := engine.AsError(domain.NewOpError("x", code, "api", "", nil)); got.Code != want || got.Data["domainCode"] != string(code) {
			t.Errorf("AsError(%s) = %+v, want %s", code, got, want)
		}
	}
}

// updateFixture is a workspace "feat" mounting api and web, with every
// repo 2 commits behind origin/develop.
func newUpdateEngineFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.eng.CreateWorkspace(ctx, engine.CreateWorkspaceParams{Name: "feat", Projects: []string{"api", "web"}, Options: &engine.Options{BaseBranch: strPtr("develop")}}, nil); err != nil {
		t.Fatal(err)
	}
	f.git.Calls = nil
	f.git.CurrentBranchFunc = func(domain.Path) (domain.BranchName, bool, error) { return "feat", false, nil }
	head := map[domain.Path]string{}
	f.git.HeadCommitFunc = func(wt domain.Path) (string, error) {
		if h, ok := head[wt]; ok {
			return h, nil
		}
		return "aaa", nil
	}
	f.git.BehindCountFunc = func(wt domain.Path, _ string) (int, error) {
		if head[wt] == "bbb" {
			return 0, nil
		}
		return 2, nil
	}
	f.git.IntegrateFunc = func(wt domain.Path, _ ports.IntegrateSpec) error { head[wt] = "bbb"; return nil }
	return f
}

func strPtr(s string) *string { return &s }

func TestUpdateRepo_ReturnsTheResultAndStreamsProgress(t *testing.T) {
	f := newUpdateEngineFixture(t)
	var events []engine.ProgressEvent

	res, err := f.eng.UpdateRepo(context.Background(), engine.UpdateRepoParams{Workspace: "feat", Repo: "api", Strategy: "rebase"}, func(ev engine.ProgressEvent) { events = append(events, ev) })

	if err != nil {
		t.Fatalf("UpdateRepo() = %v", err)
	}
	if res.Repo != "api" || res.Strategy != "rebase" || res.Base != "origin/develop" || res.BeforeHead != "aaa" ||
		res.AfterHead != "bbb" || res.UpToDate || res.CommitsIntegrated != 2 || res.Conflicts == nil || res.Error != nil {
		t.Fatalf("result = %+v", res)
	}
	if len(events) != 2 || events[0].Op != "workspace.update" || events[0].Phase != "started" || events[1].Phase != "finished" {
		t.Fatalf("events = %+v", events)
	}
	b, _ := json.Marshal(res)
	for _, field := range []string{`"repo":"api"`, `"strategy":"rebase"`, `"base":"origin/develop"`, `"beforeHead":"aaa"`, `"afterHead":"bbb"`, `"upToDate":false`, `"commitsIntegrated":2`, `"conflicts":[]`} {
		if !strings.Contains(string(b), field) {
			t.Fatalf("wire result %s lacks %s", b, field)
		}
	}
	if strings.Contains(string(b), `"error"`) {
		t.Fatalf("wire result %s has an error on success", b)
	}

	again, err := f.eng.UpdateRepo(context.Background(), engine.UpdateRepoParams{Workspace: "feat", Repo: "api"}, nil)
	if err != nil || !again.UpToDate || again.Strategy != "merge" {
		t.Fatalf("second UpdateRepo() = %+v, %v; want up to date with the default strategy", again, err)
	}
}

func TestUpdateRepo_DirtyRefusalListsTheFiles(t *testing.T) {
	f := newUpdateEngineFixture(t)
	f.git.StatusFunc = func(domain.Path) ([]domain.PorcelainEntry, error) {
		return []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "main.go"}}, nil
	}

	res, err := f.eng.UpdateRepo(context.Background(), engine.UpdateRepoParams{Workspace: "feat", Repo: "api"}, nil)

	if err != nil || res.Error == nil || res.Error.Code != engine.CodeConflict || res.Error.Data["domainCode"] != "worktree_dirty" {
		t.Fatalf("result = %+v, %v; want a worktree_dirty refusal", res, err)
	}
	if files, _ := res.Error.Data["files"].([]string); len(files) != 1 || files[0] != "main.go" {
		t.Fatalf("error data = %+v, want files [main.go]", res.Error.Data)
	}
	if res.AfterHead != res.BeforeHead {
		t.Fatalf("result = %+v, want HEAD unchanged", res)
	}
}

func TestUpdateRepo_ConflictCarriesThePathsAndTheRestore(t *testing.T) {
	f := newUpdateEngineFixture(t)
	stopped := domain.UpdateStrategy("")
	f.git.IntegrateFunc = func(domain.Path, ports.IntegrateSpec) error {
		stopped = domain.UpdateMerge
		return domain.NewOpError("git.integrate", domain.CodeGitFailed, "", "", nil)
	}
	f.git.IntegrationInProgressFunc = func(domain.Path) (domain.UpdateStrategy, error) { return stopped, nil }
	f.git.ConflictedPathsFunc = func(domain.Path) ([]string, error) { return []string{"a.go"}, nil }
	f.git.AbortIntegrationFunc = func(domain.Path, domain.UpdateStrategy) error { stopped = ""; return nil }

	res, err := f.eng.UpdateRepo(context.Background(), engine.UpdateRepoParams{Workspace: "feat", Repo: "api"}, nil)

	if err != nil || res.Error == nil || res.Error.Data["domainCode"] != "update_conflict" {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0] != "a.go" || res.Error.Data["restored"] != true {
		t.Fatalf("result = %+v, data = %+v", res, res.Error.Data)
	}
}

func TestUpdateRepo_RejectsBadInput(t *testing.T) {
	f := newUpdateEngineFixture(t)
	ctx := context.Background()
	if _, err := f.eng.UpdateRepo(ctx, engine.UpdateRepoParams{Workspace: "feat", Repo: "api", Strategy: "squash"}, nil); codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("bad strategy error = %v, want invalid_params", err)
	}
	if _, err := f.eng.UpdateRepo(ctx, engine.UpdateRepoParams{Workspace: "feat", Repo: "ghost"}, nil); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("unknown repo error = %v, want not_found", err)
	}
	if _, err := f.eng.UpdateWorkspace(ctx, engine.UpdateWorkspaceParams{Workspace: "../x"}, nil); codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("bad workspace error = %v, want invalid_params", err)
	}
	if _, err := f.eng.UpdateWorkspace(ctx, engine.UpdateWorkspaceParams{Workspace: "ghost"}, nil); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("missing workspace error = %v, want not_found", err)
	}
}

func TestUpdateWorkspace_ReportsEveryRepo(t *testing.T) {
	f := newUpdateEngineFixture(t)
	f.git.CurrentBranchFunc = func(wt domain.Path) (domain.BranchName, bool, error) {
		return "", strings.HasSuffix(string(wt), "/web"), nil
	}

	res, err := f.eng.UpdateWorkspace(context.Background(), engine.UpdateWorkspaceParams{Workspace: "feat", Autostash: true}, nil)

	if err != nil || len(res.Repos) != 2 {
		t.Fatalf("UpdateWorkspace() = %+v, %v", res, err)
	}
	if res.Repos[0].Repo != "api" || res.Repos[0].Error != nil || res.Repos[0].CommitsIntegrated != 2 {
		t.Fatalf("api = %+v", res.Repos[0])
	}
	if res.Repos[1].Repo != "web" || res.Repos[1].Error == nil || res.Repos[1].Error.Data["domainCode"] != "detached_head" {
		t.Fatalf("web = %+v, want a detached_head refusal", res.Repos[1])
	}
}

func TestWorkspaceStatus_ReportsBaseBehind(t *testing.T) {
	f := newUpdateEngineFixture(t)
	f.git.AheadBehindFunc = func(domain.Path, string) (int, int, error) {
		return 0, 0, domain.NewOpError("git.ahead_behind", domain.CodeGitFailed, "", "no upstream", nil)
	}

	st, err := f.eng.WorkspaceStatus(context.Background(), engine.WorkspaceRef{Workspace: "feat"})

	if err != nil || st.Repos[0].BaseBehind == nil || *st.Repos[0].BaseBehind != 2 {
		t.Fatalf("status = %+v, %v; want baseBehind 2", st, err)
	}
}
