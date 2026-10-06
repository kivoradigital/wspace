// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// TestConfirm_ForwardsToPrompter covers the one generic yes/no prompt
// internal/cli needs (e.g. "destroy --force" confirmation, tasks.md 4b.11)
// without ever holding a bare ports.Prompter itself (R6).
func TestConfirm_ForwardsToPrompter(t *testing.T) {
	prompter := portstest.NewScriptedPrompter(t, portstest.ConfirmAnswer(true))
	deps := app.PrompterDeps{Prompter: prompter}

	got, err := app.Confirm(context.Background(), deps, messages.CLIConfirmDestroy, []any{"feature-x"}, false)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !got {
		t.Fatal("Confirm = false, want true (scripted answer)")
	}
	prompter.CheckUnconsumed()
}

// helpPrompter records the confirm field it is asked.
type helpPrompter struct {
	*portstest.ScriptedPrompter
	field ports.ConfirmField
}

func (p *helpPrompter) Confirm(ctx context.Context, f ports.ConfirmField) (bool, error) {
	p.field = f
	return p.ScriptedPrompter.Confirm(ctx, f)
}

func TestConfirmWithHelp_ForwardsTheHelpAndItsArguments(t *testing.T) {
	p := &helpPrompter{ScriptedPrompter: portstest.NewScriptedPrompter(t, portstest.ConfirmAnswer(true))}
	args := []any{"stash@{0}", "half done", "api"}

	got, err := app.ConfirmWithHelp(context.Background(), app.PrompterDeps{Prompter: p}, messages.CLIConfirmStashDrop, messages.CLIConfirmStashDropHelp, args, false)
	if err != nil || !got {
		t.Fatalf("ConfirmWithHelp = %v, %v", got, err)
	}
	if p.field.Label != messages.CLIConfirmStashDrop || p.field.Help != messages.CLIConfirmStashDropHelp || len(p.field.Args) != 3 || p.field.Default {
		t.Fatalf("field = %+v", p.field)
	}
}
