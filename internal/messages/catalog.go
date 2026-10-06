// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package messages

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/domain"
)

// Catalog is one locale's Key -> format-string map. Entries use indexed
// verbs only (e.g. "%[1]s already exists on %[2]s") so a translator can
// reorder arguments without a code change (design.md §10).
type Catalog struct {
	locale  string
	entries map[Key]string
}

var (
	catalogs = map[string]*Catalog{}
	active   *Catalog
)

// Register stores entries under locale, called from each locale_*.go
// init(). The full English catalog is populated starting in phase 4b.
func Register(locale string, entries map[Key]string) {
	catalogs[locale] = &Catalog{locale: locale, entries: entries}
}

// Use selects the active catalog. "" auto-detects in later phases; for now
// callers must pass a registered locale explicitly.
func Use(locale string) error {
	c, ok := catalogs[locale]
	if !ok {
		return fmt.Errorf("messages: locale %q is not registered", locale)
	}
	active = c
	return nil
}

// T renders k through the active catalog. A missing catalog or key falls
// back to the key's own string rather than panicking — there is no runtime
// file-loading failure mode in a tray app (design.md §10).
func T(k Key, args ...any) string {
	if active == nil {
		return string(k)
	}
	tmpl, ok := active.entries[k]
	if !ok {
		return string(k)
	}
	if len(args) == 0 {
		return tmpl
	}
	return fmt.Sprintf(tmpl, args...)
}

// Plural picks one or other based on n, for callers that render a plural
// form through T.
func Plural(n int, one, other Key) Key {
	if n == 1 {
		return one
	}
	return other
}

// errCodeKeys maps every domain.ErrCode to its catalog Key. ForCode is the
// single bridge design.md §10 requires: domain and adapters never know a
// sentence exists, only a code.
var errCodeKeys = map[domain.ErrCode]Key{
	domain.CodeGitMissing:            ErrGitMissing,
	domain.CodeGitTooOld:             ErrGitTooOld,
	domain.CodeGitFailed:             ErrGitFailed,
	domain.CodeNotAMainClone:         ErrNotAMainClone,
	domain.CodeRefNotFound:           ErrRefNotFound,
	domain.CodeBranchCheckedOut:      ErrBranchCheckedOut,
	domain.CodeWorktreeExists:        ErrWorktreeExists,
	domain.CodeWorktreeMissing:       ErrWorktreeMissing,
	domain.CodeWorktreeDirty:         ErrWorktreeDirty,
	domain.CodeNoContext:             ErrNoContext,
	domain.CodeContextNotFound:       ErrContextNotFound,
	domain.CodeWorkspaceExists:       ErrWorkspaceExists,
	domain.CodeWorkspaceNotFound:     ErrWorkspaceNotFound,
	domain.CodeOverlayScope:          ErrOverlayScope,
	domain.CodeUnsafeTeardown:        ErrUnsafeTeardown,
	domain.CodeTimeout:               ErrTimeout,
	domain.CodeSchemaUnsupported:     ErrSchemaUnsupported,
	domain.CodeExecFailed:            ErrExecFailed,
	domain.CodeContextActive:         ErrContextActive,
	domain.CodeLegacyConfigNotFound:  ErrLegacyConfigNotFound,
	domain.CodeContextExists:         ErrContextExists,
	domain.CodeProjectExists:         ErrProjectExists,
	domain.CodeProjectNotFound:       ErrProjectNotFound,
	domain.CodeRepoNotFound:          ErrRepoNotFound,
	domain.CodeDetachedHead:          ErrDetachedHead,
	domain.CodeIntegrationInProgress: ErrIntegrationInProgress,
	domain.CodeBaseMissing:           ErrBaseMissing,
	domain.CodeUpdateConflict:        ErrUpdateConflict,
	domain.CodeNoUpstream:            ErrNoUpstream,
	domain.CodeDiverged:              ErrDiverged,
	domain.CodePathNotChanged:        ErrPathNotChanged,
	domain.CodeStashChanged:          ErrStashChanged,
	domain.CodeStashConflict:         ErrStashConflict,
	domain.CodePathNotUntracked:      ErrPathNotUntracked,
	domain.CodeAlreadyInWorkspace:    ErrAlreadyInWorkspace,
	domain.CodePathIsUntracked:       ErrPathIsUntracked,
	domain.CodeStagedOnly:            ErrStagedOnly,
	domain.CodeNothingStaged:         ErrNothingStaged,
	domain.CodeIdentityMissing:       ErrIdentityMissing,
	domain.CodeHookFailed:            ErrHookFailed,
	domain.CodePushRejected:          ErrPushRejected,
	domain.CodeAuthFailed:            ErrAuthFailed,
	domain.CodeNothingToStash:        ErrNothingToStash,
	domain.CodeSkillSourceUnstable:   ErrSkillSourceUnstable,
	domain.CodeUnknownAgent:          ErrUnknownAgent,
	domain.CodeRemoteMissing:         ErrRemoteMissing,
	domain.CodeNoInput:               ErrNoInput,
	domain.CodeCwdInsideWorkspace:    ErrCwdInsideWorkspace,
	domain.CodeManagedInstall:        ErrManagedInstall,
}

// ForCode maps a domain.ErrCode to its catalog Key, falling back to
// ErrUnknown for any code without a registered mapping (including the
// empty ErrCode of an error that is not a *domain.OpError at all).
func ForCode(c domain.ErrCode) Key {
	if k, ok := errCodeKeys[c]; ok {
		return k
	}
	return ErrUnknown
}

// EnglishEntriesForTest exposes the registered "en" catalog's raw entries
// map for TestCatalogCompleteness (tasks.md 4b.1), which needs to compare
// it against every Key constant declared in keys.go. Not used outside
// internal/messages's own tests.
func EnglishEntriesForTest() map[Key]string {
	c, ok := catalogs["en"]
	if !ok {
		return nil
	}
	return c.entries
}
