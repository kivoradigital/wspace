// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"errors"
	"fmt"
)

// ErrCode is a typed error code, never a raw exit code or stderr string, so
// error handling switches on a code instead of on substrings and messages
// can be localized (design.md ADR D4).
type ErrCode string

const (
	CodeGitMissing           ErrCode = "git_missing"
	CodeGitTooOld            ErrCode = "git_too_old"
	CodeGitFailed            ErrCode = "git_failed"
	CodeNotAMainClone        ErrCode = "not_a_main_clone"
	CodeRefNotFound          ErrCode = "ref_not_found"
	CodeBranchCheckedOut     ErrCode = "branch_checked_out"
	CodeWorktreeExists       ErrCode = "worktree_exists"
	CodeWorktreeMissing      ErrCode = "worktree_missing"
	CodeWorktreeDirty        ErrCode = "worktree_dirty"
	CodeNoContext            ErrCode = "no_context"
	CodeContextNotFound      ErrCode = "context_not_found"
	CodeWorkspaceExists      ErrCode = "workspace_exists"
	CodeWorkspaceNotFound    ErrCode = "workspace_not_found"
	CodeOverlayScope         ErrCode = "overlay_scope_violation"
	CodeUnsafeTeardown       ErrCode = "unsafe_teardown"
	CodeTimeout              ErrCode = "timeout"
	CodeSchemaUnsupported    ErrCode = "schema_unsupported"
	CodeExecFailed           ErrCode = "exec_failed"
	CodeContextActive        ErrCode = "context_active"
	CodeLegacyConfigNotFound ErrCode = "legacy_config_not_found"
	CodeContextExists        ErrCode = "context_exists"
	CodeProjectExists        ErrCode = "project_exists"
	CodeProjectNotFound      ErrCode = "project_not_found"
	CodeRepoNotFound         ErrCode = "repo_not_found"
	// The update (merge/rebase from the base) refusals and outcomes.
	CodeDetachedHead          ErrCode = "detached_head"
	CodeIntegrationInProgress ErrCode = "integration_in_progress"
	CodeBaseMissing           ErrCode = "base_missing"
	CodeUpdateConflict        ErrCode = "update_conflict"
	// The repository inspector's pull (fast-forward only) and diff refusals.
	CodeNoUpstream     ErrCode = "no_upstream"
	CodeDiverged       ErrCode = "diverged"
	CodePathNotChanged ErrCode = "path_not_changed"

	// Repository inspector actions (stash apply/pop/drop, untracked clean).
	CodeStashChanged     ErrCode = "stash_changed"
	CodeStashConflict    ErrCode = "stash_conflict"
	CodePathNotUntracked ErrCode = "path_not_untracked"
	// Repository inspector write actions (stage, discard, commit, push,
	// stash create).
	CodePathIsUntracked ErrCode = "path_is_untracked"
	CodeStagedOnly      ErrCode = "staged_only"
	CodeNothingStaged   ErrCode = "nothing_staged"
	CodeIdentityMissing ErrCode = "identity_missing"
	CodeHookFailed      ErrCode = "hook_failed"
	CodePushRejected    ErrCode = "push_rejected"
	CodeAuthFailed      ErrCode = "auth_failed"
	CodeNothingToStash  ErrCode = "nothing_to_stash"
	// CodeAlreadyInWorkspace: the project is already a repo of the workspace.
	CodeAlreadyInWorkspace ErrCode = "already_in_workspace"
	// The install target is a symlink managed by a desktop app.
	CodeManagedInstall ErrCode = "managed_install"
	// Agent skill installation: the skill source would be a disk image or
	// a translocated app, or an agent id is not in the catalog.
	CodeSkillSourceUnstable ErrCode = "skill_source_unstable"
	CodeUnknownAgent        ErrCode = "unknown_agent"
	// The repository has no such remote configured (a local-only clone).
	CodeRemoteMissing ErrCode = "remote_missing"
	// A prompt needed an answer but the input is not interactive (closed stdin).
	CodeNoInput ErrCode = "no_input"
	// The shell running the command is inside the workspace to remove.
	CodeCwdInsideWorkspace ErrCode = "cwd_inside_workspace"
)

// OpError is the only error type crossing a port boundary outward. Error()
// renders code and subject only; raw subprocess output is retained in the
// unexported details field and surfaced exclusively through Details() (used
// by `doctor` and `--verbose`), never printed directly to a user.
type OpError struct {
	Op      string // e.g. "worktree.add", "config.load"
	Code    ErrCode
	Subject string // branch, alias or path — never command output
	details string // unexported: stderr, exit status
	wrapped error
}

// NewOpError constructs an OpError. details and wrapped may be empty/nil.
func NewOpError(op string, code ErrCode, subject, details string, wrapped error) *OpError {
	return &OpError{Op: op, Code: code, Subject: subject, details: details, wrapped: wrapped}
}

func (e *OpError) Error() string {
	if e.Subject == "" {
		return fmt.Sprintf("%s: %s", e.Op, e.Code)
	}
	return fmt.Sprintf("%s: %s (%s)", e.Op, e.Code, e.Subject)
}

func (e *OpError) Unwrap() error {
	return e.wrapped
}

// Details returns the raw implementation detail (stderr, exit status) that
// Error() deliberately omits.
func (e *OpError) Details() string {
	return e.details
}

// Code extracts the ErrCode from err if it is (or wraps) an *OpError, and
// returns the empty ErrCode otherwise.
func Code(err error) ErrCode {
	var opErr *OpError
	if errors.As(err, &opErr) {
		return opErr.Code
	}
	return ""
}
