// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/kivoradigital/wspace/internal/ports"
)

// RunDeleteContextWizard lets the user pick, by name, one registered
// context to remove (tray-gui gap-closure #2: "Delete context… must exist
// in the tray"). Unlike the CLI's "context remove <name>", the tray has no
// argument to name a target with — a menu click carries nothing — so this
// shared use case prompts for it instead.
//
// The actual removal, including its refusal to delete the active context,
// is never reimplemented here: RemoveContext already owns that guard
// (internal/app/remove_context.go), and this function delegates to it
// verbatim so the tray and the CLI can never drift on what "cannot be
// removed" means.
func RunDeleteContextWizard(ctx context.Context, store ports.ConfigStore, prompter ports.Prompter) (domain.ContextName, error) {
	names, err := store.ListContexts(ctx)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", domain.NewOpError("context.delete", domain.CodeContextNotFound, "", "", nil)
	}

	opts := make([]ports.Option, len(names))
	for i, n := range names {
		opts[i] = ports.Option{Raw: string(n)}
	}
	idx, err := prompter.Choose(ctx, ports.ChoiceField{
		Field:   ports.Field{Label: messages.WizardContextToDelete},
		Options: opts,
	})
	if err != nil {
		return "", err
	}
	name := names[idx]

	if err := RemoveContext(ctx, store, name); err != nil {
		return "", err
	}
	return name, nil
}
