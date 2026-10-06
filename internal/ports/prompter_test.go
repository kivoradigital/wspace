// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TestFieldSpec_LabelIdentifiesWhicheverKindIsSet covers the Step
// contract's own identifier rule: FieldSpec.Label must return the wrapped
// field's Label regardless of which of Text/Confirm/Choice/MultiChoice
// constructed it, since StepAnswers is keyed by exactly this value.
func TestFieldSpec_LabelIdentifiesWhicheverKindIsSet(t *testing.T) {
	cases := []struct {
		name string
		spec ports.FieldSpec
		want messages.Key
	}{
		{"text", ports.TextFieldSpec(ports.TextField{Field: ports.Field{Label: messages.WizardWorkspacesRoot}}), messages.WizardWorkspacesRoot},
		{"confirm", ports.ConfirmFieldSpec(ports.ConfirmField{Field: ports.Field{Label: messages.WizardCopyEnvDefault}}), messages.WizardCopyEnvDefault},
		{"choice", ports.ChoiceFieldSpec(ports.ChoiceField{Field: ports.Field{Label: messages.WizardContextToDelete}}), messages.WizardContextToDelete},
		{"multichoice", ports.MultiChoiceFieldSpec(ports.ChoiceField{Field: ports.Field{Label: messages.WizardPickProjects}}), messages.WizardPickProjects},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.spec.Label(); got != c.want {
				t.Fatalf("Label() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestStepAnswers_GettersReadBackByLabel covers StepAnswers's own
// contract: every Get* reads back exactly what was stored under a field's
// Label, and an unasked key reads as that kind's zero value rather than
// panicking on a nil map.
func TestStepAnswers_GettersReadBackByLabel(t *testing.T) {
	a := ports.NewStepAnswers()
	a.Text[messages.WizardWorkspacesRoot] = "/tmp/work"
	a.Confirm[messages.WizardCopyEnvDefault] = true
	a.Choice[messages.WizardContextToDelete] = 2
	a.MultiChoice[messages.WizardPickProjects] = []int{0, 2}

	if got := a.GetText(messages.WizardWorkspacesRoot); got != "/tmp/work" {
		t.Fatalf("GetText = %q, want %q", got, "/tmp/work")
	}
	if got := a.GetConfirm(messages.WizardCopyEnvDefault); got != true {
		t.Fatalf("GetConfirm = %v, want true", got)
	}
	if got := a.GetChoice(messages.WizardContextToDelete); got != 2 {
		t.Fatalf("GetChoice = %d, want 2", got)
	}
	if got := a.GetMultiChoice(messages.WizardPickProjects); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("GetMultiChoice = %v, want [0 2]", got)
	}

	// An unasked key reads as the zero value, never panics.
	if got := a.GetText(messages.WizardBaseBranch); got != "" {
		t.Fatalf("GetText(unasked) = %q, want empty", got)
	}
	if got := a.GetConfirm(messages.WizardFetchBeforeCreate); got != false {
		t.Fatalf("GetConfirm(unasked) = %v, want false", got)
	}
}
