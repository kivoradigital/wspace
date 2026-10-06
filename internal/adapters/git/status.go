// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
)

// Status runs `status --porcelain=v1 -z --untracked-files=all` and parses
// its NUL-delimited records. -z is mandatory: it is the only way a path
// containing a space, a quote or a newline survives intact (design.md §7,
// named §13 test TestParseStatus_PathWithNewline).
func (a *Adapter) Status(ctx context.Context, worktree domain.Path) ([]domain.PorcelainEntry, error) {
	out, err := a.run(ctx, "git.status", worktree, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parseStatusZ(out), nil
}

// parseStatusZ parses `git status --porcelain=v1 -z` output. Each record is
// "XY<space><path>", NUL-terminated. A rename or copy (X in {R, C})
// consumes one additional NUL-terminated field: the original path, which
// (with -z) is a separate field rather than the "old -> new" arrow form
// used without -z (design.md §7).
func parseStatusZ(raw string) []domain.PorcelainEntry {
	if raw == "" {
		return nil
	}
	tokens := strings.Split(raw, "\x00")
	if n := len(tokens); n > 0 && tokens[n-1] == "" {
		tokens = tokens[:n-1]
	}

	var entries []domain.PorcelainEntry
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if len(tok) < 3 {
			continue
		}
		x, y := tok[0], tok[1]
		path := tok[3:]
		entry := domain.PorcelainEntry{X: x, Y: y, RelPath: path}
		if (x == 'R' || x == 'C' || y == 'R' || y == 'C') && i+1 < len(tokens) {
			i++ // the next field is the rename/copy source path
			entry.OrigPath = tokens[i]
		}
		entries = append(entries, entry)
	}
	return entries
}

// parseLeftRightCount parses the single tab-separated line produced by
// `rev-list --left-right --count <upstream>...HEAD`: behind<TAB>ahead.
func parseLeftRightCount(out string) (behind, ahead int, err error) {
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("expected 2 tab-separated fields, got %q", out)
	}
	behind, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parsing behind count %q: %w", fields[0], err)
	}
	ahead, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parsing ahead count %q: %w", fields[1], err)
	}
	return behind, ahead, nil
}
