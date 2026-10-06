// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"sort"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// notifySkippedLinkedWorktree reports a scanned candidate excluded because
// it is a linked worktree, guarding the nil Reporter a caller that does
// not care to surface the notice may leave unset.
func notifySkippedLinkedWorktree(reporter ports.Reporter, dir domain.Path) {
	if reporter == nil {
		return
	}
	reporter.Warn(messages.WizardSkippedLinkedWorktree, string(dir))
}

// notifyScanDepthLimitReached reports that the recursive scan's own depth
// cap was reached while directories below it were still left unexplored
// (WalkGitReposResult.Truncated) — a partial scan must say so honestly,
// never present itself as complete, exactly like notifySkippedLinkedWorktree
// never silently drops a rejected candidate.
func notifyScanDepthLimitReached(reporter ports.Reporter, root domain.Path, maxDepth int) {
	if reporter == nil {
		return
	}
	reporter.Warn(messages.WizardScanDepthLimitReached, string(root), maxDepth)
}

// ProjectWizardDeps bundles the driven ports RunProjectWizard needs: a
// config store to load/save the context, a filesystem port for directory
// scanning, a git port to reject linked-worktree candidates, a prompter to
// drive either surface, and a reporter to notify (never abort over) a
// scanned candidate skipped as a linked worktree. Reporter may be left nil
// by a caller that does not care to surface that notice (it is guarded
// before every use); the CLI path always has one bound by
// internal/cli/reporter.go's bindReporter, exactly like app.Deps.Reporter.
type ProjectWizardDeps struct {
	Store    ports.ConfigStore
	FS       ports.FileSystemPort
	Git      ports.GitPort
	Prompter ports.Prompter
	Reporter ports.Reporter
}

// RunProjectWizard optionally scans the context's ProjectsRoot for
// candidate main-clone directories, lets the user pick which ones to
// register, and appends the resulting project records to the context
// (project-configuration spec: "Discovery scan as wizard pre-fill only",
// "Main-clone detection"). Scanning is skipped entirely when the context
// has no ProjectsRoot configured — manual registration remains possible by
// construction, since a project record needs no scan to exist.
//
// The scan walks ProjectsRoot recursively (scanCandidates's own doc
// comment), applying ignore_patterns as it descends rather than as a
// separate pass afterward — kept separate from env_prune_dirs, which never
// applies here (context-management spec: "Ignore patterns do not affect
// env pruning").
func RunProjectWizard(ctx context.Context, deps ProjectWizardDeps, contextName domain.ContextName) (domain.Context, error) {
	return runProjectWizard(ctx, deps, contextName, ports.Step{})
}

// runProjectWizard is RunProjectWizard's own implementation, taking the
// selection Step's own Index/Total/Back as selectionStep — the zero
// ports.Step (no indicator, no Back) for RunProjectWizard's own standalone
// callers (the tray's "New workspace…" item has no earlier step to go
// back to), or {Index: 2, Total: 2, Back: true} when
// RunInitializeContextWizard drives this same scan-and-select logic as
// its own wizard's second step (design's own concrete example: "Project
// selection after the scan is a genuine second step") — never a forked
// copy of the scan/select/register pipeline itself.
func runProjectWizard(ctx context.Context, deps ProjectWizardDeps, contextName domain.ContextName, selectionStep ports.Step) (domain.Context, error) {
	c, err := deps.Store.LoadContext(ctx, contextName)
	if err != nil {
		return domain.Context{}, err
	}

	candidates, err := scanCandidates(ctx, deps, c)
	if err != nil {
		return domain.Context{}, err
	}

	selected, err := pickCandidates(ctx, deps, c.ProjectsRoot, candidates, selectionStep)
	if err != nil {
		return domain.Context{}, err
	}

	for _, dir := range selected {
		keyStr, err := deps.Prompter.Text(ctx, ports.TextField{
			Field:   ports.Field{Label: messages.WizardProjectKey},
			Default: defaultProjectKey(c, candidates, dir),
		})
		if err != nil {
			return domain.Context{}, err
		}
		key := domain.ProjectKey(keyStr)

		originBranch, destBranch, worktreeDir, err := promptProjectDetails(ctx, deps, c, key)
		if err != nil {
			return domain.Context{}, err
		}

		c.Projects = append(c.Projects, domain.Project{
			Key:          key,
			SourceDir:    dir,
			OriginBranch: originBranch,
			DestBranch:   destBranch,
			WorktreeDir:  worktreeDir,
		})
	}

	// Scanning found nothing to offer (no ProjectsRoot, or every candidate
	// was filtered out): fall back to manual entry, so a project can
	// always be registered without a scan (project-configuration spec:
	// "Manual registration without scanning").
	if len(selected) == 0 {
		manual, err := collectManualProjects(ctx, deps, c)
		if err != nil {
			return domain.Context{}, err
		}
		c.Projects = append(c.Projects, manual...)
	}

	if err := deps.Store.SaveContext(ctx, c); err != nil {
		return domain.Context{}, err
	}
	return c, nil
}

// scanCandidates recursively walks c.ProjectsRoot (when set) looking for
// initialized git repositories at any depth — the fix for a repository
// nested more than one level below the root (e.g. "<root>/team/api"),
// which the single-level ListDirs this used to call could never see
// (project-configuration spec: "Discovery scan as wizard pre-fill only").
//
// The walk itself (deps.FS.WalkGitRepos) prunes before descending: a hidden
// directory (leading ".") or one matching c.IgnorePatterns is skipped
// without ever being listed, and a directory it does visit that turns out
// to itself be a git repository (main clone or linked worktree) is recorded
// but never descended into — its own contents are not more candidates.
// This also means ignore-pattern filtering happens once, during the walk,
// not as a second pass over the result: a directory the walk skipped was
// never a candidate to begin with. The walk is capped at
// c.ProjectScanMaxDepth directory levels below the root (or
// domain.DefaultProjectScanMaxDepth when unset); reaching that cap while
// directories remained unexplored is reported through Reporter rather than
// silently presenting a partial list as a complete one.
//
// Every directory the walk finds structurally shaped like a main clone
// (".git" is a directory) is still confirmed through deps.Git.IsMainClone —
// the git port, not the filesystem walk, owns the judgment of whether a
// directory is a genuinely usable repository (design.md §7): the walk's own
// stat-only check cannot tell a healthy repository from a corrupted one
// that fails "git rev-parse --git-dir". That per-candidate outcome is never
// allowed to abort the whole scan except for a genuine operational failure
// (an abort over one bad candidate would defeat scanning's own
// "optional convenience" requirement) — only a rejection the git port
// itself raises (domain.CodeNotAMainClone) is treated as "not a candidate,
// reported, scan continues", exactly like a directory the walk already
// found shaped like a linked worktree (".git" is a file) is reported
// without ever asking the git port at all, since that shape is already
// unambiguous.
func scanCandidates(ctx context.Context, deps ProjectWizardDeps, c domain.Context) ([]domain.Path, error) {
	if c.ProjectsRoot == "" {
		return nil, nil
	}
	out, err := walkCandidates(ctx, deps.FS, deps.Git, c.ProjectsRoot, c.IgnorePatterns, c.ProjectScanMaxDepth)
	if err != nil {
		return nil, err
	}
	if out.truncated {
		notifyScanDepthLimitReached(deps.Reporter, c.ProjectsRoot, out.maxDepth)
	}
	for _, dir := range out.linked {
		notifySkippedLinkedWorktree(deps.Reporter, dir)
	}
	return out.candidates, nil
}

// scanOutcome is walkCandidates's result: confirmed main clones (sorted),
// every directory rejected as a linked worktree (in discovery order),
// whether the depth cap cut the walk short, and the effective cap used.
type scanOutcome struct {
	candidates    []domain.Path
	linked        []domain.Path
	truncated     bool
	truncatedDirs []domain.Path
	maxDepth      int
}

// walkCandidates is scanCandidates's reporter-free core, shared with the
// non-interactive ScanProjects: walk root (pruning hidden and ignored
// directories while descending), then confirm each main-clone-shaped
// directory through the git port. maxDepth <= 0 selects
// domain.DefaultProjectScanMaxDepth.
func walkCandidates(ctx context.Context, fs ports.FileSystemPort, git ports.GitPort, root domain.Path, ignore []domain.Glob, maxDepth int) (scanOutcome, error) {
	// domain.MatchesIgnorePattern's only possible error is a malformed
	// glob — a property of the pattern itself, never of the name it is
	// matched against — so it is validated once, upfront, against a
	// throwaway name. That lets the walk's own per-directory prune
	// callback stay a plain bool below, never silently swallowing a
	// bad-pattern failure the old single-level scan would have aborted on.
	if _, err := domain.MatchesIgnorePattern(ignore, ""); err != nil {
		return scanOutcome{}, err
	}
	prune := func(name string) bool {
		ignored, _ := domain.MatchesIgnorePattern(ignore, name)
		return ignored
	}

	if maxDepth <= 0 {
		maxDepth = domain.DefaultProjectScanMaxDepth
	}

	walked, err := fs.WalkGitRepos(root, maxDepth, prune)
	if err != nil {
		return scanOutcome{}, err
	}

	out := scanOutcome{truncated: walked.Truncated, truncatedDirs: walked.TruncatedDirs, maxDepth: maxDepth}
	out.linked = append(out.linked, walked.LinkedWorktrees...)
	for _, dir := range walked.MainClones {
		ok, err := git.IsMainClone(ctx, dir)
		if err != nil {
			if domain.Code(err) == domain.CodeNotAMainClone {
				out.linked = append(out.linked, dir)
				continue
			}
			return scanOutcome{}, err
		}
		if ok {
			out.candidates = append(out.candidates, dir)
		}
	}

	sort.Slice(out.candidates, func(i, j int) bool { return out.candidates[i] < out.candidates[j] })
	return out, nil
}

// relativeToRoot returns dir's path relative to root (root.Join's own
// inverse), for display and for the leaf-name-collision key fallback
// below. dir is always root itself or a descendant of it, since every
// candidate this file ever builds comes from root.Join(...); a dir that
// somehow is not (defensive only) falls back to its own Base() rather than
// a confusing empty or malformed string.
func relativeToRoot(root, dir domain.Path) string {
	if rel, ok := strings.CutPrefix(string(dir), string(root)+"/"); ok {
		return rel
	}
	return dir.Base()
}

// defaultProjectKey resolves a scanned candidate's default project key
// with the same rule ScanProjects uses (domain.SuggestProjectKeys over
// every candidate, avoiding the context's registered keys), so the
// terminal wizard and the app always suggest the same keys. The key
// remains user-editable either way.
func defaultProjectKey(c domain.Context, candidates []domain.Path, dir domain.Path) string {
	rels := make([]string, len(candidates))
	at := -1
	for i, cand := range candidates {
		rels[i] = relativeToRoot(c.ProjectsRoot, cand)
		if cand == dir {
			at = i
		}
	}
	taken := make([]domain.ProjectKey, 0, len(c.Projects))
	for _, p := range c.Projects {
		taken = append(taken, p.Key)
	}
	keys := domain.SuggestProjectKeys(rels, taken)
	if at < 0 {
		return dir.Base()
	}
	return keys[at]
}

// pickCandidates lets the user choose which scanned candidates to
// register. With zero candidates, it returns immediately without
// prompting at all (project-configuration spec: "Manual registration
// without scanning"). step carries this screen's own position within its
// caller's wizard (RunProjectWizard's own zero-value Step, or
// RunInitializeContextWizard's {Index: 2, Total: 2, Back: true} — see
// runProjectWizard's own doc comment); the field itself is always exactly
// this one multi-choice selection, driven through Group rather than
// MultiChoose directly, so a Back button can appear on it when step says
// so.
//
// Each option shows candidate's path relative to root, never just its leaf
// directory name: a recursive scan can easily surface "team-a/api" and
// "team-b/api" side by side, and the leaf name alone ("api", "api") would
// read as two identical, indistinguishable entries.
func pickCandidates(ctx context.Context, deps ProjectWizardDeps, root domain.Path, candidates []domain.Path, step ports.Step) ([]domain.Path, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	opts := make([]ports.Option, len(candidates))
	for i, d := range candidates {
		opts[i] = ports.Option{Raw: relativeToRoot(root, d)}
	}
	step.Fields = []ports.FieldSpec{ports.MultiChoiceFieldSpec(ports.ChoiceField{
		Field:   ports.Field{Label: messages.WizardPickProjects},
		Options: opts,
	})}
	answers, err := deps.Prompter.Group(ctx, step)
	if err != nil {
		return nil, err
	}
	picks := answers.GetMultiChoice(messages.WizardPickProjects)

	selected := make([]domain.Path, 0, len(picks))
	for _, i := range picks {
		selected = append(selected, candidates[i])
	}
	return selected, nil
}

// collectManualProjects repeatedly offers to register one more project by
// hand, stopping the first time the user declines
// (project-configuration spec: "Manual registration without scanning" —
// the resulting record must be usable exactly as a scan-derived one
// would be, so it is built through the identical domain.Project shape).
func collectManualProjects(ctx context.Context, deps ProjectWizardDeps, c domain.Context) ([]domain.Project, error) {
	var out []domain.Project
	for {
		again, err := deps.Prompter.Confirm(ctx, ports.ConfirmField{
			Field:   ports.Field{Label: messages.WizardAddProjectManually},
			Default: false,
		})
		if err != nil {
			return nil, err
		}
		if !again {
			return out, nil
		}

		dirStr, err := deps.Prompter.Text(ctx, ports.TextField{
			Field:    ports.Field{Label: messages.WizardProjectSourceDir, Help: messages.WizardProjectSourceDirHelp},
			Validate: validateAbsolutePath,
			Kind:     ports.TextKindDirectory,
		})
		if err != nil {
			return nil, err
		}
		dir := domain.Path(dirStr)

		// A manually-typed source must be a genuine main clone exactly
		// like a scanned one — the "Main-clone detection" spec
		// requirement is not scan-specific (WARNING, verify-report.md:
		// "Manual project registration never validates main-clone
		// status"). Unlike scanCandidates's silent skip over many
		// candidates, this one path was explicitly typed by the user, so
		// it is rejected with a notice and re-prompted, never silently
		// dropped.
		if ok, err := deps.Git.IsMainClone(ctx, dir); err != nil || !ok {
			if err != nil && domain.Code(err) != domain.CodeNotAMainClone {
				return nil, err
			}
			if deps.Reporter != nil {
				deps.Reporter.Warn(messages.WizardManualSourceNotAMainClone, string(dir))
			}
			continue
		}

		keyStr, err := deps.Prompter.Text(ctx, ports.TextField{
			Field:   ports.Field{Label: messages.WizardProjectKey},
			Default: dir.Base(),
		})
		if err != nil {
			return nil, err
		}
		key := domain.ProjectKey(keyStr)

		originBranch, destBranch, worktreeDir, err := promptProjectDetails(ctx, deps, c, key)
		if err != nil {
			return nil, err
		}

		out = append(out, domain.Project{
			Key:          key,
			SourceDir:    dir,
			OriginBranch: originBranch,
			DestBranch:   destBranch,
			WorktreeDir:  worktreeDir,
		})
	}
}

// promptProjectDetails collects the three OPTIONAL per-project fields the
// product owner's own requirement names alongside source_dir — origin
// branch, destination branch template and worktree directory template —
// shared by both the scanned-path flow and the manual-registration flow
// above, so neither path can ever drift from the other (exactly the same
// reason collectManualProjects itself is a single shared helper rather
// than inlined twice).
//
// An empty answer to any of the three means "inherit": OriginBranch stays
// nil, DestBranch and WorktreeDir stay their zero value, so a user who
// presses ENTER through all three ends up with exactly the domain.Project
// this wizard produced before this function existed — the resolver's own
// existing precedence chain (domain.Resolver.BaseBranch/DestBranch/
// WorktreePath) is what actually fills in a concrete value later, never a
// second default story duplicated here.
//
// c is the context-in-progress, not yet containing key's own project
// record (it is only appended by the caller right after this returns).
// Each prompt shows what pressing ENTER resolves to today by running that
// exact same domain.Resolver against c — a live preview, never a
// hand-rolled description of the precedence rules that could drift from
// them.
//
// This function, and the WizardProjectKey prompt immediately before it in
// both callers (runProjectWizard, collectManualProjects), are audited
// single-field prompts left as-is (this change's own lone-field-prompt
// audit, alongside the label/Help split and the standard window size):
// key must be answered before any of these three, because the
// worktree_dir preview's own fallback path is built from key itself
// (worktreePreview below), so the preview cannot be computed before key is
// known — the same kind of genuine sequential dependency that already
// justifies RunInitializeContextWizard's own projects-folder-then-
// selection Step split, not a "step for its own sake". Folding all four
// fields into one Step would mean either dropping the live preview this
// function's own doc comment above calls out as deliberate, or computing
// it from a placeholder before key is known, which the resolver-driven
// preview design explicitly rules out. This is unrelated to the two
// confirmed lone-field-window defects this change fixes (the context-name
// screen and the initialize-context workspaces-root screen): neither of
// those had a same-page-eligible sibling field artificially forced apart.
func promptProjectDetails(ctx context.Context, deps ProjectWizardDeps, c domain.Context, key domain.ProjectKey) (originBranch *domain.BranchName, destBranch domain.BranchTemplate, worktreeDir domain.PathTemplate, err error) {
	r := domain.Resolver{Context: &c}

	effectiveOrigin := r.BaseBranch(key)
	originStr, err := deps.Prompter.Text(ctx, ports.TextField{
		Field:    ports.Field{Label: messages.WizardProjectOriginBranch, Help: messages.WizardProjectOriginBranchHelp, Args: []any{string(effectiveOrigin.Value)}},
		Validate: validateOptionalBranchName,
	})
	if err != nil {
		return nil, "", "", err
	}
	if originStr != "" {
		bn, err := domain.NewBranchName(originStr)
		if err != nil {
			return nil, "", "", err
		}
		originBranch = &bn
	}

	// No real workspace exists yet at wizard time, so the workspace name
	// half of the preview is a placeholder token, not a real branch name;
	// dest_branch's own fallback (the workspace's own branch, or
	// branch_prefix + workspace name — see domain.Resolver.DestBranch) is
	// still computed through the real resolver, just fed a stand-in
	// Workspace value to substitute wherever the real one will later go.
	destPreview := "<workspace>"
	if resolved, derr := r.DestBranch(key, domain.Vars{Workspace: destPreview}); derr == nil {
		destPreview = string(resolved.Value)
	}
	destStr, err := deps.Prompter.Text(ctx, ports.TextField{
		Field:    ports.Field{Label: messages.WizardProjectDestBranch, Help: messages.WizardProjectDestBranchHelp, Args: []any{destPreview}},
		Validate: validateDestBranchTemplate,
	})
	if err != nil {
		return nil, "", "", err
	}
	destBranch = domain.BranchTemplate(destStr)

	worktreePreview := c.WorkspacesRoot.Join(string(key))
	if resolved, werr := r.WorktreePath(key, domain.Vars{Project: string(key)}, c.WorkspacesRoot); werr == nil {
		worktreePreview = resolved.Value
	}
	worktreeStr, err := deps.Prompter.Text(ctx, ports.TextField{
		Field:    ports.Field{Label: messages.WizardProjectWorktreeDir, Help: messages.WizardProjectWorktreeDirHelp, Args: []any{string(worktreePreview)}},
		Validate: validateWorktreeDirTemplate,
	})
	if err != nil {
		return nil, "", "", err
	}
	worktreeDir = domain.PathTemplate(worktreeStr)

	return originBranch, destBranch, worktreeDir, nil
}
