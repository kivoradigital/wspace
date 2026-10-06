// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"errors"
	"fmt"
	"strings"
)

// BranchName is a git branch/ref name validated against git's
// check-ref-format rules (a practical subset) before any adapter or
// subprocess ever sees the value. This is what makes a branch or alias
// literally named "--upload-pack=..." unable to become a git flag
// (design.md §13, threat: ref/path argument injection into git).
type BranchName string

// ErrInvalidBranchName is the sentinel wrapped by every NewBranchName
// rejection; callers may match it with errors.Is.
var ErrInvalidBranchName = errors.New("invalid branch name")

// invalidBranchChars are characters git's check-ref-format always rejects.
const invalidBranchChars = " ~^:?*[\\"

// NewBranchName validates s against git's check-ref-format rules:
//   - not empty, not "-"-leading (flag-like), not exactly "@"
//   - no ".." sequence, no "@{", no control characters
//   - none of " ~^:?*[\\"
//   - no leading/trailing/double "/"
//   - no path component starting with "."
//   - does not end with ".lock" or "."
func NewBranchName(s string) (BranchName, error) {
	if reason := invalidBranchNameReason(s); reason != "" {
		return "", fmt.Errorf("%w: %q %s", ErrInvalidBranchName, s, reason)
	}
	return BranchName(s), nil
}

func invalidBranchNameReason(s string) string {
	switch {
	case s == "":
		return "must not be empty"
	case s == "@":
		return "must not be exactly \"@\""
	case strings.HasPrefix(s, "-"):
		return "must not start with \"-\" (flag-like)"
	case strings.Contains(s, ".."):
		return "must not contain \"..\""
	case strings.Contains(s, "@{"):
		return "must not contain \"@{\""
	case strings.ContainsAny(s, invalidBranchChars):
		return "must not contain any of \" ~^:?*[\\\\\""
	case strings.HasSuffix(s, ".lock"):
		return "must not end with \".lock\""
	case strings.HasSuffix(s, "."):
		return "must not end with \".\""
	case strings.HasPrefix(s, "/"), strings.HasSuffix(s, "/"):
		return "must not start or end with \"/\""
	case strings.Contains(s, "//"):
		return "must not contain \"//\""
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "must not contain control characters"
		}
	}
	for _, component := range strings.Split(s, "/") {
		if strings.HasPrefix(component, ".") {
			return "must not have a path component starting with \".\""
		}
	}
	return ""
}
