// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"

	"github.com/kivoradigital/wspace/internal/domain"
)

// ScanProjectsInput parameterizes ScanProjects: the roots to walk, the
// ignore patterns pruned while descending, the include patterns a
// repository must match to be offered (empty: every repository), the depth
// cap (<= 0 selects domain.DefaultProjectScanMaxDepth), and the projects
// already registered in the target context, so each candidate can say
// whether it is new and suggested keys never collide with a taken one.
type ScanProjectsInput struct {
	Roots      []domain.Path
	Ignore     []domain.Glob
	Include    []domain.Glob
	MaxDepth   int
	Registered []domain.Project
}

// ScanCandidate is one confirmed main clone found by ScanProjects.
// SuggestedKey follows domain.SuggestProjectKeys across every candidate of
// every root, so all suggestions can be registered together. RegisteredKey
// is set when Registered is true (SuggestedKey is then that key).
type ScanCandidate struct {
	Path          domain.Path
	Root          domain.Path
	RelativePath  string
	SuggestedKey  string
	Registered    bool
	RegisteredKey domain.ProjectKey
}

// ScanProjectsResult is ScanProjects's outcome. Truncated is true when any
// root's walk reached the depth cap with directories left unexplored;
// TruncatedDirs names those directories and MaxDepth is the cap used.
type ScanProjectsResult struct {
	Candidates      []ScanCandidate
	LinkedWorktrees []domain.Path
	Truncated       bool
	TruncatedDirs   []domain.Path
	MaxDepth        int
}

// ScanProjects is the non-interactive counterpart of RunProjectWizard's
// scan step: it walks every root exactly like the wizard does
// (walkCandidates) and returns what it found, without prompting, without
// reporting through a Reporter and without registering anything — a
// caller (the rpc/mcp surfaces) decides what to register afterwards.
func ScanProjects(ctx context.Context, deps Deps, in ScanProjectsInput) (ScanProjectsResult, error) {
	if _, err := domain.MatchesIncludePattern(in.Include, ""); err != nil {
		return ScanProjectsResult{}, err
	}
	registered := make(map[domain.Path]domain.ProjectKey, len(in.Registered))
	taken := make([]domain.ProjectKey, 0, len(in.Registered))
	for _, p := range in.Registered {
		registered[p.SourceDir] = p.Key
		taken = append(taken, p.Key)
	}

	res := ScanProjectsResult{MaxDepth: in.MaxDepth}
	if res.MaxDepth <= 0 {
		res.MaxDepth = domain.DefaultProjectScanMaxDepth
	}
	var fresh []int // indexes of unregistered candidates, for key suggestion
	for _, root := range in.Roots {
		out, err := walkCandidates(ctx, deps.FS, deps.Git, root, in.Ignore, in.MaxDepth)
		if err != nil {
			return ScanProjectsResult{}, err
		}
		res.Truncated = res.Truncated || out.truncated
		res.TruncatedDirs = append(res.TruncatedDirs, out.truncatedDirs...)
		res.LinkedWorktrees = append(res.LinkedWorktrees, out.linked...)
		for _, dir := range out.candidates {
			rel := relativeToRoot(root, dir)
			if ok, _ := domain.MatchesIncludePattern(in.Include, rel); !ok {
				continue
			}
			key, isRegistered := registered[dir]
			if !isRegistered {
				fresh = append(fresh, len(res.Candidates))
			}
			res.Candidates = append(res.Candidates, ScanCandidate{
				Path:          dir,
				Root:          root,
				RelativePath:  rel,
				SuggestedKey:  string(key),
				Registered:    isRegistered,
				RegisteredKey: key,
			})
		}
	}

	rels := make([]string, len(fresh))
	for i, idx := range fresh {
		rels[i] = res.Candidates[idx].RelativePath
	}
	for i, key := range domain.SuggestProjectKeys(rels, taken) {
		res.Candidates[fresh[i]].SuggestedKey = key
	}
	return res, nil
}
