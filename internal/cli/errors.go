// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

// renderError turns any error returned from a full command invocation into
// user-facing text (tasks.md 4b.15-4b.16, cli-surface spec: every
// user-facing string through the catalog). It never emits raw stderr or a
// bare Go error string in its non-verbose output for an operational
// error — that part of this function's behavior, and every existing
// golden file that pins it, is unchanged.
//
// reachedRunE distinguishes the two kinds of error this function ever
// sees (set by Execute's PersistentPreRunE hook, the one point cobra
// guarantees runs after flag parsing and Args validation both succeeded,
// strictly before RunE):
//
//   - false: err was produced by cobra itself, before any of this
//     package's own code ran at all — an unknown flag, an unknown
//     subcommand, or a failed Args validator (defect B). These are
//     invocation-shape mistakes, not operational failures: err.Error() is
//     already cobra's own precise, correct text ("unknown flag: --yes"),
//     and routing it through messages.ForCode's ErrUnknown fallback only
//     replaces that with the uninformative "an unexpected error
//     occurred". renderCobraError shows cobra's message plus the failing
//     command's usage string instead.
//   - true: err came back from RunE, exactly as before this defect's fix — a
//     *cliError, a domain-coded *OpError, or (see
//     TestCLI_VerboseFlag_ShowsRawErrorForNonOpError) a plain operational
//     error that never became either. All three keep going through the
//     catalog/ForCode path unchanged.
//
// When verbose is true, renderError appends the diagnostic block
// renderVerbose builds. This is the flag ADR D4 and OpError.Details()'s own
// doc comment ("used by `doctor` and `--verbose`") anticipated but that
// never got built: without it, any error whose root cause lives in a git
// subprocess was undiagnosable beyond one catalog sentence.
func renderError(cmd *cobra.Command, err error, reachedRunE, verbose bool) string {
	var already *cliError
	if errors.As(err, &already) {
		text := already.text
		if verbose {
			text = appendVerbose(text, err)
		}
		return text
	}

	// Not a *cliError (that case, e.g. the --json interception in
	// json.go, is handled above regardless of reachedRunE): anything else
	// that never reached RunE is cobra's own error, not this package's.
	if !reachedRunE {
		return renderCobraError(cmd, err, verbose)
	}

	code := domain.Code(err)
	text := messages.T(messages.ForCode(code))

	var opErr *domain.OpError
	if errors.As(err, &opErr) && opErr.Subject != "" {
		text = fmt.Sprintf("%s: %s", text, opErr.Subject)
	}
	if code == domain.CodeNoContext {
		text = text + "\n" + messages.T(messages.CLINoContext)
	}

	var g *guidanceError
	if errors.As(err, &g) {
		text = text + "\n" + g.guidance
	}
	if verbose {
		text = appendVerbose(text, err)
	}
	return text
}

// renderCobraError renders an error cobra produced before any of this
// package's own RunE code ran (defect B): a flag-parsing error, an unknown
// subcommand, or a failed Args validator. Cobra's own message is already
// precise and correct ("unknown flag: --yes"), so it is shown as-is,
// followed by the failing command's usage string where one is available.
//
// err.Error() here is a variable, not a literal, so TestNoInlineUserStrings
// (R8: no inline English literal reaches a print-like call in this
// package) is not bypassed, only correctly scoped: that check exists to
// stop *this package* inventing untranslated prose of its own, and cobra's
// message is not this package's prose — it is a dependency's own output,
// authored and maintained by cobra, that this package has no principled
// way to fork into the catalog (translating it would mean re-deriving
// cobra's exact wording for every flag/arg-count/subcommand-lookup
// failure it can produce, and silently drifting from it on every cobra
// upgrade). Showing it plainly is more honest than paraphrasing it through
// a catalog entry this package does not actually control.
//
// verbose is accepted for symmetry with renderError's other branch but is
// deliberately a no-op here: cobra's message and the usage hint are
// already everything there is to say about a usage error, so appending
// renderVerbose's raw-error block would only repeat err.Error() a second
// time without adding information.
func renderCobraError(cmd *cobra.Command, err error, _ bool) string {
	text := err.Error()
	if cmd != nil {
		if usage := strings.TrimRight(cmd.UsageString(), "\n"); usage != "" {
			text += "\n" + usage
		}
	}
	return text
}

// appendVerbose adds renderVerbose's block to text, unless there is
// nothing to add.
func appendVerbose(text string, err error) string {
	if extra := renderVerbose(err); extra != "" {
		text += "\n" + extra
	}
	return text
}

// renderVerbose builds the --verbose diagnostic block for err: an
// *OpError's Op, Code and Details() plus its unwrapped error chain, or —
// when err never became an *OpError at all — err's own Error() text,
// because in that case the catalog (ForCode falls back to ErrUnknown, "an
// unexpected error occurred") has nothing more specific to say.
//
// This block is deliberately still routed through messages.T instead of
// being written as inline literals. It is developer-facing rather than
// end-user prose, but TestNoInlineUserStrings enforces "every print-like
// call in this package reaches the catalog" without a carve-out for
// audience, and there is no principled reason to invent one: the English
// wording here is exactly as much a reviewable, changeable detail as any
// other string in this package. What ADR D4 protects is kept intact
// regardless — OpError.Details() (raw stderr/exit-status text) is passed
// only as a %[1]s argument here, never folded into catalog prose itself,
// and it still never reaches renderError's non-verbose path.
func renderVerbose(err error) string {
	var opErr *domain.OpError
	if errors.As(err, &opErr) {
		lines := []string{
			messages.T(messages.CLIVerboseOp, opErr.Op),
			messages.T(messages.CLIVerboseCode, string(opErr.Code)),
		}
		if d := opErr.Details(); d != "" {
			lines = append(lines, messages.T(messages.CLIVerboseDetails, d))
		}
		if chain := unwrapChain(opErr); len(chain) > 0 {
			lines = append(lines, messages.T(messages.CLIVerboseChain, strings.Join(chain, " -> ")))
		}
		return strings.Join(lines, "\n")
	}
	return messages.T(messages.CLIVerboseRaw, err.Error())
}

// unwrapChain walks err's Unwrap() chain (excluding err itself, whose text
// renderVerbose's caller already rendered some form of) and returns each
// link's Error() text, outermost first.
func unwrapChain(err error) []string {
	var chain []string
	for next := errors.Unwrap(err); next != nil; next = errors.Unwrap(next) {
		chain = append(chain, next.Error())
	}
	return chain
}

// guidanceError decorates an app-layer error with extra CLI-rendered
// guidance text, appended after the normal catalog-rendered error.
// CreateWorkspace now rolls back a partial mutate failure itself, but a
// rollback step can still fail (reported as a warning), so the CLI keeps
// telling the user what to inspect and run next if anything remains.
type guidanceError struct {
	err      error
	guidance string
}

func (e *guidanceError) Error() string { return e.err.Error() }
func (e *guidanceError) Unwrap() error { return e.err }
