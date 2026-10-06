// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build windows

package agenthost

import (
	"os/exec"
	"syscall"
)

// applyCmdLine hands Windows the complete, already quoted command line
// (a batch file run through cmd.exe), bypassing os/exec's own quoting.
func applyCmdLine(cmd *exec.Cmd, line string) {
	if line == "" {
		return
	}
	cmd.Args = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
}
