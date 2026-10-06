// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

// FakeTreeCloner is a ports.TreeCloner over a FakeFS: a successful
// CloneTree creates dst with a ".cloned" marker file, so later Exists
// checks (and rollback's RemoveAll) see it. Method is what it reports
// (CloneMethodCopy when empty); Err makes every call fail without
// creating anything. Calls records each [src, dst].
type FakeTreeCloner struct {
	FS     *FakeFS
	Method string
	Err    error
	Calls  [][2]domain.Path
}

var _ ports.TreeCloner = (*FakeTreeCloner)(nil)

func (c *FakeTreeCloner) CloneTree(src, dst domain.Path) (string, error) {
	c.Calls = append(c.Calls, [2]domain.Path{src, dst})
	if c.Err != nil {
		return "", c.Err
	}
	if err := c.FS.MkdirAll(dst); err != nil {
		return "", err
	}
	if err := c.FS.WriteFile(dst.Join(".cloned"), []byte(src), 0o644); err != nil {
		return "", err
	}
	if c.Method == "" {
		return ports.CloneMethodCopy, nil
	}
	return c.Method, nil
}
