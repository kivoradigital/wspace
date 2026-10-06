// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package termprompt_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/termprompt"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TestTerminalPrompter_Text_TrimsAndRePromptsOnValidationError covers the
// shared domain.Validate loop TerminalPrompter and the (future) GUI
// FormPrompter both drive (design.md §4: "a pure domain validator, reused
// by both surfaces").
func TestTerminalPrompter_Text_TrimsAndRePromptsOnValidationError(t *testing.T) {
	in := strings.NewReader("bad\n  good-value  \n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	calls := 0
	got, err := p.Text(context.Background(), ports.TextField{
		Validate: func(s string) error {
			calls++
			if s == "bad" {
				return errTest("bad value")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if got != "good-value" {
		t.Fatalf("Text = %q, want %q", got, "good-value")
	}
	if calls != 2 {
		t.Fatalf("Validate called %d times, want 2 (one failing, one passing)", calls)
	}
}

// TestTerminalPrompter_Text_EmptyLineUsesDefault covers an empty line
// falling back to Default rather than being treated as the literal value.
func TestTerminalPrompter_Text_EmptyLineUsesDefault(t *testing.T) {
	in := strings.NewReader("\n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	got, err := p.Text(context.Background(), ports.TextField{Default: "fallback"})
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if got != "fallback" {
		t.Fatalf("Text = %q, want %q", got, "fallback")
	}
}

// TestTerminalPrompter_Confirm_AcceptsYAndNAndEmptyDefault covers the y/n
// parsing contract, including an empty line falling back to Default.
func TestTerminalPrompter_Confirm_AcceptsYAndNAndEmptyDefault(t *testing.T) {
	cases := []struct {
		line string
		def  bool
		want bool
	}{
		{"y\n", false, true},
		{"yes\n", false, true},
		{"n\n", true, false},
		{"no\n", true, false},
		{"\n", true, true},
		{"\n", false, false},
	}
	for _, c := range cases {
		in := strings.NewReader(c.line)
		var out bytes.Buffer
		p := termprompt.New(in, &out)
		got, err := p.Confirm(context.Background(), ports.ConfirmField{Default: c.def})
		if err != nil {
			t.Fatalf("Confirm(%q): %v", c.line, err)
		}
		if got != c.want {
			t.Fatalf("Confirm(%q, default=%v) = %v, want %v", c.line, c.def, got, c.want)
		}
	}
}

// TestTerminalPrompter_Choose_RePromptsOnOutOfRange covers Choose's
// single-selection loop rejecting an out-of-range index before accepting a
// valid one.
func TestTerminalPrompter_Choose_RePromptsOnOutOfRange(t *testing.T) {
	in := strings.NewReader("9\n1\n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	got, err := p.Choose(context.Background(), ports.ChoiceField{
		Options: []ports.Option{{Raw: "alpha"}, {Raw: "beta"}},
	})
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != 0 {
		t.Fatalf("Choose = %d, want 0 (first option, 1-based input \"1\")", got)
	}
}

// TestTerminalPrompter_MultiChoose_ParsesCommaSeparatedIndices covers the
// multi-select parsing contract.
func TestTerminalPrompter_MultiChoose_ParsesCommaSeparatedIndices(t *testing.T) {
	in := strings.NewReader("1,3\n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	got, err := p.MultiChoose(context.Background(), ports.ChoiceField{
		Options: []ports.Option{{Raw: "a"}, {Raw: "b"}, {Raw: "c"}},
	})
	if err != nil {
		t.Fatalf("MultiChoose: %v", err)
	}
	want := []int{0, 2}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("MultiChoose = %v, want %v", got, want)
	}
}

// TestTerminalPrompter_MultiChoose_EmptyLineSelectsNone covers an empty
// answer meaning "select nothing" rather than a parse error.
func TestTerminalPrompter_MultiChoose_EmptyLineSelectsNone(t *testing.T) {
	in := strings.NewReader("\n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	got, err := p.MultiChoose(context.Background(), ports.ChoiceField{
		Options: []ports.Option{{Raw: "a"}},
	})
	if err != nil {
		t.Fatalf("MultiChoose: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("MultiChoose = %v, want empty", got)
	}
}

// TestTerminalPrompter_MultiChoose_AllWordSelectsEveryOption covers the
// terminal counterpart of the GUI's own select-all toggle (product-owner
// report: a real projects root can list 50+ scanned candidates, so typing
// every index by hand is not reasonable) — ports.MultiChooseAllWord,
// matched case-insensitively, resolves to every option's index.
func TestTerminalPrompter_MultiChoose_AllWordSelectsEveryOption(t *testing.T) {
	in := strings.NewReader("ALL\n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	got, err := p.MultiChoose(context.Background(), ports.ChoiceField{
		Options: []ports.Option{{Raw: "a"}, {Raw: "b"}, {Raw: "c"}},
	})
	if err != nil {
		t.Fatalf("MultiChoose: %v", err)
	}
	want := []int{0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("MultiChoose(%q) = %v, want %v", "ALL", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MultiChoose(%q) = %v, want %v", "ALL", got, want)
		}
	}
}

// TestTerminalPrompter_MultiChoose_NoneWordSelectsNothing covers
// ports.MultiChooseNoneWord as an explicit, mnemonic alternative to an
// empty line — same outcome, matched case-insensitively.
func TestTerminalPrompter_MultiChoose_NoneWordSelectsNothing(t *testing.T) {
	in := strings.NewReader("None\n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	got, err := p.MultiChoose(context.Background(), ports.ChoiceField{
		Options: []ports.Option{{Raw: "a"}, {Raw: "b"}},
	})
	if err != nil {
		t.Fatalf("MultiChoose: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("MultiChoose(%q) = %v, want empty", "None", got)
	}
}

// TestTerminalPrompter_MultiChoose_HintMentionsAllAndNone covers the
// re-prompt hint: it must actually advertise the two words above, or a
// user typing them from muscle memory would never learn they exist.
func TestTerminalPrompter_MultiChoose_HintMentionsAllAndNone(t *testing.T) {
	if err := messages.Use("en"); err != nil {
		t.Fatalf("messages.Use(en): %v", err)
	}
	in := strings.NewReader("not-a-number\n1\n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	if _, err := p.MultiChoose(context.Background(), ports.ChoiceField{
		Options: []ports.Option{{Raw: "a"}},
	}); err != nil {
		t.Fatalf("MultiChoose: %v", err)
	}
	transcript := out.String()
	if !strings.Contains(transcript, ports.MultiChooseAllWord) || !strings.Contains(transcript, ports.MultiChooseNoneWord) {
		t.Fatalf("re-prompt hint = %q, want it to mention both %q and %q", transcript, ports.MultiChooseAllWord, ports.MultiChooseNoneWord)
	}
}

// TestTerminalPrompter_PrintLabel_PassesArgsToHelp covers the fix
// alongside the label/Help split: a field's Help template may itself carry
// a placeholder (e.g. a live-preview value like the effective origin
// branch — messages.WizardProjectOriginBranchHelp's own "%[1]s"), so
// printLabel must forward f.Args to Help exactly as it already does for
// Label. Before this fix, messages.T(f.Help) was called with no arguments
// at all, which prints fmt's own "%!s(MISSING)" noise in place of the
// preview value for every Help key this change adds one to.
func TestTerminalPrompter_PrintLabel_PassesArgsToHelp(t *testing.T) {
	if err := messages.Use("en"); err != nil {
		t.Fatalf("messages.Use(en): %v", err)
	}
	in := strings.NewReader("\n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	if _, err := p.Text(context.Background(), ports.TextField{
		Field: ports.Field{
			Label: messages.WizardProjectOriginBranch,
			Help:  messages.WizardProjectOriginBranchHelp,
			Args:  []any{"main"},
		},
	}); err != nil {
		t.Fatalf("Text: %v", err)
	}
	transcript := out.String()
	if strings.Contains(transcript, "MISSING") {
		t.Fatalf("transcript = %q, want Args forwarded to Help (no fmt MISSING noise)", transcript)
	}
	if !strings.Contains(transcript, "main") {
		t.Fatalf("transcript = %q, want the preview value %q substituted into Help", transcript, "main")
	}
	// The Label template (messages.WizardProjectOriginBranch, "Origin
	// branch") carries no placeholder at all — every verb moved into Help
	// when the label/Help split landed. Forwarding f.Args to Label too
	// (the actual production-screenshot defect this test now also covers)
	// makes fmt.Sprintf report the unconsumed argument as literal
	// "%!(EXTRA string=main)" noise appended to the label text.
	if strings.Contains(transcript, "EXTRA") {
		t.Fatalf("transcript = %q, want Args applied to Help only, never to Label (fmt EXTRA noise means Label was formatted with an argument its template never uses)", transcript)
	}
}

// TestTerminalPrompter_SequentialCallsShareOneBufferedReader guards against
// a real bug class: constructing a fresh bufio.Reader per call would
// silently discard already-buffered-but-unread input whenever a line
// arrives together with the next prompt's answer in the same read.
func TestTerminalPrompter_SequentialCallsShareOneBufferedReader(t *testing.T) {
	in := strings.NewReader("first\nsecond\nthird\n")
	var out bytes.Buffer
	p := termprompt.New(in, &out)

	for i, want := range []string{"first", "second", "third"} {
		got, err := p.Text(context.Background(), ports.TextField{})
		if err != nil {
			t.Fatalf("Text[%d]: %v", i, err)
		}
		if got != want {
			t.Fatalf("Text[%d] = %q, want %q", i, got, want)
		}
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
