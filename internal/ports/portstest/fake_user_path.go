// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import "github.com/kivoradigital/wspace/internal/ports"

// FakeUserPath is an in-memory ports.UserPathStore. A zero value is a
// missing Path value. Writes and Broadcasts count the calls.
type FakeUserPath struct {
	Value      string
	Expand     bool
	Writes     int
	Broadcasts int
	Err        error
}

var _ ports.UserPathStore = (*FakeUserPath)(nil)

func (f *FakeUserPath) UserPath() (string, bool, error) {
	if f.Err != nil {
		return "", false, f.Err
	}
	if f.Value == "" {
		return "", true, nil
	}
	return f.Value, f.Expand, nil
}

func (f *FakeUserPath) SetUserPath(value string, expand bool) error {
	if f.Err != nil {
		return f.Err
	}
	f.Value, f.Expand = value, expand
	f.Writes++
	return nil
}

func (f *FakeUserPath) BroadcastEnvironmentChange() error {
	f.Broadcasts++
	return nil
}
