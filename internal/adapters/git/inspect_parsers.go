// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
)

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parsePatch parses unified diff output produced with fixed a/ and b/
// prefixes. File names come from the extended headers (rename/copy
// from/to) or the ---/+++ lines, C-unquoted, so names with spaces, quotes
// or newlines survive. byteTruncated says raw was cut by the byte cap:
// its partial last line is dropped and the patch is marked truncated.
// limits.MaxLines caps the number of hunk lines kept.
func parsePatch(raw string, byteTruncated bool, limits domain.DiffLimits) domain.Patch {
	p := domain.Patch{Files: []domain.FileDiff{}}
	if byteTruncated {
		if i := strings.LastIndexByte(raw, '\n'); i >= 0 {
			raw = raw[:i+1]
		} else {
			raw = ""
		}
	}
	if raw == "" {
		p.Truncated = byteTruncated
		return p
	}
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")

	var cur *domain.FileDiff
	var hunk *domain.DiffHunk
	kept := 0
	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			if cur.Status == "" {
				cur.Status = domain.FileModified
			}
			if cur.Hunks == nil {
				cur.Hunks = []domain.DiffHunk{}
			}
			p.Files = append(p.Files, *cur)
		}
		cur = nil
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "diff --cc ") || strings.HasPrefix(line, "diff --combined "):
			flushFile()
			cur = &domain.FileDiff{}
			cur.Path = headerPath(line)
		case cur == nil:
			// preamble (none expected with these flags)
		case hunk != nil && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "\\") || line == ""):
			if limits.MaxLines > 0 && kept >= limits.MaxLines {
				cur.Truncated = true
				p.Truncated = true
				flushFile()
				return p
			}
			kept++
			hunk.Lines = append(hunk.Lines, line)
			// combined diffs carry one prefix column per parent; only the
			// plain two-way prefix counts toward additions/deletions.
			if strings.HasPrefix(line, "+") {
				cur.Additions++
			} else if strings.HasPrefix(line, "-") {
				cur.Deletions++
			}
		case strings.HasPrefix(line, "@@"):
			flushHunk()
			hunk = &domain.DiffHunk{Header: line, Lines: []string{}}
			if m := hunkHeaderPattern.FindStringSubmatch(line); m != nil {
				hunk.OldStart, _ = strconv.Atoi(m[1])
				hunk.OldLines = 1
				if m[2] != "" {
					hunk.OldLines, _ = strconv.Atoi(m[2])
				}
				hunk.NewStart, _ = strconv.Atoi(m[3])
				hunk.NewLines = 1
				if m[4] != "" {
					hunk.NewLines, _ = strconv.Atoi(m[4])
				}
			}
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = domain.FileAdded
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = domain.FileDeleted
		case strings.HasPrefix(line, "rename from "):
			cur.Status = domain.FileRenamed
			cur.OrigPath = unquoteC(strings.TrimPrefix(line, "rename from "))
		case strings.HasPrefix(line, "rename to "):
			cur.Path = unquoteC(strings.TrimPrefix(line, "rename to "))
		case strings.HasPrefix(line, "copy from "):
			cur.Status = domain.FileCopied
			cur.OrigPath = unquoteC(strings.TrimPrefix(line, "copy from "))
		case strings.HasPrefix(line, "copy to "):
			cur.Path = unquoteC(strings.TrimPrefix(line, "copy to "))
		case strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " differ"), line == "GIT binary patch":
			cur.Binary = true
		case strings.HasPrefix(line, "--- "):
			if name, ok := patchName(strings.TrimPrefix(line, "--- "), "a/"); ok && cur.Status != domain.FileRenamed && cur.Status != domain.FileCopied {
				cur.Path = name
			}
		case strings.HasPrefix(line, "+++ "):
			if name, ok := patchName(strings.TrimPrefix(line, "+++ "), "b/"); ok {
				cur.Path = name
			}
		}
	}
	if byteTruncated && cur != nil {
		cur.Truncated = true
	}
	flushFile()
	p.Truncated = byteTruncated
	return p
}

// patchName reads a ---/+++ name: "/dev/null" is no name; git appends a
// TAB to an unquoted name that contains a space.
func patchName(s, prefix string) (string, bool) {
	s = strings.TrimSuffix(s, "\t")
	if s == "/dev/null" {
		return "", false
	}
	return strings.TrimPrefix(unquoteC(s), prefix), true
}

// headerPath reads the file name from a "diff --git a/X b/Y" (or "diff
// --cc X") line. It is only the fallback for sections without ---/+++
// lines (binary files, mode-only changes); an unquoted header is split
// assuming both names are equal, as git apply does.
func headerPath(line string) string {
	if rest, ok := strings.CutPrefix(line, "diff --cc "); ok {
		return unquoteC(rest)
	}
	if rest, ok := strings.CutPrefix(line, "diff --combined "); ok {
		return unquoteC(rest)
	}
	rest := strings.TrimPrefix(line, "diff --git ")
	if strings.HasPrefix(rest, "\"") {
		if end := closingQuote(rest); end > 0 {
			second := strings.TrimSpace(rest[end+1:])
			return strings.TrimPrefix(unquoteC(second), "b/")
		}
	}
	if n := len(rest); n >= 5 && (n-1)%2 == 0 {
		half := (n - 1) / 2
		a, b := rest[:half], rest[half+1:]
		if strings.HasPrefix(a, "a/") && strings.HasPrefix(b, "b/") && a[2:] == b[2:] {
			return b[2:]
		}
	}
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return rest[i+3:]
	}
	return rest
}

func closingQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

// unquoteC reverses git's C-style quoting of a path ("..." with \" \\ \t
// \n and \ooo octal escapes). An unquoted string is returned unchanged.
func unquoteC(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	s = s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch e := s[i]; e {
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'v':
			b.WriteByte('\v')
		case 'f':
			b.WriteByte('\f')
		case 'r':
			b.WriteByte('\r')
		case '0', '1', '2', '3':
			if i+2 < len(s) {
				if v, err := strconv.ParseUint(s[i:i+3], 8, 8); err == nil {
					b.WriteByte(byte(v))
					i += 2
					continue
				}
			}
			b.WriteByte(e)
		default:
			b.WriteByte(e)
		}
	}
	return b.String()
}

// splitRecords splits NUL-terminated records, dropping a trailing empty.
func splitRecords(raw string) []string {
	recs := strings.Split(raw, "\x00")
	out := recs[:0]
	for _, r := range recs {
		r = strings.TrimPrefix(r, "\n")
		if r != "" {
			out = append(out, r)
		}
	}
	return out
}

// commitLogFormat is `log -z --format=` for parseCommitLog: hash, short
// hash, author name, strict ISO author date, subject. Never %ae (privacy).
const commitLogFormat = "%H%x1f%h%x1f%an%x1f%aI%x1f%s"

func parseCommitLog(raw string) ([]domain.CommitInfo, error) {
	out := []domain.CommitInfo{}
	for _, rec := range splitRecords(raw) {
		f := strings.SplitN(rec, "\x1f", 5)
		if len(f) != 5 {
			return nil, fmt.Errorf("unexpected log record %q", rec)
		}
		date, err := time.Parse(time.RFC3339, f[3])
		if err != nil {
			return nil, fmt.Errorf("parsing date %q: %w", f[3], err)
		}
		out = append(out, domain.CommitInfo{Hash: f[0], ShortHash: f[1], AuthorName: f[2], AuthorDate: date, Subject: f[4]})
	}
	return out, nil
}

// stashListFormat is `stash list -z --format=` for parseStashList.
const stashListFormat = "%gd%x1f%H%x1f%gs%x1f%aI"

func parseStashList(raw string) ([]domain.StashEntry, error) {
	out := []domain.StashEntry{}
	for _, rec := range splitRecords(raw) {
		f := strings.SplitN(rec, "\x1f", 4)
		if len(f) != 4 {
			return nil, fmt.Errorf("unexpected stash record %q", rec)
		}
		idx, err := stashIndex(f[0])
		if err != nil {
			return nil, err
		}
		date, err := time.Parse(time.RFC3339, f[3])
		if err != nil {
			return nil, fmt.Errorf("parsing date %q: %w", f[3], err)
		}
		branch, msg := domain.ParseStashSubject(f[2])
		out = append(out, domain.StashEntry{Index: idx, Ref: f[0], Hash: f[1], Branch: branch, Message: msg, Date: date})
	}
	return out, nil
}

func stashIndex(ref string) (int, error) {
	open, end := strings.Index(ref, "@{"), strings.LastIndex(ref, "}")
	if open < 0 || end < open {
		return 0, fmt.Errorf("unexpected stash ref %q", ref)
	}
	return strconv.Atoi(ref[open+2 : end])
}

// upstreamFormat is `for-each-ref --format=` for parseUpstream.
const upstreamFormat = "%(upstream)%1f%(upstream:short)%1f%(upstream:remotename)%1f%(upstream:track,nobracket)%1f%(upstream:remoteref)"

func parseUpstream(raw string) (domain.UpstreamInfo, bool) {
	f := strings.SplitN(strings.TrimRight(raw, "\n"), "\x1f", 5)
	if len(f) < 4 || f[0] == "" {
		return domain.UpstreamInfo{}, false
	}
	u := domain.UpstreamInfo{Ref: f[1], Remote: f[2], Gone: f[3] == "gone"}
	if len(f) == 5 {
		u.RemoteRef = f[4]
	}
	return u, true
}

// parseOverwrittenFiles lists the TAB-indented paths git prints when a
// merge refuses to overwrite local changes or untracked files; nil when
// stderr is not that refusal.
func parseOverwrittenFiles(stderr string) []string {
	if !strings.Contains(stderr, "would be overwritten by merge") {
		return nil
	}
	var files []string
	in := false
	for _, line := range strings.Split(stderr, "\n") {
		switch {
		case strings.Contains(line, "would be overwritten by merge"):
			in = true
		case in && strings.HasPrefix(line, "\t"):
			files = append(files, strings.TrimPrefix(line, "\t"))
		default:
			in = false
		}
	}
	return files
}
