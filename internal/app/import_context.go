// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"path"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// legacyFolderConfigName is the folder-level marker ImportLegacyContext
// walks up from cwd looking for (this change's import feature, spec:
// "Two sources, discovered automatically"). It is never named after any
// real product — see this change's own naming rule.
const legacyFolderConfigName = "ws.config"

// ImportLegacyContextDeps bundles the driven ports ImportLegacyContext
// needs: a config store to load/save the target context, a filesystem
// port to read the source file and discover it, a git port to validate
// each imported project is a usable main clone, a prompter to drive
// either surface, and an optional reporter for the one non-fatal notice
// this use case emits along the way (a rejected duplicate name while
// still resolving one). Shaped like ProjectWizardDeps/ContextWizardDeps
// rather than reusing either verbatim, since this use case is the first
// to need all four plus a fifth (Git) at once.
type ImportLegacyContextDeps struct {
	Store    ports.ConfigStore
	FS       ports.FileSystemPort
	Git      ports.GitPort
	Prompter ports.Prompter
	Reporter ports.Reporter
}

// ImportLegacyContextInput bundles the CLI/tray-facing parameters.
type ImportLegacyContextInput struct {
	// From forces a specific source file, skipping discovery entirely.
	// Empty means "discover automatically" (spec: "Two sources,
	// discovered automatically").
	From domain.Path

	// TargetName selects "into an existing context" mode when non-empty:
	// the named context must already exist, and its name is never asked
	// for again (spec: "no name asked — already has one"). Empty selects
	// "into a new context" mode.
	TargetName domain.ContextName

	// NewName is "into a new context" mode's optional CLI-supplied name
	// (the --name flag). Empty means the wizard prompts for one, exactly
	// as the spec requires ("name is ALWAYS asked for... never derived
	// silently").
	NewName string
}

// ImportedFieldChange is one option field the existing-context diff
// (spec: "show a diff... before writing anything") reports as about to
// change. Field is a plain identifier (e.g. "workspaces_root"), not
// prose — internal/cli and internal/gui render it, never internal/app.
type ImportedFieldChange struct {
	Field    string
	Current  string
	Incoming string
}

// SkippedProject is one [projects] name that could not be registered,
// paired with why (spec: "surface those rather than registering them
// blindly").
type SkippedProject struct {
	Name   string
	Reason messages.Key
	Detail string
}

// ImportLegacyContextResult is the one shared outcome both the CLI and the
// tray render into their own summary (spec: "driven by one shared result
// struct returned by the internal/app use case").
type ImportLegacyContextResult struct {
	SourcePath domain.Path
	// ContextName is the context actually written to (the new name
	// chosen, or TargetName echoed back), even when Cancelled is true.
	ContextName domain.ContextName
	// Created is true for "into a new context" mode, false for "into an
	// existing context" mode.
	Created bool
	// Cancelled is true when an existing-context import's diff confirm
	// was declined; nothing was written to the store when this is true.
	Cancelled bool

	ImportedProjects []string
	SkippedProjects  []SkippedProject
	// UnmappedKeys reports every recognized-but-undestined key
	// (project_prefixes) found in the source.
	UnmappedKeys []domain.LegacyIssue
	// FieldIssues reports every mapped key whose value was missing or
	// failed validation (never invented, never a hard failure — spec
	// requirement 3).
	FieldIssues []domain.LegacyIssue
	// FieldChanges is the existing-context diff: which option values
	// would change (empty in "new context" mode, where every field is
	// simply the wizard's own answer).
	FieldChanges []ImportedFieldChange
	// ProjectsKept lists already-registered project keys an
	// existing-context import found no change for.
	ProjectsKept []string
	// Adoption reports the legacy workspaces under the written context's
	// WorkspacesRoot that were given a wspace manifest (or skipped, with
	// why) right after the context was saved. Empty when nothing was
	// written (a declined merge).
	Adoption AdoptLegacyWorkspacesResult
	// Claim reports the orphaned workspaces (their owner context no longer
	// exists) under the written context's WorkspacesRoot that were claimed
	// for it right after it was saved, before adoption. Empty when nothing
	// was written.
	Claim ClaimWorkspacesResult
}

// ImportLegacyContext turns a legacy flat "key = value" workspace
// configuration file into a wspace context (this change's own import
// feature). It drives exactly one internal/app use case regardless of
// front end (CLI's "context import", the tray's "Import context…"), so
// neither surface ever forks the discovery/parse/validate/confirm logic.
func ImportLegacyContext(ctx context.Context, deps ImportLegacyContextDeps, in ImportLegacyContextInput) (ImportLegacyContextResult, error) {
	sourcePath, err := resolveLegacySource(ctx, deps, in.From)
	if err != nil {
		return ImportLegacyContextResult{}, err
	}

	data, err := deps.FS.ReadFile(sourcePath)
	if err != nil {
		return ImportLegacyContextResult{}, err
	}
	parsed := domain.ParseLegacyFlatConfig(data)

	var unmapped, fieldIssues []domain.LegacyIssue
	for _, issue := range parsed.Issues {
		switch issue.Reason {
		case domain.LegacyReasonUnknownKey, domain.LegacyReasonNotImported:
			unmapped = append(unmapped, issue)
		default:
			fieldIssues = append(fieldIssues, issue)
		}
	}

	registered, skipped := validateLegacyProjects(ctx, deps, parsed.Context.ProjectsRoot, parsed.ProjectNames)

	if in.TargetName != "" {
		return importIntoExistingContext(ctx, deps, in.TargetName, sourcePath, parsed, registered, skipped, unmapped, fieldIssues)
	}
	return importIntoNewContext(ctx, deps, in.NewName, sourcePath, parsed, registered, skipped, unmapped, fieldIssues)
}

// resolveLegacySource implements spec requirement 1: --from forces a
// specific file and skips discovery entirely; otherwise both the global
// legacy directory and a folder-level marker found by walking up from cwd
// are checked, and finding both means asking the user to choose (never
// silently guessing).
func resolveLegacySource(ctx context.Context, deps ImportLegacyContextDeps, from domain.Path) (domain.Path, error) {
	if from != "" {
		return from, nil
	}

	var candidates []domain.Path

	if global := deps.FS.LegacyFlatConfigPath(); global != "" {
		if ok, _ := deps.FS.Exists(global); ok {
			candidates = append(candidates, global)
		}
	}

	if cwd, err := deps.FS.Cwd(); err == nil {
		if dir, found, werr := deps.FS.WalkUp(cwd, legacyFolderConfigName); werr == nil && found {
			candidates = append(candidates, dir.Join(legacyFolderConfigName))
		}
	}

	switch len(candidates) {
	case 0:
		return "", domain.NewOpError("import_legacy_config.discover", domain.CodeLegacyConfigNotFound, "", "", nil)
	case 1:
		return candidates[0], nil
	default:
		opts := make([]ports.Option, len(candidates))
		for i, c := range candidates {
			opts[i] = ports.Option{Raw: string(c)}
		}
		pick, err := deps.Prompter.Choose(ctx, ports.ChoiceField{
			Field:   ports.Field{Label: messages.WizardImportSourceChoice, Help: messages.WizardImportSourceChoiceHelp},
			Options: opts,
		})
		if err != nil {
			return "", err
		}
		return candidates[pick], nil
	}
}

// validateLegacyProjects joins each parsed project name onto projectsRoot
// and checks it is a usable main clone before it may be registered (spec:
// "Validate imported projects"). Any failure skips only that one name,
// with a reason, exactly like RunProjectWizard's own scanCandidates never
// aborts a whole scan over one bad candidate.
func validateLegacyProjects(ctx context.Context, deps ImportLegacyContextDeps, projectsRoot domain.Path, names []string) ([]domain.Project, []SkippedProject) {
	if projectsRoot == "" {
		skipped := make([]SkippedProject, len(names))
		for i, name := range names {
			skipped[i] = SkippedProject{Name: name, Reason: messages.ImportProjectNoProjectsRoot}
		}
		return nil, skipped
	}

	var registered []domain.Project
	var skipped []SkippedProject
	for _, name := range names {
		dir := projectsRoot.Join(name)

		exists, err := deps.FS.Exists(dir)
		if err != nil {
			skipped = append(skipped, SkippedProject{Name: name, Reason: messages.ImportProjectGitCheckFailed, Detail: err.Error()})
			continue
		}
		if !exists {
			skipped = append(skipped, SkippedProject{Name: name, Reason: messages.ImportProjectMissing})
			continue
		}

		ok, err := deps.Git.IsMainClone(ctx, dir)
		if err != nil {
			if domain.Code(err) == domain.CodeNotAMainClone {
				skipped = append(skipped, SkippedProject{Name: name, Reason: messages.ImportProjectLinkedWorktree})
				continue
			}
			skipped = append(skipped, SkippedProject{Name: name, Reason: messages.ImportProjectGitCheckFailed, Detail: err.Error()})
			continue
		}
		if !ok {
			skipped = append(skipped, SkippedProject{Name: name, Reason: messages.ImportProjectNotAGitRepo})
			continue
		}

		registered = append(registered, domain.Project{Key: domain.ProjectKey(name), SourceDir: dir})
	}
	return registered, skipped
}

// importIntoNewContext is spec requirement 2's "into a new context" mode:
// a name is always asked for (never derived silently), an existing name
// is refused with a chance to pick another, and every remaining field
// goes through the exact same promptContextFieldsOnly sequence "context
// create"/"context edit" already use, prefilled from parsed.Context (spec
// requirement 3 — "do not grow a second prefill path").
func importIntoNewContext(ctx context.Context, deps ImportLegacyContextDeps, cliName string, sourcePath domain.Path, parsed domain.LegacyFlatConfig, registered []domain.Project, skipped []SkippedProject, unmapped, fieldIssues []domain.LegacyIssue) (ImportLegacyContextResult, error) {
	name, err := resolveNewContextName(ctx, deps, cliName, suggestLegacyContextName(sourcePath))
	if err != nil {
		return ImportLegacyContextResult{}, err
	}

	c, err := promptContextFieldsOnly(ctx, deps.Prompter, name, parsed.Context)
	if err != nil {
		return ImportLegacyContextResult{}, err
	}
	// promptContextFieldsOnly has no prompt for branch_prefix or
	// env_prune_dirs at all (a pre-existing gap it shares with every one
	// of its other callers, "context create" and "context edit" — those
	// two Options fields have simply never had a wizard question of their
	// own), so its own return value always drops them regardless of what
	// defaults carried in. Restoring them here from the legacy source's
	// own successfully-parsed values is what actually honors this
	// change's own mapping table ("branch_prefix -> Defaults.BranchPrefix",
	// "env_prune_dirs -> Defaults.EnvPruneDirs") — without it, an import
	// would silently lose both fields even though the parser read them
	// correctly, which is exactly the kind of silent drop spec
	// requirement 3 rules out. This is not a second prefill mechanism: it
	// never overrides anything the wizard itself collected, only fills in
	// the two fields the wizard structurally cannot ask about.
	if parsed.Context.Defaults.BranchPrefix != nil {
		c.Defaults.BranchPrefix = parsed.Context.Defaults.BranchPrefix
	}
	if parsed.Context.Defaults.EnvPruneDirs != nil {
		c.Defaults.EnvPruneDirs = parsed.Context.Defaults.EnvPruneDirs
	}
	c.Projects = registered

	if err := deps.Store.SaveContext(ctx, c); err != nil {
		return ImportLegacyContextResult{}, err
	}

	claim := claimAfterImport(ctx, deps, c)
	return ImportLegacyContextResult{
		Claim:            claim,
		Adoption:         adoptAfterImport(ctx, deps, c),
		SourcePath:       sourcePath,
		ContextName:      c.Name,
		Created:          true,
		ImportedProjects: projectKeys(registered),
		SkippedProjects:  skipped,
		UnmappedKeys:     unmapped,
		FieldIssues:      fieldIssues,
	}, nil
}

// resolveNewContextName picks the new context's name: cliName when it is
// both syntactically valid and not already registered; otherwise (empty,
// invalid, or taken) the user is prompted, with suggestion as the first
// prompt's Default, and re-prompted (with the just-rejected name warned
// about through Reporter, when one is set) until an available name is
// entered. This never overwrites an existing context (spec: "An existing
// name is refused with a chance to pick another — never an overwrite").
func resolveNewContextName(ctx context.Context, deps ImportLegacyContextDeps, cliName string, suggestion domain.ContextName) (domain.ContextName, error) {
	candidate := domain.ContextName(cliName)
	if candidate != "" {
		if err := validateContextName(cliName); err != nil {
			candidate = ""
		}
	}

	for {
		if candidate == "" {
			n, err := promptContextName(ctx, deps.Prompter, suggestion)
			if err != nil {
				return "", err
			}
			candidate = n
		}

		_, loadErr := deps.Store.LoadContext(ctx, candidate)
		if loadErr == nil {
			if deps.Reporter != nil {
				deps.Reporter.Warn(messages.WizardImportNameTaken, string(candidate))
			}
			candidate = ""
			suggestion = ""
			continue
		}
		if domain.Code(loadErr) != domain.CodeContextNotFound {
			return "", loadErr
		}
		return candidate, nil
	}
}

// suggestLegacyContextName derives an editable name suggestion from the
// source file's own path (spec: "derived from the source file name/parent
// dir, but still editable"). A source literally named "config" or
// "ws.config" (both of this change's own two discovered locations) carries
// no useful name of its own, so the parent directory's name is used
// instead; any other filename (an arbitrary --from path) has its
// extension stripped.
func suggestLegacyContextName(p domain.Path) domain.ContextName {
	base := p.Base()
	if base == "config" || base == legacyFolderConfigName {
		if parent := p.Join("..").Base(); parent != "" && parent != "." && parent != "/" {
			return domain.ContextName(parent)
		}
		return ""
	}
	if ext := path.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return domain.ContextName(base)
}

// importIntoExistingContext is spec requirement 2's "into an existing
// context" mode: no name is asked (TargetName is already resolved), a
// diff of what would change is built, and nothing is written unless the
// user confirms it (spec: "A declined confirm writes nothing").
func importIntoExistingContext(ctx context.Context, deps ImportLegacyContextDeps, name domain.ContextName, sourcePath domain.Path, parsed domain.LegacyFlatConfig, registered []domain.Project, skipped []SkippedProject, unmapped, fieldIssues []domain.LegacyIssue) (ImportLegacyContextResult, error) {
	existing, err := deps.Store.LoadContext(ctx, name)
	if err != nil {
		return ImportLegacyContextResult{}, err
	}

	changes, newProjects, keptProjects := diffLegacyImport(existing, parsed.Context, registered)

	confirmed, err := deps.Prompter.Confirm(ctx, ports.ConfirmField{
		Field:   ports.Field{Label: messages.WizardImportConfirmDiff, Args: []any{renderImportDiff(changes, newProjects, keptProjects)}},
		Default: false,
	})
	if err != nil {
		return ImportLegacyContextResult{}, err
	}

	result := ImportLegacyContextResult{
		SourcePath:       sourcePath,
		ContextName:      name,
		Created:          false,
		ImportedProjects: projectKeys(newProjects),
		SkippedProjects:  skipped,
		UnmappedKeys:     unmapped,
		FieldIssues:      fieldIssues,
		FieldChanges:     changes,
		ProjectsKept:     keptProjects,
	}
	if !confirmed {
		result.Cancelled = true
		return result, nil
	}

	merged := mergeLegacyImport(existing, parsed.Context, registered)
	if err := deps.Store.SaveContext(ctx, merged); err != nil {
		return ImportLegacyContextResult{}, err
	}
	result.Claim = claimAfterImport(ctx, deps, merged)
	result.Adoption = adoptAfterImport(ctx, deps, merged)
	return result, nil
}

// claimAfterImport claims, for the context an import just wrote, every
// orphaned workspace under its WorkspacesRoot (a manifest whose owner
// context no longer exists, e.g. after a rename by an older version), so
// re-importing recovers workspaces that vanished from the listing. The
// context is already saved, so a failure never fails the import: it is
// reported as one skipped entry for the whole WorkspacesRoot.
func claimAfterImport(ctx context.Context, deps ImportLegacyContextDeps, c domain.Context) ClaimWorkspacesResult {
	res, err := ClaimWorkspaces(ctx, Deps{Store: deps.Store, Git: deps.Git, FS: deps.FS, Reporter: deps.Reporter}, ClaimWorkspacesInput{Context: c})
	if err != nil {
		return ClaimWorkspacesResult{Skipped: []SkippedClaim{{Name: string(c.WorkspacesRoot), Root: c.WorkspacesRoot, Reason: messages.AdoptSkipScanFailed, Detail: err.Error()}}}
	}
	return res
}

// adoptAfterImport runs AdoptLegacyWorkspaces for the context an import
// just wrote, so the workspaces the legacy tool already created show up.
// The context is already saved, so a scan failure never fails the import:
// it is reported as one skipped entry for the whole WorkspacesRoot.
func adoptAfterImport(ctx context.Context, deps ImportLegacyContextDeps, c domain.Context) AdoptLegacyWorkspacesResult {
	res, err := AdoptLegacyWorkspaces(ctx, Deps{Store: deps.Store, Git: deps.Git, FS: deps.FS, Reporter: deps.Reporter}, AdoptLegacyWorkspacesInput{Context: c})
	if err != nil {
		return AdoptLegacyWorkspacesResult{Skipped: []SkippedLegacyWorkspace{{Root: c.WorkspacesRoot, Reason: messages.AdoptSkipScanFailed, Detail: err.Error()}}}
	}
	return res
}

// diffLegacyImport computes what an existing-context import would change:
// every option field the incoming, successfully-parsed value differs from
// the current one, which project keys are genuinely new, and which are
// already registered (spec: "which Defaults/root values would be
// overwritten... which projects would be added, which are already
// registered").
func diffLegacyImport(existing, incoming domain.Context, registered []domain.Project) (changes []ImportedFieldChange, newProjects []domain.Project, keptKeys []string) {
	existingKeys := make(map[domain.ProjectKey]bool, len(existing.Projects))
	for _, p := range existing.Projects {
		existingKeys[p.Key] = true
	}
	for _, p := range registered {
		if existingKeys[p.Key] {
			keptKeys = append(keptKeys, string(p.Key))
		} else {
			newProjects = append(newProjects, p)
		}
	}

	addChange := func(field, current, next string) {
		if current != next {
			changes = append(changes, ImportedFieldChange{Field: field, Current: current, Incoming: next})
		}
	}

	if incoming.WorkspacesRoot != "" {
		addChange("workspaces_root", string(existing.WorkspacesRoot), string(incoming.WorkspacesRoot))
	}
	if incoming.ProjectsRoot != "" {
		addChange("projects_root", string(existing.ProjectsRoot), string(incoming.ProjectsRoot))
	}
	if incoming.Defaults.BaseBranch != nil {
		cur := ""
		if existing.Defaults.BaseBranch != nil {
			cur = string(*existing.Defaults.BaseBranch)
		}
		addChange("base_branch", cur, string(*incoming.Defaults.BaseBranch))
	}
	if incoming.Defaults.BranchPrefix != nil {
		cur := ""
		if existing.Defaults.BranchPrefix != nil {
			cur = *existing.Defaults.BranchPrefix
		}
		addChange("branch_prefix", cur, *incoming.Defaults.BranchPrefix)
	}
	if incoming.Defaults.CopyEnv != nil {
		cur := ""
		if existing.Defaults.CopyEnv != nil {
			cur = boolString(*existing.Defaults.CopyEnv)
		}
		addChange("copy_env_default", cur, boolString(*incoming.Defaults.CopyEnv))
	}
	if incoming.Defaults.FetchBeforeCreate != nil {
		cur := ""
		if existing.Defaults.FetchBeforeCreate != nil {
			cur = boolString(*existing.Defaults.FetchBeforeCreate)
		}
		addChange("fetch_before_create", cur, boolString(*incoming.Defaults.FetchBeforeCreate))
	}
	if incoming.Defaults.EnvPruneDirs != nil {
		addChange("env_prune_dirs", strings.Join(existing.Defaults.EnvPruneDirs, ","), strings.Join(incoming.Defaults.EnvPruneDirs, ","))
	}

	return changes, newProjects, keptKeys
}

func boolString(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// renderImportDiff renders diffLegacyImport's result as the single
// argument WizardImportConfirmDiff's %[1]s carries, entirely through
// messages.T so every line is still catalog-driven prose, never a literal
// internal/app invents (design.md §10's spirit, even though R8's lint
// itself only scans internal/cli).
func renderImportDiff(changes []ImportedFieldChange, newProjects []domain.Project, keptKeys []string) string {
	var lines []string
	if len(changes) == 0 {
		lines = append(lines, messages.T(messages.ImportDiffNoFieldChange))
	}
	for _, c := range changes {
		lines = append(lines, messages.T(messages.ImportDiffFieldChange, c.Field, c.Current, c.Incoming))
	}
	if len(newProjects) > 0 {
		lines = append(lines, messages.T(messages.ImportDiffProjectsToAdd, strings.Join(projectKeys(newProjects), ", ")))
	}
	if len(keptKeys) > 0 {
		lines = append(lines, messages.T(messages.ImportDiffProjectsKept, strings.Join(keptKeys, ", ")))
	}
	return strings.Join(lines, "\n")
}

// mergeLegacyImport applies incoming's successfully-parsed fields onto
// existing, overwriting only what incoming actually set (an absent field
// never clobbers an existing value — spec requirement 3 applies here
// exactly as it does to the new-context wizard path), and appends
// registered projects that are not already present by key.
func mergeLegacyImport(existing, incoming domain.Context, registered []domain.Project) domain.Context {
	merged := existing
	if incoming.WorkspacesRoot != "" {
		merged.WorkspacesRoot = incoming.WorkspacesRoot
	}
	if incoming.ProjectsRoot != "" {
		merged.ProjectsRoot = incoming.ProjectsRoot
	}
	if incoming.Defaults.BaseBranch != nil {
		merged.Defaults.BaseBranch = incoming.Defaults.BaseBranch
	}
	if incoming.Defaults.BranchPrefix != nil {
		merged.Defaults.BranchPrefix = incoming.Defaults.BranchPrefix
	}
	if incoming.Defaults.CopyEnv != nil {
		merged.Defaults.CopyEnv = incoming.Defaults.CopyEnv
	}
	if incoming.Defaults.FetchBeforeCreate != nil {
		merged.Defaults.FetchBeforeCreate = incoming.Defaults.FetchBeforeCreate
	}
	if incoming.Defaults.EnvPruneDirs != nil {
		merged.Defaults.EnvPruneDirs = incoming.Defaults.EnvPruneDirs
	}

	existingKeys := make(map[domain.ProjectKey]bool, len(existing.Projects))
	for _, p := range existing.Projects {
		existingKeys[p.Key] = true
	}
	for _, p := range registered {
		if !existingKeys[p.Key] {
			merged.Projects = append(merged.Projects, p)
		}
	}
	return merged
}

// RenderImportSummary renders result into the exact lines both the CLI's
// "context import" command and the tray's "Import context…" outcome
// dialog print (spec: "A summary at the end... driven by one shared
// result struct... do not build the summary text twice") — every line
// goes through messages.T, so this is the only place that text is ever
// assembled.
func RenderImportSummary(result ImportLegacyContextResult) []string {
	var lines []string

	switch {
	case result.Cancelled:
		return append(lines, messages.T(messages.ImportSummaryCancelled))
	case result.Created:
		lines = append(lines, messages.T(messages.ImportSummaryCreated, string(result.ContextName), string(result.SourcePath)))
	default:
		lines = append(lines, messages.T(messages.ImportSummaryMerged, string(result.SourcePath), string(result.ContextName)))
	}

	if len(result.ImportedProjects) > 0 {
		lines = append(lines, messages.T(messages.ImportSummaryImportedHeader))
		for _, name := range result.ImportedProjects {
			lines = append(lines, messages.T(messages.ImportSummaryImportedRow, name))
		}
	}

	if len(result.SkippedProjects) > 0 {
		lines = append(lines, messages.T(messages.ImportSummarySkippedHeader))
		for _, s := range result.SkippedProjects {
			lines = append(lines, messages.T(messages.ImportSummarySkippedRow, s.Name, skippedProjectReasonText(s)))
		}
	}

	var attention []domain.LegacyIssue
	attention = append(attention, result.FieldIssues...)
	attention = append(attention, result.UnmappedKeys...)
	if len(attention) > 0 {
		lines = append(lines, messages.T(messages.ImportSummaryAttentionHeader))
		for _, issue := range attention {
			lines = append(lines, messages.T(messages.ImportSummaryAttentionRow, issue.Key, legacyReasonText(issue)))
		}
	}

	if len(result.Claim.Claimed) > 0 || len(result.Claim.Skipped) > 0 {
		lines = append(lines, RenderClaimSummary(result.Claim)...)
	}

	if len(result.Adoption.Adopted) > 0 || len(result.Adoption.Skipped) > 0 {
		lines = append(lines, RenderAdoptSummary(result.Adoption)...)
	}

	return lines
}

// skippedProjectReasonText renders a SkippedProject's reason, passing
// Detail as an argument only for the one reason key whose catalog
// template actually has a placeholder for it (ImportProjectGitCheckFailed)
// — every other reason key is a complete, self-contained sentence with no
// placeholder, and messages.T would otherwise append fmt's "%!(EXTRA ...)"
// noise for an argument the template never consumes.
func skippedProjectReasonText(s SkippedProject) string {
	if s.Reason == messages.ImportProjectGitCheckFailed {
		return messages.T(s.Reason, s.Detail)
	}
	return messages.T(s.Reason)
}

// legacyReasonKey bridges domain.LegacyReason to its catalog Key, the same
// ForCode-style bridge every domain.ErrCode already has (ForCode itself is
// not reused here since LegacyReason is a distinct typed code, never an
// ErrCode).
func legacyReasonKey(r domain.LegacyReason) messages.Key {
	switch r {
	case domain.LegacyReasonUnknownKey:
		return messages.LegacyReasonUnknownKey
	case domain.LegacyReasonNotImported:
		return messages.LegacyReasonNotImported
	case domain.LegacyReasonInvalidValue:
		return messages.LegacyReasonInvalidValue
	case domain.LegacyReasonMalformedLine:
		return messages.LegacyReasonMalformedLine
	case domain.LegacyReasonMissingProjectsSection:
		return messages.LegacyReasonMissingProjectsSection
	case domain.LegacyReasonMissingReposSection:
		return messages.LegacyReasonMissingReposSection
	default:
		return messages.ErrUnknown
	}
}

// legacyReasonText mirrors skippedProjectReasonText for a domain.LegacyIssue:
// only LegacyReasonInvalidValue's catalog template has a placeholder for
// Detail (the raw value that failed).
func legacyReasonText(issue domain.LegacyIssue) string {
	key := legacyReasonKey(issue.Reason)
	if issue.Reason == domain.LegacyReasonInvalidValue {
		return messages.T(key, issue.Detail)
	}
	return messages.T(key)
}

func projectKeys(projects []domain.Project) []string {
	if len(projects) == 0 {
		return nil
	}
	out := make([]string, len(projects))
	for i, p := range projects {
		out[i] = string(p.Key)
	}
	return out
}
