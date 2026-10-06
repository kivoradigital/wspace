// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"errors"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestNewProjectKey(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{in: "api"},
		{in: "team-a-web"},
		{in: "web_2.0"},
		{in: "", wantErr: true},
		{in: "  ", wantErr: true},
		{in: ".", wantErr: true},
		{in: "..", wantErr: true},
		{in: "a/b", wantErr: true},
		{in: `a\b`, wantErr: true},
		{in: "-flag", wantErr: true},
		{in: ".hidden", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			k, err := domain.NewProjectKey(tt.in)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidProjectKey) {
					t.Fatalf("NewProjectKey(%q) error = %v, want ErrInvalidProjectKey", tt.in, err)
				}
				return
			}
			if err != nil || string(k) != tt.in {
				t.Fatalf("NewProjectKey(%q) = %q, %v", tt.in, k, err)
			}
		})
	}
}

func TestNewWorkspaceName(t *testing.T) {
	for _, bad := range []string{"", " ", ".", "..", "a/b", `a\b`, "-x", ".x"} {
		if _, err := domain.NewWorkspaceName(bad); !errors.Is(err, domain.ErrInvalidWorkspaceName) {
			t.Fatalf("NewWorkspaceName(%q) error = %v, want ErrInvalidWorkspaceName", bad, err)
		}
	}
	if n, err := domain.NewWorkspaceName("payments-fix"); err != nil || n != "payments-fix" {
		t.Fatalf("NewWorkspaceName(payments-fix) = %q, %v", n, err)
	}
}

func TestSuggestProjectKeys(t *testing.T) {
	got := domain.SuggestProjectKeys(
		[]string{"a/api", "b/api", "-flags", "x/API", "web", "team/web.app", "c/-"},
		[]domain.ProjectKey{"web"},
	)
	want := []string{"a-api", "b-api", "flags", "x-API", "web-2", "web.app", "project"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SuggestProjectKeys()[%d] = %q, want %q (all %v)", i, got[i], want[i], got)
		}
		if _, err := domain.NewProjectKey(got[i]); err != nil {
			t.Errorf("suggestion %q is not a valid key: %v", got[i], err)
		}
	}
}
