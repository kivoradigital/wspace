// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package messages

// Key is a typed message identifier. A caller cannot pass a raw string
// without an explicit conversion, which a later lint test (R8, phase 4b)
// flags. The full catalog is populated in phase 4b; this file only
// declares the keys phase 1's types reference.
type Key string

const (
	WorkspaceCreated   Key = "workspace.created"
	WorkspaceDestroyed Key = "workspace.destroyed"
	EnvCopyNotIgnored  Key = "env.copy_not_ignored"
	// BaseBranchFallback warns that a project lacks its configured base
	// branch and its remote default branch is used instead.
	BaseBranchFallback Key = "workspace.base_branch_fallback"
	// FetchSkippedNoRemote warns that a project has no such remote, so the
	// fetch before creating its worktree is skipped.
	FetchSkippedNoRemote Key = "workspace.fetch_skipped_no_remote"
	// BaseBranchMissing warns that no base branch was found in a project,
	// so its worktree starts from the clone's current HEAD.
	BaseBranchMissing   Key = "workspace.base_branch_missing"
	ErrBranchCheckedOut Key = "err.branch_checked_out"
	WizardPickProjects  Key = "wizard.pick_projects"

	// WorkspaceCreateProject announces one project's worktree creation as
	// app.CreateWorkspace's own mutation loop reaches it — the fetch/
	// worktree-add work each project needs is seconds of real work, and a
	// caller driving a progress view (internal/gui/wizard's project
	// window) needs one Step per project to render as that work actually
	// happens, exactly like app.Exec already announces each repo through
	// the identically-shaped CLIExecRepoHeader.
	WorkspaceCreateProject Key = "workspace.create_project"

	// Context wizard fields (phase 3, app.RunContextWizard).
	WizardContextName           Key = "wizard.context_name"
	WizardWorkspacesRoot        Key = "wizard.workspaces_root"
	WizardProjectsRoot          Key = "wizard.projects_root"
	WizardBaseBranch            Key = "wizard.base_branch"
	WizardCopyEnvDefault        Key = "wizard.copy_env_default"
	WizardFetchBeforeCreate     Key = "wizard.fetch_before_create"
	WizardCopyEnvDefaultHelp    Key = "wizard.copy_env_default_help"
	WizardFetchBeforeCreateHelp Key = "wizard.fetch_before_create_help"
	WizardIgnorePatterns        Key = "wizard.ignore_patterns"

	// *Help keys carry the explanation, optionality note, example and/or
	// available-variable list that used to be crammed into the label
	// itself (this change's own fix: widget.Form sizes its label column to
	// the widest label, so one paragraph-length label pushed every field
	// far to the right and forced a window roughly 2000 points wide). The
	// label stays a short noun phrase; ports.Field.Help carries the rest,
	// rendered as a wrapped caption under the field on the GUI
	// (internal/adapters/formprompt's formItem) and appended in
	// parentheses on the terminal (internal/adapters/termprompt's
	// printLabel) — the mechanism already existed on ports.Field, it was
	// simply never populated before this change.
	WizardWorkspacesRootHelp Key = "wizard.workspaces_root_help"
	WizardProjectsRootHelp   Key = "wizard.projects_root_help"
	WizardIgnorePatternsHelp Key = "wizard.ignore_patterns_help"

	// Project wizard fields (phase 3, app.RunProjectWizard). Note:
	// wizard.scan_projects was declared through phase 3 but never wired to
	// any prompt (RunProjectWizard scans automatically whenever
	// ProjectsRoot is set — phase 3's own recorded decision) — removed here
	// per phase 3's own handoff instruction to wire it or drop it.
	WizardProjectKey           Key = "wizard.project_key"
	WizardAddProjectManually   Key = "wizard.add_project_manually"
	WizardProjectSourceDir     Key = "wizard.project_source_dir"
	WizardProjectSourceDirHelp Key = "wizard.project_source_dir_help"

	// WizardSkippedLinkedWorktree reports one scanned candidate skipped
	// during project discovery because it is a linked worktree, not a main
	// clone (project-configuration spec: "Main-clone detection"). This is
	// a per-candidate notice, never a scan-aborting error — see
	// internal/app/project_wizard.go's scanCandidates.
	WizardSkippedLinkedWorktree Key = "wizard.skipped_linked_worktree"

	// WizardScanDepthLimitReached reports that the recursive project-
	// discovery scan (internal/app/project_wizard.go's scanCandidates)
	// stopped descending at its own depth cap while at least one directory
	// below it was still left unexplored — a partial scan must say so,
	// never present itself as a complete one (this change's own recursive-
	// discovery fix). %[1]s is the projects root scanned, %[2]d the depth
	// cap that was reached.
	WizardScanDepthLimitReached Key = "wizard.scan_depth_limit_reached"

	// WizardManualSourceNotAMainClone rejects one manually-typed source
	// directory that is not a main clone (a linked worktree, a bare repo,
	// or an ordinary folder), re-prompting rather than registering it —
	// the manual-registration counterpart of WizardSkippedLinkedWorktree
	// above (project-configuration spec: "Main-clone detection" applies to
	// a manually-typed source exactly as much as a scanned one; see
	// internal/app/project_wizard.go's collectManualProjects).
	WizardManualSourceNotAMainClone Key = "wizard.manual_source_not_a_main_clone"

	// Per-project fields (this change's own authorized gap-closure):
	// origin_branch, dest_branch and worktree_dir were declared on
	// domain.Project and fully resolved by domain.Resolver from the
	// original design onward, but RunProjectWizard never prompted for any
	// of the three — only Key and SourceDir ever were, for either the
	// scanned-path or the manual-registration flow.
	WizardProjectOriginBranch     Key = "wizard.project_origin_branch"
	WizardProjectDestBranch       Key = "wizard.project_dest_branch"
	WizardProjectWorktreeDir      Key = "wizard.project_worktree_dir"
	WizardProjectOriginBranchHelp Key = "wizard.project_origin_branch_help"
	WizardProjectDestBranchHelp   Key = "wizard.project_dest_branch_help"
	WizardProjectWorktreeDirHelp  Key = "wizard.project_worktree_dir_help"

	// Create-workspace wizard fields (app.RunCreateWorkspace, "New
	// workspace…"): a workspace name, an optional branch override (with a
	// live preview of what an empty answer resolves to, exactly like
	// WizardProjectOriginBranch's own preview), and the multi-choice
	// picker over the active context's already-registered projects —
	// never a filesystem scan, unlike WizardPickProjects above, which
	// picks from freshly scanned candidates during project registration.
	WizardWorkspaceName       Key = "wizard.workspace_name"
	WizardWorkspaceBranch     Key = "wizard.workspace_branch"
	WizardWorkspaceBranchHelp Key = "wizard.workspace_branch_help"
	WizardWorkspaceProjects   Key = "wizard.workspace_projects"

	// internal/gui/wizard's own progress view for "New workspace…":
	// app.CreateWorkspace's Reporter calls, plus a final success/failure
	// line, are rendered here while the window stays open and visible
	// (product-owner report: "no loading, no progress" — the window used
	// to simply close with nothing shown). WizardCreateWorkspaceInProgress
	// is the view's own header, shown from the moment the first Reporter
	// call arrives; WizardCreateWorkspaceSucceeded/Failed are its two
	// terminal outcomes, %[1]s carrying the workspace name (succeeded, so
	// the user can find what was just made) or the error text (failed).
	WizardCreateWorkspaceInProgress Key = "wizard.create_workspace_in_progress"
	WizardCreateWorkspaceSucceeded  Key = "wizard.create_workspace_succeeded"
	WizardCreateWorkspaceFailed     Key = "wizard.create_workspace_failed"

	// Phase 4a use-case results and warnings (internal/app).
	RepoAdded                  Key = "repo.added"
	RepoRemoved                Key = "repo.removed"
	WorktreeRecreated          Key = "workspace.worktree_recreated"
	EnvSynced                  Key = "env.synced"
	GitVersionTooOld           Key = "doctor.git_version_too_old"
	GitMissing                 Key = "doctor.git_missing"
	WorktreeRegistrationPruned Key = "doctor.worktree_registration_pruned"
	TeardownBlockedChange      Key = "teardown.blocked_change"
	TeardownBlockedUnpushed    Key = "teardown.blocked_unpushed"
	OpWarning                  Key = "op.warning"
	RollbackStepFailed         Key = "rollback.step_failed"

	// ErrCode -> Key bridge (ForCode, design.md §10: "the single bridge").
	// One key per domain.ErrCode, plus ErrUnknown for any code ForCode does
	// not recognize (including the empty ErrCode of a non-OpError).
	ErrGitMissing        Key = "err.git_missing"
	ErrGitTooOld         Key = "err.git_too_old"
	ErrGitFailed         Key = "err.git_failed"
	ErrNotAMainClone     Key = "err.not_a_main_clone"
	ErrRefNotFound       Key = "err.ref_not_found"
	ErrWorktreeExists    Key = "err.worktree_exists"
	ErrWorktreeMissing   Key = "err.worktree_missing"
	ErrWorktreeDirty     Key = "err.worktree_dirty"
	ErrNoContext         Key = "err.no_context"
	ErrContextNotFound   Key = "err.context_not_found"
	ErrWorkspaceExists   Key = "err.workspace_exists"
	ErrWorkspaceNotFound Key = "err.workspace_not_found"
	ErrOverlayScope      Key = "err.overlay_scope_violation"
	ErrUnsafeTeardown    Key = "err.unsafe_teardown"
	ErrTimeout           Key = "err.timeout"
	ErrSchemaUnsupported Key = "err.schema_unsupported"
	ErrExecFailed        Key = "err.exec_failed"
	ErrContextActive     Key = "err.context_active"
	ErrUnknown           Key = "err.unknown"

	// CLI output (phase 4b: internal/cli renderers, JSON contract fields
	// aside — see cli-surface spec's --json schemas for field names, which
	// are data keys, not localizable prose).
	CLIJSONUnsupported Key = "cli.json_unsupported"
	CLINoContext       Key = "cli.no_context_hint"

	CLIListEmpty      Key = "cli.list.empty"
	CLIListRow        Key = "cli.list.row"
	CLIListRowDamaged Key = "cli.list.row_damaged"
	// CLIListRowLegacy renders a directory only the legacy bash tool
	// manages (".ws/workspace.conf", no wspace manifest yet).
	CLIListRowLegacy Key = "cli.list.row_legacy"
	// CLIListRowOrphan renders a workspace whose manifest names a context
	// that no longer exists (claimable with "wspace claim").
	CLIListRowOrphan Key = "cli.list.row_orphan"
	CLIStatusRow     Key = "cli.status.row"
	// CLIStatusBaseMissing follows a status row whose repo lacks its
	// comparison base, so its ahead/behind counts are unknown.
	CLIStatusBaseMissing Key = "cli.status.base_missing"
	CLIStatusDirty       Key = "cli.status.dirty"
	CLIStatusClean       Key = "cli.status.clean"

	CLIInfoContext   Key = "cli.info.context"
	CLIInfoConfigDir Key = "cli.info.config_dir"
	CLIInfoOptionRow Key = "cli.info.option_row"

	CLIConfirmDestroy Key = "cli.confirm.destroy"
	// CLIConfirmDestroyTarget names the workspace in the destroy prompt's help.
	CLIConfirmDestroyTarget Key = "cli.confirm.destroy_target"

	CLIShellInitUsage Key = "cli.shell_init.usage"
	CLIExecNoCommand  Key = "cli.exec.no_command"
	CLIVersionLine    Key = "cli.version.line"

	CLIPartialCreateGuidance Key = "cli.partial_create_guidance"

	CLIContextCreated      Key = "cli.context.created"
	CLIContextSwitched     Key = "cli.context.switched"
	CLIContextListEmpty    Key = "cli.context.list_empty"
	CLIContextListRow      Key = "cli.context.list_row"
	CLIProjectListEmpty    Key = "cli.project.list_empty"
	CLIProjectListRow      Key = "cli.project.list_row"
	CLIContextActiveMarker Key = "cli.context.active_marker"
	CLIContextEdited       Key = "cli.context.edited"
	// CLIContextReassigned / CLIContextReassignFailed follow a rename: the
	// workspaces whose manifests were moved to the new name, and each one
	// that could not be.
	CLIContextReassigned     Key = "cli.context.reassigned"
	CLIContextReassignFailed Key = "cli.context.reassign_failed"
	CLIContextRemoved        Key = "cli.context.removed"

	CLIDoctorPruned Key = "cli.doctor.pruned"

	CLIExecExitNonZero Key = "cli.exec.exit_non_zero"
	CLIExecRepoHeader  Key = "cli.exec.repo_header"

	CLIJumpTTYNote Key = "cli.jump.tty_note"

	// `wspace update`: one line per repository, then a failure count.
	CLIRepoUpdateIntegrated         Key = "cli.repo_update.integrated"
	CLIRepoUpdateUpToDate           Key = "cli.repo_update.up_to_date"
	CLIRepoUpdateDirty              Key = "cli.repo_update.dirty"
	CLIRepoUpdateConflict           Key = "cli.repo_update.conflict"
	CLIRepoUpdateConflictUnverified Key = "cli.repo_update.conflict_unverified"
	CLIRepoUpdateFailed             Key = "cli.repo_update.failed"
	CLIRepoUpdateSomeFailed         Key = "cli.repo_update.some_failed"
	CLIRepoUpdateNoWorkspace        Key = "cli.repo_update.no_workspace"
	CLIRepoBranch                   Key = "cli.repo.branch"
	CLIRepoDetached                 Key = "cli.repo.detached"
	CLIRepoUpstream                 Key = "cli.repo.upstream"
	CLIRepoUpstreamGone             Key = "cli.repo.upstream_gone"
	CLIRepoNoUpstream               Key = "cli.repo.no_upstream"
	CLIRepoBase                     Key = "cli.repo.base"
	CLIRepoBaseUnknown              Key = "cli.repo.base_unknown"
	CLIRepoBaseMissing              Key = "cli.repo.base_missing"
	CLIRepoLastFetch                Key = "cli.repo.last_fetch"
	CLIRepoNeverFetched             Key = "cli.repo.never_fetched"
	CLIRepoSection                  Key = "cli.repo.section"
	CLIRepoStaged                   Key = "cli.repo.staged"
	CLIRepoUnstaged                 Key = "cli.repo.unstaged"
	CLIRepoUntracked                Key = "cli.repo.untracked"
	CLIRepoConflicted               Key = "cli.repo.conflicted"
	CLIRepoClean                    Key = "cli.repo.clean"
	CLIRepoStashCount               Key = "cli.repo.stash_count"
	CLIRepoCommitsHeader            Key = "cli.repo.commits_header"
	CLIRepoMoreCommits              Key = "cli.repo.more_commits"
	CLIRepoNoStashes                Key = "cli.repo.no_stashes"
	CLIRepoBinary                   Key = "cli.repo.binary"
	CLIRepoTruncated                Key = "cli.repo.truncated"
	CLIRepoNoDiff                   Key = "cli.repo.no_diff"
	CLIRepoUntrackedNote            Key = "cli.repo.untracked_note"
	CLIRepoFetched                  Key = "cli.repo.fetched"
	CLIRepoPulled                   Key = "cli.repo.pulled"
	CLIRepoPullUpToDate             Key = "cli.repo.pull_up_to_date"
	CLIRepoPullDirty                Key = "cli.repo.pull_dirty"
	CLIRepoPullDiverged             Key = "cli.repo.pull_diverged"
	CLIRepoPullFailed               Key = "cli.repo.pull_failed"
	CLIRepoUnknownAction            Key = "cli.repo.unknown_action"
	CLIRepoMissingArg               Key = "cli.repo.missing_arg"
	CLIRepoStashApplied             Key = "cli.repo.stash_applied"
	CLIRepoStashPopped              Key = "cli.repo.stash_popped"
	CLIRepoStashIndexNotRestored    Key = "cli.repo.stash_index_not_restored"
	CLIRepoStashKept                Key = "cli.repo.stash_kept"
	CLIRepoStashConflicts           Key = "cli.repo.stash_conflicts"
	CLIRepoStashDirty               Key = "cli.repo.stash_dirty"
	CLIRepoStashFailed              Key = "cli.repo.stash_failed"
	CLIRepoStashDropped             Key = "cli.repo.stash_dropped"
	CLIRepoCleaned                  Key = "cli.repo.cleaned"
	CLIRepoCleanKept                Key = "cli.repo.clean_kept"
	CLIConfirmStashDrop             Key = "cli.confirm.stash_drop"
	CLIConfirmStashDropHelp         Key = "cli.confirm.stash_drop_help"
	CLIConfirmClean                 Key = "cli.confirm.clean"
	CLIConfirmCleanHelp             Key = "cli.confirm.clean_help"
	CLIRepoBadNumber                Key = "cli.repo.bad_number"
	CLIRepoStagedPaths              Key = "cli.repo.staged_paths"
	CLIRepoUnstagedPaths            Key = "cli.repo.unstaged_paths"
	CLIRepoDiscarded                Key = "cli.repo.discarded"
	CLIRepoDiscardBackup            Key = "cli.repo.discard_backup"
	CLIConfirmDiscard               Key = "cli.confirm.discard"
	CLIConfirmDiscardHelp           Key = "cli.confirm.discard_help"
	CLIRepoCommitted                Key = "cli.repo.committed"
	CLIRepoCommitFailed             Key = "cli.repo.commit_failed"
	CLIRepoSubjectTooLong           Key = "cli.repo.subject_too_long"
	CLIRepoIdentityHelp             Key = "cli.repo.identity_help"
	CLIRepoHookOutput               Key = "cli.repo.hook_output"
	CLIRepoPushed                   Key = "cli.repo.pushed"
	CLIRepoPublished                Key = "cli.repo.published"
	CLIRepoPushUpToDate             Key = "cli.repo.push_up_to_date"
	CLIRepoPushNoUpstream           Key = "cli.repo.push_no_upstream"
	CLIRepoPushRejected             Key = "cli.repo.push_rejected"
	CLIRepoPushFailed               Key = "cli.repo.push_failed"
	CLIRepoStashCreated             Key = "cli.repo.stash_created"

	// Phase 5: install command and version --check (internal/app.Install,
	// internal/app.CheckForUpdate; internal/cli's install/version commands).
	InstallPlaced             Key = "install.placed"
	InstallManual             Key = "install.manual"
	InstallShellRCUpdated     Key = "install.shell_rc_updated"
	InstallShellRCConfirm     Key = "install.shell_rc_confirm"
	InstallUninstalled        Key = "install.uninstalled"
	InstallNothingToUninstall Key = "install.nothing_to_uninstall"
	InstallAlreadyInstalled   Key = "install.already_installed"
	InstallOnPath             Key = "install.on_path"
	InstallPathConfirm        Key = "install.path_confirm"
	InstallPathConfirmHelp    Key = "install.path_confirm_help"
	InstallShellRCConfirmHelp Key = "install.shell_rc_confirm_help"
	InstallPathUpdated        Key = "install.path_updated"
	InstallPathConfigured     Key = "install.path_configured"
	InstallPathNotOnPath      Key = "install.path_not_on_path"
	InstallOpenNewTerminal    Key = "install.open_new_terminal"
	InstallPathRemoved        Key = "install.path_removed"
	InstallPendingRemoval     Key = "install.pending_removal"

	CLIUpdateUpToDate         Key = "cli.update.up_to_date"
	CLIUpdateAvailable        Key = "cli.update.available"
	CLIUpdateCheckUnavailable Key = "cli.update.unavailable"

	// --verbose diagnostic block (internal/cli/errors.go's renderVerbose).
	// Developer-facing, not end-user prose, but still routed through the
	// catalog like everything else this binary prints (R8) — see
	// renderVerbose's comment for why that is deliberate rather than an
	// exemption.
	CLIVerboseOp      Key = "cli.verbose.op"
	CLIVerboseCode    Key = "cli.verbose.code"
	CLIVerboseDetails Key = "cli.verbose.details"
	CLIVerboseChain   Key = "cli.verbose.chain"
	CLIVerboseRaw     Key = "cli.verbose.raw"

	// TerminalPrompter's own re-prompt hints (internal/adapters/termprompt).
	// Not covered by R8's enforcement scope (internal/cli, internal/gui
	// only), but routed through the catalog anyway for the same reason
	// every other user-facing string is: one place to translate, one place
	// to review a wording change.
	TermPrompterYesNoHint       Key = "termprompter.yes_no_hint"
	TermPrompterChooseHint      Key = "termprompter.choose_hint"
	TermPrompterMultiChooseHint Key = "termprompter.multi_choose_hint"

	// Phase 6: internal/gui/tray's menu labels (tray-gui spec: "Tray
	// strings route through the message catalog"). Badge keys are pure
	// symbols/verbs (design.md §9's legend), still routed through the
	// catalog like every other tray label so a translator/theme change has
	// exactly one place to edit, matching R8's enforcement spirit even
	// though TestNoInlineUserStrings itself only scans internal/cli.
	//
	// TrayActiveContextHeader/TraySwitchContext are gone as of this
	// change's menu restructure (product-owner report #2: "contexts
	// first"): the disabled "context: x" header is redundant with the
	// checkmark the Contexts submenu already renders on the active entry,
	// and "switch context" is now just Contexts's own top-level label.
	TrayContexts            Key = "tray.contexts"
	TrayNoContexts          Key = "tray.no_contexts"
	TrayNoContextConfigured Key = "tray.no_context_configured"
	TrayWorkspaceLabel      Key = "tray.workspace_label"
	TrayWorkspaceLoadError  Key = "tray.workspace_load_error"
	TrayBadgeClean          Key = "tray.badge.clean"
	TrayBadgeDirty          Key = "tray.badge.dirty"
	TrayBadgeUnpushed       Key = "tray.badge.unpushed"
	// TrayOpenInFinder replaces TrayOpenFolder's label (product-owner
	// report #4: "jump has the options of console or Finder/Explorer" —
	// the old "open folder" label never said which surface it opened,
	// which reads as ambiguous once a second jump destination exists next
	// to it). The CommandKind it drives (CmdOpenFolder) is unchanged.
	TrayOpenInFinder   Key = "tray.open_in_finder"
	TrayOpenInTerminal Key = "tray.open_in_terminal"
	// TrayOpenTerminalUnsupported reports CmdOpenTerminal's platform seam
	// finding no terminal integration for the current OS (only macOS is
	// wired today) — a clear, Reporter-recorded message rather than a
	// silent no-op.
	TrayOpenTerminalUnsupported Key = "tray.open_terminal_unsupported"
	TrayCopyPath                Key = "tray.copy_path"
	TraySyncEnv                 Key = "tray.sync_env"
	TrayRepair                  Key = "tray.repair"
	TrayDestroy                 Key = "tray.destroy"
	TrayRefreshNow              Key = "tray.refresh_now"
	TrayQuit                    Key = "tray.quit"
	TrayBadgeDamaged            Key = "tray.badge.damaged"

	// Phase 7: internal/gui/tray's new menu entries (tasks.md 7.11-7.12)
	// and internal/gui/wizard's two windows + About dialog.
	TrayNewContext        Key = "tray.new_context"
	TrayNewWorkspace      Key = "tray.new_workspace"
	TrayAboutLabel        Key = "tray.about_label"
	TrayAboutUpdateSuffix Key = "tray.about_update_suffix"

	// Phase 8's gap closure: the "Edit context…" entry tasks.md 7.12
	// named but phase 7 left deliberately unwired (no app.RunEditContextWizard
	// existed yet — see this change's own authorized gap-closure note).
	TrayEditContext Key = "tray.edit_context"

	// TrayInitializeContext, TrayDeleteContext and its three outcomes are
	// this change's own gap-closure (product-owner report #2): "Initialize
	// context…" runs app.RunInitializeContextWizard end to end (projects
	// folder -> scan/select -> workspaces folder); "Delete context…" runs
	// app.RunDeleteContextWizard, which delegates to the existing
	// app.RemoveContext guard against removing the active context.
	TrayInitializeContext          Key = "tray.initialize_context"
	TrayDeleteContext              Key = "tray.delete_context"
	TrayContextDeleted             Key = "tray.context_deleted"
	TrayDeleteContextActiveRefusal Key = "tray.delete_context_active_refusal"
	TrayDeleteContextFailed        Key = "tray.delete_context_failed"

	// wizard.FormPrompter's own chrome (internal/adapters/formprompt),
	// shared by every field kind's rendered form.
	WizardNext                 Key = "wizard.next"
	WizardMinSelectionRequired Key = "wizard.min_selection_required"

	// WizardBack and WizardStepIndicator are Group's own multi-step chrome
	// (internal/adapters/formprompt's step_form.go): a Step whose Back is
	// true renders a Back action alongside Next, and a Step whose Total is
	// more than one renders a "Step X of Y" indicator — both withheld
	// entirely for a single-step wizard (the product layout rule: "one
	// page when the fields fit").
	WizardBack          Key = "wizard.back"
	WizardBrowse        Key = "wizard.browse"
	WizardStepIndicator Key = "wizard.step_indicator"

	// WizardSelectAll labels the select-all/deselect-all toggle every
	// multi-choice field renders above its own option list (product-owner
	// report: ticking dozens of scanned candidates one by one, or hunting
	// for the one to exclude, is not a reasonable interaction once a real
	// projects root has 50+ entries). Checked means every option is
	// currently selected; unchecked means it is not (none or some) — the
	// checkbox's own state is what conveys which, never a second label
	// string that would have to be kept in sync with it.
	WizardSelectAll Key = "wizard.select_all"

	// WizardContextToDelete is app.RunDeleteContextWizard's own single
	// Choose prompt: which registered context to remove.
	WizardContextToDelete Key = "wizard.context_to_delete"

	// The About dialog (internal/gui/wizard.ShowAbout). CLIUpdateUpToDate/
	// CLIUpdateAvailable/CLIUpdateCheckUnavailable (phase 5) are reused
	// verbatim for the update-state line — the same ReleaseChecker, the
	// same three catalog strings, never a second wording for the same
	// three outcomes.
	AboutTitle         Key = "about.title"
	AboutDismiss       Key = "about.dismiss"
	AboutVersionLine   Key = "about.version_line"
	AboutCommitLine    Key = "about.commit_line"
	AboutBuildDateLine Key = "about.build_date_line"

	// This change's own import feature: turning a legacy flat
	// "key = value" workspace configuration file into a wspace context
	// (internal/domain.ParseLegacyFlatConfig, internal/app.ImportLegacyContext).
	// ErrLegacyConfigNotFound bridges domain.CodeLegacyConfigNotFound
	// exactly like every other Err* key bridges its own ErrCode (ForCode).
	ErrLegacyConfigNotFound Key = "err.legacy_config_not_found"

	// ErrContextExists, ErrProjectExists and ErrProjectNotFound bridge the
	// codes the non-interactive context/project administration use cases
	// (app.CreateContext, app.RegisterProject, ...) raise.
	ErrContextExists   Key = "err.context_exists"
	ErrProjectExists   Key = "err.project_exists"
	ErrProjectNotFound Key = "err.project_not_found"
	// ErrRepoNotFound bridges domain.CodeRepoNotFound: a repo alias that
	// is not part of the workspace's manifest.
	ErrRepoNotFound Key = "err.repo_not_found"

	// The update use case's refusals and conflict outcome (app.UpdateRepo).
	ErrDetachedHead          Key = "err.detached_head"
	ErrIntegrationInProgress Key = "err.integration_in_progress"
	ErrBaseMissing           Key = "err.base_missing"
	ErrUpdateConflict        Key = "err.update_conflict"
	ErrNoUpstream            Key = "err.no_upstream"
	ErrDiverged              Key = "err.diverged"
	ErrPathNotChanged        Key = "err.path_not_changed"
	ErrStashChanged          Key = "err.stash_changed"
	ErrStashConflict         Key = "err.stash_conflict"
	ErrPathNotUntracked      Key = "err.path_not_untracked"
	ErrAlreadyInWorkspace    Key = "err.already_in_workspace"
	ErrPathIsUntracked       Key = "err.path_is_untracked"
	ErrStagedOnly            Key = "err.staged_only"
	ErrNothingStaged         Key = "err.nothing_staged"
	ErrIdentityMissing       Key = "err.identity_missing"
	ErrHookFailed            Key = "err.hook_failed"
	ErrPushRejected          Key = "err.push_rejected"
	ErrAuthFailed            Key = "err.auth_failed"
	ErrNothingToStash        Key = "err.nothing_to_stash"

	// WizardImportSourceChoice is shown when both a global and a
	// folder-level legacy config file are found and neither was forced by
	// --from: the user picks which one to import, never a silent guess.
	WizardImportSourceChoice     Key = "wizard.import_source_choice"
	WizardImportSourceChoiceHelp Key = "wizard.import_source_choice_help"

	// WizardImportNameTaken re-prompts for a new context's name after the
	// one just entered turned out to already be registered — an import
	// into a new context never overwrites an existing one.
	WizardImportNameTaken Key = "wizard.import_name_taken"

	// WizardImportConfirmDiff is the existing-context mode's one
	// Confirm prompt: %[1]s carries the rendered diff (built from the
	// ImportDiff* keys below, joined by ImportRenderDiff), and declining
	// writes nothing.
	WizardImportConfirmDiff Key = "wizard.import_confirm_diff"

	// ImportDiff* render one line each of the existing-context diff
	// WizardImportConfirmDiff shows before its confirm question.
	ImportDiffFieldChange   Key = "import.diff.field_change"
	ImportDiffProjectsToAdd Key = "import.diff.projects_to_add"
	ImportDiffProjectsKept  Key = "import.diff.projects_kept"
	ImportDiffNoFieldChange Key = "import.diff.no_field_change"

	// Legacy-reason keys bridge domain.LegacyReason (an unknown/unmapped
	// key, an invalid mapped value, a malformed line, or a missing
	// "[projects]" section) to catalog prose, the same ForCode-style
	// bridge every domain.ErrCode already has.
	LegacyReasonUnknownKey             Key = "legacy_reason.unknown_key"
	LegacyReasonNotImported            Key = "legacy_reason.not_imported"
	LegacyReasonInvalidValue           Key = "legacy_reason.invalid_value"
	LegacyReasonMalformedLine          Key = "legacy_reason.malformed_line"
	LegacyReasonMissingProjectsSection Key = "legacy_reason.missing_projects_section"
	LegacyReasonMissingReposSection    Key = "legacy_reason.missing_repos_section"

	// Import project-validation skip reasons (internal/app.ImportLegacyContext):
	// every [projects] name is checked against ProjectsRoot before being
	// registered, and a name that fails is skipped with one of these
	// reasons rather than silently dropped or silently registered.
	ImportProjectMissing        Key = "import.project.missing"
	ImportProjectNotAGitRepo    Key = "import.project.not_a_git_repo"
	ImportProjectLinkedWorktree Key = "import.project.linked_worktree"
	ImportProjectGitCheckFailed Key = "import.project.git_check_failed"
	ImportProjectNoProjectsRoot Key = "import.project.no_projects_root"

	// Import summary lines (internal/app.RenderImportSummary), shared
	// verbatim by the CLI and tray outcomes so the summary text is built
	// exactly once regardless of which surface renders it.
	ImportSummaryCreated         Key = "import.summary.created"
	ImportSummaryMerged          Key = "import.summary.merged"
	ImportSummaryCancelled       Key = "import.summary.cancelled"
	ImportSummaryImportedHeader  Key = "import.summary.imported_header"
	ImportSummaryImportedRow     Key = "import.summary.imported_row"
	ImportSummarySkippedHeader   Key = "import.summary.skipped_header"
	ImportSummarySkippedRow      Key = "import.summary.skipped_row"
	ImportSummaryAttentionHeader Key = "import.summary.attention_header"
	ImportSummaryAttentionRow    Key = "import.summary.attention_row"

	// Legacy workspace adoption (internal/app.AdoptLegacyWorkspaces). Each
	// skip reason template takes exactly one argument: the detail that
	// explains it (a project name, an alias, an error text, a path).
	AdoptSkipManifestUnreadable Key = "adopt.skip.manifest_unreadable"
	AdoptSkipConfUnreadable     Key = "adopt.skip.conf_unreadable"
	AdoptSkipInvalidName        Key = "adopt.skip.invalid_name"
	AdoptSkipNoRepos            Key = "adopt.skip.no_repos"
	AdoptSkipUnresolvedProject  Key = "adopt.skip.unresolved_project"
	AdoptSkipBranchUnknown      Key = "adopt.skip.branch_unknown"
	AdoptSkipBranchAmbiguous    Key = "adopt.skip.branch_ambiguous"
	AdoptSkipWriteFailed        Key = "adopt.skip.write_failed"
	AdoptSkipScanFailed         Key = "adopt.skip.scan_failed"
	// A legacy folder that already has a wspace manifest: owned by this
	// context (or by none), by another existing context (left alone), or
	// by a context that no longer exists (claimed for this one). Detail is
	// the owner context's name.
	AdoptSkipAlreadyAdopted Key = "adopt.skip.already_adopted"
	AdoptSkipOwnedByOther   Key = "adopt.skip.owned_by_other"
	AdoptSkipOrphanClaimed  Key = "adopt.skip.orphan_claimed"

	// Workspace claiming (internal/app.ClaimWorkspaces). Each skip reason
	// template takes exactly one argument (Detail).
	ClaimSkipNotFound           Key = "claim.skip.not_found"
	ClaimSkipInvalidName        Key = "claim.skip.invalid_name"
	ClaimSkipAlreadyOwned       Key = "claim.skip.already_owned"
	ClaimSkipShared             Key = "claim.skip.shared"
	ClaimSkipOwnedByOther       Key = "claim.skip.owned_by_other"
	ClaimSkipManifestUnreadable Key = "claim.skip.manifest_unreadable"
	ClaimSkipWriteFailed        Key = "claim.skip.write_failed"

	// Claim summary lines (internal/app.RenderClaimSummary), shared by
	// "wspace claim" and the import summary.
	ClaimSummaryNone          Key = "claim.summary.none"
	ClaimSummaryClaimedHeader Key = "claim.summary.claimed_header"
	ClaimSummaryClaimedRow    Key = "claim.summary.claimed_row"
	ClaimSummarySkippedHeader Key = "claim.summary.skipped_header"
	ClaimSummarySkippedRow    Key = "claim.summary.skipped_row"

	// Adoption summary lines (internal/app.RenderAdoptSummary), shared by
	// "wspace adopt-legacy" and the import summary.
	AdoptSummaryNone          Key = "adopt.summary.none"
	AdoptSummaryAdoptedHeader Key = "adopt.summary.adopted_header"
	AdoptSummaryAdoptedRow    Key = "adopt.summary.adopted_row"
	AdoptSummarySkippedHeader Key = "adopt.summary.skipped_header"
	AdoptSummarySkippedRow    Key = "adopt.summary.skipped_row"

	// TrayImportContext is the tray's "Import context…" entry label.
	TrayImportContext Key = "tray.import_context"
	// TrayImportContextFailed renders app.ImportLegacyContext's error
	// outcome (e.g. no legacy source file could be found) in the same
	// plain information dialog showDeleteContextOutcome already uses for
	// its own failure case.
	TrayImportContextFailed Key = "tray.import_context_failed"

	// Engine facade (internal/engine) messages, carried as the "message"
	// of a structured error the rpc and mcp surfaces return.
	EngineInvalidParam               Key = "engine.invalid_param"
	EngineNeedsInput                 Key = "engine.needs_input"
	EngineDestroyNeedsConfirmation   Key = "engine.destroy_needs_confirmation"
	EngineRemoveNeedsConfirmation    Key = "engine.remove_needs_confirmation"
	EngineConfirmRequired            Key = "engine.confirm_required"
	EngineStashDropNeedsConfirmation Key = "engine.stash_drop_needs_confirmation"
	EngineCleanNeedsConfirmation     Key = "engine.clean_needs_confirmation"
	EngineDiscardNeedsConfirmation   Key = "engine.discard_needs_confirmation"
	EngineAgentsUnavailable          Key = "engine.agents_unavailable"
	EngineBlockerTrackedChange       Key = "engine.blocker.tracked_change"
	EngineBlockerForeignFile         Key = "engine.blocker.foreign_file"
	EngineBlockerUnpushedCommits     Key = "engine.blocker.unpushed_commits"

	// projects.registerMany per-project problems (the batch is
	// all-or-nothing, so every problem is reported before anything is
	// written).
	EngineRegisterInvalidKey       Key = "engine.register.invalid_key"
	EngineRegisterKeyTaken         Key = "engine.register.key_taken"
	EngineRegisterKeyDuplicate     Key = "engine.register.key_duplicate"
	EngineRegisterSourceInvalid    Key = "engine.register.source_invalid"
	EngineRegisterSourceRegistered Key = "engine.register.source_registered"
	EngineRegisterSourceDuplicate  Key = "engine.register.source_duplicate"
	EngineRegisterNotAMainClone    Key = "engine.register.not_a_main_clone"
	EngineRegisterGitFailed        Key = "engine.register.git_failed"

	// RPC protocol (internal/rpc) error messages.
	RPCInvalidRequest Key = "rpc.invalid_request"
	RPCMethodNotFound Key = "rpc.method_not_found"
	RPCInvalidParams  Key = "rpc.invalid_params"
	// Agent skill installation (wspace agents status|install|uninstall).
	// Change templates all receive: agent, path, target, backup, detail,
	// MCP server name (app.RenderAgentChanges).
	AgentsSource               Key = "agents.source"
	AgentsMCPServer            Key = "agents.mcp_server"
	AgentsDetected             Key = "agents.detected"
	AgentsNotDetected          Key = "agents.not_detected"
	AgentsStatusRow            Key = "agents.status.row"
	AgentsStatusTarget         Key = "agents.status.target"
	AgentsStatusLegacy         Key = "agents.status.legacy"
	AgentsChangeLinked         Key = "agents.change.linked"
	AgentsChangeCopied         Key = "agents.change.copied"
	AgentsChangeRefreshed      Key = "agents.change.refreshed"
	AgentsChangeUnchanged      Key = "agents.change.unchanged"
	AgentsChangeReplaced       Key = "agents.change.replaced"
	AgentsChangeConflict       Key = "agents.change.conflict"
	AgentsChangeLegacyFound    Key = "agents.change.legacy_found"
	AgentsChangeLegacyDisabled Key = "agents.change.legacy_disabled"
	AgentsChangeRemoved        Key = "agents.change.removed"
	AgentsChangeKept           Key = "agents.change.kept"
	AgentsChangeMCPRegistered  Key = "agents.change.mcp_registered"
	AgentsChangeMCPAlready     Key = "agents.change.mcp_already"
	AgentsChangeMCPFailed      Key = "agents.change.mcp_failed"
	AgentsChangeMCPManual      Key = "agents.change.mcp_manual"
	AgentsChangeMCPRunManually Key = "agents.change.mcp_run_manually"

	// node_modules copy into a new worktree (app.copyNodeModules).
	NodeModulesCopying        Key = "node_modules.copying"
	NodeModulesCopied         Key = "node_modules.copied"
	NodeModulesAlreadyPresent Key = "node_modules.already_present"
	NodeModulesCopyFailed     Key = "node_modules.copy_failed"
	NodeModulesNotIgnored     Key = "node_modules.not_ignored"
	AgentsChangeMCPUnknown    Key = "agents.change.mcp_unknown"
	AgentsChangeFailed        Key = "agents.change.failed"
	AgentsChangeNoAgents      Key = "agents.change.no_agents"
	AgentsChangeNothing       Key = "agents.change.nothing"

	AgentsChangeMCPLocalDuplicate       Key = "agents.change.mcp_local_duplicate"
	AgentsChangeMCPDuplicateRemoved     Key = "agents.change.mcp_duplicate_removed"
	AgentsChangeMCPDuplicateWouldRemove Key = "agents.change.mcp_duplicate_would_remove"
	AgentsChangeMCPDuplicateKept        Key = "agents.change.mcp_duplicate_kept"
	AgentsChangeMCPCleanFailed          Key = "agents.change.mcp_clean_failed"
	AgentsMCPConfigUnreadable           Key = "agents.mcp.config_unreadable"
	AgentsMCPCLIMissing                 Key = "agents.mcp.cli_missing"
	AgentsMCPProjectMissing             Key = "agents.mcp.project_missing"
	AgentsStatusMCPUser                 Key = "agents.status.mcp_user"
	AgentsStatusMCPLocal                Key = "agents.status.mcp_local"
	AgentsStatusMCPStale                Key = "agents.status.mcp_stale"
	AgentsStatusMCPDuplicate            Key = "agents.status.mcp_duplicate"
	ErrSkillSourceUnstable              Key = "err.skill_source_unstable"
	ErrUnknownAgent                     Key = "err.unknown_agent"
	ErrRemoteMissing                    Key = "err.remote_missing"
	ErrNoInput                          Key = "err.no_input"
	ErrCwdInsideWorkspace               Key = "err.cwd_inside_workspace"
	ErrManagedInstall                   Key = "err.managed_install"
)
