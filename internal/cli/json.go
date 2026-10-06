// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"encoding/json"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

// jsonFlagName is registered only on the read-only commands (info, list,
// status, context list, project list — cli-surface spec: "--json contract
// on read-only commands"), plus adopt-legacy and claim, whose structured
// results scripts consume like rpc's workspaces.adoptLegacy and
// workspaces.claim, and update,
// whose per-repo results scripts check the same way. Any other command
// invoked with --json is rejected by cobra's own unknown-flag handling,
// intercepted in root.go's SetFlagErrorFunc to render a catalog message
// instead of cobra's raw text.
const jsonFlagName = "json"

func addJSONFlag(cmd *cobra.Command) {
	cmd.Flags().Bool(jsonFlagName, false, "emit JSON instead of human-readable output")
}

func jsonRequested(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool(jsonFlagName)
	return v
}

func jsonUnsupportedError(cmd *cobra.Command) error {
	return &cliError{text: messages.T(messages.CLIJSONUnsupported, cmd.Name())}
}

// cliError is a plain, already-rendered error: its Error() text is already
// catalog text, so the root error handler must not run it through
// renderError a second time.
type cliError struct{ text string }

func (e *cliError) Error() string { return e.text }

// listItemJSON is list --json's per-workspace shape (cli-surface spec:
// "name, path, project_count").
type listItemJSON struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	ProjectCount int    `json:"project_count"`
	// Error is set (and every other field beyond Name/Path left at its
	// zero value) when this one workspace's status could not be
	// collected — app.List reports it inline rather than aborting the
	// whole list (see app.WorkspaceStatus.Err's own doc comment).
	Error string `json:"error,omitempty"`
	// Legacy is true for a workspace only the legacy bash tool manages
	// (no wspace manifest yet; see "wspace adopt-legacy").
	Legacy bool `json:"legacy,omitempty"`
	// OrphanOf names the removed context the workspace's manifest still
	// names (see "wspace claim"); omitted when the workspace is owned.
	OrphanOf string `json:"orphan_of,omitempty"`
}

func encodeListJSON(statuses []app.WorkspaceStatus) ([]byte, error) {
	items := make([]listItemJSON, 0, len(statuses))
	for _, s := range statuses {
		item := listItemJSON{Name: s.Name, Path: string(s.Root), ProjectCount: len(s.Repos), Legacy: s.Legacy, OrphanOf: string(s.OrphanOf)}
		if s.Err != nil {
			item.Error = s.Err.Error()
		}
		items = append(items, item)
	}
	return json.MarshalIndent(items, "", "  ")
}

// statusItemJSON is status --json's per-repo shape (cli-surface spec:
// "alias, branch, ahead, behind, and dirty").
type statusItemJSON struct {
	Alias  string `json:"alias"`
	Branch string `json:"branch"`
	Ahead  int    `json:"ahead"`
	Behind int    `json:"behind"`
	Dirty  bool   `json:"dirty"`
	// BaseBranch and BaseMissing mirror RepoStatus's comparison base
	// (additive; omitted when unset).
	BaseBranch  string `json:"base_branch,omitempty"`
	BaseMissing bool   `json:"base_missing,omitempty"`
}

func encodeStatusJSON(status app.WorkspaceStatus) ([]byte, error) {
	items := make([]statusItemJSON, 0, len(status.Repos))
	for _, r := range status.Repos {
		items = append(items, statusItemJSON{
			Alias:       r.Alias,
			Branch:      string(r.Branch),
			Ahead:       r.Ahead,
			Behind:      r.Behind,
			Dirty:       r.Dirty,
			BaseBranch:  string(r.BaseBranch),
			BaseMissing: r.BaseMissing,
		})
	}
	return json.MarshalIndent(items, "", "  ")
}

// resolvedOptionJSON carries one Chain B key's value and the Layer that
// won it (ADR D6: "ws info --json showing provenance turns 'why is this on
// develop?' into a command").
type resolvedOptionJSON struct {
	Value any    `json:"value"`
	From  string `json:"from"`
}

// infoJSON is info --json's schema (cli-surface spec: "at least
// context_name and the resolved option keys").
type infoJSON struct {
	ContextName string                        `json:"context_name"`
	ConfigDir   string                        `json:"config_dir"`
	Options     map[string]resolvedOptionJSON `json:"options"`
}

func encodeInfoJSON(name domain.ContextName, configDir domain.Path, r domain.Resolver) ([]byte, error) {
	base := r.BaseBranch("")
	copyEnv := r.CopyEnv("")
	fetch := r.FetchBeforeCreate()
	prune := r.EnvPruneDirs("")
	remote := r.Remote("")

	info := infoJSON{
		ContextName: string(name),
		ConfigDir:   string(configDir),
		Options: map[string]resolvedOptionJSON{
			"base_branch":         {Value: string(base.Value), From: base.From.String()},
			"copy_env":            {Value: copyEnv.Value, From: copyEnv.From.String()},
			"fetch_before_create": {Value: fetch.Value, From: fetch.From.String()},
			"env_prune_dirs":      {Value: prune.Value, From: prune.From.String()},
			"remote":              {Value: remote.Value, From: remote.From.String()},
		},
	}
	return json.MarshalIndent(info, "", "  ")
}

// contextListItemJSON is context list --json's per-context shape.
type contextListItemJSON struct {
	Name           string `json:"name"`
	Active         bool   `json:"active"`
	WorkspacesRoot string `json:"workspaces_root"`
	ProjectsRoot   string `json:"projects_root,omitempty"`
	ProjectCount   int    `json:"project_count"`
}

func encodeContextListJSON(contexts []domain.Context, active domain.ContextName) ([]byte, error) {
	items := make([]contextListItemJSON, 0, len(contexts))
	for _, c := range contexts {
		items = append(items, contextListItemJSON{
			Name:           string(c.Name),
			Active:         c.Name == active,
			WorkspacesRoot: string(c.WorkspacesRoot),
			ProjectsRoot:   string(c.ProjectsRoot),
			ProjectCount:   len(c.Projects),
		})
	}
	return json.MarshalIndent(items, "", "  ")
}

// projectListItemJSON is project list --json's per-project shape.
type projectListItemJSON struct {
	Key          string `json:"key"`
	SourceDir    string `json:"source_dir"`
	OriginBranch string `json:"origin_branch,omitempty"`
	DestBranch   string `json:"dest_branch,omitempty"`
	WorktreeDir  string `json:"worktree_dir,omitempty"`
}

func encodeProjectListJSON(projects []domain.Project) ([]byte, error) {
	items := make([]projectListItemJSON, 0, len(projects))
	for _, p := range projects {
		item := projectListItemJSON{
			Key:         string(p.Key),
			SourceDir:   string(p.SourceDir),
			DestBranch:  string(p.DestBranch),
			WorktreeDir: string(p.WorktreeDir),
		}
		if p.OriginBranch != nil {
			item.OriginBranch = string(*p.OriginBranch)
		}
		items = append(items, item)
	}
	return json.MarshalIndent(items, "", "  ")
}

// adoptJSON is adopt-legacy --json's shape, identical to the rpc method
// workspaces.adoptLegacy's result.
type adoptJSON struct {
	Adopted []adoptedJSON `json:"adopted"`
	Skipped []skippedJSON `json:"skipped"`
}

type adoptedJSON struct {
	Name string `json:"name"`
	Root string `json:"root"`
}

type skippedJSON struct {
	Root    string `json:"root"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func encodeAdoptJSON(r app.AdoptLegacyWorkspacesResult) ([]byte, error) {
	out := adoptJSON{Adopted: []adoptedJSON{}, Skipped: []skippedJSON{}}
	for _, a := range r.Adopted {
		out.Adopted = append(out.Adopted, adoptedJSON{Name: a.Name, Root: string(a.Root)})
	}
	for _, s := range r.Skipped {
		out.Skipped = append(out.Skipped, skippedJSON{Root: string(s.Root), Reason: string(s.Reason), Message: s.Message()})
	}
	return json.MarshalIndent(out, "", "  ")
}

// claimJSON is claim --json's shape: rpc workspaces.claim's result, with
// the CLI's snake_case field names.
type claimJSON struct {
	Claimed []claimedJSON      `json:"claimed"`
	Skipped []claimSkippedJSON `json:"skipped"`
}

type claimedJSON struct {
	Name            string `json:"name"`
	Root            string `json:"root"`
	PreviousContext string `json:"previous_context"`
}

type claimSkippedJSON struct {
	Name    string `json:"name"`
	Root    string `json:"root,omitempty"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func encodeClaimJSON(r app.ClaimWorkspacesResult) ([]byte, error) {
	out := claimJSON{Claimed: []claimedJSON{}, Skipped: []claimSkippedJSON{}}
	for _, c := range r.Claimed {
		out.Claimed = append(out.Claimed, claimedJSON{Name: c.Name, Root: string(c.Root), PreviousContext: string(c.PreviousContext)})
	}
	for _, s := range r.Skipped {
		out.Skipped = append(out.Skipped, claimSkippedJSON{Name: s.Name, Root: string(s.Root), Reason: string(s.Reason), Message: s.Message()})
	}
	return json.MarshalIndent(out, "", "  ")
}
