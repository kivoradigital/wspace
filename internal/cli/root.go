// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package cli is the cobra command tree driving internal/app. It never
// imports an adapter package directly and never imports internal/gui
// (design.md §1, R6).
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

// Execute builds the command tree, runs it against args, and renders any
// returned error through the catalog (tasks.md 4b.16: "wire error
// rendering in the root command's error handler"). It returns the process
// exit code cmd/ws should use — this is the one function cmd/ws's
// composition root calls.
//
// Locale activation happens here, once per invocation (design.md §10:
// "Resolved once in cmd/*" — this is that one call site). Only "en" is
// registered through phase 4b; a real preference/WS_LANG/LANG chain is a
// future locale's problem, not this one's.
func Execute(rt *Runtime, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = messages.Use("en")

	bindReporter(rt, stdout, stderr)

	root, exitCode := NewRootCommand(rt)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)

	// reachedRunE marks the one boundary cobra guarantees: PersistentPreRunE
	// runs after flag parsing and Args validation both succeeded, strictly
	// before the leaf command's RunE. Any error surfacing from ExecuteC
	// while this is still false was produced by cobra itself — an unknown
	// flag, an unknown subcommand, or a failed Args validator — never by
	// this codebase's own RunE bodies, so it is routed differently below
	// (see renderError's cmd/reachedRunE parameters).
	reachedRunE := false
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		reachedRunE = true
		return nil
	}

	cmd, err := root.ExecuteC()
	if err != nil {
		verbose, _ := root.Flags().GetBool(verboseFlagName)
		_, _ = fmt.Fprintln(stderr, renderError(cmd, err, reachedRunE, verbose))
		return 1
	}
	return *exitCode
}

// NewRootCommand builds the full "ws" command tree wired against rt.
// exitCode is 0 unless a command explicitly sets it (only `exec`,
// propagating the last non-zero exit code observed across a workspace's
// repos, per the shell-integration/cli-surface specs).
func NewRootCommand(rt *Runtime) (*cobra.Command, *int) {
	exitCode := new(int)

	root := &cobra.Command{
		Use:           "wspace",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	addContextFlag(root)
	addVerboseFlag(root)

	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if strings.Contains(err.Error(), "--json") {
			return jsonUnsupportedError(cmd)
		}
		return err
	})

	root.AddCommand(
		newCreateCommand(rt),
		newListCommand(rt),
		newStatusCommand(rt),
		newAddCommand(rt),
		newRmCommand(rt),
		newRepairCommand(rt),
		newAdoptLegacyCommand(rt),
		newClaimCommand(rt),
		newSyncEnvCommand(rt),
		newUpdateCommand(rt),
		newRepoCommand(rt),
		newExecCommand(rt, exitCode),
		newDestroyCommand(rt),
		newDoctorCommand(rt),
		newJumpCommand(rt),
		newShellInitCommand(),
		newInfoCommand(rt),
		newContextCommand(rt),
		newProjectCommand(rt),
		newRPCCommand(rt),
		newMCPCommand(rt),
		newVersionCommand(rt),
		newInstallCommand(rt),
		newAgentsCommand(rt),
	)

	return root, exitCode
}
