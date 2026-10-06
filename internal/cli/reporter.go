// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// HumanReporter renders every Step/Info/Warn/Result call as human text.
// --json is never supported on a command that calls a Reporter-driven use
// case (only info/list/status support --json, and neither of those two
// calls Reporter at all), so HumanReporter never needs a JSON mode.
//
// HumanReporter satisfies ports.Reporter structurally: its method set uses
// only messages.Key and "any", both already legitimate for internal/cli to
// name (R6), so this file never imports internal/ports.
type HumanReporter struct {
	Out io.Writer
	Err io.Writer
}

func (r *HumanReporter) Step(k messages.Key, args ...any) {
	_, _ = fmt.Fprintln(r.Out, messages.T(k, args...))
}
func (r *HumanReporter) Info(k messages.Key, args ...any) {
	_, _ = fmt.Fprintln(r.Out, messages.T(k, args...))
}
func (r *HumanReporter) Warn(k messages.Key, args ...any) {
	_, _ = fmt.Fprintln(r.Err, messages.T(k, args...))
}

// Result renders a use case's terminal payload. Every phase-4a use case
// that calls Reporter.Result at all passes one of the concrete types
// switched on below.
func (r *HumanReporter) Result(payload any) {
	switch v := payload.(type) {
	case app.CreateWorkspaceResult:
		_, _ = fmt.Fprintln(r.Out, messages.T(messages.WorkspaceCreated, v.Workspace.Name, string(v.Workspace.Root)))
	case domain.RepoEntry:
		_, _ = fmt.Fprintln(r.Out, messages.T(messages.RepoAdded, v.Alias))
	case app.RepairResult:
		joined := "(none)"
		if len(v.Recreated) > 0 {
			joined = strings.Join(v.Recreated, ", ")
		}
		_, _ = fmt.Fprintln(r.Out, messages.T(messages.WorktreeRecreated, joined))
	case app.SyncEnvResult:
		_, _ = fmt.Fprintln(r.Out, messages.T(messages.EnvSynced, len(v.Copied)))
	case app.DoctorResult:
		if v.PrunedWorktrees > 0 {
			_, _ = fmt.Fprintln(r.Out, messages.T(messages.CLIDoctorPruned, v.PrunedWorktrees))
		}
	case app.InstallResult:
		r.renderInstallResult(v)
	// app.ExecResult is deliberately not handled here: Exec now reports a
	// non-zero exit per repo, by alias and code, through Reporter.Warn as
	// it happens (defect A) — a final aggregate Result call here would
	// only repeat the last repo's code without saying which repo it was.
	case struct{ WorkspaceRoot domain.Path }:
		_, _ = fmt.Fprintln(r.Out, messages.T(messages.WorkspaceDestroyed, string(v.WorkspaceRoot)))
	case struct{ Alias string }:
		_, _ = fmt.Fprintln(r.Out, messages.T(messages.RepoRemoved, v.Alias))
	}
}

// renderInstallResult renders every outcome app.Install can produce:
// where the binary went (or the manual command when it could not be
// copied), what happened to PATH, shell integration, and whether a new
// terminal is needed; for --uninstall, everything removed.
func (r *HumanReporter) renderInstallResult(v app.InstallResult) {
	say := func(key messages.Key, args ...any) { _, _ = fmt.Fprintln(r.Out, messages.T(key, args...)) }
	if v.Uninstall {
		if len(v.Removed) == 0 && len(v.PathRemoved) == 0 && !v.ShellRCUpdated {
			say(messages.InstallNothingToUninstall)
			return
		}
		if len(v.Removed) > 0 {
			say(messages.InstallUninstalled, strings.Join(v.Removed, ", "))
		}
		if v.PendingRemoval != "" {
			say(messages.InstallPendingRemoval, strings.Join(v.Removed, ", "), v.PendingRemoval)
		}
		for _, p := range v.PathRemoved {
			say(messages.InstallPathRemoved, p)
		}
		if v.ShellRCUpdated {
			say(messages.InstallUninstalled, v.ShellRCPath)
		}
		return
	}
	if v.ManualCommand != "" {
		say(messages.InstallManual, v.ManualCommand)
		return
	}
	if v.AlreadyInstalled {
		say(messages.InstallAlreadyInstalled, v.Path)
	} else {
		say(messages.InstallPlaced, v.Path)
	}
	switch {
	case v.OnPath:
		say(messages.InstallOnPath, v.Dir)
	case v.PathUpdated:
		say(messages.InstallPathUpdated, v.Dir, strings.Join(v.PathTargets, ", "))
	case v.PathConfigured:
		say(messages.InstallPathConfigured, v.Dir)
	default:
		say(messages.InstallPathNotOnPath, v.Dir)
	}
	if v.ShellRCUpdated {
		say(messages.InstallShellRCUpdated, v.ShellRCPath)
	}
	if v.NewTerminal {
		say(messages.InstallOpenNewTerminal)
	}
}

// bindReporter attaches a HumanReporter bound to this invocation's actual
// output writers to every dependency bundle that drives a Reporter.
//
// The composition root cannot do this itself: it builds Runtime before
// anyone knows which writers the invocation will use, so the Reporter
// fields it hands over are nil. Execute is the first point where both the
// bundles and the writers exist, which makes it the only correct place to
// join them. A use case that calls Reporter on a nil field panics, and a
// test suite that injects its own fake reporter can never observe that —
// only a run through the real command path can.
//
// An already-populated Reporter is left untouched so a caller that wires
// its own (a test, or a future GUI-hosted invocation) keeps it.
func bindReporter(rt *Runtime, stdout, stderr io.Writer) {
	reporter := &HumanReporter{Out: stdout, Err: stderr}
	if rt.Deps.Reporter == nil {
		rt.Deps.Reporter = reporter
	}
	if rt.ExecDeps.Reporter == nil {
		rt.ExecDeps.Reporter = reporter
	}
	if rt.InstallDeps.Reporter == nil {
		rt.InstallDeps.Reporter = reporter
	}
	if rt.ProjectWizard.Reporter == nil {
		rt.ProjectWizard.Reporter = reporter
	}
}
