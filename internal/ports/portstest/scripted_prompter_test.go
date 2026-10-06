// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func TestScriptedPrompter_FailsOnUnexpectedPrompt(t *testing.T) {
	rt := &recordingT{}
	p := portstest.NewScriptedPrompter(rt) // no answers scripted

	_, _ = p.Text(context.Background(), ports.TextField{})

	if !rt.failed {
		t.Fatal("ScriptedPrompter did not fail the test on an unscripted prompt")
	}
}

func TestScriptedPrompter_FailsOnUnconsumedAnswer(t *testing.T) {
	rt := &recordingT{}
	p := portstest.NewScriptedPrompter(rt, portstest.TextAnswer("payments-fix"))

	p.CheckUnconsumed()

	if !rt.failed {
		t.Fatal("ScriptedPrompter did not fail the test on an unconsumed scripted answer")
	}
}

func TestScriptedPrompter_ConsumesAnswersInOrder(t *testing.T) {
	rt := &recordingT{}
	p := portstest.NewScriptedPrompter(rt,
		portstest.TextAnswer("payments-fix"),
		portstest.ConfirmAnswer(true),
		portstest.ChooseAnswer(2),
		portstest.MultiChooseAnswer([]int{0, 2}),
	)

	text, err := p.Text(context.Background(), ports.TextField{})
	if err != nil || text != "payments-fix" {
		t.Fatalf("Text() = (%q, %v), want (%q, nil)", text, err, "payments-fix")
	}

	confirm, err := p.Confirm(context.Background(), ports.ConfirmField{})
	if err != nil || !confirm {
		t.Fatalf("Confirm() = (%v, %v), want (true, nil)", confirm, err)
	}

	choice, err := p.Choose(context.Background(), ports.ChoiceField{})
	if err != nil || choice != 2 {
		t.Fatalf("Choose() = (%d, %v), want (2, nil)", choice, err)
	}

	multi, err := p.MultiChoose(context.Background(), ports.ChoiceField{})
	if err != nil || len(multi) != 2 || multi[0] != 0 || multi[1] != 2 {
		t.Fatalf("MultiChoose() = (%v, %v), want ([0 2], nil)", multi, err)
	}

	p.CheckUnconsumed()
	if rt.failed {
		t.Fatal("CheckUnconsumed() failed the test after every answer was consumed")
	}
}

// recordingT satisfies portstest.TestingT without pulling in a real
// *testing.T, so these tests can assert failure without actually failing.
type recordingT struct {
	failed bool
}

func (r *recordingT) Helper() {}
func (r *recordingT) Fatalf(format string, args ...any) {
	r.failed = true
}
