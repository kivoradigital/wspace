// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestBranchTemplate_Resolve(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    domain.BranchTemplate
		vars    domain.Vars
		want    string
		wantErr bool
	}{
		{
			name: "prefix and workspace",
			tmpl: "{prefix}{workspace}",
			vars: domain.Vars{Workspace: "payments-fix", Prefix: "feature/"},
			want: "feature/payments-fix",
		},
		{
			name: "branch var only",
			tmpl: "{branch}",
			vars: domain.Vars{Branch: "release-1"},
			want: "release-1",
		},
		{
			name:    "unknown variable rejected",
			tmpl:    "{unknown}",
			vars:    domain.Vars{},
			wantErr: true,
		},
		{
			name:    "resolves to invalid ref rejected",
			tmpl:    "{workspace}",
			vars:    domain.Vars{Workspace: "-bad"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.tmpl.Resolve(tt.vars)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Resolve() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPathTemplate_Resolve(t *testing.T) {
	root, err := domain.NewPath("/workspaces/payments-fix")
	if err != nil {
		t.Fatalf("NewPath() unexpected error: %v", err)
	}

	tests := []struct {
		name    string
		tmpl    domain.PathTemplate
		vars    domain.Vars
		want    string
		wantErr bool
	}{
		{
			name: "project under root",
			tmpl: "{project}",
			vars: domain.Vars{Project: "api"},
			want: "/workspaces/payments-fix/api",
		},
		{
			name: "nested template",
			tmpl: "apps/{project}",
			vars: domain.Vars{Project: "web"},
			want: "/workspaces/payments-fix/apps/web",
		},
		{
			name:    "unknown variable rejected",
			tmpl:    "{unknown}",
			vars:    domain.Vars{},
			wantErr: true,
		},
		{
			name:    "escaping via dotdot rejected",
			tmpl:    "../{project}",
			vars:    domain.Vars{Project: "api"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.tmpl.Resolve(tt.vars, root)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Resolve() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}
