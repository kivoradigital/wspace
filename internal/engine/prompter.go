// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"
	"fmt"

	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// needsInputError is what nonInteractivePrompter returns whenever a use
// case asks for a value it cannot supply; AsError maps it to
// invalid_params with the field's catalog key in Data.
type needsInputError struct{ field messages.Key }

func (e *needsInputError) Error() string { return fmt.Sprintf("needs input: %s", e.field) }

// nonInteractivePrompter is the ports.Prompter the engine hands to the few
// app use cases that still drive one (ImportLegacyContext). It never
// blocks: a text field answers its validated Default, a confirm field
// answers confirm, and anything else — or a Default that is empty or fails
// validation — fails with needsInputError instead of asking.
type nonInteractivePrompter struct{ confirm bool }

var _ ports.Prompter = nonInteractivePrompter{}

func (p nonInteractivePrompter) Text(_ context.Context, f ports.TextField) (string, error) {
	if f.Default == "" && f.Validate == nil {
		return "", nil
	}
	if f.Validate != nil {
		if err := f.Validate(f.Default); err != nil {
			return "", &needsInputError{field: f.Label}
		}
	}
	return f.Default, nil
}

func (p nonInteractivePrompter) Confirm(context.Context, ports.ConfirmField) (bool, error) {
	return p.confirm, nil
}

func (p nonInteractivePrompter) Choose(_ context.Context, f ports.ChoiceField) (int, error) {
	return 0, &needsInputError{field: f.Label}
}

func (p nonInteractivePrompter) MultiChoose(_ context.Context, f ports.ChoiceField) ([]int, error) {
	return nil, &needsInputError{field: f.Label}
}

// Group answers every field of step the same way the single-field methods
// would; a text or confirm field answers its Default (a confirm field in a
// Step is a form value, not a "may I proceed?" question).
func (p nonInteractivePrompter) Group(ctx context.Context, step ports.Step) (ports.StepAnswers, error) {
	answers := ports.NewStepAnswers()
	for _, field := range step.Fields {
		switch {
		case field.Text != nil:
			v, err := p.Text(ctx, *field.Text)
			if err != nil {
				return ports.StepAnswers{}, err
			}
			answers.Text[field.Text.Label] = v
		case field.Confirm != nil:
			answers.Confirm[field.Confirm.Label] = field.Confirm.Default
		default:
			return ports.StepAnswers{}, &needsInputError{field: field.Label()}
		}
	}
	return answers, nil
}
