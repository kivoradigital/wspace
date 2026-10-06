// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func init() {
	if err := messages.Use("en"); err != nil {
		panic(err)
	}
}

// fixture is one engine over fresh fakes, with a "work" context holding
// two projects (api, web) and set as active.
type fixture struct {
	eng   *engine.Engine
	store *portstest.FakeConfigStore
	git   *portstest.FakeGit
	fs    *portstest.FakeFS
	ctx   domain.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	fs := portstest.NewFakeFS(t)
	store := portstest.NewFakeConfigStore()
	git := portstest.NewFakeGit()
	home := fs.Paths().Home
	c := domain.Context{
		Name:           "work",
		WorkspacesRoot: home.Join("workspaces"),
		ProjectsRoot:   home.Join("src"),
		Projects: []domain.Project{
			{Key: "api", SourceDir: home.Join("src", "api")},
			{Key: "web", SourceDir: home.Join("src", "web")},
		},
	}
	store.PutContext(c)
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatal(err)
	}
	git.WorktreeAddFunc = func(_ domain.Path, spec ports.WorktreeSpec) error { return fs.MkdirAll(spec.Target) }
	eng := engine.New(engine.Deps{Store: store, Git: git, FS: fs, Checker: portstest.NewFakeReleaseChecker(), Version: "1.2.3"})
	return &fixture{eng: eng, store: store, git: git, fs: fs, ctx: c}
}

func codeOf(err error) engine.Code {
	var e *engine.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestAsError_MapsDomainCodesToStableWireCodes(t *testing.T) {
	tests := []struct {
		err  error
		want engine.Code
	}{
		{domain.NewOpError("x", domain.CodeContextNotFound, "a", "", nil), engine.CodeNotFound},
		{domain.NewOpError("x", domain.CodeWorkspaceNotFound, "a", "", nil), engine.CodeNotFound},
		{domain.NewOpError("x", domain.CodeWorkspaceExists, "a", "", nil), engine.CodeAlreadyExists},
		{domain.NewOpError("x", domain.CodeContextExists, "a", "", nil), engine.CodeAlreadyExists},
		{domain.NewOpError("x", domain.CodeBranchCheckedOut, "a", "", nil), engine.CodeConflict},
		{domain.NewOpError("x", domain.CodeContextActive, "a", "", nil), engine.CodeConflict},
		{domain.NewOpError("x", domain.CodeGitFailed, "a", "secret stderr", nil), engine.CodeGitFailed},
		{domain.NewOpError("x", domain.CodeNotAMainClone, "a", "", nil), engine.CodeInvalidParams},
		{fmt.Errorf("wrapped: %w", domain.ErrInvalidBranchName), engine.CodeInvalidParams},
		{errors.New("something odd"), engine.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			got := engine.AsError(tt.err)
			if got.Code != tt.want {
				t.Fatalf("AsError(%v).Code = %s, want %s", tt.err, got.Code, tt.want)
			}
		})
	}

	op := engine.AsError(domain.NewOpError("git.fetch", domain.CodeGitFailed, "/src/api", "fatal: secret stderr", nil))
	if op.Data["domainCode"] != "git_failed" || op.Data["subject"] != "/src/api" {
		t.Fatalf("Data = %v, want domainCode and subject", op.Data)
	}
	if got := fmt.Sprint(op.Message, op.Data); contains(got, "secret stderr") {
		t.Fatalf("raw subprocess detail leaked: %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestContexts_Lifecycle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	created, err := f.eng.CreateContext(ctx, engine.CreateContextParams{Name: "home", WorkspacesRoot: "/h/ws", IgnorePatterns: []string{"vendor"}})
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if created.Name != "home" || created.Active || len(created.IgnorePatterns) != 1 {
		t.Fatalf("created = %+v", created)
	}
	if _, err := f.eng.CreateContext(ctx, engine.CreateContextParams{Name: "home", WorkspacesRoot: "/h/ws"}); codeOf(err) != engine.CodeAlreadyExists {
		t.Fatalf("duplicate create error = %v, want already_exists", err)
	}
	if _, err := f.eng.CreateContext(ctx, engine.CreateContextParams{Name: "Bad Name", WorkspacesRoot: "/h"}); codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("invalid name error = %v, want invalid_params", err)
	}

	list, err := f.eng.ListContexts(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListContexts() = %+v, %v", list, err)
	}
	for _, c := range list {
		if (c.Name == "work") != c.Active {
			t.Fatalf("context %s active = %v", c.Name, c.Active)
		}
	}

	newName := "house"
	renamed, err := f.eng.UpdateContext(ctx, engine.UpdateContextParams{Name: "home", NewName: &newName})
	if err != nil || renamed.Name != "house" || renamed.WorkspacesRoot != "/h/ws" {
		t.Fatalf("UpdateContext() = %+v, %v", renamed, err)
	}

	if _, err := f.eng.SwitchContext(ctx, engine.ContextRef{Name: "house"}); err != nil {
		t.Fatalf("SwitchContext() error = %v", err)
	}
	if err := f.eng.DeleteContext(ctx, engine.DeleteContextParams{Name: "house"}); codeOf(err) != engine.CodeConflict {
		t.Fatalf("deleting the active context error = %v, want conflict", err)
	}
	if err := f.eng.DeleteContext(ctx, engine.DeleteContextParams{Name: "work"}); err != nil {
		t.Fatalf("DeleteContext(work) error = %v", err)
	}
	if _, err := f.eng.GetContext(ctx, engine.ContextRef{Name: "work"}); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("GetContext(deleted) error = %v, want not_found", err)
	}
	got, err := f.eng.GetContext(ctx, engine.ContextRef{})
	if err != nil || got.Name != "house" || !got.Active {
		t.Fatalf("GetContext(active) = %+v, %v", got, err)
	}
}

// Deleting the active (here the only) context needs the explicit
// allowActive opt-in; it then deletes it and clears the active context.
func TestDeleteContext_AllowActiveDeletesTheOnlyContext(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if err := f.eng.DeleteContext(ctx, engine.DeleteContextParams{Name: "work", AllowActive: true}); err != nil {
		t.Fatalf("DeleteContext(work, allowActive) error = %v", err)
	}
	list, err := f.eng.ListContexts(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("ListContexts() = %+v, %v, want empty", list, err)
	}
	info, err := f.eng.Info(ctx, engine.InfoParams{})
	if err == nil && info.ContextName != "" {
		t.Fatalf("Info().ContextName = %q, want no active context", info.ContextName)
	}
}

func TestProjects_RegisterUpdateRemove(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	p, err := f.eng.RegisterProject(ctx, engine.RegisterProjectParams{Key: "docs", SourceDir: "/src/docs", OriginBranch: "main"})
	if err != nil || p.Key != "docs" || p.OriginBranch == nil || *p.OriginBranch != "main" {
		t.Fatalf("RegisterProject() = %+v, %v", p, err)
	}
	if _, err := f.eng.RegisterProject(ctx, engine.RegisterProjectParams{Key: "docs", SourceDir: "/src/docs2"}); codeOf(err) != engine.CodeAlreadyExists {
		t.Fatalf("duplicate key error = %v, want already_exists", err)
	}
	if _, err := f.eng.RegisterProject(ctx, engine.RegisterProjectParams{Key: "../evil", SourceDir: "/src/x"}); codeOf(err) != engine.CodeInvalidParams {
		t.Fatalf("traversal key error = %v, want invalid_params", err)
	}

	dest := "feature/{workspace}"
	updated, err := f.eng.UpdateProject(ctx, engine.UpdateProjectParams{Key: "docs", DestBranch: &dest})
	if err != nil || updated.DestBranch != dest {
		t.Fatalf("UpdateProject() = %+v, %v", updated, err)
	}

	list, err := f.eng.ListProjects(ctx, engine.ProjectsRef{})
	if err != nil || len(list) != 3 {
		t.Fatalf("ListProjects() = %+v, %v", list, err)
	}
	if err := f.eng.RemoveProject(ctx, engine.ProjectRef{Key: "docs"}); err != nil {
		t.Fatalf("RemoveProject() error = %v", err)
	}
	if err := f.eng.RemoveProject(ctx, engine.ProjectRef{Key: "docs"}); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("second RemoveProject() error = %v, want not_found", err)
	}
}

func TestScanProjects_DefaultsToTheContextProjectsRoot(t *testing.T) {
	f := newFixture(t)
	root := f.ctx.ProjectsRoot
	for _, d := range []string{"api/.git", "fresh/.git"} {
		if err := f.fs.MkdirAll(root.Join(d)); err != nil {
			t.Fatal(err)
		}
	}
	res, err := f.eng.ScanProjects(context.Background(), engine.ScanProjectsParams{})
	if err != nil {
		t.Fatalf("ScanProjects() error = %v", err)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("candidates = %+v, want api and fresh", res.Candidates)
	}
	byKey := map[string]engine.ScanCandidate{}
	for _, c := range res.Candidates {
		byKey[c.SuggestedKey] = c
	}
	if !byKey["api"].Registered || byKey["api"].RegisteredKey != "api" || byKey["fresh"].Registered {
		t.Fatalf("registration flags wrong: %+v", res.Candidates)
	}
}

func TestCreateWorkspace_StreamsProgressAndReturnsTheWorkspace(t *testing.T) {
	f := newFixture(t)
	var events []engine.ProgressEvent
	ws, err := f.eng.CreateWorkspace(context.Background(), engine.CreateWorkspaceParams{Name: "feat", Branch: "feat-x", Projects: []string{"api"}}, func(ev engine.ProgressEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if ws.Name != "feat" || ws.Context != "work" || len(ws.Repos) != 1 || ws.Repos[0].Branch != "feat-x" || ws.Created == "" {
		t.Fatalf("workspace = %+v", ws)
	}
	var phases []string
	for _, ev := range events {
		if ev.Kind == engine.KindRepo {
			phases = append(phases, ev.Repo+":"+ev.Phase)
		}
	}
	if fmt.Sprint(phases) != "[api:started api:finished]" {
		t.Fatalf("repo progress = %v, want [api:started api:finished]", phases)
	}
}

func TestCreateWorkspace_RejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		p    engine.CreateWorkspaceParams
		want engine.Code
	}{
		{"path traversal name", engine.CreateWorkspaceParams{Name: "../escape"}, engine.CodeInvalidParams},
		{"empty name", engine.CreateWorkspaceParams{}, engine.CodeInvalidParams},
		{"bad branch", engine.CreateWorkspaceParams{Name: "ok", Branch: "has space"}, engine.CodeInvalidParams},
		{"unknown project", engine.CreateWorkspaceParams{Name: "ok", Projects: []string{"nope"}}, engine.CodeNotFound},
		{"unknown context", engine.CreateWorkspaceParams{Context: "nope", Name: "ok"}, engine.CodeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if _, err := f.eng.CreateWorkspace(context.Background(), tt.p, nil); codeOf(err) != tt.want {
				t.Fatalf("error = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestCreateWorkspace_ExistingWorkspaceIsAlreadyExists(t *testing.T) {
	f := newFixture(t)
	if _, err := f.eng.CreateWorkspace(context.Background(), engine.CreateWorkspaceParams{Name: "feat"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.CreateWorkspace(context.Background(), engine.CreateWorkspaceParams{Name: "feat"}, nil); codeOf(err) != engine.CodeAlreadyExists {
		t.Fatalf("error = %v, want already_exists", err)
	}
}

func TestDestroyWorkspace_NeedsConfirmationListsEveryReason(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.eng.CreateWorkspace(ctx, engine.CreateWorkspaceParams{Name: "feat"}, nil); err != nil {
		t.Fatal(err)
	}
	f.git.StatusFunc = func(wt domain.Path) ([]domain.PorcelainEntry, error) {
		if wt.Base() == "api" {
			return []domain.PorcelainEntry{{X: ' ', Y: 'M', RelPath: "main.go"}, {X: '?', Y: '?', RelPath: "notes.txt"}}, nil
		}
		return nil, nil
	}
	f.git.AheadBehindFunc = func(wt domain.Path, _ string) (int, int, error) {
		if wt.Base() == "web" {
			return 3, 0, nil
		}
		return 0, 0, nil
	}

	_, err := f.eng.DestroyWorkspace(ctx, engine.DestroyWorkspaceParams{Workspace: "feat"}, nil)
	var e *engine.Error
	if !errors.As(err, &e) || e.Code != engine.CodeNeedsConfirmation {
		t.Fatalf("error = %v, want needs_confirmation", err)
	}
	reasons, _ := e.Data["reasons"].([]engine.Blocker)
	want := []string{"api:tracked_change:main.go", "api:foreign_file:notes.txt", "web:unpushed_commits:3"}
	if len(reasons) != len(want) {
		t.Fatalf("reasons = %+v, want %v", reasons, want)
	}
	for i, r := range reasons {
		got := r.Repo + ":" + r.Kind + ":" + r.Path
		if r.Kind == "unpushed_commits" {
			got = fmt.Sprintf("%s:%s:%d", r.Repo, r.Kind, r.Count)
		}
		if got != want[i] || r.Message == "" {
			t.Fatalf("reasons[%d] = %+v, want %s", i, r, want[i])
		}
	}
	if exists, _ := f.fs.Exists(f.ctx.WorkspacesRoot.Join("feat")); !exists {
		t.Fatal("workspace was removed despite needs_confirmation")
	}

	res, err := f.eng.DestroyWorkspace(ctx, engine.DestroyWorkspaceParams{Workspace: "feat", Force: true}, nil)
	if err != nil || res.Path != string(f.ctx.WorkspacesRoot.Join("feat")) {
		t.Fatalf("forced destroy = %+v, %v", res, err)
	}
}

func TestRemoveRepo_NeedsConfirmationScopedToThatRepo(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.eng.CreateWorkspace(ctx, engine.CreateWorkspaceParams{Name: "feat"}, nil); err != nil {
		t.Fatal(err)
	}
	f.git.AheadBehindFunc = func(wt domain.Path, _ string) (int, int, error) {
		if wt.Base() == "api" {
			return 1, 0, nil
		}
		return 0, 0, nil
	}
	if _, err := f.eng.RemoveRepo(ctx, engine.RemoveRepoParams{Workspace: "feat", Repo: "api"}, nil); codeOf(err) != engine.CodeNeedsConfirmation {
		t.Fatalf("RemoveRepo(api) error = %v, want needs_confirmation", err)
	}
	if res, err := f.eng.RemoveRepo(ctx, engine.RemoveRepoParams{Workspace: "feat", Repo: "web"}, nil); err != nil || res.Alias != "web" {
		t.Fatalf("RemoveRepo(web) = %+v, %v", res, err)
	}
	if _, err := f.eng.RemoveRepo(ctx, engine.RemoveRepoParams{Workspace: "feat", Repo: "nope"}, nil); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("RemoveRepo(nope) error = %v, want not_found", err)
	}
}

func TestWorkspaces_ListStatusAddRepairSync(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.eng.CreateWorkspace(ctx, engine.CreateWorkspaceParams{Name: "feat", Projects: []string{"api"}}, nil); err != nil {
		t.Fatal(err)
	}
	if entry, err := f.eng.AddRepo(ctx, engine.AddRepoParams{Workspace: "feat", Project: "web"}, nil); err != nil || entry.Alias != "web" {
		t.Fatalf("AddRepo() = %+v, %v", entry, err)
	}
	if _, err := f.eng.AddRepo(ctx, engine.AddRepoParams{Workspace: "feat", Project: "web"}, nil); codeOf(err) != engine.CodeAlreadyExists {
		t.Fatalf("second AddRepo() error = %v, want already_exists", err)
	}

	list, err := f.eng.ListWorkspaces(ctx, engine.WorkspacesRef{})
	if err != nil || len(list) != 1 || list[0].RepoCount != 2 {
		t.Fatalf("ListWorkspaces() = %+v, %v", list, err)
	}
	st, err := f.eng.WorkspaceStatus(ctx, engine.WorkspaceRef{Workspace: "feat"})
	if err != nil || len(st.Repos) != 2 {
		t.Fatalf("WorkspaceStatus() = %+v, %v", st, err)
	}
	if _, err := f.eng.WorkspaceStatus(ctx, engine.WorkspaceRef{Workspace: "nope"}); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("status of missing workspace error = %v, want not_found", err)
	}

	_ = f.fs.RemoveAll(f.ctx.WorkspacesRoot.Join("feat", "web"))
	rep, err := f.eng.Repair(ctx, engine.WorkspaceRef{Workspace: "feat"}, nil)
	if err != nil || fmt.Sprint(rep.Recreated) != "[web]" {
		t.Fatalf("Repair() = %+v, %v", rep, err)
	}
	if _, err := f.eng.SyncEnv(ctx, engine.WorkspaceRef{Workspace: "feat"}, nil); err != nil {
		t.Fatalf("SyncEnv() error = %v", err)
	}
}

func TestSystem_VersionInfoDoctorUpdate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if v := f.eng.Version(); v.Version != "1.2.3" {
		t.Fatalf("Version() = %+v", v)
	}
	info, err := f.eng.Info(ctx, engine.InfoParams{})
	if err != nil || info.ContextName != "work" || info.Options["baseBranch"].From != "builtin" {
		t.Fatalf("Info() = %+v, %v", info, err)
	}
	if _, err := f.eng.Doctor(ctx, engine.InfoParams{}, nil); err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	up, err := f.eng.CheckForUpdate(ctx)
	if err != nil || !up.Unavailable || up.CurrentVersion != "1.2.3" {
		t.Fatalf("CheckForUpdate() = %+v, %v (unbranded build must report unavailable)", up, err)
	}
}

const legacySource = `workspaces_root = /abs/workspaces
projects_root = /abs/projects
base_branch = develop
[projects]
alpha
`

func TestImportLegacyContext_NonInteractive(t *testing.T) {
	t.Run("new context from an explicit file", func(t *testing.T) {
		f := newFixture(t)
		_ = f.fs.MkdirAll("/abs/projects/alpha")
		_ = f.fs.WriteFile("/legacy/config", []byte(legacySource), 0o644)
		res, err := f.eng.ImportLegacyContext(context.Background(), engine.ImportLegacyParams{From: "/legacy/config", Name: "legacy"})
		if err != nil {
			t.Fatalf("ImportLegacyContext() error = %v", err)
		}
		if !res.Created || !res.Applied || res.Context != "legacy" || fmt.Sprint(res.ImportedProjects) != "[alpha]" {
			t.Fatalf("result = %+v", res)
		}
		c, err := f.eng.GetContext(context.Background(), engine.ContextRef{Name: "legacy"})
		if err != nil || c.WorkspacesRoot != "/abs/workspaces" || c.Defaults.BaseBranch == nil || *c.Defaults.BaseBranch != "develop" {
			t.Fatalf("imported context = %+v, %v", c, err)
		}
	})
	t.Run("merge without confirm is a dry run", func(t *testing.T) {
		f := newFixture(t)
		_ = f.fs.WriteFile("/legacy/config", []byte(legacySource), 0o644)
		res, err := f.eng.ImportLegacyContext(context.Background(), engine.ImportLegacyParams{From: "/legacy/config", Into: "work"})
		if err != nil || res.Applied || len(res.FieldChanges) == 0 {
			t.Fatalf("dry run = %+v, %v; want Applied=false with field changes", res, err)
		}
		c, _ := f.eng.GetContext(context.Background(), engine.ContextRef{Name: "work"})
		if c.WorkspacesRoot == "/abs/workspaces" {
			t.Fatal("dry run wrote the merge")
		}
	})
	t.Run("name and into are mutually exclusive", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.eng.ImportLegacyContext(context.Background(), engine.ImportLegacyParams{From: "/x", Name: "a", Into: "work"}); codeOf(err) != engine.CodeInvalidParams {
			t.Fatalf("error = %v, want invalid_params", err)
		}
	})
	t.Run("existing name is already_exists", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.eng.ImportLegacyContext(context.Background(), engine.ImportLegacyParams{From: "/x", Name: "work"}); codeOf(err) != engine.CodeAlreadyExists {
			t.Fatalf("error = %v, want already_exists", err)
		}
	})
	t.Run("missing workspaces root needs input", func(t *testing.T) {
		f := newFixture(t)
		_ = f.fs.WriteFile("/legacy/config", []byte("[projects]\n"), 0o644)
		_, err := f.eng.ImportLegacyContext(context.Background(), engine.ImportLegacyParams{From: "/legacy/config", Name: "legacy"})
		var e *engine.Error
		if !errors.As(err, &e) || e.Code != engine.CodeInvalidParams || e.Data["field"] == nil {
			t.Fatalf("error = %v, want invalid_params naming the missing field", err)
		}
	})
}

// TestRenameAndClaim_KeepWorkspacesVisible: renaming a context moves its
// workspaces to the new name (reported in the result); a workspace whose
// owner no longer exists is listed flagged orphanOf and can be claimed.
func TestRenameAndClaim_KeepWorkspacesVisible(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	seed := func(name string, owner domain.ContextName) {
		root := f.ctx.WorkspacesRoot.Join(name)
		f.store.PutManifest(root, domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{Name: name, Root: root, Context: owner}})
		if err := f.fs.MkdirAll(root); err != nil {
			t.Fatal(err)
		}
	}
	seed("mine", "work")
	seed("lost", "legacy")

	newName := "job"
	renamed, err := f.eng.UpdateContext(ctx, engine.UpdateContextParams{Name: "work", NewName: &newName})
	if err != nil {
		t.Fatalf("UpdateContext: %v", err)
	}
	if len(renamed.ReassignedWorkspaces) != 1 || renamed.ReassignedWorkspaces[0] != "mine" || len(renamed.ReassignFailures) != 0 {
		t.Fatalf("renamed = %+v, want mine reassigned", renamed)
	}

	byName := func() map[string]engine.WorkspaceStatus {
		list, err := f.eng.ListWorkspaces(ctx, engine.WorkspacesRef{Context: "job"})
		if err != nil {
			t.Fatalf("ListWorkspaces: %v", err)
		}
		out := map[string]engine.WorkspaceStatus{}
		for _, w := range list {
			out[w.Name] = w
		}
		return out
	}
	list := byName()
	if list["mine"].OrphanOf != "" || list["lost"].OrphanOf != "legacy" {
		t.Fatalf("list = %+v, want mine owned and lost orphanOf legacy", list)
	}

	res, err := f.eng.ClaimWorkspaces(ctx, engine.ClaimWorkspacesParams{Context: "job", Workspaces: []string{"lost", "mine"}})
	if err != nil {
		t.Fatalf("ClaimWorkspaces: %v", err)
	}
	if len(res.Claimed) != 1 || res.Claimed[0].Name != "lost" || res.Claimed[0].PreviousContext != "legacy" {
		t.Fatalf("Claimed = %+v", res.Claimed)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Name != "mine" || res.Skipped[0].Reason != "claim.skip.already_owned" || res.Skipped[0].Message == "" {
		t.Fatalf("Skipped = %+v", res.Skipped)
	}
	if byName()["lost"].OrphanOf != "" {
		t.Fatalf("lost still orphaned after claim")
	}
	if _, err := f.eng.ClaimWorkspaces(ctx, engine.ClaimWorkspacesParams{Context: "ghost"}); codeOf(err) != engine.CodeNotFound {
		t.Fatalf("claim for a missing context error = %v, want not_found", err)
	}
	again, err := f.eng.ClaimWorkspaces(ctx, engine.ClaimWorkspacesParams{})
	if err != nil || again.Claimed == nil || again.Skipped == nil || len(again.Claimed) != 0 {
		t.Fatalf("idempotent claim = %+v, %v; want empty arrays", again, err)
	}
}
