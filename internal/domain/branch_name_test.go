// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestNewBranchName_RejectsFlagLikeAndInvalidRefs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple name", "main", false},
		{"slash-namespaced", "feature/payments-fix", false},
		{"digits and dashes", "release-2026.09", false},
		{"empty rejected", "", true},
		{"flag-like dash prefix rejected", "-upload-pack=x", true},
		{"flag-like double-dash rejected", "--force", true},
		{"double dot rejected", "release..2026", true},
		{"tilde rejected", "feature~1", true},
		{"caret rejected", "feature^1", true},
		{"colon rejected", "feature:1", true},
		{"question mark rejected", "feature?1", true},
		{"asterisk rejected", "feature*1", true},
		{"open bracket rejected", "feature[1", true},
		{"trailing dot-lock rejected", "feature.lock", true},
		{"trailing dot rejected", "feature.", true},
		{"leading slash rejected", "/feature", true},
		{"trailing slash rejected", "feature/", true},
		{"double slash rejected", "feature//x", true},
		{"space rejected", "feature x", true},
		{"at-brace rejected", "feature@{1}", true},
		{"bare at rejected", "@", true},
		{"component leading dot rejected", "feature/.hidden", true},
		{"backslash rejected", `feature\x`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewBranchName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewBranchName(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewBranchName(%q) unexpected error: %v", tt.input, err)
			}
			if string(got) != tt.input {
				t.Fatalf("NewBranchName(%q) = %q, want %q", tt.input, got, tt.input)
			}
		})
	}
}
