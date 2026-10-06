// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"strconv"
	"strings"
)

// LegacyReason classifies why one entry of a legacy flat workspace
// configuration file was not applied as-is. It is a typed code, not a
// rendered sentence (mirroring ErrCode/OpError, R1: internal/domain must
// never construct user-facing prose) — internal/messages owns the
// code -> catalog-string bridge for these exactly like domain.ErrCode's
// own ForCode bridge.
type LegacyReason string

const (
	// LegacyReasonUnknownKey marks a key this parser does not recognize at
	// all — reported, never silently dropped, so a typo or a future legacy
	// field is always visible to the person running the import.
	LegacyReasonUnknownKey LegacyReason = "unknown_key"
	// LegacyReasonNotImported marks a recognized key that has no
	// destination in the wspace model (project_prefixes: a picker
	// pre-selection convenience the legacy tool used, with nothing for
	// wspace to map it to, since wspace registers projects explicitly).
	LegacyReasonNotImported LegacyReason = "not_imported"
	// LegacyReasonInvalidValue marks a mapped key whose value failed
	// validation; the field is treated as absent (never coerced, never
	// defaulted) and reported with the raw value in Detail.
	LegacyReasonInvalidValue LegacyReason = "invalid_value"
	// LegacyReasonMalformedLine marks a non-blank, non-comment line before
	// "[projects]" that has no "key = value" shape at all.
	LegacyReasonMalformedLine LegacyReason = "malformed_line"
	// LegacyReasonMissingProjectsSection marks a source file with no
	// "[projects]" section at all — not fatal, just means zero projects are
	// offered for import.
	LegacyReasonMissingProjectsSection LegacyReason = "missing_projects_section"
)

// LegacyIssue is one parse-time or mapping-time note: a key/line that was
// not applied as written, plus why.
type LegacyIssue struct {
	Key    string // the raw key, or the raw line text for a malformed line
	Reason LegacyReason
	Detail string // the raw value that failed, when applicable
}

// LegacyFlatConfig is the pure parse result of a legacy flat
// "key = value" workspace configuration file's contents (this change's
// own import feature). Context carries every successfully parsed,
// successfully validated field — WorkspacesRoot, ProjectsRoot and
// Defaults only; Context.Projects is always left nil here, since turning
// a project name into a domain.Project requires joining it onto
// ProjectsRoot and checking the result on disk, both of which are I/O
// this pure parser (R1: no I/O) cannot perform — that join and validation
// is internal/app's job (this change's import use case), fed
// ProjectNames below.
type LegacyFlatConfig struct {
	Context      Context
	ProjectNames []string // raw names from "[projects]", in file order
	Issues       []LegacyIssue
}

// legacy flat config key names (the source format's own vocabulary, never
// a real product's name — see this change's own naming rule).
const (
	legacyKeyWorkspacesRoot    = "workspaces_root"
	legacyKeyProjectsRoot      = "projects_root"
	legacyKeyProjectPrefixes   = "project_prefixes"
	legacyKeyBaseBranch        = "base_branch"
	legacyKeyBranchPrefix      = "branch_prefix"
	legacyKeyCopyEnvDefault    = "copy_env_default"
	legacyKeyFetchBeforeCreate = "fetch_before_create"
	legacyKeyEnvPruneDirs      = "env_prune_dirs"
)

const legacyProjectsSectionHeader = "[projects]"

// ParseLegacyFlatConfig parses the contents of a legacy flat workspace
// configuration file (this change's import feature, spec: "Source
// format"). It never fails outright: a malformed line, an unknown key, or
// an invalid value for a mapped key is reported as a LegacyIssue and
// treated as absent, never invented, coerced, or allowed to abort the
// rest of the parse.
func ParseLegacyFlatConfig(data []byte) LegacyFlatConfig {
	var out LegacyFlatConfig
	seenProjectsSection := false

	lines := strings.Split(string(data), "\n")
	i := 0
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == legacyProjectsSectionHeader {
			seenProjectsSection = true
			i++
			break
		}

		key, value, ok := splitLegacyLine(line)
		if !ok {
			out.Issues = append(out.Issues, LegacyIssue{Key: line, Reason: LegacyReasonMalformedLine})
			continue
		}
		applyLegacyKey(&out, key, value)
	}

	if seenProjectsSection {
		for ; i < len(lines); i++ {
			name := strings.TrimSpace(lines[i])
			if name == "" || strings.HasPrefix(name, "#") {
				continue
			}
			out.ProjectNames = append(out.ProjectNames, name)
		}
	} else {
		out.Issues = append(out.Issues, LegacyIssue{Key: legacyProjectsSectionHeader, Reason: LegacyReasonMissingProjectsSection})
	}

	return out
}

// splitLegacyLine splits "key = value" on the first "=", trimming both
// sides. A line with no "=" at all is malformed.
func splitLegacyLine(line string) (key, value string, ok bool) {
	idx := strings.IndexByte(line, '=')
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

// applyLegacyKey maps one parsed key/value pair onto out.Context, or
// records an issue when the key is unknown, unmapped, or its value fails
// validation. Duplicate keys resolve last-write-wins, matching how a flat
// key=value file is ordinarily read.
func applyLegacyKey(out *LegacyFlatConfig, key, value string) {
	switch key {
	case legacyKeyWorkspacesRoot:
		if p, err := NewPath(value); err == nil {
			out.Context.WorkspacesRoot = p
		} else {
			out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonInvalidValue, Detail: value})
		}

	case legacyKeyProjectsRoot:
		if p, err := NewPath(value); err == nil {
			out.Context.ProjectsRoot = p
		} else {
			out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonInvalidValue, Detail: value})
		}

	case legacyKeyProjectPrefixes:
		// Recognized, but has no destination in the wspace model (it was a
		// picker pre-selection convenience; wspace registers projects
		// explicitly) — always reported, never applied.
		out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonNotImported, Detail: value})

	case legacyKeyBaseBranch:
		if bn, err := NewBranchName(value); err == nil {
			out.Context.Defaults.BaseBranch = &bn
		} else {
			out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonInvalidValue, Detail: value})
		}

	case legacyKeyBranchPrefix:
		if value == "" {
			out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonInvalidValue, Detail: value})
			return
		}
		v := value
		out.Context.Defaults.BranchPrefix = &v

	case legacyKeyCopyEnvDefault:
		if b, err := parseLegacyYesNo(value); err == nil {
			out.Context.Defaults.CopyEnv = &b
		} else {
			out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonInvalidValue, Detail: value})
		}

	case legacyKeyFetchBeforeCreate:
		if b, err := parseLegacyYesNo(value); err == nil {
			out.Context.Defaults.FetchBeforeCreate = &b
		} else {
			out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonInvalidValue, Detail: value})
		}

	case legacyKeyEnvPruneDirs:
		dirs := splitLegacyList(value)
		if len(dirs) == 0 {
			out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonInvalidValue, Detail: value})
			return
		}
		out.Context.Defaults.EnvPruneDirs = dirs

	default:
		out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonUnknownKey, Detail: value})
	}
}

// splitLegacyList splits a comma-separated legacy value into trimmed,
// non-empty entries, mirroring app.parseIgnorePatterns's own comma
// handling (a directory name may contain a space but essentially never a
// comma).
func splitLegacyList(value string) []string {
	var out []string
	for _, raw := range strings.Split(value, ",") {
		v := strings.TrimSpace(raw)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// parseLegacyYesNo parses the legacy format's boolean vocabulary: "yes" or
// "no", case-insensitively (this change's own documented choice — the
// sample source file only ever shows lower-case "yes", but matching case
// loosely costs nothing and avoids rejecting an otherwise-valid file over
// a harmless capitalization difference). Any other spelling ("true",
// "1", "on", ...) is rejected rather than guessed at, since the legacy
// format's own vocabulary is exactly "yes"/"no" (spec: "Source format").
func parseLegacyYesNo(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes":
		return true, nil
	case "no":
		return false, nil
	default:
		return false, strconv.ErrSyntax
	}
}
