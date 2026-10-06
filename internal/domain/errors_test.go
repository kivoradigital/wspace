// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestOpError(t *testing.T) {
	wrapped := errors.New("exit status 128")
	err := domain.NewOpError("worktree.add", domain.CodeBranchCheckedOut, "feature/x", "stderr: already checked out", wrapped)

	t.Run("Error format includes op, code and subject", func(t *testing.T) {
		want := "worktree.add: branch_checked_out (feature/x)"
		if got := err.Error(); got != want {
			t.Fatalf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("Error format omits subject when empty", func(t *testing.T) {
		noSubject := domain.NewOpError("config.load", domain.CodeNoContext, "", "", nil)
		want := "config.load: no_context"
		if got := noSubject.Error(); got != want {
			t.Fatalf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("Unwrap returns the wrapped error", func(t *testing.T) {
		if !errors.Is(err, wrapped) {
			t.Fatalf("errors.Is(err, wrapped) = false, want true")
		}
	})

	t.Run("Details hides raw stderr from Error", func(t *testing.T) {
		if got := err.Details(); got != "stderr: already checked out" {
			t.Fatalf("Details() = %q, want %q", got, "stderr: already checked out")
		}
		if want, got := "already checked out", err.Error(); strings.Contains(got, want) {
			t.Fatalf("Error() = %q must not leak raw stderr %q", got, want)
		}
	})

	t.Run("Code extracts the ErrCode from an OpError", func(t *testing.T) {
		if got := domain.Code(err); got != domain.CodeBranchCheckedOut {
			t.Fatalf("Code(err) = %q, want %q", got, domain.CodeBranchCheckedOut)
		}
	})

	t.Run("Code returns empty for a non-OpError", func(t *testing.T) {
		if got := domain.Code(errors.New("plain error")); got != "" {
			t.Fatalf("Code(plain error) = %q, want empty", got)
		}
	})
}
