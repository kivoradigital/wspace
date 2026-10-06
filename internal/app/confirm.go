// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// PrompterDeps bundles the one port a generic yes/no confirmation needs, so
// internal/cli can hold an app-owned value instead of a bare
// ports.Prompter (R6: internal/cli may only import app, domain, messages).
type PrompterDeps struct {
	Prompter ports.Prompter
}

// Confirm drives a single yes/no prompt through Prompter.Confirm. It exists
// so a CLI command (e.g. `destroy` without --force, tasks.md 4b.11) can ask
// for confirmation without ever naming ports.ConfirmField itself.
func Confirm(ctx context.Context, deps PrompterDeps, label messages.Key, args []any, def bool) (bool, error) {
	return deps.Prompter.Confirm(ctx, ports.ConfirmField{
		Field:   ports.Field{Label: label, Args: args},
		Default: def,
	})
}

// ConfirmWithHelp is Confirm whose args fill a help text shown after the
// label (a prompt label itself never takes arguments).
func ConfirmWithHelp(ctx context.Context, deps PrompterDeps, label, help messages.Key, args []any, def bool) (bool, error) {
	return deps.Prompter.Confirm(ctx, ports.ConfirmField{
		Field:   ports.Field{Label: label, Help: help, Args: args},
		Default: def,
	})
}
