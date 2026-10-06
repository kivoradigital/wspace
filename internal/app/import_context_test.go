// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func init() {
	// RenderImportSummary renders through messages.T; activate the real
	// English catalog exactly like every other package's own tests do
	// when they need to assert on rendered text rather than structured
	// result fields.
	if err := messages.Use("en"); err != nil {
		panic(err)
	}
}

// autoAcceptPrompter is a ports.Prompter that accepts every field's own
// Default unless a specific call index has been overridden, and records
// every Text prompt's Default so a test can assert a wizard field was
// prefilled from a partial, successfully-parsed legacy context (this
// change's requirement 3) without hardcoding the exact sequence
// promptContextFields happens to ask in.
type autoAcceptPrompter struct {
	textOverrides    map[int]string
	confirmOverrides map[int]bool
	chooseOverrides  map[int]int

	textCalls    int
	confirmCalls int
	chooseCalls  int

	seenTextDefaults []string
}

func (p *autoAcceptPrompter) Text(_ context.Context, f ports.TextField) (string, error) {
	p.seenTextDefaults = append(p.seenTextDefaults, f.Default)
	idx := p.textCalls
	p.textCalls++
	if v, ok := p.textOverrides[idx]; ok {
		return v, nil
	}
	return f.Default, nil
}

func (p *autoAcceptPrompter) Confirm(_ context.Context, f ports.ConfirmField) (bool, error) {
	idx := p.confirmCalls
	p.confirmCalls++
	if v, ok := p.confirmOverrides[idx]; ok {
		return v, nil
	}
	return f.Default, nil
}

func (p *autoAcceptPrompter) Choose(_ context.Context, f ports.ChoiceField) (int, error) {
	idx := p.chooseCalls
	p.chooseCalls++
	if v, ok := p.chooseOverrides[idx]; ok {
		return v, nil
	}
	return 0, nil
}

func (p *autoAcceptPrompter) MultiChoose(_ context.Context, f ports.ChoiceField) ([]int, error) {
	return nil, nil
}

// Group dispatches every field in step to this fake's own Text/Confirm/
// Choose/MultiChoose (portstest.DispatchGroup) — importIntoNewContext now
// asks its option fields as one Step (promptContextFieldsOnly), so this
// fake must still answer it the same way it always answered each field
// individually, preserving every override index and seenTextDefaults
// recording this file's tests already depend on.
func (p *autoAcceptPrompter) Group(ctx context.Context, step ports.Step) (ports.StepAnswers, error) {
	return portstest.DispatchGroup(ctx, p, step)
}

const validLegacySource = `# a legacy flat workspace configuration
workspaces_root = /abs/workspaces
projects_root = /abs/projects
project_prefixes = Foo,bar
base_branch = develop
branch_prefix = feature
copy_env_default = yes
fetch_before_create = no
env_prune_dirs = .git,node_modules,dist
[projects]
alpha
beta
missing-dir
linked-wt
plain-folder
`

// newImportGit builds a FakeGit scripted to classify this file's fixture
// project directories exactly like a real main-clone check would: alpha
// and beta are usable main clones, linked-wt is a linked worktree, and
// plain-folder is an ordinary (non-git) directory.
func newImportGit() *portstest.FakeGit {
	git := portstest.NewFakeGit()
	git.IsMainCloneFunc = func(dir domain.Path) (bool, error) {
		switch dir {
		case domain.Path("/abs/projects/alpha"), domain.Path("/abs/projects/beta"):
			return true, nil
		case domain.Path("/abs/projects/linked-wt"):
			return false, portstest.LinkedWorktreeErr(dir)
		}
		return false, nil
	}
	return git
}

// seedProjectDirs marks every directory FakeFS.Exists should answer true
// for, except "missing-dir" which is deliberately left unregistered.
func seedProjectDirs(fs *portstest.FakeFS) {
	for _, name := range []string{"alpha", "beta", "linked-wt", "plain-folder"} {
		if err := fs.MkdirAll(domain.Path("/abs/projects/" + name)); err != nil {
			panic(err)
		}
	}
}

func TestImportLegacyContext_NewContext_HappyPath(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fs := portstest.NewFakeFS(t)
	seedProjectDirs(fs)
	if err := fs.WriteFile("/legacy/ws.config", []byte(validLegacySource), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	git := newImportGit()

	prompter := &autoAcceptPrompter{}
	deps := app.ImportLegacyContextDeps{Store: store, FS: fs, Git: git, Prompter: prompter}

	result, err := app.ImportLegacyContext(context.Background(), deps, app.ImportLegacyContextInput{
		From:    "/legacy/ws.config",
		NewName: "imported",
	})
	if err != nil {
		t.Fatalf("ImportLegacyContext: %v", err)
	}
	if !result.Created {
		t.Fatalf("result.Created = false, want true")
	}
	if result.ContextName != "imported" {
		t.Fatalf("result.ContextName = %q, want %q", result.ContextName, "imported")
	}

	persisted, err := store.LoadContext(context.Background(), "imported")
	if err != nil {
		t.Fatalf("LoadContext(imported): %v", err)
	}
	if persisted.WorkspacesRoot != "/abs/workspaces" {
		t.Errorf("WorkspacesRoot = %q, want /abs/workspaces", persisted.WorkspacesRoot)
	}
	if persisted.ProjectsRoot != "/abs/projects" {
		t.Errorf("ProjectsRoot = %q, want /abs/projects", persisted.ProjectsRoot)
	}
	if persisted.Defaults.BaseBranch == nil || *persisted.Defaults.BaseBranch != "develop" {
		t.Errorf("Defaults.BaseBranch = %v, want develop", persisted.Defaults.BaseBranch)
	}
	if persisted.Defaults.CopyEnv == nil || *persisted.Defaults.CopyEnv != true {
		t.Errorf("Defaults.CopyEnv = %v, want true", persisted.Defaults.CopyEnv)
	}
	if persisted.Defaults.FetchBeforeCreate == nil || *persisted.Defaults.FetchBeforeCreate != false {
		t.Errorf("Defaults.FetchBeforeCreate = %v, want false", persisted.Defaults.FetchBeforeCreate)
	}
	// branch_prefix and env_prune_dirs are never asked about by
	// promptContextFields at all (a pre-existing gap shared by "context
	// create"/"context edit"), so ImportLegacyContext must carry them
	// through from the successfully-parsed source itself rather than
	// silently losing them the way a plain promptContextFields call
	// otherwise would.
	if persisted.Defaults.BranchPrefix == nil || *persisted.Defaults.BranchPrefix != "feature" {
		t.Errorf("Defaults.BranchPrefix = %v, want feature", persisted.Defaults.BranchPrefix)
	}
	wantPrune := []string{".git", "node_modules", "dist"}
	if len(persisted.Defaults.EnvPruneDirs) != len(wantPrune) {
		t.Errorf("Defaults.EnvPruneDirs = %v, want %v", persisted.Defaults.EnvPruneDirs, wantPrune)
	}

	wantProjects := map[domain.ProjectKey]domain.Path{
		"alpha": "/abs/projects/alpha",
		"beta":  "/abs/projects/beta",
	}
	if len(persisted.Projects) != len(wantProjects) {
		t.Fatalf("Projects = %+v, want exactly %d entries", persisted.Projects, len(wantProjects))
	}
	for _, p := range persisted.Projects {
		if wantProjects[p.Key] != p.SourceDir {
			t.Errorf("project %q SourceDir = %q, want %q", p.Key, p.SourceDir, wantProjects[p.Key])
		}
	}

	// Skip reasons: missing-dir (no such directory), linked-wt (linked
	// worktree), plain-folder (not a git repository at all).
	skipped := map[string]bool{}
	for _, s := range result.SkippedProjects {
		skipped[s.Name] = true
	}
	for _, name := range []string{"missing-dir", "linked-wt", "plain-folder"} {
		if !skipped[name] {
			t.Errorf("SkippedProjects = %+v, want %q present", result.SkippedProjects, name)
		}
	}

	// project_prefixes is recognized but has no destination.
	foundUnmapped := false
	for _, issue := range result.UnmappedKeys {
		if issue.Key == "project_prefixes" {
			foundUnmapped = true
		}
	}
	if !foundUnmapped {
		t.Errorf("UnmappedKeys = %+v, want project_prefixes reported", result.UnmappedKeys)
	}
}

func TestImportLegacyContext_NewContext_PartialConfigPrefillsWizardDefaults(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fs := portstest.NewFakeFS(t)
	// Deliberately missing branch_prefix, and everything except base_branch.
	src := "base_branch = develop\n[projects]\n"
	if err := fs.WriteFile("/legacy/ws.config", []byte(src), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	git := portstest.NewFakeGit()

	prompter := &autoAcceptPrompter{}
	deps := app.ImportLegacyContextDeps{Store: store, FS: fs, Git: git, Prompter: prompter}

	result, err := app.ImportLegacyContext(context.Background(), deps, app.ImportLegacyContextInput{
		From:    "/legacy/ws.config",
		NewName: "partial",
	})
	if err != nil {
		t.Fatalf("ImportLegacyContext: %v", err)
	}
	if result.ContextName != "partial" {
		t.Fatalf("result.ContextName = %q, want partial", result.ContextName)
	}

	persisted, err := store.LoadContext(context.Background(), "partial")
	if err != nil {
		t.Fatalf("LoadContext(partial): %v", err)
	}
	// base_branch was present and valid: prefilled and accepted verbatim.
	if persisted.Defaults.BaseBranch == nil || *persisted.Defaults.BaseBranch != "develop" {
		t.Errorf("Defaults.BaseBranch = %v, want develop (from the partial source)", persisted.Defaults.BaseBranch)
	}
	// copy_env_default was absent: promptContextFields must have fallen
	// back to domain.BuiltinOptions.CopyEnv, exactly like "context edit"
	// falls back for any field a persisted context never set.
	if persisted.Defaults.CopyEnv == nil || *persisted.Defaults.CopyEnv != domain.BuiltinOptions.CopyEnv {
		t.Errorf("Defaults.CopyEnv = %v, want built-in default %v", persisted.Defaults.CopyEnv, domain.BuiltinOptions.CopyEnv)
	}

	// The prefill itself is proven by what promptContextFields's Text
	// prompts were seeded with: the base_branch prompt's Default must be
	// "develop", not the built-in "main".
	foundPrefilledBaseBranch := false
	for _, d := range prompter.seenTextDefaults {
		if d == "develop" {
			foundPrefilledBaseBranch = true
		}
	}
	if !foundPrefilledBaseBranch {
		t.Errorf("seenTextDefaults = %+v, want \"develop\" among them (base_branch prefill)", prompter.seenTextDefaults)
	}
}

func TestImportLegacyContext_NewContext_DuplicateNameRefusedThenRetried(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	store.PutContext(domain.Context{Name: "taken"})
	fs := portstest.NewFakeFS(t)
	if err := fs.WriteFile("/legacy/ws.config", []byte("base_branch = main\n[projects]\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	git := portstest.NewFakeGit()

	prompter := &autoAcceptPrompter{textOverrides: map[int]string{0: "brand-new"}}
	deps := app.ImportLegacyContextDeps{Store: store, FS: fs, Git: git, Prompter: prompter}

	result, err := app.ImportLegacyContext(context.Background(), deps, app.ImportLegacyContextInput{
		From:    "/legacy/ws.config",
		NewName: "taken",
	})
	if err != nil {
		t.Fatalf("ImportLegacyContext: %v", err)
	}
	if result.ContextName != "brand-new" {
		t.Fatalf("result.ContextName = %q, want brand-new (the taken name must never be overwritten)", result.ContextName)
	}
	if _, err := store.LoadContext(context.Background(), "taken"); err != nil {
		t.Fatalf("the original \"taken\" context must be untouched: %v", err)
	}
}

func TestImportLegacyContext_ExistingContext_ConfirmMerges(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	base := domain.BranchName("main")
	store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: "/old/workspaces",
		Defaults:       domain.Options{BaseBranch: &base},
		Projects:       []domain.Project{{Key: "alpha", SourceDir: "/abs/projects/alpha"}},
	})
	fs := portstest.NewFakeFS(t)
	seedProjectDirs(fs)
	if err := fs.WriteFile("/legacy/ws.config", []byte(validLegacySource), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	git := newImportGit()

	prompter := &autoAcceptPrompter{confirmOverrides: map[int]bool{0: true}}
	deps := app.ImportLegacyContextDeps{Store: store, FS: fs, Git: git, Prompter: prompter}

	result, err := app.ImportLegacyContext(context.Background(), deps, app.ImportLegacyContextInput{
		From:       "/legacy/ws.config",
		TargetName: "work",
	})
	if err != nil {
		t.Fatalf("ImportLegacyContext: %v", err)
	}
	if result.Cancelled {
		t.Fatalf("result.Cancelled = true, want false (confirmed)")
	}
	if result.Created {
		t.Fatalf("result.Created = true, want false (merged into an existing context)")
	}

	persisted, err := store.LoadContext(context.Background(), "work")
	if err != nil {
		t.Fatalf("LoadContext(work): %v", err)
	}
	if persisted.WorkspacesRoot != "/abs/workspaces" {
		t.Errorf("WorkspacesRoot = %q, want /abs/workspaces (overwritten from the legacy source)", persisted.WorkspacesRoot)
	}
	if persisted.Defaults.BaseBranch == nil || *persisted.Defaults.BaseBranch != "develop" {
		t.Errorf("Defaults.BaseBranch = %v, want develop", persisted.Defaults.BaseBranch)
	}
	// alpha was already registered; beta is new.
	keys := map[domain.ProjectKey]bool{}
	for _, p := range persisted.Projects {
		keys[p.Key] = true
	}
	if !keys["alpha"] || !keys["beta"] {
		t.Errorf("Projects = %+v, want both alpha (kept) and beta (added)", persisted.Projects)
	}
	if len(persisted.Projects) != 2 {
		t.Errorf("Projects = %+v, want exactly 2 (alpha not duplicated)", persisted.Projects)
	}
}

func TestImportLegacyContext_ExistingContext_DeclineWritesNothing(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	original := domain.Context{Name: "work", WorkspacesRoot: "/old/workspaces"}
	store.PutContext(original)
	fs := portstest.NewFakeFS(t)
	seedProjectDirs(fs)
	if err := fs.WriteFile("/legacy/ws.config", []byte(validLegacySource), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	git := newImportGit()

	prompter := &autoAcceptPrompter{confirmOverrides: map[int]bool{0: false}}
	deps := app.ImportLegacyContextDeps{Store: store, FS: fs, Git: git, Prompter: prompter}

	result, err := app.ImportLegacyContext(context.Background(), deps, app.ImportLegacyContextInput{
		From:       "/legacy/ws.config",
		TargetName: "work",
	})
	if err != nil {
		t.Fatalf("ImportLegacyContext: %v", err)
	}
	if !result.Cancelled {
		t.Fatalf("result.Cancelled = false, want true (declined)")
	}

	persisted, err := store.LoadContext(context.Background(), "work")
	if err != nil {
		t.Fatalf("LoadContext(work): %v", err)
	}
	if persisted.WorkspacesRoot != "/old/workspaces" {
		t.Errorf("WorkspacesRoot = %q, want unchanged /old/workspaces after a decline", persisted.WorkspacesRoot)
	}
}

func TestImportLegacyContext_Discover_TwoSourcesAmbiguous(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fs := portstest.NewFakeFS(t)
	global := fs.LegacyFlatConfigPath()
	if err := fs.WriteFile(global, []byte("base_branch = main\n[projects]\n"), 0o644); err != nil {
		t.Fatalf("seed global: %v", err)
	}
	cwd, err := fs.Cwd()
	if err != nil {
		t.Fatalf("Cwd: %v", err)
	}
	folderConfig := cwd.Join("ws.config")
	if err := fs.WriteFile(folderConfig, []byte("base_branch = develop\n[projects]\n"), 0o644); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	git := portstest.NewFakeGit()

	// Choose index 1: the second candidate offered. resolveLegacySource
	// must order candidates deterministically (global first, then
	// folder-level) for this assertion to be meaningful.
	prompter := &autoAcceptPrompter{chooseOverrides: map[int]int{0: 1}}
	deps := app.ImportLegacyContextDeps{Store: store, FS: fs, Git: git, Prompter: prompter}

	result, err := app.ImportLegacyContext(context.Background(), deps, app.ImportLegacyContextInput{
		NewName: "picked",
	})
	if err != nil {
		t.Fatalf("ImportLegacyContext: %v", err)
	}
	if result.SourcePath != folderConfig {
		t.Fatalf("result.SourcePath = %q, want the folder-level candidate %q (Choose index 1)", result.SourcePath, folderConfig)
	}

	persisted, err := store.LoadContext(context.Background(), "picked")
	if err != nil {
		t.Fatalf("LoadContext(picked): %v", err)
	}
	if persisted.Defaults.BaseBranch == nil || *persisted.Defaults.BaseBranch != "develop" {
		t.Errorf("Defaults.BaseBranch = %v, want develop (the folder-level file's value)", persisted.Defaults.BaseBranch)
	}
}

func TestImportLegacyContext_Discover_NoneFoundIsAClearError(t *testing.T) {
	store := portstest.NewFakeConfigStore()
	fs := portstest.NewFakeFS(t)
	git := portstest.NewFakeGit()
	prompter := &autoAcceptPrompter{}
	deps := app.ImportLegacyContextDeps{Store: store, FS: fs, Git: git, Prompter: prompter}

	_, err := app.ImportLegacyContext(context.Background(), deps, app.ImportLegacyContextInput{NewName: "x"})
	if err == nil {
		t.Fatal("ImportLegacyContext: want an error when no legacy source can be found")
	}
	if domain.Code(err) != domain.CodeLegacyConfigNotFound {
		t.Fatalf("domain.Code(err) = %q, want %q", domain.Code(err), domain.CodeLegacyConfigNotFound)
	}
}

// TestRenderImportSummary_CreatedLineNamesContextThenSource is a
// regression test for a real defect this change's own manual
// verification caught: ImportSummaryCreated's catalog template
// ("created context %[1]s from %[2]s") names the context first and the
// source file second, so the call site must pass them in exactly that
// order — an earlier version of this code passed them swapped, which
// rendered "created context <path> from <name>" instead of "created
// context <name> from <path>".
func TestRenderImportSummary_CreatedLineNamesContextThenSource(t *testing.T) {
	lines := app.RenderImportSummary(app.ImportLegacyContextResult{
		Created:     true,
		ContextName: "scenario1",
		SourcePath:  "/legacy/ws.config",
	})
	if len(lines) == 0 {
		t.Fatal("RenderImportSummary returned no lines")
	}
	want := "created context scenario1 from /legacy/ws.config"
	if lines[0] != want {
		t.Fatalf("lines[0] = %q, want %q", lines[0], want)
	}
}

// seedLegacyWorkspace writes a workspace the legacy bash tool created under
// /abs/workspaces (validLegacySource's workspaces_root), mounting alpha.
func seedLegacyWorkspace(t *testing.T, fs *portstest.FakeFS) domain.Path {
	t.Helper()
	root := domain.Path("/abs/workspaces/demo")
	for _, d := range []domain.Path{root.Join(".ws"), root.Join("alpha")} {
		if err := fs.MkdirAll(d); err != nil {
			t.Fatal(err)
		}
	}
	conf := "name = demo\nbranch = feature/demo\n\n[repos]\nalpha|alpha|feature/demo\n"
	if err := fs.WriteFile(root.Join(".ws", "workspace.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestImportLegacyContext_AdoptsLegacyWorkspacesAfterWriting(t *testing.T) {
	for _, mode := range []string{"new", "merge", "merge-declined"} {
		t.Run(mode, func(t *testing.T) {
			store := portstest.NewFakeConfigStore()
			fs := portstest.NewFakeFS(t)
			seedProjectDirs(fs)
			if err := fs.WriteFile("/legacy/ws.config", []byte(validLegacySource), 0o644); err != nil {
				t.Fatal(err)
			}
			wsRoot := seedLegacyWorkspace(t, fs)

			in := app.ImportLegacyContextInput{From: "/legacy/ws.config", NewName: "imported"}
			prompter := &autoAcceptPrompter{}
			wantContext := domain.ContextName("imported")
			if mode != "new" {
				store.PutContext(domain.Context{Name: "work", WorkspacesRoot: "/old/workspaces"})
				in = app.ImportLegacyContextInput{From: "/legacy/ws.config", TargetName: "work"}
				prompter.confirmOverrides = map[int]bool{0: mode == "merge"}
				wantContext = "work"
			}

			result, err := app.ImportLegacyContext(context.Background(), app.ImportLegacyContextDeps{Store: store, FS: fs, Git: newImportGit(), Prompter: prompter}, in)
			if err != nil {
				t.Fatalf("ImportLegacyContext: %v", err)
			}

			m, loadErr := store.LoadManifest(context.Background(), wsRoot)
			if mode == "merge-declined" {
				if loadErr == nil || len(result.Adoption.Adopted) != 0 {
					t.Fatalf("a declined merge must adopt nothing: %+v", result.Adoption)
				}
				return
			}
			if loadErr != nil {
				t.Fatalf("legacy workspace not adopted: %v (result %+v)", loadErr, result.Adoption)
			}
			if m.Workspace.Context != wantContext || len(m.Workspace.Repos) != 1 || m.Workspace.Repos[0].SourceDir != "/abs/projects/alpha" {
				t.Errorf("manifest = %+v", m.Workspace)
			}
			if len(result.Adoption.Adopted) != 1 || result.Adoption.Adopted[0].Name != "demo" {
				t.Errorf("Adoption = %+v, want demo adopted", result.Adoption)
			}
			summary := strings.Join(app.RenderImportSummary(result), "\n")
			if !strings.Contains(summary, "demo") {
				t.Errorf("summary %q does not mention the adopted workspace", summary)
			}
		})
	}
}

// TestImportLegacyContext_ClaimsOrphansUnderTheTargetRoot: after an import
// (both modes), workspaces under the written context's WorkspacesRoot whose
// owner context no longer exists are claimed and listed in the summary;
// workspaces owned by an existing context are left alone.
func TestImportLegacyContext_ClaimsOrphansUnderTheTargetRoot(t *testing.T) {
	for _, mode := range []string{"new", "merge"} {
		t.Run(mode, func(t *testing.T) {
			store := portstest.NewFakeConfigStore()
			fs := portstest.NewFakeFS(t)
			seedProjectDirs(fs)
			if err := fs.WriteFile("/legacy/ws.config", []byte(validLegacySource), 0o644); err != nil {
				t.Fatal(err)
			}
			store.PutContext(domain.Context{Name: "acme", WorkspacesRoot: "/abs/workspaces"})
			for name, owner := range map[string]domain.ContextName{"findings": "legacy", "theirs": "acme"} {
				root := domain.Path("/abs/workspaces/" + name)
				store.PutManifest(root, domain.Manifest{SchemaVersion: 1, Workspace: domain.Workspace{Name: name, Root: root, Context: owner}})
				if err := fs.MkdirAll(root); err != nil {
					t.Fatal(err)
				}
			}

			in := app.ImportLegacyContextInput{From: "/legacy/ws.config", NewName: "imported"}
			prompter := &autoAcceptPrompter{}
			wantContext := domain.ContextName("imported")
			if mode == "merge" {
				store.PutContext(domain.Context{Name: "globex", WorkspacesRoot: "/old/workspaces"})
				in = app.ImportLegacyContextInput{From: "/legacy/ws.config", TargetName: "globex"}
				prompter.confirmOverrides = map[int]bool{0: true}
				wantContext = "globex"
			}

			result, err := app.ImportLegacyContext(context.Background(), app.ImportLegacyContextDeps{Store: store, FS: fs, Git: newImportGit(), Prompter: prompter}, in)
			if err != nil {
				t.Fatalf("ImportLegacyContext: %v", err)
			}
			if len(result.Claim.Claimed) != 1 || result.Claim.Claimed[0].Name != "findings" || result.Claim.Claimed[0].PreviousContext != "legacy" {
				t.Fatalf("Claim = %+v, want findings claimed from legacy", result.Claim)
			}
			m, _ := store.LoadManifest(context.Background(), "/abs/workspaces/findings")
			if m.Workspace.Context != wantContext {
				t.Errorf("findings owner = %q, want %q", m.Workspace.Context, wantContext)
			}
			theirs, _ := store.LoadManifest(context.Background(), "/abs/workspaces/theirs")
			if theirs.Workspace.Context != "acme" {
				t.Errorf("theirs stolen: %+v", theirs.Workspace)
			}
			summary := strings.Join(app.RenderImportSummary(result), "\n")
			if !strings.Contains(summary, "findings") || !strings.Contains(summary, "claimed") {
				t.Errorf("summary %q does not list the claimed workspace", summary)
			}
		})
	}
}
