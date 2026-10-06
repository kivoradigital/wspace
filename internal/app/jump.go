// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
)

// JumpInput parameterizes Jump.
type JumpInput struct {
	WorkspacesRoot domain.Path
	Name           string
}

// Jump resolves the absolute path of an existing workspace. It is a pure
// path lookup: it never changes any process's working directory — only
// the shell function ShellInit renders can do that, by running `cd` on the
// path Jump returns (shell-integration: "Jump resolves and returns a path
// — it MUST NOT attempt to change any process's working directory").
func Jump(ctx context.Context, deps Deps, in JumpInput) (domain.Path, error) {
	target := in.WorkspacesRoot.Join(in.Name)
	if _, err := deps.Store.LoadManifest(ctx, target); err != nil {
		return "", err
	}
	return target, nil
}
