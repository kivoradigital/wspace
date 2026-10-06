// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestRunProjectWizard_ScriptedPrompter_ScansAndFiltersByIgnorePatterns
// covers tasks.md 3.18: ListDirs -> IsMainClone -> ignore-pattern
// filtering, in that order, then a MultiChoose over the surviving
// candidates (project-configuration spec: "Reject a linked-worktree
// source", context-management spec: "Ignore pattern excludes a directory
// from discovery").
//
// "web" and "plain-notes" are scripted through the exact two shapes the
// real adapter returns for a non-main-clone directory (see
// portstest.FakeGit.IsMainCloneFunc's own doc comment): a linked worktree
// returns (false, err with CodeNotAMainClone), and an ordinary non-repo
// directory returns (false, nil) — never the same shape for both. Before
// this test was corrected, "web" was scripted as (false, nil), which the
// real adapter never returns for a linked worktree; that mismatch is
// exactly what let CRITICAL-1 (a scan-wide abort on any non-main-clone
// directory) ship behind a green suite.
func TestRunProjectWizard_ScriptedPrompter_ScansAndFiltersByIgnorePatterns(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	for _, d := range []string{"api", "web", "vendor", "archive-old", "plain-notes"} {
		if err := fakeFS.MkdirAll(projectsRoot.Join(d)); err != nil {
			t.Fatalf("mkdir %q: %v", d, err)
		}
	}
	// "api" is a main clone: its ".git" is a directory.
	if err := fakeFS.MkdirAll(projectsRoot.Join("api", ".git")); err != nil {
		t.Fatalf("mkdir api/.git: %v", err)
	}
	// "web" is a linked worktree: its ".git" is a file, which the walk
	// itself (via IsGitDirEntry) already distinguishes structurally — no
	// git command is needed to classify it, only to validate a genuine
	// main clone.
	if err := fakeFS.WriteFile(projectsRoot.Join("web", ".git"), []byte("gitdir: ../.git/worktrees/web\n"), 0o644); err != nil {
		t.Fatalf("write web/.git: %v", err)
	}
	// "vendor" and "archive-old" have no ".git" marker at all; whether they
	// did wouldn't matter, since both are pruned by IgnorePatterns before
	// the walk ever inspects them. "plain-notes" also has none: an
	// ordinary, empty container directory, walked into and found to hold
	// nothing.

	store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: fakeFS.Paths().Home.Join("workspaces"),
		ProjectsRoot:   projectsRoot,
		IgnorePatterns: []domain.Glob{"vendor", "archive-*"},
	})

	fakeGit := portstest.NewFakeGit()

	reporter := portstest.NewRecordingReporter()
	prompter := portstest.NewScriptedPrompter(t,
		portstest.MultiChooseAnswer([]int{0}),
		portstest.TextAnswer("api"),
		portstest.TextAnswer(""), // origin_branch -> inherit
		portstest.TextAnswer(""), // dest_branch -> inherit
		portstest.TextAnswer(""), // worktree_dir -> inherit
	)

	deps := app.ProjectWizardDeps{
		Store:    store,
		FS:       fakeFS,
		Git:      fakeGit,
		Reporter: reporter,
		Prompter: prompter,
	}

	got, err := app.RunProjectWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunProjectWizard: %v", err)
	}
	prompter.CheckUnconsumed()

	// IsMainClone is the git port's own validation of a genuinely usable
	// main clone; the walk only ever calls it for a directory it already
	// found structurally shaped like one. "vendor" and "archive-old" are
	// pruned by IgnorePatterns before the walk ever visits them, "web" is
	// already unambiguous as a linked worktree from its own ".git" file,
	// and "plain-notes" has no ".git" at all — none of the four should
	// ever reach the git port; only "api" should.
	var probed []string
	for _, c := range fakeGit.Calls {
		if c.Method == "IsMainClone" {
			probed = append(probed, c.Repo.Base())
		}
	}
	if len(probed) != 1 || probed[0] != "api" {
		t.Fatalf("IsMainClone calls = %v, want exactly one, for %q", probed, "api")
	}

	// The linked worktree is reported, not silently dropped; the ignored
	// and plain-folder directories produce no notice at all (none of the
	// three was ever a candidate to begin with).
	if len(reporter.Warnings) != 1 {
		t.Fatalf("reporter.Warnings = %+v, want exactly one (the linked worktree)", reporter.Warnings)
	}
	w := reporter.Warnings[0]
	if w.Key != messages.WizardSkippedLinkedWorktree || len(w.Args) != 1 || w.Args[0] != string(projectsRoot.Join("web")) {
		t.Fatalf("reporter.Warnings[0] = %+v, want WizardSkippedLinkedWorktree for %q", w, projectsRoot.Join("web"))
	}

	if len(got.Projects) != 1 {
		t.Fatalf("Projects = %+v, want exactly one (only \"api\" survives web=linked-worktree, vendor/archive-old=ignored, plain-notes=not-a-repo)", got.Projects)
	}
	if got.Projects[0].Key != "api" {
		t.Fatalf("Projects[0].Key = %q, want %q", got.Projects[0].Key, "api")
	}
	if got.Projects[0].SourceDir != projectsRoot.Join("api") {
		t.Fatalf("Projects[0].SourceDir = %q, want %q", got.Projects[0].SourceDir, projectsRoot.Join("api"))
	}
	// Requirement 5 (this change's own gap-closure note): pressing ENTER
	// through the three new optional fields must reproduce exactly the
	// zero-valued/inherited fields this wizard always produced before
	// they existed.
	if got.Projects[0].OriginBranch != nil {
		t.Fatalf("Projects[0].OriginBranch = %v, want nil (empty answer means inherit)", got.Projects[0].OriginBranch)
	}
	if got.Projects[0].DestBranch != "" {
		t.Fatalf("Projects[0].DestBranch = %q, want empty (empty answer means inherit)", got.Projects[0].DestBranch)
	}
	if got.Projects[0].WorktreeDir != "" {
		t.Fatalf("Projects[0].WorktreeDir = %q, want empty (empty answer means inherit)", got.Projects[0].WorktreeDir)
	}

	persisted, err := store.LoadContext(context.Background(), "work")
	if err != nil {
		t.Fatalf("LoadContext(work) after wizard: %v", err)
	}
	if len(persisted.Projects) != 1 {
		t.Fatalf("persisted Projects = %+v, want exactly one (SaveContext must have been called)", persisted.Projects)
	}
}

// TestRunProjectWizard_ManualRegistrationWithoutScanning covers
// project-configuration spec's "Manual registration without scanning": a
// context with no ProjectsRoot has nothing to scan, so the wizard offers
// manual entry instead, and the resulting record is saved exactly as a
// scan-derived one would be.
func TestRunProjectWizard_ManualRegistrationWithoutScanning(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: fakeFS.Paths().Home.Join("workspaces")})

	manualDir := "/Users/me/manual/api"
	deps := app.ProjectWizardDeps{
		Store: store,
		FS:    fakeFS,
		Git:   portstest.NewFakeGit(),
		Prompter: portstest.NewScriptedPrompter(t,
			portstest.ConfirmAnswer(true),
			portstest.TextAnswer(manualDir),
			portstest.TextAnswer("api"),
			portstest.TextAnswer(""), // origin_branch -> inherit
			portstest.TextAnswer(""), // dest_branch -> inherit
			portstest.TextAnswer(""), // worktree_dir -> inherit
			portstest.ConfirmAnswer(false),
		),
	}

	got, err := app.RunProjectWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunProjectWizard with no ProjectsRoot configured: %v", err)
	}
	if len(got.Projects) != 1 {
		t.Fatalf("Projects = %+v, want exactly one manually-entered project", got.Projects)
	}
	if got.Projects[0].Key != "api" || got.Projects[0].SourceDir != domain.Path(manualDir) {
		t.Fatalf("Projects[0] = %+v, want Key=api SourceDir=%q", got.Projects[0], manualDir)
	}
	// Requirement 5: the empty answers above must reproduce exactly the
	// zero-valued/inherited fields this wizard produced before this
	// change.
	if got.Projects[0].OriginBranch != nil {
		t.Fatalf("Projects[0].OriginBranch = %v, want nil (empty answer means inherit)", got.Projects[0].OriginBranch)
	}
	if got.Projects[0].DestBranch != "" {
		t.Fatalf("Projects[0].DestBranch = %q, want empty (empty answer means inherit)", got.Projects[0].DestBranch)
	}
	if got.Projects[0].WorktreeDir != "" {
		t.Fatalf("Projects[0].WorktreeDir = %q, want empty (empty answer means inherit)", got.Projects[0].WorktreeDir)
	}

	persisted, err := store.LoadContext(context.Background(), "work")
	if err != nil {
		t.Fatalf("LoadContext(work) after wizard: %v", err)
	}
	if len(persisted.Projects) != 1 {
		t.Fatalf("persisted Projects = %+v, want exactly one (SaveContext must have been called)", persisted.Projects)
	}
}

// TestRunProjectWizard_ManualRegistrationRejectsNonMainCloneSource covers
// the WARNING finding verify-report.md names alongside CRITICAL-1 ("Manual
// project registration never validates main-clone status"): a manually
// typed source directory that is a linked worktree must be rejected and
// re-prompted, exactly like a scanned candidate is — only through a
// different UX (a per-attempt notice plus a fresh chance to type a path,
// since the user explicitly asked to register this exact one, unlike a
// scan's silent skip).
func TestRunProjectWizard_ManualRegistrationRejectsNonMainCloneSource(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: fakeFS.Paths().Home.Join("workspaces")})

	linkedWorktreeDir := "/Users/me/manual/linked-worktree"
	validDir := "/Users/me/manual/api"

	fakeGit := portstest.NewFakeGit()
	fakeGit.IsMainCloneFunc = func(dir domain.Path) (bool, error) {
		if dir == domain.Path(linkedWorktreeDir) {
			return false, portstest.LinkedWorktreeErr(dir)
		}
		return true, nil
	}

	reporter := portstest.NewRecordingReporter()
	deps := app.ProjectWizardDeps{
		Store:    store,
		FS:       fakeFS,
		Git:      fakeGit,
		Reporter: reporter,
		Prompter: portstest.NewScriptedPrompter(t,
			portstest.ConfirmAnswer(true),
			portstest.TextAnswer(linkedWorktreeDir), // rejected: linked worktree
			portstest.ConfirmAnswer(true),           // try again
			portstest.TextAnswer(validDir),          // accepted
			portstest.TextAnswer("api"),
			portstest.TextAnswer(""), // origin_branch -> inherit
			portstest.TextAnswer(""), // dest_branch -> inherit
			portstest.TextAnswer(""), // worktree_dir -> inherit
			portstest.ConfirmAnswer(false),
		),
	}

	got, err := app.RunProjectWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunProjectWizard: %v", err)
	}

	if len(got.Projects) != 1 {
		t.Fatalf("Projects = %+v, want exactly one (the linked worktree must never be registered)", got.Projects)
	}
	if got.Projects[0].SourceDir != domain.Path(validDir) {
		t.Fatalf("Projects[0].SourceDir = %q, want %q", got.Projects[0].SourceDir, validDir)
	}

	if len(reporter.Warnings) != 1 {
		t.Fatalf("reporter.Warnings = %+v, want exactly one (the rejected linked worktree)", reporter.Warnings)
	}
	w := reporter.Warnings[0]
	if w.Key != messages.WizardManualSourceNotAMainClone || len(w.Args) != 1 || w.Args[0] != linkedWorktreeDir {
		t.Fatalf("reporter.Warnings[0] = %+v, want WizardManualSourceNotAMainClone for %q", w, linkedWorktreeDir)
	}
}

// TestRunProjectWizard_CollectsOriginDestWorktreeFields_ScannedFlow is the
// failing-test-first proof for this change's own authorized gap-closure:
// domain.Project's OriginBranch/DestBranch/WorktreeDir fields, and every
// precedence-resolution method that already reads them
// (domain.Resolver.BaseBranch/DestBranch/WorktreePath), were fully built
// and tested from the original design onward, but RunProjectWizard never
// prompted for any of the three on the scanned-path flow — only Key ever
// was. Before the fix, this script's three extra scripted text answers
// (meant for origin_branch, dest_branch, worktree_dir) are never consumed
// at all, which prompter.CheckUnconsumed below fails the test over.
func TestRunProjectWizard_CollectsOriginDestWorktreeFields_ScannedFlow(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("api", ".git")); err != nil {
		t.Fatalf("mkdir api/.git: %v", err)
	}

	store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: fakeFS.Paths().Home.Join("workspaces"),
		ProjectsRoot:   projectsRoot,
	})

	prompter := portstest.NewScriptedPrompter(t,
		portstest.MultiChooseAnswer([]int{0}),
		portstest.TextAnswer("api"),                 // project key
		portstest.TextAnswer("develop"),             // origin_branch
		portstest.TextAnswer("{prefix}{workspace}"), // dest_branch template
		portstest.TextAnswer("apps/{project}"),      // worktree_dir template
	)

	deps := app.ProjectWizardDeps{
		Store:    store,
		FS:       fakeFS,
		Git:      portstest.NewFakeGit(),
		Prompter: prompter,
	}

	got, err := app.RunProjectWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunProjectWizard: %v", err)
	}
	prompter.CheckUnconsumed()

	if len(got.Projects) != 1 {
		t.Fatalf("Projects = %+v, want exactly one", got.Projects)
	}
	p := got.Projects[0]
	if p.OriginBranch == nil || *p.OriginBranch != domain.BranchName("develop") {
		t.Fatalf("OriginBranch = %v, want pointer to %q", p.OriginBranch, "develop")
	}
	if p.DestBranch != domain.BranchTemplate("{prefix}{workspace}") {
		t.Fatalf("DestBranch = %q, want %q", p.DestBranch, "{prefix}{workspace}")
	}
	if p.WorktreeDir != domain.PathTemplate("apps/{project}") {
		t.Fatalf("WorktreeDir = %q, want %q", p.WorktreeDir, "apps/{project}")
	}
}

// TestRunProjectWizard_CollectsOriginDestWorktreeFields_ManualFlow is the
// manual-registration-flow counterpart of the scanned-flow test above:
// the same three fields must be collected there too, since
// project-configuration spec's four-field wizard applies to manual
// registration exactly as much as to a scanned pick.
func TestRunProjectWizard_CollectsOriginDestWorktreeFields_ManualFlow(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	store.PutContext(domain.Context{Name: "work", WorkspacesRoot: fakeFS.Paths().Home.Join("workspaces")})

	manualDir := "/Users/me/manual/api"
	deps := app.ProjectWizardDeps{
		Store: store,
		FS:    fakeFS,
		Git:   portstest.NewFakeGit(),
		Prompter: portstest.NewScriptedPrompter(t,
			portstest.ConfirmAnswer(true),
			portstest.TextAnswer(manualDir),
			portstest.TextAnswer("api"),
			portstest.TextAnswer("release"),            // origin_branch
			portstest.TextAnswer("{branch}"),           // dest_branch template
			portstest.TextAnswer("{project}-worktree"), // worktree_dir template
			portstest.ConfirmAnswer(false),
		),
	}

	got, err := app.RunProjectWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunProjectWizard with no ProjectsRoot configured: %v", err)
	}
	if len(got.Projects) != 1 {
		t.Fatalf("Projects = %+v, want exactly one manually-entered project", got.Projects)
	}
	p := got.Projects[0]
	if p.OriginBranch == nil || *p.OriginBranch != domain.BranchName("release") {
		t.Fatalf("OriginBranch = %v, want pointer to %q", p.OriginBranch, "release")
	}
	if p.DestBranch != domain.BranchTemplate("{branch}") {
		t.Fatalf("DestBranch = %q, want %q", p.DestBranch, "{branch}")
	}
	if p.WorktreeDir != domain.PathTemplate("{project}-worktree") {
		t.Fatalf("WorktreeDir = %q, want %q", p.WorktreeDir, "{project}-worktree")
	}
}

// TestRunProjectWizard_PreviewsEffectiveDefaults covers requirement 2 of
// this change's own gap-closure note: each optional field's prompt must
// show the user what pressing ENTER actually resolves to today, computed
// through the exact same domain.Resolver production code resolves with —
// never a second, hand-rolled description of the precedence rules that
// could silently drift from them.
func TestRunProjectWizard_PreviewsEffectiveDefaults(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	baseBranch := domain.BranchName("main")
	workspacesRoot := fakeFS.Paths().Home.Join("workspaces")
	store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: workspacesRoot,
		Defaults:       domain.Options{BaseBranch: &baseBranch},
	})

	prompter := &capturingPrompter{
		t:        t,
		captured: map[string]ports.Field{},
		confirms: []bool{true, false},
		texts:    []string{"/Users/me/manual/api", "api", "", "", ""},
	}

	deps := app.ProjectWizardDeps{Store: store, FS: fakeFS, Git: portstest.NewFakeGit(), Prompter: prompter}

	if _, err := app.RunProjectWizard(context.Background(), deps, "work"); err != nil {
		t.Fatalf("RunProjectWizard: %v", err)
	}

	origin, ok := prompter.captured[string(messages.WizardProjectOriginBranch)]
	if !ok || len(origin.Args) != 1 || origin.Args[0] != "main" {
		t.Fatalf("origin_branch preview Args = %+v, want [\"main\"] (the context's resolved base branch)", origin.Args)
	}

	dest, ok := prompter.captured[string(messages.WizardProjectDestBranch)]
	if !ok || len(dest.Args) != 1 || dest.Args[0] != "<workspace>" {
		t.Fatalf("dest_branch preview Args = %+v, want [\"<workspace>\"] (no branch_prefix set, so the preview is exactly the placeholder workspace name)", dest.Args)
	}

	worktree, ok := prompter.captured[string(messages.WizardProjectWorktreeDir)]
	wantWorktreePreview := string(workspacesRoot.Join("api"))
	if !ok || len(worktree.Args) != 1 || worktree.Args[0] != wantWorktreePreview {
		t.Fatalf("worktree_dir preview Args = %+v, want [%q] (the built-in {project} layout resolved under workspaces_root)", worktree.Args, wantWorktreePreview)
	}
}

// capturingPrompter is a minimal ports.Prompter used only by
// TestRunProjectWizard_PreviewsEffectiveDefaults, which needs to inspect
// every Text field's Args (portstest.ScriptedPrompter discards the field
// it was asked, only ever returning its scripted answer).
type capturingPrompter struct {
	t        *testing.T
	texts    []string
	textI    int
	confirms []bool
	confirmI int
	captured map[string]ports.Field
}

func (p *capturingPrompter) Text(_ context.Context, f ports.TextField) (string, error) {
	p.captured[string(f.Label)] = f.Field
	if p.textI >= len(p.texts) {
		p.t.Fatalf("Text called more times than scripted (label %q)", f.Label)
	}
	v := p.texts[p.textI]
	p.textI++
	return v, nil
}

func (p *capturingPrompter) Confirm(_ context.Context, f ports.ConfirmField) (bool, error) {
	if p.confirmI >= len(p.confirms) {
		p.t.Fatalf("Confirm called more times than scripted (label %q)", f.Label)
	}
	v := p.confirms[p.confirmI]
	p.confirmI++
	return v, nil
}

func (p *capturingPrompter) Choose(_ context.Context, f ports.ChoiceField) (int, error) {
	p.t.Fatalf("Choose unexpectedly called (label %q)", f.Label)
	return 0, nil
}

func (p *capturingPrompter) MultiChoose(_ context.Context, f ports.ChoiceField) ([]int, error) {
	p.t.Fatalf("MultiChoose unexpectedly called (label %q)", f.Label)
	return nil, nil
}

// Group dispatches every field in step to this fake's own Text/Confirm
// (portstest.DispatchGroup) — satisfies ports.Prompter even though this
// test's target, promptProjectDetails, does not call Group today.
func (p *capturingPrompter) Group(ctx context.Context, step ports.Step) (ports.StepAnswers, error) {
	return portstest.DispatchGroup(ctx, p, step)
}

// recordingPrompter wraps a *portstest.ScriptedPrompter to additionally
// record what the scanned-candidate selection field's own Options were
// (multiChoiceOptions) and what Default every WizardProjectKey Text prompt
// carried, in call order (keyDefaults) — the two things the recursive-scan
// tests below need to assert (relative-path display, leaf-collision key
// fallback) that ScriptedPrompter itself discards once it returns its
// scripted answer. Its own Group must dispatch through itself
// (portstest.DispatchGroup(ctx, p, step)), not through the embedded
// ScriptedPrompter's Group: that embedded Group dispatches through the
// embedded value directly, which would call the embedded Text/MultiChoose
// methods and skip this wrapper's own overrides entirely.
type recordingPrompter struct {
	*portstest.ScriptedPrompter
	multiChoiceOptions []ports.Option
	keyDefaults        []string
}

func (p *recordingPrompter) Text(ctx context.Context, f ports.TextField) (string, error) {
	if f.Label == messages.WizardProjectKey {
		p.keyDefaults = append(p.keyDefaults, f.Default)
	}
	return p.ScriptedPrompter.Text(ctx, f)
}

func (p *recordingPrompter) MultiChoose(ctx context.Context, f ports.ChoiceField) ([]int, error) {
	p.multiChoiceOptions = f.Options
	return p.ScriptedPrompter.MultiChoose(ctx, f)
}

func (p *recordingPrompter) Group(ctx context.Context, step ports.Step) (ports.StepAnswers, error) {
	return portstest.DispatchGroup(ctx, p, step)
}

// TestRunProjectWizard_RecursiveScan_DisplaysPathsRelativeToRoot is the
// failing-test-first proof for this change's own recursive-discovery fix:
// a repository nested below a plain container directory
// ("team-a/api", two levels below root) is found at all (the old
// single-level ListDirs scan could never see it), and the selection list
// shows its path relative to root, never just its leaf name — otherwise
// "team-a/api" and a sibling repo directly under root both named
// differently would still be indistinguishable from any repo simply named
// "api".
func TestRunProjectWizard_RecursiveScan_DisplaysPathsRelativeToRoot(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("solo", ".git")); err != nil {
		t.Fatalf("mkdir solo/.git: %v", err)
	}
	if err := fakeFS.MkdirAll(projectsRoot.Join("team-a", "api", ".git")); err != nil {
		t.Fatalf("mkdir team-a/api/.git: %v", err)
	}
	store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: fakeFS.Paths().Home.Join("workspaces"),
		ProjectsRoot:   projectsRoot,
	})

	inner := portstest.NewScriptedPrompter(t,
		portstest.MultiChooseAnswer([]int{0, 1}), // both candidates
		portstest.TextAnswer("solo"),
		portstest.TextAnswer(""), portstest.TextAnswer(""), portstest.TextAnswer(""),
		portstest.TextAnswer("team-a-api"),
		portstest.TextAnswer(""), portstest.TextAnswer(""), portstest.TextAnswer(""),
	)
	prompter := &recordingPrompter{ScriptedPrompter: inner}

	deps := app.ProjectWizardDeps{Store: store, FS: fakeFS, Git: portstest.NewFakeGit(), Prompter: prompter}

	got, err := app.RunProjectWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunProjectWizard: %v", err)
	}
	inner.CheckUnconsumed()

	if len(got.Projects) != 2 {
		t.Fatalf("Projects = %+v, want exactly two (the recursive scan must find the nested repo too)", got.Projects)
	}

	wantOptions := map[string]bool{"solo": false, "team-a/api": false}
	if len(prompter.multiChoiceOptions) != 2 {
		t.Fatalf("selection Options = %+v, want exactly two", prompter.multiChoiceOptions)
	}
	for _, o := range prompter.multiChoiceOptions {
		if _, ok := wantOptions[o.Raw]; !ok {
			t.Fatalf("selection Option.Raw = %q, want a path relative to root (one of %v) — never just the leaf directory name", o.Raw, wantOptions)
		}
		wantOptions[o.Raw] = true
	}
	for raw, seen := range wantOptions {
		if !seen {
			t.Fatalf("expected a selection option displaying relative path %q, never saw it", raw)
		}
	}
}

// TestRunProjectWizard_KeyDefaultFallsBackToRelativePathOnLeafCollision is
// the failing-test-first proof for the other subtle part of this change's
// own fix: two candidates that share a leaf directory name ("team-a/api",
// "team-b/api") cannot both default their project key to "api" (a
// domain.Context cannot hold two projects with the same key) — the default
// must fall back to something derived from each one's own path relative to
// root, unique and still recognisable.
func TestRunProjectWizard_KeyDefaultFallsBackToRelativePathOnLeafCollision(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("team-a", "api", ".git")); err != nil {
		t.Fatalf("mkdir team-a/api/.git: %v", err)
	}
	if err := fakeFS.MkdirAll(projectsRoot.Join("team-b", "api", ".git")); err != nil {
		t.Fatalf("mkdir team-b/api/.git: %v", err)
	}
	store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: fakeFS.Paths().Home.Join("workspaces"),
		ProjectsRoot:   projectsRoot,
	})

	// Candidates are sorted by full path, so "team-a/api" is offered before
	// "team-b/api"; both are picked and both project-key prompts are
	// accepted at their own scripted (non-empty, distinct) values, since
	// what this test asserts is each prompt's Default, not its answer.
	inner := portstest.NewScriptedPrompter(t,
		portstest.MultiChooseAnswer([]int{0, 1}),
		portstest.TextAnswer("team-a-api"),
		portstest.TextAnswer(""), portstest.TextAnswer(""), portstest.TextAnswer(""),
		portstest.TextAnswer("team-b-api"),
		portstest.TextAnswer(""), portstest.TextAnswer(""), portstest.TextAnswer(""),
	)
	prompter := &recordingPrompter{ScriptedPrompter: inner}

	deps := app.ProjectWizardDeps{Store: store, FS: fakeFS, Git: portstest.NewFakeGit(), Prompter: prompter}

	if _, err := app.RunProjectWizard(context.Background(), deps, "work"); err != nil {
		t.Fatalf("RunProjectWizard: %v", err)
	}
	inner.CheckUnconsumed()

	want := []string{"team-a/api", "team-b/api"}
	if len(prompter.keyDefaults) != 2 {
		t.Fatalf("key Defaults = %v, want exactly two", prompter.keyDefaults)
	}
	for i, got := range prompter.keyDefaults {
		if got == "api" {
			t.Fatalf("key Default[%d] = %q, want the leaf-collision fallback (a value derived from %q), never the ambiguous leaf name shared by both candidates", i, got, want[i])
		}
	}
}

// TestRunProjectWizard_ReportsScanDepthLimitReached covers the honesty
// requirement: a ProjectScanMaxDepth too shallow to reach a real repository
// must not silently present its scan as complete — Reporter must carry a
// notice, and the too-deep repository must not appear as a candidate.
func TestRunProjectWizard_ReportsScanDepthLimitReached(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fakeFS := portstest.NewFakeFS(t)
	projectsRoot := fakeFS.Paths().Home.Join("src")
	if err := fakeFS.MkdirAll(projectsRoot.Join("org", "team", "deep", ".git")); err != nil {
		t.Fatalf("mkdir org/team/deep/.git: %v", err)
	}
	store.PutContext(domain.Context{
		Name:                "work",
		WorkspacesRoot:      fakeFS.Paths().Home.Join("workspaces"),
		ProjectsRoot:        projectsRoot,
		ProjectScanMaxDepth: 1,
	})

	reporter := portstest.NewRecordingReporter()
	prompter := portstest.NewScriptedPrompter(t,
		portstest.ConfirmAnswer(false), // no candidates found -> decline manual registration
	)

	deps := app.ProjectWizardDeps{Store: store, FS: fakeFS, Git: portstest.NewFakeGit(), Reporter: reporter, Prompter: prompter}

	got, err := app.RunProjectWizard(context.Background(), deps, "work")
	if err != nil {
		t.Fatalf("RunProjectWizard: %v", err)
	}
	prompter.CheckUnconsumed()

	if len(got.Projects) != 0 {
		t.Fatalf("Projects = %+v, want none (the repo sits past the depth-1 cap)", got.Projects)
	}

	var found bool
	for _, w := range reporter.Warnings {
		if w.Key == messages.WizardScanDepthLimitReached {
			found = true
		}
	}
	if !found {
		t.Fatalf("reporter.Warnings = %+v, want a WizardScanDepthLimitReached notice", reporter.Warnings)
	}
}
