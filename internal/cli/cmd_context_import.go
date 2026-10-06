// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/spf13/cobra"
)

// newContextImportCommand drives app.ImportLegacyContext (this change's
// own import feature): turning a legacy flat "key = value" workspace
// configuration file into a wspace context.
//
// Mode selection is by argument shape, deliberately mirroring "context
// edit [name]"'s own convention rather than adding a second flag to
// choose between them: a positional name argument means "into an
// existing context" (that context must already exist, exactly like
// "context edit <name>" targets one by name) and --name is never read in
// that mode; omitting the positional argument means "into a new context",
// where --name is optional (the wizard prompts for one, with a suggested,
// editable default, when it is left unset) — this is documented here as
// a deliberate design decision resolving the spec's own ambiguity about
// how a CLI caller expresses which of the two modes they want.
func newContextImportCommand(rt *Runtime) *cobra.Command {
	var from string
	var name string

	cmd := &cobra.Command{
		Use:   "import [name]",
		Short: "import a legacy flat key=value workspace configuration file into a context",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps := depsWithHumanReporter(cmd, rt)
			importDeps := app.ImportLegacyContextDeps{
				Store:    deps.Store,
				FS:       deps.FS,
				Git:      deps.Git,
				Reporter: deps.Reporter,
				Prompter: rt.ContextWizard.Prompter,
			}

			in := app.ImportLegacyContextInput{From: domain.Path(from), NewName: name}
			if len(args) == 1 {
				targetName, err := domain.NewContextName(args[0])
				if err != nil {
					return err
				}
				in.TargetName = targetName
			}

			result, err := app.ImportLegacyContext(cmd.Context(), importDeps, in)
			if err != nil {
				return err
			}
			// RenderImportSummary's own first line already states the
			// outcome (created/merged/cancelled) and every project/field
			// note beneath it — the CLI never builds a second, separate
			// "imported" sentence of its own (spec: "do not build the
			// summary text twice").
			for _, line := range app.RenderImportSummary(result) {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "force a specific legacy configuration file, skipping discovery")
	cmd.Flags().StringVar(&name, "name", "", "name for the new context (only used when importing into a new context)")
	return cmd
}
