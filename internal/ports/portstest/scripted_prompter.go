// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package portstest holds fake implementations of every internal/ports
// interface (design.md §12), so internal/app use cases and their driving
// adapters can be tested with zero real I/O.
package portstest

import (
	"context"

	"github.com/kivoradigital/wspace/internal/ports"
)

// TestingT is the minimal subset of *testing.T a fake needs. Accepting an
// interface rather than *testing.T keeps portstest importable from
// non-test code paths if a future phase needs that, and lets these fakes'
// own tests substitute a recording double.
type TestingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// answerKind distinguishes which Prompter method an answer was scripted
// for, so ScriptedPrompter can fail loudly on a kind mismatch instead of
// silently returning the wrong shape.
type answerKind int

const (
	answerText answerKind = iota
	answerConfirm
	answerChoose
	answerMultiChoose
)

// Answer is one scripted response. Construct one with TextAnswer,
// ConfirmAnswer, ChooseAnswer or MultiChooseAnswer.
type Answer struct {
	kind        answerKind
	text        string
	confirm     bool
	choice      int
	multiChoice []int
}

func TextAnswer(s string) Answer        { return Answer{kind: answerText, text: s} }
func ConfirmAnswer(b bool) Answer       { return Answer{kind: answerConfirm, confirm: b} }
func ChooseAnswer(i int) Answer         { return Answer{kind: answerChoose, choice: i} }
func MultiChooseAnswer(is []int) Answer { return Answer{kind: answerMultiChoose, multiChoice: is} }

// ScriptedPrompter is a ports.Prompter backed by a FIFO queue of answers.
// It fails the test on an unexpected prompt (the queue is empty, or the
// next answer's kind does not match the method called) and on any
// unconsumed answer left in the queue at the end of a test.
type ScriptedPrompter struct {
	t       TestingT
	answers []Answer
	pos     int
}

// NewScriptedPrompter constructs a ScriptedPrompter with the given answers,
// consumed in order.
func NewScriptedPrompter(t TestingT, answers ...Answer) *ScriptedPrompter {
	return &ScriptedPrompter{t: t, answers: answers}
}

func (p *ScriptedPrompter) next(kind answerKind) (Answer, bool) {
	p.t.Helper()
	if p.pos >= len(p.answers) {
		p.t.Fatalf("ScriptedPrompter: unexpected prompt, no scripted answer remains")
		return Answer{}, false
	}
	a := p.answers[p.pos]
	if a.kind != kind {
		p.t.Fatalf("ScriptedPrompter: next scripted answer is kind %d, prompt asked for kind %d", a.kind, kind)
		return Answer{}, false
	}
	p.pos++
	return a, true
}

func (p *ScriptedPrompter) Text(ctx context.Context, f ports.TextField) (string, error) {
	a, ok := p.next(answerText)
	if !ok {
		return "", nil
	}
	return a.text, nil
}

func (p *ScriptedPrompter) Confirm(ctx context.Context, f ports.ConfirmField) (bool, error) {
	a, ok := p.next(answerConfirm)
	if !ok {
		return false, nil
	}
	return a.confirm, nil
}

func (p *ScriptedPrompter) Choose(ctx context.Context, f ports.ChoiceField) (int, error) {
	a, ok := p.next(answerChoose)
	if !ok {
		return 0, nil
	}
	return a.choice, nil
}

func (p *ScriptedPrompter) MultiChoose(ctx context.Context, f ports.ChoiceField) ([]int, error) {
	a, ok := p.next(answerMultiChoose)
	if !ok {
		return nil, nil
	}
	return a.multiChoice, nil
}

// Group consumes one scripted answer per field in step, in order, exactly
// as if the caller had called Text/Confirm/Choose/MultiChoose directly for
// each one (DispatchGroup) — a scripted answer sequence written against
// the field-by-field contract keeps working unchanged once a caller
// groups those same fields into a Step (this is deliberate:
// RunContextWizard's own migration to one Step never required rewriting a
// single existing scripted test).
func (p *ScriptedPrompter) Group(ctx context.Context, step ports.Step) (ports.StepAnswers, error) {
	return DispatchGroup(ctx, p, step)
}

var _ ports.Prompter = (*ScriptedPrompter)(nil)

// CheckUnconsumed fails the test if any scripted answer was never consumed.
// Call it at the end of a test that uses ScriptedPrompter.
func (p *ScriptedPrompter) CheckUnconsumed() {
	p.t.Helper()
	if p.pos < len(p.answers) {
		p.t.Fatalf("ScriptedPrompter: %d scripted answer(s) were never consumed", len(p.answers)-p.pos)
	}
}
