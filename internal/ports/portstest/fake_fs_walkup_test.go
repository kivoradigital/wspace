// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest_test

import (
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// A Windows-style start path climbs to its volume ("C:") and then to ".";
// WalkUp must report "not found" there instead of looping forever.
func TestFakeFS_WalkUpStopsAtAWindowsVolume(t *testing.T) {
	ffs := portstest.NewFakeFS(t)

	for _, start := range []string{"C:/Users/dev/project", "C:", "/home/dev/project"} {
		dir, found, err := ffs.WalkUp(domain.Path(start), ".wspace")
		if err != nil || found || dir != "" {
			t.Errorf("WalkUp(%q) = (%q, %v, %v), want not found", start, dir, found, err)
		}
	}
}
