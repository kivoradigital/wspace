// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
)

// TestValidateOverlayScope_RejectsContextKeys covers ADR D7 at the pure
// domain layer: an overlay document naming "context" or "contexts" among
// its top-level keys is rejected with CodeOverlayScope, before any struct
// decode is attempted.
func TestValidateOverlayScope_RejectsContextKeys(t *testing.T) {
	tests := []struct {
		name    string
		keys    []string
		wantErr bool
	}{
		{name: "plain options-only overlay is fine", keys: []string{"schema_version", "options", "projects"}, wantErr: false},
		{name: "context key is rejected", keys: []string{"schema_version", "context"}, wantErr: true},
		{name: "contexts key is rejected", keys: []string{"schema_version", "contexts"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateOverlayScope(tt.keys)
			if tt.wantErr {
				if domain.Code(err) != domain.CodeOverlayScope {
					t.Fatalf("ValidateOverlayScope(%v): Code(err) = %q, want %q (err=%v)", tt.keys, domain.Code(err), domain.CodeOverlayScope, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateOverlayScope(%v): err = %v, want nil", tt.keys, err)
			}
		})
	}
}
