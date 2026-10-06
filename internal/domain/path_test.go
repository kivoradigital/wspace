// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestNewPath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"posix absolute", "/home/me/src/api", "/home/me/src/api", false},
		{"posix absolute cleans double slash", "/home//me", "/home/me", false},
		{"windows absolute", `C:\Users\me\src`, "C:/Users/me/src", false},
		{"empty rejected", "", "", true},
		{"relative rejected", "relative/path", "", true},
		{"dot relative rejected", "./relative", "", true},
		{"posix escaping rejected", "/home/../etc/passwd", "", true},
		{"escaping in middle rejected", "/home/me/../../etc", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewPath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewPath(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewPath(%q) unexpected error: %v", tt.input, err)
			}
			if string(got) != tt.want {
				t.Fatalf("NewPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPath_JoinAndBase(t *testing.T) {
	root, err := domain.NewPath("/workspaces/payments-fix")
	if err != nil {
		t.Fatalf("NewPath() unexpected error: %v", err)
	}

	joined := root.Join("api")
	if string(joined) != "/workspaces/payments-fix/api" {
		t.Fatalf("Join(%q) = %q, want %q", "api", joined, "/workspaces/payments-fix/api")
	}

	if got := joined.Base(); got != "api" {
		t.Fatalf("Base() = %q, want %q", got, "api")
	}

	multi := root.Join("apps", "web")
	if string(multi) != "/workspaces/payments-fix/apps/web" {
		t.Fatalf("Join(multi) = %q, want %q", multi, "/workspaces/payments-fix/apps/web")
	}
}
