// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package ports

import (
	"context"
	"errors"

	"github.com/kivoradigital/wspace/internal/messages"
)

// Prompter lets one wizard use case drive a terminal prompt and a GUI form.
// Field labels are message keys, never literal text, so both surfaces
// localize (design.md §4, §8.4).
//
// Text/Confirm/Choose/MultiChoose present exactly one field at a time. That
// granularity is deliberately kept — and still used directly by every
// single-field prompt a wizard needs (a rename field, a single choice) —
// but it is also the reason an early GUI built on it could only ever show
// one field per window, with no way to lay out several related fields
// together, no notion of "this field is a directory", and no notion of a
// step a wizard could go back to.
//
// Group raises the unit a wizard can ask for from a field to a Step: a
// named set of fields presented together. A terminal renders a Step as
// this same sequence of single-field prompts, one after another
// (TerminalPrompter.Group); a GUI renders every field in the Step on one
// page at once (FormPrompter.Group). Whether a wizard has one Step or
// several is a decision the use case itself makes (app.RunContextWizard's
// fields all fit on one page; app.RunInitializeContextWizard's project
// selection genuinely depends on the projects folder scanned in an earlier
// step) — Group's own contract is the same regardless of how many Steps a
// caller happens to drive it with.
type Prompter interface {
	Text(ctx context.Context, f TextField) (string, error)
	Confirm(ctx context.Context, f ConfirmField) (bool, error)
	Choose(ctx context.Context, f ChoiceField) (int, error)
	MultiChoose(ctx context.Context, f ChoiceField) ([]int, error)

	// Group presents every field in step together, then returns one
	// StepAnswers holding every answer keyed by that field's own Label. A
	// wizard step whose Back is true may return ErrStepBack instead: the
	// caller is expected to return to whatever it asked for in the
	// previous step and offer this one again, never abort the wizard.
	Group(ctx context.Context, step Step) (StepAnswers, error)
}

// ErrStepBack is returned by Group when the user asked to go back to the
// previous step instead of answering the current one. Only meaningful for
// a Step whose Back is true; a caller driving a single-step wizard never
// sees it, since such a Step always leaves Back false.
var ErrStepBack = errors.New("ports: went back to the previous step")

// Field is the common shape shared by every prompt kind.
type Field struct {
	Label messages.Key
	Help  messages.Key
	Args  []any
}

// TextFieldKind distinguishes a plain free-text field from one that
// collects a filesystem directory. The zero value, TextKindPlain, is
// today's ordinary text field — every existing TextField literal keeps
// its current rendering unchanged.
type TextFieldKind int

const (
	// TextKindPlain is an ordinary single-line text field.
	TextKindPlain TextFieldKind = iota
	// TextKindDirectory marks a field whose value is a filesystem
	// directory: FormPrompter renders it as a text entry plus a Browse
	// button opening a folder-choice dialog; TerminalPrompter renders it
	// exactly like TextKindPlain, a plain path prompt, since a terminal
	// has no folder-choice dialog to offer.
	TextKindDirectory
)

// TextField is a free-text prompt.
type TextField struct {
	Field
	Default  string
	Validate func(string) error // a pure domain validator, reused by both surfaces
	Kind     TextFieldKind
}

// ConfirmField is a yes/no prompt.
type ConfirmField struct {
	Field
	Default bool
}

// ChoiceField is a single- or multi-select prompt.
type ChoiceField struct {
	Field
	Options []Option
	Min     int // MultiChoose only
}

// MultiChooseAllWord and MultiChooseNoneWord are the words
// TerminalPrompter's MultiChoose recognizes (case-insensitively) as
// shorthand for "every option" and "no options", alongside the existing
// comma-separated index list — the terminal counterpart of the GUI's own
// select-all/deselect-all toggle (product-owner report: ticking dozens of
// scanned candidates one by one, or hunting for the one to exclude, is not
// a reasonable interaction once a real projects root has 50+ entries).
// Exported here, on the shared field spec, rather than declared privately
// inside internal/adapters/termprompt, so the one place that advertises
// them (the catalog's TermPrompterMultiChooseHint) and the one place that
// parses them never drift into two different words for the same thing.
const (
	MultiChooseAllWord  = "all"
	MultiChooseNoneWord = "none"
)

// Option is one selectable choice within a ChoiceField.
type Option struct {
	Label    messages.Key
	Raw      string // shown verbatim when Label is empty (paths, branch names)
	Detail   string
	Selected bool
}

// FieldSpec is one field within a Step, tagged by which single-field kind
// it wraps. Exactly one of Text/Confirm/Choice/MultiChoice is non-nil; a
// caller builds one with TextFieldSpec/ConfirmFieldSpec/ChoiceFieldSpec/
// MultiChoiceFieldSpec rather than filling this struct by hand, so a Step
// can never accidentally carry a field tagged with more than one kind (or
// none).
type FieldSpec struct {
	Text        *TextField
	Confirm     *ConfirmField
	Choice      *ChoiceField
	MultiChoice *ChoiceField
}

// TextFieldSpec wraps f as a Step field.
func TextFieldSpec(f TextField) FieldSpec { return FieldSpec{Text: &f} }

// ConfirmFieldSpec wraps f as a Step field.
func ConfirmFieldSpec(f ConfirmField) FieldSpec { return FieldSpec{Confirm: &f} }

// ChoiceFieldSpec wraps f as a single-select Step field.
func ChoiceFieldSpec(f ChoiceField) FieldSpec { return FieldSpec{Choice: &f} }

// MultiChoiceFieldSpec wraps f as a multi-select Step field.
func MultiChoiceFieldSpec(f ChoiceField) FieldSpec { return FieldSpec{MultiChoice: &f} }

// Label returns the message key identifying this field, regardless of
// which kind it wraps — the same key StepAnswers is keyed by.
func (fs FieldSpec) Label() messages.Key {
	switch {
	case fs.Text != nil:
		return fs.Text.Label
	case fs.Confirm != nil:
		return fs.Confirm.Label
	case fs.Choice != nil:
		return fs.Choice.Label
	case fs.MultiChoice != nil:
		return fs.MultiChoice.Label
	default:
		return ""
	}
}

// Step is one group of fields presented together: a GUI renders every
// field in Fields on one page at once; a terminal renders them as
// sequential prompts, in order (Prompter.Group's own doc comment).
//
// Index/Total/Back only matter to a wizard with more than one Step (the
// product layout rule: "one page when the fields fit" — a single-step
// wizard leaves all three at their zero value, and neither surface renders
// any step chrome — no indicator, no Back — for it). A multi-step wizard
// sets Index (1-based) and Total on every Step, and sets Back true on
// every Step after the first, so the first step of a wizard can never be
// "gone back" past.
type Step struct {
	Title  messages.Key // optional page heading; "" renders none
	Fields []FieldSpec
	Index  int
	Total  int
	Back   bool
}

// StepAnswers holds Group's result, one entry per field in the Step that
// produced it, keyed by that field's own Label (FieldSpec.Label) — stable
// because every field within one Step carries its own distinct message
// key. Only the map matching the field's own kind is ever populated for a
// given key; a caller that built the Step already knows which one to read.
type StepAnswers struct {
	Text        map[messages.Key]string
	Confirm     map[messages.Key]bool
	Choice      map[messages.Key]int
	MultiChoice map[messages.Key][]int
}

// newStepAnswers returns a StepAnswers with every map ready to receive
// entries.
func newStepAnswers() StepAnswers {
	return StepAnswers{
		Text:        map[messages.Key]string{},
		Confirm:     map[messages.Key]bool{},
		Choice:      map[messages.Key]int{},
		MultiChoice: map[messages.Key][]int{},
	}
}

// NewStepAnswers is newStepAnswers exported for adapters (internal/adapters/
// termprompt, internal/adapters/formprompt) building a StepAnswers to
// return from their own Group implementation.
func NewStepAnswers() StepAnswers { return newStepAnswers() }

// GetText returns the text answer for k ("" when k was never asked as a
// text field) — a caller reading its own Step back already knows which
// key was asked as which kind, so no second "was this key even present"
// check is needed.
func (a StepAnswers) GetText(k messages.Key) string { return a.Text[k] }

// GetConfirm returns the confirm answer for k.
func (a StepAnswers) GetConfirm(k messages.Key) bool { return a.Confirm[k] }

// GetChoice returns the choice answer for k.
func (a StepAnswers) GetChoice(k messages.Key) int { return a.Choice[k] }

// GetMultiChoice returns the multi-choice answer for k.
func (a StepAnswers) GetMultiChoice(k messages.Key) []int { return a.MultiChoice[k] }
