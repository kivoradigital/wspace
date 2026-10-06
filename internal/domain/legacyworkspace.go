// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import (
	"strings"
	"time"
)

// LegacyReasonMissingReposSection marks a legacy per-workspace manifest
// with no "[repos]" section at all: it declares no worktrees.
const LegacyReasonMissingReposSection LegacyReason = "missing_repos_section"

// LegacyWorkspaceConfRel is where the legacy bash tool marks a directory
// as one of its workspaces ("<workspace>/.ws/workspace.conf", its own
// WORKSPACE_CONF_REL). wspace only ever reads this file, never writes it:
// the legacy tool may still be managing the same workspace.
const LegacyWorkspaceConfRel = ".ws/workspace.conf"

// LegacyWorkspaceRepo is one "[repos]" line of a legacy per-workspace
// manifest: "alias|project|branch". Alias is the worktree folder inside the
// workspace; Project is the clone's folder name under the legacy tool's
// projects root; Branch may be empty (older manifests), in which case the
// legacy tool falls back to the header's "branch" key.
type LegacyWorkspaceRepo struct {
	Alias   string
	Project string
	Branch  BranchName
}

// LegacyWorkspaceConf is the pure parse result of a legacy per-workspace
// manifest. Every header field is optional: absent or invalid values stay
// at their zero value (nil, "", zero time) and are never defaulted here.
type LegacyWorkspaceConf struct {
	Name         string
	Created      time.Time
	ProjectsRoot Path
	BaseBranch   *BranchName
	Branch       BranchName
	CopyEnv      *bool
	Repos        []LegacyWorkspaceRepo
	Issues       []LegacyIssue
}

const (
	legacyWsKeyName         = "name"
	legacyWsKeyCreated      = "created"
	legacyWsKeyProjectsRoot = "projects_root"
	legacyWsKeyBaseBranch   = "base_branch"
	legacyWsKeyBranch       = "branch"
	legacyWsKeyCopyEnv      = "copy_env"

	legacyReposSectionHeader = "[repos]"
)

// ParseLegacyWorkspaceConf parses a legacy per-workspace manifest. The
// format, as the legacy tool writes it:
//
//	# comment
//	name = <workspace>
//	created = <UTC RFC 3339>
//	projects_root = <abs path>
//	base_branch = <branch>
//	branch = <branch>
//	copy_env = yes|no
//
//	[repos]
//	<alias>|<project>|<branch>
//
// Reading mirrors the legacy tool: "key = value" with surrounding spaces
// ignored, last duplicate wins, "#" lines and blank lines skipped. CRLF
// endings and one matching pair of surrounding quotes on a header value
// are tolerated. It never fails outright: every line it cannot use is
// reported as a LegacyIssue and left out.
func ParseLegacyWorkspaceConf(data []byte) LegacyWorkspaceConf {
	var out LegacyWorkspaceConf
	inRepos := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !inRepos && line == legacyReposSectionHeader {
			inRepos = true
			continue
		}
		if inRepos {
			parseLegacyRepoLine(&out, line)
			continue
		}
		key, value, ok := splitLegacyLine(line)
		if !ok {
			out.Issues = append(out.Issues, LegacyIssue{Key: line, Reason: LegacyReasonMalformedLine})
			continue
		}
		applyLegacyWorkspaceKey(&out, key, unquoteLegacyValue(value))
	}
	if !inRepos {
		out.Issues = append(out.Issues, LegacyIssue{Key: legacyReposSectionHeader, Reason: LegacyReasonMissingReposSection})
	}
	return out
}

// unquoteLegacyValue strips exactly one matching pair of surrounding
// single or double quotes. An unbalanced quote is kept as written.
func unquoteLegacyValue(v string) string {
	if len(v) >= 2 {
		first, last := v[0], v[len(v)-1]
		if (first == '"' || first == '\'') && first == last {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func applyLegacyWorkspaceKey(out *LegacyWorkspaceConf, key, value string) {
	invalid := func() {
		out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonInvalidValue, Detail: value})
	}
	switch key {
	case legacyWsKeyName:
		if value == "" {
			invalid()
			return
		}
		out.Name = value
	case legacyWsKeyCreated:
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			invalid()
			return
		}
		out.Created = t.UTC()
	case legacyWsKeyProjectsRoot:
		p, err := NewPath(value)
		if err != nil {
			invalid()
			return
		}
		out.ProjectsRoot = p
	case legacyWsKeyBaseBranch:
		b, err := NewBranchName(value)
		if err != nil {
			invalid()
			return
		}
		out.BaseBranch = &b
	case legacyWsKeyBranch:
		b, err := NewBranchName(value)
		if err != nil {
			invalid()
			return
		}
		out.Branch = b
	case legacyWsKeyCopyEnv:
		v, err := parseLegacyYesNo(value)
		if err != nil {
			invalid()
			return
		}
		out.CopyEnv = &v
	default:
		out.Issues = append(out.Issues, LegacyIssue{Key: key, Reason: LegacyReasonUnknownKey, Detail: value})
	}
}

// parseLegacyRepoLine parses one "alias|project|branch" line. Alias and
// project must each be a single folder name; a line with fewer than two or
// more than three fields is malformed (the legacy tool never writes one,
// and guessing which field is which would invent data).
func parseLegacyRepoLine(out *LegacyWorkspaceConf, line string) {
	fields := strings.Split(line, "|")
	if len(fields) < 2 || len(fields) > 3 {
		out.Issues = append(out.Issues, LegacyIssue{Key: line, Reason: LegacyReasonMalformedLine})
		return
	}
	alias := strings.TrimSpace(fields[0])
	project := strings.TrimSpace(fields[1])
	if invalidComponentReason(alias) != "" || invalidComponentReason(project) != "" {
		out.Issues = append(out.Issues, LegacyIssue{Key: line, Reason: LegacyReasonMalformedLine})
		return
	}
	repo := LegacyWorkspaceRepo{Alias: alias, Project: project}
	if len(fields) == 3 {
		if raw := strings.TrimSpace(fields[2]); raw != "" {
			b, err := NewBranchName(raw)
			if err != nil {
				out.Issues = append(out.Issues, LegacyIssue{Key: line, Reason: LegacyReasonInvalidValue, Detail: raw})
			} else {
				repo.Branch = b
			}
		}
	}
	out.Repos = append(out.Repos, repo)
}
