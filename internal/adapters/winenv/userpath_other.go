// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

//go:build !windows

package winenv

import "github.com/kivoradigital/wspace/internal/ports"

// Store is the non-Windows stub.
type Store struct{}

var _ ports.UserPathStore = Store{}

// New returns the stub store.
func New() Store { return Store{} }

func (Store) UserPath() (string, bool, error)   { return "", false, ErrUnsupported }
func (Store) SetUserPath(string, bool) error    { return ErrUnsupported }
func (Store) BroadcastEnvironmentChange() error { return ErrUnsupported }
