// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package app

import (
	"context"
	"strings"

	"github.com/kivoradigital/wspace/internal/domain"
)

// RegisterProblemReason is a stable, machine-readable reason one project
// of a RegisterProjects batch cannot be registered.
type RegisterProblemReason string

const (
	ProblemInvalidKey       RegisterProblemReason = "invalid_key"
	ProblemKeyTaken         RegisterProblemReason = "key_taken"
	ProblemKeyDuplicate     RegisterProblemReason = "key_duplicate"
	ProblemSourceInvalid    RegisterProblemReason = "source_invalid"
	ProblemSourceRegistered RegisterProblemReason = "source_registered"
	ProblemSourceDuplicate  RegisterProblemReason = "source_duplicate"
	ProblemNotAMainClone    RegisterProblemReason = "not_a_main_clone"
	ProblemGitFailed        RegisterProblemReason = "git_failed"
)

// RegisterProblem is one reason projects[Index] cannot be registered.
// Other names the conflicting key or folder when there is one (the
// earlier batch entry, or the registered project's key); Detail carries a
// validation sentence for ProblemInvalidKey.
type RegisterProblem struct {
	Index  int
	Reason RegisterProblemReason
	Other  string
	Detail string
}

// RegisterProjects registers a batch of projects all-or-nothing: every
// project is validated first — key rules, keys unique within the batch and
// against the context (case-insensitively, since a key is a worktree
// folder name and macOS file systems ignore case), source folders absolute,
// unique and not already registered, and each a git main clone — and the
// context is written once only when no project has a problem. With
// problems, nothing is written and every problem is returned, so a caller
// can show which project failed and why, and let the person fix the keys
// before retrying. A non-nil error is an operational failure (the context
// could not be loaded or saved).
func RegisterProjects(ctx context.Context, deps Deps, contextName domain.ContextName, projects []domain.Project) ([]RegisterProblem, error) {
	c, err := deps.Store.LoadContext(ctx, contextName)
	if err != nil {
		return nil, err
	}
	fold := strings.ToLower
	takenKeys := map[string]domain.ProjectKey{}
	takenDirs := map[domain.Path]domain.ProjectKey{}
	for _, p := range c.Projects {
		takenKeys[fold(string(p.Key))] = p.Key
		takenDirs[p.SourceDir] = p.Key
	}
	batchKeys := map[string]int{}
	batchDirs := map[domain.Path]int{}

	var problems []RegisterProblem
	add := func(i int, r RegisterProblemReason, other, detail string) {
		problems = append(problems, RegisterProblem{Index: i, Reason: r, Other: other, Detail: detail})
	}
	for i, p := range projects {
		if reason := invalidKeyReason(string(p.Key)); reason != "" {
			add(i, ProblemInvalidKey, "", reason)
		} else if existing, ok := takenKeys[fold(string(p.Key))]; ok {
			add(i, ProblemKeyTaken, string(existing), "")
		} else if j, ok := batchKeys[fold(string(p.Key))]; ok {
			add(i, ProblemKeyDuplicate, string(projects[j].SourceDir), "")
		} else {
			batchKeys[fold(string(p.Key))] = i
		}

		if validateAbsolutePath(string(p.SourceDir)) != nil {
			add(i, ProblemSourceInvalid, "", "")
			continue
		}
		if key, ok := takenDirs[p.SourceDir]; ok {
			add(i, ProblemSourceRegistered, string(key), "")
			continue
		}
		if j, ok := batchDirs[p.SourceDir]; ok {
			add(i, ProblemSourceDuplicate, string(projects[j].Key), "")
			continue
		}
		batchDirs[p.SourceDir] = i
		ok, err := deps.Git.IsMainClone(ctx, p.SourceDir)
		switch {
		case domain.Code(err) == domain.CodeNotAMainClone, err == nil && !ok:
			add(i, ProblemNotAMainClone, "", "")
		case err != nil:
			add(i, ProblemGitFailed, "", "")
		}
	}
	if len(problems) > 0 {
		return problems, nil
	}
	c.Projects = append(c.Projects, projects...)
	return nil, deps.Store.SaveContext(ctx, c)
}

// invalidKeyReason is NewProjectKey's rejection without its wrapping, for
// a per-project message ("" when the key is valid).
func invalidKeyReason(key string) string {
	if _, err := domain.NewProjectKey(key); err != nil {
		msg := err.Error()
		if i := strings.LastIndex(msg, "\" "); i >= 0 {
			return msg[i+2:]
		}
		return msg
	}
	return ""
}

// RepoChanges lists every changed path of one repository in a workspace
// (staged, unstaged and untracked), in git's order. An alias the
// workspace's manifest does not hold is domain.CodeRepoNotFound.
func RepoChanges(ctx context.Context, deps Deps, workspaceRoot domain.Path, alias string) ([]domain.FileChange, error) {
	manifest, err := deps.Store.LoadManifest(ctx, workspaceRoot)
	if err != nil {
		return nil, err
	}
	found := false
	for _, r := range manifest.Workspace.Repos {
		if r.Alias == alias {
			found = true
			break
		}
	}
	if !found {
		return nil, domain.NewOpError("workspace.repo_changes", domain.CodeRepoNotFound, alias, "", nil)
	}
	entries, err := deps.Git.Status(ctx, manifest.Workspace.Root.Join(alias))
	if err != nil {
		return nil, err
	}
	out := make([]domain.FileChange, 0, len(entries))
	for _, e := range entries {
		out = append(out, domain.DescribeChange(e))
	}
	return out, nil
}
