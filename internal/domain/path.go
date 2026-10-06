// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Path is a cleaned, absolute, slash-normalized filesystem path. It carries
// no I/O capability of its own (design.md §1, R1) — it is a validated
// string, not a handle.
type Path string

// ErrInvalidPath is the sentinel wrapped by every NewPath rejection; callers
// may match it with errors.Is.
var ErrInvalidPath = errors.New("invalid path")

var windowsDrivePrefix = regexp.MustCompile(`^[A-Za-z]:`)

// NewPath validates s as an absolute path and rejects any ".." component
// anywhere in it, then returns it cleaned and slash-normalized. Absolute is
// accepted either POSIX-style ("/a/b") or Windows-style ("C:\a\b").
func NewPath(s string) (Path, error) {
	if s == "" {
		return "", fmt.Errorf("%w: empty path", ErrInvalidPath)
	}

	normalized := strings.ReplaceAll(s, `\`, "/")

	drive := ""
	if m := windowsDrivePrefix.FindString(normalized); m != "" {
		drive = m
		normalized = normalized[len(m):]
	}

	if !strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("%w: %q is not absolute", ErrInvalidPath, s)
	}

	if hasDotDotComponent(normalized) {
		return "", fmt.Errorf("%w: %q escapes its root via \"..\"", ErrInvalidPath, s)
	}

	cleaned := path.Clean(normalized)
	return Path(drive + cleaned), nil
}

// hasDotDotComponent reports whether a slash-normalized path string contains
// a literal ".." path segment anywhere, shared by NewPath and
// PathTemplate.Resolve so both reject escaping input the same way.
func hasDotDotComponent(normalized string) bool {
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// Join appends elem to p and returns the cleaned result.
func (p Path) Join(elem ...string) Path {
	all := append([]string{string(p)}, elem...)
	return Path(path.Join(all...))
}

// Base returns the last element of p, mirroring path.Base.
func (p Path) Base() string {
	return path.Base(string(p))
}
