// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"errors"
	"fmt"
	"regexp"
)

// ContextName identifies a context: filesystem-safe by construction so it
// can be used directly as a directory name under <config>/ws/contexts.
type ContextName string

// ErrInvalidContextName is the sentinel wrapped by every NewContextName
// rejection; callers may match it with errors.Is.
var ErrInvalidContextName = errors.New("invalid context name")

var contextNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// NewContextName validates s against [a-z0-9][a-z0-9_-]{0,63}.
func NewContextName(s string) (ContextName, error) {
	if !contextNamePattern.MatchString(s) {
		return "", fmt.Errorf("%w: %q", ErrInvalidContextName, s)
	}
	return ContextName(s), nil
}
