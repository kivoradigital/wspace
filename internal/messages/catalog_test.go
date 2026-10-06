// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package messages_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/messages"
)

func TestRegisterUseT_RoundTrip(t *testing.T) {
	messages.Register("test-locale", map[messages.Key]string{
		messages.WorkspaceCreated: "workspace %[1]s created",
	})

	if err := messages.Use("test-locale"); err != nil {
		t.Fatalf("Use() unexpected error: %v", err)
	}

	if got, want := messages.T(messages.WorkspaceCreated, "payments-fix"), "workspace payments-fix created"; got != want {
		t.Fatalf("T() = %q, want %q", got, want)
	}
}

func TestUse_UnknownLocaleErrors(t *testing.T) {
	if err := messages.Use("does-not-exist"); err == nil {
		t.Fatal("Use(unknown locale) = nil error, want error")
	}
}

func TestT_UnregisteredKeyFallsBackToKeyString(t *testing.T) {
	messages.Register("fallback-locale", map[messages.Key]string{})
	if err := messages.Use("fallback-locale"); err != nil {
		t.Fatalf("Use() unexpected error: %v", err)
	}

	if got, want := messages.T(messages.WorkspaceDestroyed), string(messages.WorkspaceDestroyed); got != want {
		t.Fatalf("T() = %q, want fallback %q", got, want)
	}
}
