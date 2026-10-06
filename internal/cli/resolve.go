// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"os"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/spf13/cobra"
)

// contextFlagName is the persistent --context flag every command reads
// (design.md §6 Chain A step 1).
const contextFlagName = "context"

// envContextVar is Chain A step 2 (design.md §6).
const envContextVar = "WSPACE_CONTEXT"

// verboseFlagName is the persistent --verbose/-v flag every command
// inherits; root.go's Execute reads it once, after root.Execute() returns,
// to decide whether renderError appends its diagnostic block.
const verboseFlagName = "verbose"

// resolveContext runs design.md §6 Chain A for cmd, using the persistent
// --context flag and the WSPACE_CONTEXT environment variable.
func resolveContext(cmd *cobra.Command, rt *Runtime) (domain.Context, error) {
	explicit, _ := cmd.Flags().GetString(contextFlagName)
	return app.ResolveContext(cmd.Context(), rt.Deps, app.ResolveContextInput{
		ExplicitName: explicit,
		EnvName:      os.Getenv(envContextVar),
	})
}

// resolveOverlay loads the nearest .ws.yaml above the process's cwd, for
// Chain B's overlay layer (design.md §6). A missing overlay is not an
// error: found=false simply means Resolver.Overlay stays nil.
func resolveOverlay(cmd *cobra.Command, rt *Runtime) (*domain.Overlay, error) {
	cwd, err := app.Cwd(rt.Deps)
	if err != nil {
		return nil, err
	}
	overlay, _, found, err := app.LoadOverlay(cmd.Context(), rt.Deps, cwd)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &overlay, nil
}

// addContextFlag registers the persistent --context flag on cmd, inherited
// by every subcommand.
func addContextFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(contextFlagName, "", "explicit context name (overrides the active context)")
}

// addVerboseFlag registers the persistent --verbose/-v flag on cmd,
// inherited by every subcommand. Its value is read once, in Execute after
// root.Execute() returns an error — a bool persistent flag on the root
// command is a single shared pflag.Flag, so a subcommand parsing
// "--verbose" updates the same value Execute reads back from root.
func addVerboseFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP(verboseFlagName, "v", false, "on error, also print diagnostic detail (operation, code, raw detail, and the unwrapped error chain)")
}
