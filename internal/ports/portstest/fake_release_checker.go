// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
)

// FakeReleaseChecker returns a fixed Response or Err and counts calls, so
// tests can prove a caller's caching behavior without a real HTTP round
// trip.
type FakeReleaseChecker struct {
	Response domain.ReleaseInfo
	Err      error
	Calls    int
}

// NewFakeReleaseChecker constructs an empty FakeReleaseChecker.
func NewFakeReleaseChecker() *FakeReleaseChecker {
	return &FakeReleaseChecker{}
}

func (r *FakeReleaseChecker) Latest(_ context.Context, _ domain.RepoCoordinates) (domain.ReleaseInfo, error) {
	r.Calls++
	if r.Err != nil {
		return domain.ReleaseInfo{}, r.Err
	}
	return r.Response, nil
}
