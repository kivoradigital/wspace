// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

const ownRoot = domain.Path("/own/workspaces")

// ownershipFixture is a WorkspacesRoot shared by several contexts, holding
// workspaces whose manifests name different owners.
type ownershipFixture struct {
	fs    *portstest.FakeFS
	store *portstest.FakeConfigStore
	git   *portstest.FakeGit
}

func newOwnershipFixture(t *testing.T, contexts ...domain.ContextName) *ownershipFixture {
	t.Helper()
	f := &ownershipFixture{fs: portstest.NewFakeFS(t), store: portstest.NewFakeConfigStore(), git: portstest.NewFakeGit()}
	if err := f.fs.MkdirAll(ownRoot); err != nil {
		t.Fatal(err)
	}
	for _, c := range contexts {
		f.store.PutContext(domain.Context{Name: c, WorkspacesRoot: ownRoot})
	}
	return f
}

func (f *ownershipFixture) seed(t *testing.T, name string, owner domain.ContextName) domain.Path {
	t.Helper()
	root := ownRoot.Join(name)
	f.store.PutManifest(root, domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{
		Name: name, Root: root, Context: owner, Branch: "feat/" + domain.BranchName(name),
		Repos: []domain.RepoEntry{{Alias: "api", Project: "api", SourceDir: "/src/api", Branch: "feat/" + domain.BranchName(name)}},
	}})
	if err := f.fs.MkdirAll(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func (f *ownershipFixture) deps() app.Deps {
	return app.Deps{Store: f.store, Git: f.git, FS: f.fs, Reporter: portstest.NewRecordingReporter()}
}

func (f *ownershipFixture) owner(t *testing.T, name string) domain.ContextName {
	t.Helper()
	m, err := f.store.LoadManifest(context.Background(), ownRoot.Join(name))
	if err != nil {
		t.Fatalf("LoadManifest(%s): %v", name, err)
	}
	return m.Workspace.Context
}

func (f *ownershipFixture) list(t *testing.T, ctxName domain.ContextName) map[string]app.WorkspaceStatus {
	t.Helper()
	list, err := app.List(context.Background(), f.deps(), app.ListInput{WorkspacesRoot: ownRoot, Context: ctxName})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	out := map[string]app.WorkspaceStatus{}
	for _, ws := range list {
		out[ws.Name] = ws
	}
	return out
}

func TestUpdateContext_RenameReassignsTheOldNamesWorkspaces(t *testing.T) {
	f := newOwnershipFixture(t, "legacy", "acme")
	f.seed(t, "findings", "legacy")
	f.seed(t, "memoryleak", "legacy")
	f.seed(t, "theirs", "acme")
	f.seed(t, "shared", "")

	renamed, re, err := app.UpdateContext(context.Background(), f.store, f.fs, "legacy", domain.Context{Name: "globex", WorkspacesRoot: ownRoot})
	if err != nil {
		t.Fatalf("UpdateContext: %v", err)
	}
	if renamed.Name != "globex" {
		t.Fatalf("renamed = %+v", renamed)
	}
	if len(re.Reassigned) != 2 || len(re.Failures) != 0 {
		t.Fatalf("reassignment = %+v, want findings and memoryleak", re)
	}
	for _, name := range []string{"findings", "memoryleak"} {
		if got := f.owner(t, name); got != "globex" {
			t.Errorf("%s owner = %q, want globex", name, got)
		}
	}
	if got := f.owner(t, "theirs"); got != "acme" {
		t.Errorf("theirs owner = %q, want acme untouched", got)
	}
	if got := f.owner(t, "shared"); got != "" {
		t.Errorf("shared owner = %q, want empty untouched", got)
	}
	m, _ := f.store.LoadManifest(context.Background(), ownRoot.Join("findings"))
	if m.Workspace.Branch != "feat/findings" || len(m.Workspace.Repos) != 1 || m.Workspace.Repos[0].SourceDir != "/src/api" {
		t.Errorf("other manifest fields changed: %+v", m.Workspace)
	}
	listed := f.list(t, "globex")
	if _, ok := listed["findings"]; !ok {
		t.Errorf("findings not listed under globex: %+v", listed)
	}
	if listed["findings"].OrphanOf != "" {
		t.Errorf("findings still flagged orphan: %+v", listed["findings"])
	}
}

func TestUpdateContext_RenameReportsAManifestWriteFailureWithoutRollingBack(t *testing.T) {
	f := newOwnershipFixture(t, "legacy")
	broken := f.seed(t, "findings", "legacy")
	f.seed(t, "memoryleak", "legacy")
	f.store.SaveManifestErrFor = map[domain.Path]error{broken: errors.New("read-only")}

	_, re, err := app.UpdateContext(context.Background(), f.store, f.fs, "legacy", domain.Context{Name: "globex", WorkspacesRoot: ownRoot})
	if err != nil {
		t.Fatalf("UpdateContext: %v (a manifest failure must never fail the rename)", err)
	}
	if _, err := f.store.LoadContext(context.Background(), "globex"); err != nil {
		t.Fatalf("rename rolled back: %v", err)
	}
	if len(re.Reassigned) != 1 || re.Reassigned[0].Name != "memoryleak" {
		t.Errorf("Reassigned = %+v, want memoryleak", re.Reassigned)
	}
	if len(re.Failures) != 1 || re.Failures[0].Name != "findings" || re.Failures[0].Err == nil {
		t.Errorf("Failures = %+v, want findings", re.Failures)
	}
}

func TestUpdateContext_WithoutRenameTouchesNoManifest(t *testing.T) {
	f := newOwnershipFixture(t, "work")
	f.seed(t, "mine", "work")
	f.store.SaveManifestErr = errors.New("must not be called")

	_, re, err := app.UpdateContext(context.Background(), f.store, f.fs, "work", domain.Context{Name: "work", WorkspacesRoot: ownRoot})
	if err != nil {
		t.Fatalf("UpdateContext: %v", err)
	}
	if len(re.Reassigned) != 0 || len(re.Failures) != 0 {
		t.Fatalf("reassignment = %+v, want none", re)
	}
}

func TestList_ShowsOrphansOfMissingContextsFlaggedAndHidesOtherOwners(t *testing.T) {
	f := newOwnershipFixture(t, "work", "acme")
	f.seed(t, "mine", "work")
	f.seed(t, "theirs", "acme")
	f.seed(t, "findings", "legacy")
	f.seed(t, "shared", "")

	got := f.list(t, "work")
	if len(got) != 3 {
		t.Fatalf("List(work) = %+v, want mine, findings, shared", got)
	}
	if _, ok := got["theirs"]; ok {
		t.Errorf("theirs (owned by existing acme) must stay hidden")
	}
	if got["findings"].OrphanOf != "legacy" || got["findings"].Err != nil {
		t.Errorf("findings = %+v, want OrphanOf legacy", got["findings"])
	}
	if got["mine"].OrphanOf != "" || got["shared"].OrphanOf != "" {
		t.Errorf("owned/shared workspaces flagged orphan: %+v", got)
	}
}

func TestRemoveContext_ItsWorkspacesBecomeOrphansOfAnotherContextSharingTheRoot(t *testing.T) {
	f := newOwnershipFixture(t, "alpha", "beta")
	f.seed(t, "ws1", "alpha")
	if err := f.store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "beta"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.list(t, "beta")["ws1"]; ok {
		t.Fatalf("ws1 visible to beta while alpha exists")
	}

	if err := app.RemoveContextWith(context.Background(), f.store, "alpha", app.RemoveContextOptions{}); err != nil {
		t.Fatalf("RemoveContextWith: %v", err)
	}
	ws, ok := f.list(t, "beta")["ws1"]
	if !ok || ws.OrphanOf != "alpha" {
		t.Fatalf("ws1 = %+v (listed %v), want an orphan of alpha", ws, ok)
	}
	if got := f.owner(t, "ws1"); got != "alpha" {
		t.Errorf("delete rewrote the manifest: owner %q", got)
	}
}

func TestClaimWorkspaces_DefaultClaimsEveryOrphanAndNothingElse(t *testing.T) {
	f := newOwnershipFixture(t, "acme", "acme")
	f.seed(t, "findings", "legacy")
	f.seed(t, "memoryleak", "legacy")
	f.seed(t, "theirs", "acme")
	f.seed(t, "mine", "acme")
	f.seed(t, "shared", "")

	res, err := app.ClaimWorkspaces(context.Background(), f.deps(), app.ClaimWorkspacesInput{Context: domain.Context{Name: "acme", WorkspacesRoot: ownRoot}})
	if err != nil {
		t.Fatalf("ClaimWorkspaces: %v", err)
	}
	if len(res.Claimed) != 2 || len(res.Skipped) != 0 {
		t.Fatalf("result = %+v, want findings and memoryleak claimed, nothing skipped", res)
	}
	for _, c := range res.Claimed {
		if c.PreviousContext != "legacy" {
			t.Errorf("claimed %+v, want previous context legacy", c)
		}
	}
	want := map[string]domain.ContextName{"findings": "acme", "memoryleak": "acme", "theirs": "acme", "mine": "acme", "shared": ""}
	for name, owner := range want {
		if got := f.owner(t, name); got != owner {
			t.Errorf("%s owner = %q, want %q", name, got, owner)
		}
	}

	again, err := app.ClaimWorkspaces(context.Background(), f.deps(), app.ClaimWorkspacesInput{Context: domain.Context{Name: "acme", WorkspacesRoot: ownRoot}})
	if err != nil || len(again.Claimed) != 0 || len(again.Skipped) != 0 {
		t.Fatalf("second run = %+v, %v; want nothing to do", again, err)
	}
	if got := f.list(t, "acme")["findings"]; got.OrphanOf != "" {
		t.Errorf("findings still orphaned after claim: %+v", got)
	}
}

func TestClaimWorkspaces_NamedRefusesToStealAndExplainsEverySkip(t *testing.T) {
	f := newOwnershipFixture(t, "globex", "acme")
	f.seed(t, "findings", "legacy")
	f.seed(t, "theirs", "acme")
	f.seed(t, "mine", "globex")
	f.seed(t, "shared", "")
	f.seed(t, "memoryleak", "legacy")

	res, err := app.ClaimWorkspaces(context.Background(), f.deps(), app.ClaimWorkspacesInput{
		Context:    domain.Context{Name: "globex", WorkspacesRoot: ownRoot},
		Workspaces: []string{"findings", "theirs", "mine", "shared", "missing", "../escape"},
	})
	if err != nil {
		t.Fatalf("ClaimWorkspaces: %v", err)
	}
	if len(res.Claimed) != 1 || res.Claimed[0].Name != "findings" || res.Claimed[0].Root != ownRoot.Join("findings") {
		t.Fatalf("Claimed = %+v, want only findings", res.Claimed)
	}
	wantReasons := map[string]messages.Key{
		"theirs":    messages.ClaimSkipOwnedByOther,
		"mine":      messages.ClaimSkipAlreadyOwned,
		"shared":    messages.ClaimSkipShared,
		"missing":   messages.ClaimSkipNotFound,
		"../escape": messages.ClaimSkipInvalidName,
	}
	if len(res.Skipped) != len(wantReasons) {
		t.Fatalf("Skipped = %+v", res.Skipped)
	}
	for _, s := range res.Skipped {
		if wantReasons[s.Name] != s.Reason {
			t.Errorf("skip %s reason = %s, want %s", s.Name, s.Reason, wantReasons[s.Name])
		}
		if msg := s.Message(); msg == "" || strings.Contains(msg, "%!") {
			t.Errorf("skip %s message = %q", s.Name, msg)
		}
	}
	if got := f.owner(t, "theirs"); got != "acme" {
		t.Errorf("theirs stolen: owner %q", got)
	}
	if got := f.owner(t, "memoryleak"); got != "legacy" {
		t.Errorf("memoryleak claimed although not named: owner %q", got)
	}
}

func TestClaimWorkspaces_WriteFailureIsReportedPerWorkspace(t *testing.T) {
	f := newOwnershipFixture(t, "acme")
	broken := f.seed(t, "findings", "legacy")
	f.seed(t, "memoryleak", "legacy")
	f.store.SaveManifestErrFor = map[domain.Path]error{broken: errors.New("read-only")}

	res, err := app.ClaimWorkspaces(context.Background(), f.deps(), app.ClaimWorkspacesInput{Context: domain.Context{Name: "acme", WorkspacesRoot: ownRoot}})
	if err != nil {
		t.Fatalf("ClaimWorkspaces: %v", err)
	}
	if len(res.Claimed) != 1 || res.Claimed[0].Name != "memoryleak" {
		t.Errorf("Claimed = %+v", res.Claimed)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Reason != messages.ClaimSkipWriteFailed || res.Skipped[0].Detail != "read-only" {
		t.Errorf("Skipped = %+v", res.Skipped)
	}
}

func TestRenderClaimSummary(t *testing.T) {
	lines := app.RenderClaimSummary(app.ClaimWorkspacesResult{
		Claimed: []app.ClaimedWorkspace{{Name: "findings", Root: "/w/findings", PreviousContext: "legacy"}},
		Skipped: []app.SkippedClaim{{Name: "theirs", Root: "/w/theirs", Reason: messages.ClaimSkipOwnedByOther, Detail: "acme"}},
	})
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"findings", "legacy", "theirs", "acme"} {
		if !strings.Contains(joined, want) {
			t.Errorf("summary %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, "%!") {
		t.Errorf("summary has a formatting error: %q", joined)
	}
	if empty := app.RenderClaimSummary(app.ClaimWorkspacesResult{}); len(empty) != 1 {
		t.Errorf("empty summary = %q, want one line", empty)
	}
}
