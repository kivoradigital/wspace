// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"io"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/spf13/cobra"
)

// newInstallCommand implements `install`: copy the running binary into the
// per-user install directory (~/.local/bin, or %LOCALAPPDATA%\Programs\wspace
// on Windows), offer to put that directory on PATH (rc file, or the
// Windows user Path) and to add shell integration. --yes answers yes to
// both for a non-interactive run; --uninstall reverses all of it. It never
// elevates privileges.
//
// install never operates on a specific workspace/context, so unlike almost
// every other command here it does not call resolveContext first.
func newInstallCommand(rt *Runtime) *cobra.Command {
	var in app.InstallInput

	cmd := &cobra.Command{
		Use:   "install",
		Short: "install wspace for this user and put it on PATH",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonRequested(cmd) {
				// Machine output: no questions (use --yes) and no human lines;
				// warnings still go to stderr.
				deps := rt.InstallDeps
				deps.Prompter = nil
				deps.Reporter = &HumanReporter{Out: io.Discard, Err: cmd.ErrOrStderr()}
				res, err := app.Install(cmd.Context(), deps, in)
				if err != nil {
					return err
				}
				return printJSON(cmd, installJSONOf(res))
			}
			deps := installDepsWithHumanReporter(cmd, rt)

			_, err := app.Install(cmd.Context(), deps, in)
			return err
		},
	}
	cmd.Flags().BoolVar(&in.Uninstall, "uninstall", false, "remove the installed binary, its PATH entry and shell integration")
	addJSONFlag(cmd)
	cmd.Flags().BoolVarP(&in.Yes, "yes", "y", false, "do not ask: add the install directory to PATH and add shell integration")
	return cmd
}

// installJSON is `install --json`'s shape (snake_case, like every other
// --json output); paths are native, empty fields omitted.
type installJSON struct {
	Uninstall        bool     `json:"uninstall,omitempty"`
	Installed        bool     `json:"installed"`
	AlreadyInstalled bool     `json:"already_installed,omitempty"`
	Path             string   `json:"path,omitempty"`
	Dir              string   `json:"dir,omitempty"`
	ManualCommand    string   `json:"manual_command,omitempty"`
	OnPath           bool     `json:"on_path"`
	PathConfigured   bool     `json:"path_configured,omitempty"`
	PathUpdated      bool     `json:"path_updated"`
	PathDeclined     bool     `json:"path_declined,omitempty"`
	PathTargets      []string `json:"path_targets,omitempty"`
	NewTerminal      bool     `json:"new_terminal"`
	ShellRCUpdated   bool     `json:"shell_rc_updated,omitempty"`
	ShellRCPath      string   `json:"shell_rc_path,omitempty"`
	Removed          []string `json:"removed,omitempty"`
	PathRemoved      []string `json:"path_removed,omitempty"`
	PendingRemoval   string   `json:"pending_removal,omitempty"`
}

func installJSONOf(r app.InstallResult) installJSON {
	return installJSON{
		Uninstall: r.Uninstall, Installed: r.Installed, AlreadyInstalled: r.AlreadyInstalled, Path: r.Path, Dir: r.Dir,
		ManualCommand: r.ManualCommand, OnPath: r.OnPath, PathConfigured: r.PathConfigured, PathUpdated: r.PathUpdated,
		PathDeclined: r.PathDeclined, PathTargets: r.PathTargets, NewTerminal: r.NewTerminal, ShellRCUpdated: r.ShellRCUpdated,
		ShellRCPath: r.ShellRCPath, Removed: r.Removed, PathRemoved: r.PathRemoved, PendingRemoval: r.PendingRemoval,
	}
}
