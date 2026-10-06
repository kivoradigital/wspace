// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

// newContextCommand groups the context-lifecycle subcommands
// (context-management spec: "Context lifecycle commands" — create, list,
// switch and edit; remove is this change's own authorized gap-closure
// addition — see internal/app/remove_context.go's own doc comment for why
// a context with no way to remove it is the same product hole as one with
// no way to edit it).
func newContextCommand(rt *Runtime) *cobra.Command {
	root := &cobra.Command{
		Use:   "context",
		Short: "manage contexts (create, list, switch, show, edit, remove)",
	}
	root.AddCommand(
		newContextCreateCommand(rt),
		newContextListCommand(rt),
		newContextSwitchCommand(rt),
		newContextShowCommand(rt),
		newContextEditCommand(rt),
		newContextRemoveCommand(rt),
		newContextImportCommand(rt),
	)
	return root
}

func newContextCreateCommand(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "create a new context by answering a few questions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// app.RunCreateContext owns the sequence (context fields, then
			// immediately project registration in the same session) so a
			// freshly created context is usable without a second command —
			// this command never composes the two wizard calls itself
			// (that composition used to live here alone, which is why
			// internal/gui/wizard's own "New context…" silently got a
			// shorter flow; see RunCreateContext's own doc comment).
			c, err := app.RunCreateContext(cmd.Context(), app.CreateContextDeps{
				Store:    rt.ContextWizard.Store,
				FS:       rt.ProjectWizard.FS,
				Git:      rt.ProjectWizard.Git,
				Prompter: rt.ContextWizard.Prompter,
				Reporter: rt.ProjectWizard.Reporter,
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIContextCreated, string(c.Name)))
			return nil
		},
	}
}

func newContextListCommand(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list every registered context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			names, err := app.ListContexts(cmd.Context(), rt.Deps)
			if err != nil {
				return err
			}
			root, err := app.LoadRoot(cmd.Context(), rt.Deps)
			if err != nil {
				return err
			}

			if jsonRequested(cmd) {
				contexts := make([]domain.Context, 0, len(names))
				for _, n := range names {
					c, err := app.LoadContext(cmd.Context(), rt.Deps, n)
					if err != nil {
						return err
					}
					contexts = append(contexts, c)
				}
				out, err := encodeContextListJSON(contexts, root.ActiveContext)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			if len(names) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIContextListEmpty))
				return nil
			}
			for _, n := range names {
				marker := ""
				if n == root.ActiveContext {
					marker = messages.T(messages.CLIContextActiveMarker)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIContextListRow, string(n), marker))
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newContextSwitchCommand(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "switch <name>",
		Short: "set the active context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := domain.NewContextName(args[0])
			if err != nil {
				return err
			}
			if err := app.SwitchContext(cmd.Context(), rt.Deps.Store, name); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIContextSwitched, string(name)))
			return nil
		},
	}
}

func newContextShowCommand(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "show a context's roots, defaults, and every option's winning layer",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var ctxRec domain.Context
			var err error
			if len(args) == 1 {
				var name domain.ContextName
				name, err = domain.NewContextName(args[0])
				if err != nil {
					return err
				}
				ctxRec, err = app.LoadContext(cmd.Context(), rt.Deps, name)
			} else {
				ctxRec, err = resolveContext(cmd, rt)
			}
			if err != nil {
				return err
			}

			overlay, err := resolveOverlay(cmd, rt)
			if err != nil {
				return err
			}
			resolver := domain.Resolver{Context: &ctxRec, Overlay: overlay}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIInfoContext, string(ctxRec.Name)))
			printResolvedOptions(cmd, resolver)
			return nil
		},
	}
}

// newContextEditCommand drives app.RunEditContextWizard — the same
// promptContextFields sequence "context create" uses, prefilled from the
// named context's (or, with no argument, the resolved active context's)
// current values (context-management spec: "create, list, switch and edit
// operations on contexts"; this change's own authorized gap-closure note).
func newContextEditCommand(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "edit [name]",
		Short: "revisit an existing context's option set by answering the same questions again",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name domain.ContextName
			var err error
			if len(args) == 1 {
				name, err = domain.NewContextName(args[0])
				if err != nil {
					return err
				}
			} else {
				var ctxRec domain.Context
				ctxRec, err = resolveContext(cmd, rt)
				if err != nil {
					return err
				}
				name = ctxRec.Name
			}

			c, re, err := app.RunEditContextWizard(cmd.Context(), rt.ContextWizard.Store, rt.Deps.FS, rt.ContextWizard.Prompter, name)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIContextEdited, string(c.Name)))
			if len(re.Reassigned) > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIContextReassigned, len(re.Reassigned), string(c.Name)))
			}
			for _, f := range re.Failures {
				subject := f.Name
				if subject == "" {
					subject = string(f.Root)
				}
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), messages.T(messages.CLIContextReassignFailed, subject, f.Err.Error()))
			}
			return nil
		},
	}
}

// newContextRemoveCommand drives app.RemoveContext (this change's own
// authorized gap-closure addition). The context name is always required
// explicitly — unlike "edit", there is no "default to the active context"
// fallback here, since app.RemoveContext refuses to remove the active
// context (unless --allow-active) and a destructive command should never
// guess its own target.
func newContextRemoveCommand(rt *Runtime) *cobra.Command {
	var allowActive bool
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "remove a registered context (refuses the active one unless --allow-active)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := domain.NewContextName(args[0])
			if err != nil {
				return err
			}
			if err := app.RemoveContextWith(cmd.Context(), rt.Deps.Store, name, app.RemoveContextOptions{AllowActive: allowActive}); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), messages.T(messages.CLIContextRemoved, string(name)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&allowActive, "allow-active", false, "also remove the active context (even the only one), leaving no active context")
	return cmd
}
