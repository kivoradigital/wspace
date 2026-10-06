// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

func TestNewContextName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"lowercase word", "work", false},
		{"digits and dash", "a1-2", false},
		{"digits and underscore", "a1_2", false},
		{"single char", "a", false},
		{"single digit", "1", false},
		{"max length 64", "a" + repeat("b", 63), false},
		{"empty", "", true},
		{"too long 65", "a" + repeat("b", 64), true},
		{"uppercase rejected", "Work", true},
		{"leading dash rejected", "-work", true},
		{"leading underscore rejected", "_work", true},
		{"space rejected", "wo rk", true},
		{"dot rejected", "wo.rk", true},
		{"slash rejected", "wo/rk", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewContextName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewContextName(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewContextName(%q) unexpected error: %v", tt.input, err)
			}
			if string(got) != tt.input {
				t.Fatalf("NewContextName(%q) = %q, want %q", tt.input, got, tt.input)
			}
		})
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
