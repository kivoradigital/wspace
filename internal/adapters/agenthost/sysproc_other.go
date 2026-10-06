// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build !windows

package agenthost

import "os/exec"

// applyCmdLine is Windows-only: Resolver builds a raw command line only
// for a Windows batch file.
func applyCmdLine(*exec.Cmd, string) {}
