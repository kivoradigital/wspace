// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import "testing"

// TestValidateIgnorePatterns covers this change's own authorized
// gap-closure requirement: a malformed glob must be rejected immediately
// at wizard entry (surfacing path.ErrBadPattern), while an empty answer
// ("no patterns") is always valid.
func TestValidateIgnorePatterns(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "empty answer is valid", in: "", wantErr: false},
		{name: "blank-only answer is valid", in: "   ", wantErr: false},
		{name: "single valid glob", in: "vendor", wantErr: false},
		{name: "multiple valid globs with surrounding whitespace", in: " vendor , archive-* ", wantErr: false},
		{name: "trailing comma is tolerated", in: "vendor,", wantErr: false},
		{name: "malformed glob is rejected", in: "[unterminated", wantErr: true},
		{name: "one malformed entry among valid ones is rejected", in: "vendor, [bad", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIgnorePatterns(tt.in)
			if tt.wantErr && err == nil {
				t.Fatalf("validateIgnorePatterns(%q) = nil, want an error", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateIgnorePatterns(%q) = %v, want nil", tt.in, err)
			}
		})
	}
}

// TestValidateOptionalBranchName covers project-configuration spec's
// origin_branch: empty ("inherit") is always valid; a non-empty answer
// still has to satisfy the same branch-name rule every other branch field
// uses.
func TestValidateOptionalBranchName(t *testing.T) {
	if err := validateOptionalBranchName(""); err != nil {
		t.Fatalf("validateOptionalBranchName(\"\") = %v, want nil (empty means inherit)", err)
	}
	if err := validateOptionalBranchName("develop"); err != nil {
		t.Fatalf("validateOptionalBranchName(\"develop\") = %v, want nil", err)
	}
	if err := validateOptionalBranchName("-bad"); err == nil {
		t.Fatal("validateOptionalBranchName(\"-bad\") = nil, want an error (flag-like branch name)")
	}
}

// TestValidateDestBranchTemplate and TestValidateWorktreeDirTemplate cover
// this change's own gap-closure requirement that a non-empty
// dest_branch/worktree_dir answer must parse as a valid
// domain.BranchTemplate/domain.PathTemplate (known variables only), while
// an empty answer ("inherit") is always valid.
func TestValidateDestBranchTemplate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "empty answer is valid", in: "", wantErr: false},
		{name: "known variables", in: "{prefix}{workspace}", wantErr: false},
		{name: "unknown variable rejected", in: "{nope}", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDestBranchTemplate(tt.in)
			if tt.wantErr && err == nil {
				t.Fatalf("validateDestBranchTemplate(%q) = nil, want an error", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateDestBranchTemplate(%q) = %v, want nil", tt.in, err)
			}
		})
	}
}

func TestValidateWorktreeDirTemplate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "empty answer is valid", in: "", wantErr: false},
		{name: "known variable", in: "apps/{project}", wantErr: false},
		{name: "unknown variable rejected", in: "{nope}", wantErr: true},
		{name: "escaping via dotdot rejected", in: "../{project}", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorktreeDirTemplate(tt.in)
			if tt.wantErr && err == nil {
				t.Fatalf("validateWorktreeDirTemplate(%q) = nil, want an error", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateWorktreeDirTemplate(%q) = %v, want nil", tt.in, err)
			}
		})
	}
}
