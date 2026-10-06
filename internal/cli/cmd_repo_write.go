// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"fmt"
	"strings"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// writeFlags are the repo command's flags for its write actions.
type writeFlags struct {
	all              bool
	message          string
	setUpstream      bool
	includeUntracked bool
	keepIndex        bool
}

type stageJSON struct {
	Repo  string   `json:"repo"`
	Paths []string `json:"paths"`
}

type discardJSON struct {
	Repo      string   `json:"repo"`
	Discarded []string `json:"discarded"`
	Backup    string   `json:"backup,omitempty"`
}

type commitResultJSON struct {
	Repo     string           `json:"repo"`
	Commit   *commitJSON      `json:"commit,omitempty"`
	Warnings []string         `json:"warnings"`
	Commands []string         `json:"commands,omitempty"`
	Files    []string         `json:"files,omitempty"`
	Output   string           `json:"output,omitempty"`
	Error    *updateErrorJSON `json:"error,omitempty"`
}

type pushJSON struct {
	Repo        string           `json:"repo"`
	Branch      string           `json:"branch,omitempty"`
	Remote      string           `json:"remote"`
	Upstream    string           `json:"upstream,omitempty"`
	SetUpstream bool             `json:"set_upstream"`
	UpToDate    bool             `json:"up_to_date"`
	Pushed      int              `json:"pushed"`
	Output      string           `json:"output,omitempty"`
	Error       *updateErrorJSON `json:"error,omitempty"`
}

func errJSON(err error) *updateErrorJSON {
	if err == nil {
		return nil
	}
	return &updateErrorJSON{Code: string(domain.Code(err)), Message: messages.T(messages.ForCode(domain.Code(err)))}
}

// stage stages (or unstages) paths, or every applicable path with all.
func (r repoRunner) stage(stage bool, paths []string, all bool) error {
	run, key := app.UnstageChanges, messages.CLIRepoUnstagedPaths
	if stage {
		run, key = app.StageChanges, messages.CLIRepoStagedPaths
	}
	res, err := run(r.cmd.Context(), r.deps, app.ChangePathsInput{RepoRefInput: r.ref, Paths: paths, All: all})
	if err != nil {
		return err
	}
	if r.json {
		return r.emitJSON(stageJSON{Repo: res.Alias, Paths: nonNil(res.Paths)})
	}
	r.line(key, res.Alias, len(res.Paths))
	return nil
}

// discard discards unstaged changes of tracked files after showing them
// with their line counts and asking (unless yes).
func (r repoRunner) discard(rt *Runtime, paths []string, yes bool) error {
	ctx := r.cmd.Context()
	in := app.ChangePathsInput{RepoRefInput: r.ref, Paths: paths}
	prev, err := app.PreviewDiscard(ctx, r.deps, in)
	if err != nil {
		return err
	}
	if !yes {
		names := make([]string, 0, len(prev.Files))
		for _, f := range prev.Files {
			if f.Binary {
				names = append(names, f.Path+" (binary)")
				continue
			}
			names = append(names, fmt.Sprintf("%s (+%d -%d)", f.Path, f.Additions, f.Deletions))
		}
		ok, err := app.ConfirmWithHelp(ctx, rt.Prompter, messages.CLIConfirmDiscard, messages.CLIConfirmDiscardHelp, []any{len(names), r.ref.Alias, strings.Join(names, ", ")}, false)
		if err != nil || !ok {
			return err
		}
	}
	res, err := app.DiscardChanges(ctx, r.deps, in)
	if err != nil {
		return err
	}
	if r.json {
		return r.emitJSON(discardJSON{Repo: res.Alias, Discarded: nonNil(res.Discarded), Backup: string(res.Backup)})
	}
	r.line(messages.CLIRepoDiscarded, res.Alias, len(res.Discarded))
	if res.Backup != "" {
		r.line(messages.CLIRepoDiscardBackup, res.Backup, r.ref.WorkspaceRoot.Join(r.ref.Alias))
	}
	return nil
}

// commit commits the staged changes; a refusal prints what to do and
// fails.
func (r repoRunner) commit(message string) error {
	res, err := app.CommitChanges(r.cmd.Context(), r.deps, app.CommitInput{RepoRefInput: r.ref, Message: message})
	if err != nil {
		return err
	}
	if r.json {
		out := commitResultJSON{Repo: res.Alias, Warnings: nonNil(res.Warnings), Commands: res.IdentityCommands, Files: res.Files, Output: res.Output, Error: errJSON(res.Err)}
		if res.Err == nil {
			c := toCommitJSON(res.Commit)
			out.Commit = &c
		}
		if err := r.emitJSON(out); err != nil {
			return err
		}
	} else {
		for _, w := range res.Warnings {
			if w == app.WarningSubjectTooLong {
				r.line(messages.CLIRepoSubjectTooLong, app.SubjectLimit)
			}
		}
		switch domain.Code(res.Err) {
		case "":
			r.line(messages.CLIRepoCommitted, res.Alias, res.Commit.ShortHash, res.Commit.Subject)
		case domain.CodeIdentityMissing:
			r.line(messages.CLIRepoIdentityHelp)
			for _, c := range res.IdentityCommands {
				_, _ = fmt.Fprintln(r.out, "  "+c)
			}
		case domain.CodeHookFailed:
			r.line(messages.CLIRepoHookOutput)
			_, _ = fmt.Fprintln(r.out, res.Output)
		}
	}
	if res.Err != nil {
		return &cliError{text: messages.T(messages.CLIRepoCommitFailed, res.Alias, messages.T(messages.ForCode(domain.Code(res.Err))))}
	}
	return nil
}

// push pushes the branch to its upstream, or publishes it with
// setUpstream.
func (r repoRunner) push(setUpstream bool) error {
	res, err := app.PushRepo(r.cmd.Context(), r.deps, app.PushInput{RepoRefInput: r.ref, SetUpstream: setUpstream})
	if err != nil {
		return err
	}
	if r.json {
		out := pushJSON{Repo: res.Alias, Branch: res.Branch, Remote: res.Remote, Upstream: res.Upstream, SetUpstream: res.SetUpstream,
			UpToDate: res.UpToDate, Pushed: res.Pushed, Output: res.Output, Error: errJSON(res.Err)}
		if err := r.emitJSON(out); err != nil {
			return err
		}
	} else {
		switch domain.Code(res.Err) {
		case "":
			switch {
			case res.UpToDate:
				r.line(messages.CLIRepoPushUpToDate, res.Alias, res.Upstream)
			case res.SetUpstream:
				r.line(messages.CLIRepoPublished, res.Alias, res.Branch, res.Remote, res.Upstream)
			default:
				r.line(messages.CLIRepoPushed, res.Alias, res.Pushed, res.Upstream)
			}
		case domain.CodeNoUpstream:
			r.line(messages.CLIRepoPushNoUpstream, res.Alias, res.Branch, res.Remote)
		case domain.CodePushRejected:
			r.line(messages.CLIRepoPushRejected, res.Alias, res.Upstream)
		}
		if res.Output != "" {
			_, _ = fmt.Fprintln(r.out, res.Output)
		}
	}
	if res.Err != nil {
		return &cliError{text: messages.T(messages.CLIRepoPushFailed, res.Alias, messages.T(messages.ForCode(domain.Code(res.Err))))}
	}
	return nil
}

// stashPush saves the local changes as a new stash entry.
func (r repoRunner) stashPush(w writeFlags) error {
	e, err := app.CreateStash(r.cmd.Context(), r.deps, app.StashCreateInput{RepoRefInput: r.ref, Message: w.message, IncludeUntracked: w.includeUntracked, KeepIndex: w.keepIndex})
	if err != nil {
		return err
	}
	if r.json {
		return r.emitJSON(stashDropJSON{Repo: r.ref.Alias, Stash: toStashJSON(e)})
	}
	r.line(messages.CLIRepoStashCreated, r.ref.Alias, e.Ref, e.Message)
	return nil
}
