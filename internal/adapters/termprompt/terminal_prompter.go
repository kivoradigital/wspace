// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package termprompt implements ports.Prompter for a real terminal
// (stdin/stdout). It is an adapter (R5), never internal/cli itself:
// internal/cli may only import app, domain and messages (R6), and
// satisfying ports.Prompter's method signatures requires naming
// ports.TextField/ConfirmField/ChoiceField directly, which only a package
// permitted to import internal/ports can do. cmd/ws (the composition root,
// R7) constructs one TerminalPrompter and threads it into the app wizard
// use cases; internal/cli only ever holds the already-bound result.
package termprompt

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// TerminalPrompter drives ports.Prompter over an arbitrary reader/writer
// pair so tests never need a real terminal.
type TerminalPrompter struct {
	out    io.Writer
	reader *bufio.Reader
}

// New constructs a TerminalPrompter reading lines from in and writing
// prompts to out. One bufio.Reader is created here and reused for every
// subsequent call — constructing a fresh one per call would silently
// discard input bufio already buffered ahead of the caller's next read.
func New(in io.Reader, out io.Writer) *TerminalPrompter {
	return &TerminalPrompter{out: out, reader: bufio.NewReader(in)}
}

var _ ports.Prompter = (*TerminalPrompter)(nil)

func (p *TerminalPrompter) readLine() (string, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			// Nothing left to read (stdin closed or not a terminal): no
			// answer will ever come, so say so instead of failing opaquely.
			return "", domain.NewOpError("prompt", domain.CodeNoInput, "", "", err)
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (p *TerminalPrompter) printLabel(f ports.Field) {
	// f.Args' placeholders live in Help, never in Label (the label/Help
	// split moved every "%[1]s" token there) — formatting Label with an
	// argument its template never consumes is exactly the production
	// defect a screenshot once caught: fmt reports the unconsumed value as
	// literal "%!(EXTRA ...)" noise appended to the label text.
	_, _ = fmt.Fprint(p.out, messages.T(f.Label))
	if f.Help != "" {
		_, _ = fmt.Fprintf(p.out, " (%s)", messages.T(f.Help, f.Args...))
	}
}

// Text prompts for a free-text line, re-prompting on a Validate failure and
// falling back to Default on an empty line.
func (p *TerminalPrompter) Text(ctx context.Context, f ports.TextField) (string, error) {
	for {
		p.printLabel(f.Field)
		if f.Default != "" {
			_, _ = fmt.Fprintf(p.out, " [%s]", f.Default)
		}
		_, _ = fmt.Fprint(p.out, ": ")

		line, err := p.readLine()
		if err != nil {
			return "", err
		}
		if line == "" {
			line = f.Default
		}
		if f.Validate != nil {
			if verr := f.Validate(line); verr != nil {
				_, _ = fmt.Fprintln(p.out, verr)
				continue
			}
		}
		return line, nil
	}
}

// Confirm prompts for a yes/no answer, defaulting to f.Default on an empty
// line and re-prompting on anything else unrecognized.
func (p *TerminalPrompter) Confirm(ctx context.Context, f ports.ConfirmField) (bool, error) {
	for {
		p.printLabel(f.Field)
		if f.Default {
			_, _ = fmt.Fprint(p.out, " [Y/n]: ")
		} else {
			_, _ = fmt.Fprint(p.out, " [y/N]: ")
		}

		line, err := p.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(line) {
		case "":
			return f.Default, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			_, _ = fmt.Fprintln(p.out, messages.T(messages.TermPrompterYesNoHint))
		}
	}
}

func optionLabel(o ports.Option) string {
	if o.Label != "" {
		return messages.T(o.Label)
	}
	return o.Raw
}

// Choose prompts for a single 1-based index into f.Options, re-prompting
// until a value in range is given.
func (p *TerminalPrompter) Choose(ctx context.Context, f ports.ChoiceField) (int, error) {
	for {
		p.printLabel(f.Field)
		_, _ = fmt.Fprintln(p.out)
		for i, o := range f.Options {
			_, _ = fmt.Fprintf(p.out, "  %d) %s\n", i+1, optionLabel(o))
		}
		_, _ = fmt.Fprint(p.out, "> ")

		line, err := p.readLine()
		if err != nil {
			return 0, err
		}
		n, convErr := strconv.Atoi(line)
		if convErr != nil || n < 1 || n > len(f.Options) {
			_, _ = fmt.Fprintln(p.out, messages.T(messages.TermPrompterChooseHint))
			continue
		}
		return n - 1, nil
	}
}

// Group renders every field in step as a sequential run of the exact same
// single-field prompts Text/Confirm/Choose/MultiChoose already implement,
// in order — a terminal has no notion of "one page" the way a GUI form
// does, so a Step's fields are simply asked one after another, exactly as
// if the caller had called each single-field method itself. This is also
// why TerminalPrompter never renders step chrome (a "Step X of Y" header,
// a way to go back): every wizard driven from a real terminal today has
// exactly one Step, so there is nothing to indicate a position within or
// navigate back across. A future terminal-driven multi-step wizard would
// need Group extended for that; nothing in today's callers requires it.
func (p *TerminalPrompter) Group(ctx context.Context, step ports.Step) (ports.StepAnswers, error) {
	answers := ports.NewStepAnswers()
	for _, spec := range step.Fields {
		switch {
		case spec.Text != nil:
			v, err := p.Text(ctx, *spec.Text)
			if err != nil {
				return ports.StepAnswers{}, err
			}
			answers.Text[spec.Text.Label] = v
		case spec.Confirm != nil:
			v, err := p.Confirm(ctx, *spec.Confirm)
			if err != nil {
				return ports.StepAnswers{}, err
			}
			answers.Confirm[spec.Confirm.Label] = v
		case spec.Choice != nil:
			v, err := p.Choose(ctx, *spec.Choice)
			if err != nil {
				return ports.StepAnswers{}, err
			}
			answers.Choice[spec.Choice.Label] = v
		case spec.MultiChoice != nil:
			v, err := p.MultiChoose(ctx, *spec.MultiChoice)
			if err != nil {
				return ports.StepAnswers{}, err
			}
			answers.MultiChoice[spec.MultiChoice.Label] = v
		}
	}
	return answers, nil
}

// MultiChoose prompts for zero or more comma-separated 1-based indices, or
// ports.MultiChooseAllWord/ports.MultiChooseNoneWord (case-insensitive) as
// shorthand for "every option"/"no options" — the terminal counterpart of
// the GUI's own select-all/deselect-all toggle (product-owner report:
// ticking dozens of scanned candidates one by one, or hunting for the one
// to exclude, is not a reasonable interaction once a real projects root
// has 50+ entries). Re-prompts on any unparsable or out-of-range entry, or
// a selection shorter than f.Min.
func (p *TerminalPrompter) MultiChoose(ctx context.Context, f ports.ChoiceField) ([]int, error) {
	for {
		p.printLabel(f.Field)
		_, _ = fmt.Fprintln(p.out)
		for i, o := range f.Options {
			_, _ = fmt.Fprintf(p.out, "  %d) %s\n", i+1, optionLabel(o))
		}
		_, _ = fmt.Fprint(p.out, "> ")

		line, err := p.readLine()
		if err != nil {
			return nil, err
		}
		// An empty line accepts whatever this field's own Option.Selected
		// values already pre-selected (preselectedIndices returns nil when
		// none do, exactly today's "blank means none" behavior) — the
		// terminal counterpart of a freshly rendered CheckGroup already
		// starting with those same options checked (formprompt's
		// buildMultiChoiceWidget). The explicit "none" word still always
		// clears the selection outright, regardless of any pre-selection.
		if line == "" {
			return preselectedIndices(f.Options), nil
		}
		if strings.EqualFold(line, ports.MultiChooseNoneWord) {
			return nil, nil
		}
		if strings.EqualFold(line, ports.MultiChooseAllWord) {
			return allMultiChooseIndices(len(f.Options)), nil
		}

		picks, ok := parseMultiChooseIndices(line, len(f.Options))
		if !ok || len(picks) < f.Min {
			_, _ = fmt.Fprintln(p.out, messages.T(messages.TermPrompterMultiChooseHint))
			continue
		}
		return picks, nil
	}
}

// allMultiChooseIndices returns every 0-based index from 0 to n-1 — what
// ports.MultiChooseAllWord resolves to.
func allMultiChooseIndices(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// preselectedIndices returns the 0-based index of every option whose own
// Selected is true, preserving order — what an empty MultiChoose line
// accepts, and formprompt.buildMultiChoiceWidget's own initial CheckGroup
// selection (see its doc comment). A caller that sets no option's
// Selected (every existing multi-choice field before this one) gets nil
// back, exactly the "blank means none" behavior MultiChoose always had.
func preselectedIndices(opts []ports.Option) []int {
	var out []int
	for i, o := range opts {
		if o.Selected {
			out = append(out, i)
		}
	}
	return out
}

// parseMultiChooseIndices parses line as a comma-separated list of 1-based
// indices into a list of n options, returning ok=false on any entry that
// does not parse as a number in [1, n] — pulled out of MultiChoose itself
// so it is unit-testable as a pure function.
func parseMultiChooseIndices(line string, n int) (picks []int, ok bool) {
	parts := strings.Split(line, ",")
	picks = make([]int, 0, len(parts))
	for _, part := range parts {
		i, convErr := strconv.Atoi(strings.TrimSpace(part))
		if convErr != nil || i < 1 || i > n {
			return nil, false
		}
		picks = append(picks, i-1)
	}
	return picks, true
}
