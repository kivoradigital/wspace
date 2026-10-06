// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"errors"
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// Code is a stable, wire-level error code. Callers switch on it; the
// accompanying message is human text and may change.
type Code string

const (
	CodeInvalidRequest    Code = "invalid_request"
	CodeMethodNotFound    Code = "method_not_found"
	CodeInvalidParams     Code = "invalid_params"
	CodeNotFound          Code = "not_found"
	CodeAlreadyExists     Code = "already_exists"
	CodeConflict          Code = "conflict"
	CodeNeedsConfirmation Code = "needs_confirmation"
	CodeGitFailed         Code = "git_failed"
	CodeInternal          Code = "internal"
)

// Error is the only error type the engine returns. Data carries
// machine-readable detail: for a domain failure, "domainCode" (the finer
// domain.ErrCode) and "subject"; for needs_confirmation, "reasons".
type Error struct {
	Code    Code
	Message string
	Data    map[string]any
	cause   error
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func (e *Error) Unwrap() error { return e.cause }

// newError builds an *Error whose message is rendered from the catalog.
func newError(code Code, key messages.Key, args ...any) *Error {
	return &Error{Code: code, Message: messages.T(key, args...)}
}

// invalidParam reports a malformed or missing parameter by name.
func invalidParam(name string, reason error) *Error {
	e := newError(CodeInvalidParams, messages.EngineInvalidParam, name, reason.Error())
	e.Data = map[string]any{"param": name}
	e.cause = reason
	return e
}

// domainCodeMap translates each domain.ErrCode into its stable wire code.
var domainCodeMap = map[domain.ErrCode]Code{
	domain.CodeContextNotFound:      CodeNotFound,
	domain.CodeWorkspaceNotFound:    CodeNotFound,
	domain.CodeWorktreeMissing:      CodeNotFound,
	domain.CodeRefNotFound:          CodeNotFound,
	domain.CodeLegacyConfigNotFound: CodeNotFound,
	domain.CodeProjectNotFound:      CodeNotFound,
	domain.CodeRepoNotFound:         CodeNotFound,
	domain.CodeNoContext:            CodeNotFound,
	domain.CodeBaseMissing:          CodeNotFound,

	domain.CodeWorkspaceExists: CodeAlreadyExists,
	domain.CodeWorktreeExists:  CodeAlreadyExists,
	domain.CodeContextExists:   CodeAlreadyExists,
	domain.CodeProjectExists:   CodeAlreadyExists,

	domain.CodeBranchCheckedOut: CodeConflict,
	domain.CodeContextActive:    CodeConflict,
	domain.CodeWorktreeDirty:    CodeConflict,
	domain.CodeUnsafeTeardown:   CodeConflict,
	domain.CodeOverlayScope:     CodeConflict,

	domain.CodeDetachedHead:          CodeConflict,
	domain.CodeIntegrationInProgress: CodeConflict,
	domain.CodeUpdateConflict:        CodeConflict,
	domain.CodeNoUpstream:            CodeConflict,
	domain.CodeDiverged:              CodeConflict,
	domain.CodePathNotChanged:        CodeNotFound,
	domain.CodeStashChanged:          CodeConflict,
	domain.CodeStashConflict:         CodeConflict,
	domain.CodePathNotUntracked:      CodeNotFound,
	domain.CodeAlreadyInWorkspace:    CodeAlreadyExists,

	domain.CodePathIsUntracked: CodeConflict,
	domain.CodeStagedOnly:      CodeConflict,
	domain.CodeNothingStaged:   CodeConflict,
	domain.CodeIdentityMissing: CodeConflict,
	domain.CodeHookFailed:      CodeGitFailed,
	domain.CodePushRejected:    CodeConflict,
	domain.CodeAuthFailed:      CodeGitFailed,
	domain.CodeNothingToStash:  CodeConflict,

	domain.CodeGitMissing: CodeGitFailed,
	domain.CodeGitTooOld:  CodeGitFailed,
	domain.CodeGitFailed:  CodeGitFailed,
	domain.CodeExecFailed: CodeGitFailed,
	domain.CodeTimeout:    CodeGitFailed,

	domain.CodeNotAMainClone: CodeInvalidParams,
	domain.CodeUnknownAgent:  CodeInvalidParams,

	domain.CodeSkillSourceUnstable: CodeConflict,
}

// validationSentinels are the domain/app validation errors that always
// mean the caller sent a bad value.
var validationSentinels = []error{
	domain.ErrInvalidContextName,
	domain.ErrInvalidPath,
	domain.ErrInvalidBranchName,
	domain.ErrInvalidProjectKey,
	domain.ErrInvalidWorkspaceName,
	domain.ErrInvalidTemplate,
	app.ErrInvalidIgnorePattern,
	app.ErrEmptyCommitMessage,
}

// AsError converts any error into an *Error, preserving one that already
// is. A *domain.OpError keeps its finer code and subject in Data; raw
// subprocess detail (OpError.Details) is never exposed.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}

	var opErr *domain.OpError
	if errors.As(err, &opErr) {
		code, ok := domainCodeMap[opErr.Code]
		if !ok {
			code = CodeInternal
		}
		msg := messages.T(messages.ForCode(opErr.Code))
		if opErr.Subject != "" {
			msg = msg + ": " + opErr.Subject
		}
		data := map[string]any{"domainCode": string(opErr.Code)}
		if opErr.Subject != "" {
			data["subject"] = opErr.Subject
		}
		return &Error{Code: code, Message: msg, Data: data, cause: err}
	}

	for _, s := range validationSentinels {
		if errors.Is(err, s) {
			return &Error{Code: CodeInvalidParams, Message: err.Error(), cause: err}
		}
	}
	var ni *needsInputError
	if errors.As(err, &ni) {
		e := newError(CodeInvalidParams, messages.EngineNeedsInput, string(ni.field))
		e.Data = map[string]any{"field": string(ni.field)}
		e.cause = err
		return e
	}
	return &Error{Code: CodeInternal, Message: err.Error(), cause: err}
}

// wrap converts err (when non-nil) through AsError, typed as error so a
// nil result stays a true nil interface.
func wrap(err error) error {
	if err == nil {
		return nil
	}
	return AsError(err)
}
