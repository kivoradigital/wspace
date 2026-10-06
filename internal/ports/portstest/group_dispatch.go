// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"context"

	"github.com/kivoradigital/wspace/internal/ports"
)

// DispatchGroup implements ports.Prompter.Group generically for any faked
// Prompter that only defines Text/Confirm/Choose/MultiChoose itself: it
// calls asker's own single-field method for every field in step, in
// order, collecting each answer under that field's Label — exactly the
// same sequential dispatch TerminalPrompter.Group and this package's own
// ScriptedPrompter.Group perform. Every hand-rolled Prompter fake across
// this module's own tests (internal/app's autoAcceptPrompter,
// capturingPrompter, defaultCapturingPrompter) implements Group by
// delegating to this function, so a wizard's migration from several
// single-field prompts to one Step never has to be re-taught to each
// fake's own copy of the same loop.
func DispatchGroup(ctx context.Context, asker ports.Prompter, step ports.Step) (ports.StepAnswers, error) {
	answers := ports.NewStepAnswers()
	for _, spec := range step.Fields {
		switch {
		case spec.Text != nil:
			v, err := asker.Text(ctx, *spec.Text)
			if err != nil {
				return ports.StepAnswers{}, err
			}
			answers.Text[spec.Text.Label] = v
		case spec.Confirm != nil:
			v, err := asker.Confirm(ctx, *spec.Confirm)
			if err != nil {
				return ports.StepAnswers{}, err
			}
			answers.Confirm[spec.Confirm.Label] = v
		case spec.Choice != nil:
			v, err := asker.Choose(ctx, *spec.Choice)
			if err != nil {
				return ports.StepAnswers{}, err
			}
			answers.Choice[spec.Choice.Label] = v
		case spec.MultiChoice != nil:
			v, err := asker.MultiChoose(ctx, *spec.MultiChoice)
			if err != nil {
				return ports.StepAnswers{}, err
			}
			answers.MultiChoice[spec.MultiChoice.Label] = v
		}
	}
	return answers, nil
}
