// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

func newUpdateCommand(rt *Runtime) *cobra.Command {
	var repo string
	var rebase, autostash bool

	cmd := &cobra.Command{
		Use:   "update [workspace]",
		Short: "fetch and merge (or --rebase) each repo's base branch into the workspace branch",
		Long: "Fetches each repo's remote and integrates its base branch (the base status compares against) into the workspace branch.\n" +
			"A repo with uncommitted tracked changes is refused unless --autostash. A conflict is aborted: the repo is left exactly as it was.\n" +
			"--rebase rewrites the branch's local commits. Without a workspace name, the workspace around the current directory is updated.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsRoot, err := updateTarget(cmd, rt, args)
			if err != nil {
				return err
			}
			strategy := domain.UpdateMerge
			if rebase {
				strategy = domain.UpdateRebase
			}
			deps := depsWithHumanReporter(cmd, rt)

			var results []app.RepoUpdate
			if repo != "" {
				res, err := app.UpdateRepo(cmd.Context(), deps, app.UpdateRepoInput{WorkspaceRoot: wsRoot, Alias: repo, Strategy: strategy, Autostash: autostash})
				if err != nil {
					return err
				}
				results = []app.RepoUpdate{res}
			} else {
				results, err = app.UpdateWorkspace(cmd.Context(), deps, app.UpdateWorkspaceInput{WorkspaceRoot: wsRoot, Strategy: strategy, Autostash: autostash})
				if err != nil {
					return err
				}
			}

			if jsonRequested(cmd) {
				out, err := encodeUpdateJSON(results)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			} else {
				for _, r := range results {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderRepoUpdate(wsRoot, r))
				}
			}

			failed := 0
			for _, r := range results {
				if r.Err != nil {
					failed++
				}
			}
			if failed > 0 {
				return &cliError{text: messages.T(messages.CLIRepoUpdateSomeFailed, failed, len(results))}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "update only this repo (its alias)")
	cmd.Flags().BoolVar(&rebase, "rebase", false, "rebase the workspace branch onto the base instead of merging (rewrites local commits)")
	cmd.Flags().BoolVar(&autostash, "autostash", false, "stash uncommitted tracked changes around the update instead of refusing")
	addJSONFlag(cmd)
	return cmd
}

// updateTarget is the named workspace, or the one enclosing the cwd.
func updateTarget(cmd *cobra.Command, rt *Runtime, args []string) (domain.Path, error) {
	if len(args) == 1 {
		root, _, err := workspaceRoot(cmd, rt, args[0])
		return root, err
	}
	cwd, err := app.Cwd(rt.Deps)
	if err != nil {
		return "", err
	}
	root, found, err := rt.Deps.Store.FindWorkspaceRoot(cmd.Context(), cwd)
	if err != nil {
		return "", err
	}
	if !found {
		return "", &cliError{text: messages.T(messages.CLIRepoUpdateNoWorkspace)}
	}
	return root, nil
}

func renderRepoUpdate(wsRoot domain.Path, r app.RepoUpdate) string {
	switch {
	case r.Err == nil && r.UpToDate:
		return messages.T(messages.CLIRepoUpdateUpToDate, r.Alias, r.Base)
	case r.Err == nil:
		return messages.T(messages.CLIRepoUpdateIntegrated, r.Alias, r.CommitsIntegrated, r.Base, string(r.Strategy))
	}
	switch domain.Code(r.Err) {
	case domain.CodeWorktreeDirty:
		return messages.T(messages.CLIRepoUpdateDirty, r.Alias, strings.Join(r.DirtyFiles, ", "))
	case domain.CodeUpdateConflict:
		key := messages.CLIRepoUpdateConflict
		if !r.Restored {
			key = messages.CLIRepoUpdateConflictUnverified
		}
		return messages.T(key, r.Alias, string(r.Strategy), r.Base, strings.Join(r.Conflicts, ", "), string(wsRoot.Join(r.Alias)))
	}
	reason := messages.T(messages.ForCode(domain.Code(r.Err)))
	if subject := opSubject(r.Err); subject != "" && subject != r.Alias {
		reason += ": " + subject
	}
	return messages.T(messages.CLIRepoUpdateFailed, r.Alias, reason)
}

func opSubject(err error) string {
	var opErr *domain.OpError
	if errors.As(err, &opErr) {
		return opErr.Subject
	}
	return ""
}

// updateItemJSON is update --json's per-repo shape (snake_case, like the
// other CLI --json outputs).
type updateItemJSON struct {
	Repo              string           `json:"repo"`
	Strategy          string           `json:"strategy"`
	Base              string           `json:"base"`
	BeforeHead        string           `json:"before_head"`
	AfterHead         string           `json:"after_head"`
	UpToDate          bool             `json:"up_to_date"`
	CommitsIntegrated int              `json:"commits_integrated"`
	Conflicts         []string         `json:"conflicts"`
	DirtyFiles        []string         `json:"dirty_files,omitempty"`
	Restored          *bool            `json:"restored,omitempty"`
	Error             *updateErrorJSON `json:"error,omitempty"`
}

type updateErrorJSON struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func encodeUpdateJSON(results []app.RepoUpdate) ([]byte, error) {
	items := make([]updateItemJSON, 0, len(results))
	for _, r := range results {
		item := updateItemJSON{
			Repo: r.Alias, Strategy: string(r.Strategy), Base: r.Base,
			BeforeHead: r.BeforeHead, AfterHead: r.AfterHead, UpToDate: r.UpToDate,
			CommitsIntegrated: r.CommitsIntegrated, Conflicts: append([]string{}, r.Conflicts...),
			DirtyFiles: r.DirtyFiles,
		}
		if r.Err != nil {
			item.Error = &updateErrorJSON{Code: string(domain.Code(r.Err)), Message: messages.T(messages.ForCode(domain.Code(r.Err)))}
			if domain.Code(r.Err) == domain.CodeUpdateConflict {
				restored := r.Restored
				item.Restored = &restored
			}
		}
		items = append(items, item)
	}
	return json.MarshalIndent(items, "", "  ")
}
