// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
	"github.com/spf13/cobra"
)

const humanTime = "2006-01-02 15:04"

func newRepoCommand(rt *Runtime) *cobra.Command {
	var staged, yes bool
	var offset, limit int
	var commitRange string
	var w writeFlags

	cmd := &cobra.Command{
		Use:   "repo <workspace> <repo> <status|log|show <hash>|stash [index]|stash apply|pop|drop <index>|stash push|diff [path]|fetch|pull|push|clean <path>...|stage|unstage|discard <path>...|commit -m <message>>",
		Short: "inspect one workspace repo (status, log, stash, diff), stage, commit and push it, fetch / fast-forward pull it, manage its stashes or delete untracked files",
		Long: "Read-only inspection of one repo of a workspace:\n" +
			"  status          branch, upstream and base ahead/behind, last fetch, staged/unstaged/untracked changes\n" +
			"  log             the branch's commits since its base, or those not pushed yet with --range upstream (--offset, --limit)\n" +
			"  show <hash>     one commit with its diff\n" +
			"  stash [index]   the stash list, or one entry's diff\n" +
			"  diff [path]     the diff of every change, or of one path (--staged for the index side)\n" +
			"Network:\n" +
			"  fetch           git fetch --prune; local branches and files are not changed\n" +
			"  pull            fast-forward only; never a merge commit (a diverged branch is refused)\n" +
			"Local actions:\n" +
			"  stash apply <n>  apply stash@{n} (staged changes restored when possible); the entry is kept\n" +
			"  stash pop <n>    apply stash@{n} and remove it, only if it applied without conflicts\n" +
			"  stash drop <n>   delete stash@{n} and its changes (asks first; --yes skips the question)\n" +
			"  clean <path>...  permanently delete untracked files (only files status lists as untracked; asks first; --yes skips)\n" +
			"  stash push       save local changes as a new stash entry (-m <message>, -u/--include-untracked, --keep-index)\n" +
			"Changing the repo:\n" +
			"  stage <path>...    stage modified, deleted or untracked files (--all: every one)\n" +
			"  unstage <path>...  unstage files; the worktree is kept (--all: every one)\n" +
			"  discard <path>...  discard unstaged changes of tracked files; staged changes are kept and a backup patch is written first (asks first; --yes skips)\n" +
			"  commit -m <msg>    commit the staged changes with the repo's own identity and hooks (never amends, never skips hooks)\n" +
			"  push               push the branch to its upstream (--set-upstream: publish a branch without one); never forced\n" +
			"A conflicting apply or pop keeps the stash entry and leaves conflict markers to resolve by hand; nothing is reset.",
		Args: cobra.MinimumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _, err := workspaceRoot(cmd, rt, args[0])
			if err != nil {
				return err
			}
			ref := app.RepoRefInput{WorkspaceRoot: root, Alias: args[1]}
			action, extra := args[2], ""
			if len(args) >= 4 {
				extra = args[3]
			}
			if len(args) > 4 && action != "clean" && action != "stash" && action != "stage" && action != "unstage" && action != "discard" {
				return &cliError{text: messages.T(messages.CLIRepoUnknownAction, strings.Join(args[2:], " "))}
			}
			r := repoRunner{cmd: cmd, deps: depsWithHumanReporter(cmd, rt), ref: ref, out: cmd.OutOrStdout(), json: jsonRequested(cmd)}
			switch action {
			case "status":
				return r.status()
			case "log":
				return r.log(commitRange, offset, limit)
			case "show":
				if extra == "" {
					return &cliError{text: messages.T(messages.CLIRepoMissingArg, "show", "<hash>")}
				}
				return r.show(extra)
			case "stash":
				if extra == "" {
					return r.stashes()
				}
				switch extra {
				case "push":
					if len(args) > 4 {
						return &cliError{text: messages.T(messages.CLIRepoUnknownAction, strings.Join(args[2:], " "))}
					}
					return r.stashPush(w)
				case "apply", "pop", "drop":
					if len(args) != 5 {
						return &cliError{text: messages.T(messages.CLIRepoMissingArg, "stash "+extra, "<index>")}
					}
					idx, err := strconv.Atoi(args[4])
					if err != nil || idx < 0 {
						return &cliError{text: messages.T(messages.CLIRepoBadNumber, args[4], "stash index")}
					}
					return r.stashAction(rt, extra, idx, yes)
				}
				if len(args) > 4 {
					return &cliError{text: messages.T(messages.CLIRepoUnknownAction, strings.Join(args[2:], " "))}
				}
				idx, err := strconv.Atoi(extra)
				if err != nil || idx < 0 {
					return &cliError{text: messages.T(messages.CLIRepoBadNumber, extra, "stash index")}
				}
				return r.stash(idx)
			case "diff":
				return r.diff(extra, staged)
			case "fetch":
				return r.fetch()
			case "pull":
				return r.pull()
			case "clean":
				if len(args) < 4 {
					return &cliError{text: messages.T(messages.CLIRepoMissingArg, "clean", "<path>...")}
				}
				return r.clean(rt, args[3:], yes)
			case "stage", "unstage":
				if len(args) < 4 && !w.all {
					return &cliError{text: messages.T(messages.CLIRepoMissingArg, action, "<path>... or --all")}
				}
				return r.stage(action == "stage", args[3:], w.all)
			case "discard":
				if len(args) < 4 {
					return &cliError{text: messages.T(messages.CLIRepoMissingArg, "discard", "<path>...")}
				}
				return r.discard(rt, args[3:], yes)
			case "commit":
				if len(args) > 3 || strings.TrimSpace(w.message) == "" {
					return &cliError{text: messages.T(messages.CLIRepoMissingArg, "commit", "-m <message>")}
				}
				return r.commit(w.message)
			case "push":
				if len(args) > 3 {
					return &cliError{text: messages.T(messages.CLIRepoUnknownAction, strings.Join(args[2:], " "))}
				}
				return r.push(w.setUpstream)
			}
			return &cliError{text: messages.T(messages.CLIRepoUnknownAction, action)}
		},
	}
	cmd.Flags().BoolVar(&staged, "staged", false, "diff: show the staged (index) side")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "stash drop, clean, discard: do not ask for confirmation")
	cmd.Flags().BoolVar(&w.all, "all", false, "stage, unstage: every applicable path")
	cmd.Flags().StringVarP(&w.message, "message", "m", "", "commit, stash push: the message")
	cmd.Flags().BoolVar(&w.setUpstream, "set-upstream", false, "push: publish a branch without an upstream to the context's remote and set it as the upstream")
	cmd.Flags().BoolVarP(&w.includeUntracked, "include-untracked", "u", false, "stash push: also stash untracked files")
	cmd.Flags().BoolVar(&w.keepIndex, "keep-index", false, "stash push: keep the staged changes in the index")
	cmd.Flags().StringVar(&commitRange, "range", string(app.CommitRangeBase), `log: "base" (commits since the base) or "upstream" (commits not pushed to the upstream)`)
	cmd.Flags().IntVar(&offset, "offset", 0, "log: commits to skip")
	cmd.Flags().IntVar(&limit, "limit", app.DefaultCommitsPage, "log: commits to show (at most 200)")
	addJSONFlag(cmd)
	return cmd
}

type repoRunner struct {
	cmd  *cobra.Command
	deps app.Deps
	ref  app.RepoRefInput
	out  io.Writer
	json bool
}

func (r repoRunner) emitJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(r.out, string(b))
	return nil
}

func (r repoRunner) line(key messages.Key, args ...any) {
	_, _ = fmt.Fprintln(r.out, messages.T(key, args...))
}

func (r repoRunner) status() error {
	ctx := r.cmd.Context()
	insp, err := app.InspectRepo(ctx, r.deps, r.ref)
	if err != nil {
		return err
	}
	cs, err := app.RepoChangeSets(ctx, r.deps, r.ref)
	if err != nil {
		return err
	}
	if r.json {
		return r.emitJSON(repoStatusJSON{Branch: branchJSON(insp.Branch), Staged: changesJSON(cs.Staged), Unstaged: changesJSON(cs.Unstaged),
			Untracked: changesJSON(cs.Untracked), Conflicted: changesJSON(cs.Conflicted), Stashes: insp.Stashes})
	}
	r.branch(insp.Branch)
	sections := []struct {
		key  messages.Key
		list []domain.ChangeEntry
	}{{messages.CLIRepoStaged, cs.Staged}, {messages.CLIRepoUnstaged, cs.Unstaged}, {messages.CLIRepoUntracked, cs.Untracked}, {messages.CLIRepoConflicted, cs.Conflicted}}
	hasChanges := false
	for _, s := range sections {
		if len(s.list) == 0 {
			continue
		}
		hasChanges = true
		r.line(messages.CLIRepoSection, messages.T(s.key), len(s.list))
		for _, e := range s.list {
			name := e.Path
			if e.OrigPath != "" {
				name = e.OrigPath + " -> " + e.Path
			}
			_, _ = fmt.Fprintln(r.out, fmt.Sprintf("  %-10s", e.Status), name)
		}
	}
	if !hasChanges {
		r.line(messages.CLIRepoClean)
	}
	if insp.Stashes > 0 {
		r.line(messages.CLIRepoStashCount, insp.Stashes)
	}
	return nil
}

func (r repoRunner) branch(b app.BranchInfo) {
	head := b.Head
	if len(head) > 7 {
		head = head[:7]
	}
	if b.Detached {
		r.line(messages.CLIRepoDetached, b.Alias, head)
	} else {
		r.line(messages.CLIRepoBranch, b.Alias, string(b.Branch), head)
	}
	switch u := b.Upstream; {
	case u == nil:
		r.line(messages.CLIRepoNoUpstream)
	case u.Gone:
		r.line(messages.CLIRepoUpstreamGone, u.Ref)
	default:
		r.line(messages.CLIRepoUpstream, u.Ref, u.Ahead, u.Behind)
	}
	switch {
	case !b.BaseFound && b.BaseBranch != "":
		r.line(messages.CLIRepoBaseMissing, string(b.BaseBranch))
	case b.BaseFound && b.BaseAhead != nil && b.BaseBehind != nil:
		r.line(messages.CLIRepoBase, b.BaseRef, *b.BaseAhead, *b.BaseBehind)
	case b.BaseFound:
		r.line(messages.CLIRepoBaseUnknown, b.BaseRef)
	}
	if b.LastFetch != nil {
		r.line(messages.CLIRepoLastFetch, b.LastFetch.Local().Format(humanTime))
	} else {
		r.line(messages.CLIRepoNeverFetched)
	}
}

func (r repoRunner) log(commitRange string, offset, limit int) error {
	if commitRange != string(app.CommitRangeBase) && commitRange != string(app.CommitRangeUpstream) {
		return &cliError{text: messages.T(messages.CLIRepoBadNumber, commitRange, "--range")}
	}
	if offset < 0 {
		return &cliError{text: messages.T(messages.CLIRepoBadNumber, strconv.Itoa(offset), "--offset")}
	}
	if limit <= 0 || limit > app.MaxCommitsPage {
		return &cliError{text: messages.T(messages.CLIRepoBadNumber, strconv.Itoa(limit), "--limit")}
	}
	res, err := app.RepoCommitLog(r.cmd.Context(), r.deps, app.RepoCommitsInput{RepoRefInput: r.ref, Range: app.CommitRange(commitRange), Offset: offset, Limit: limit})
	if err != nil {
		return err
	}
	if r.json {
		out := logJSON{Range: res.Range, Base: res.Base, Upstream: res.Upstream, Total: res.Total, Offset: res.Offset, HasMore: res.HasMore, Commits: make([]commitJSON, 0, len(res.Commits))}
		for _, c := range res.Commits {
			out.Commits = append(out.Commits, toCommitJSON(c))
		}
		return r.emitJSON(out)
	}
	r.line(messages.CLIRepoCommitsHeader, res.Total, res.Range)
	for _, c := range res.Commits {
		_, _ = fmt.Fprintln(r.out, strings.Join([]string{c.ShortHash, c.AuthorDate.Local().Format(humanTime), c.AuthorName, c.Subject}, "  "))
	}
	if res.HasMore {
		r.line(messages.CLIRepoMoreCommits, res.Offset+len(res.Commits))
	}
	return nil
}

func (r repoRunner) show(hash string) error {
	d, err := app.RepoCommit(r.cmd.Context(), r.deps, app.RepoCommitInput{RepoRefInput: r.ref, Hash: hash})
	if err != nil {
		return err
	}
	if r.json {
		return r.emitJSON(commitDetailJSON{commitJSON: toCommitJSON(d.CommitInfo), Body: d.Body, Parents: append([]string{}, d.Parents...),
			Files: filesJSON(d.Patch.Files), Truncated: d.Patch.Truncated})
	}
	_, _ = fmt.Fprintln(r.out, strings.Join([]string{d.Hash, d.AuthorDate.Local().Format(humanTime), d.AuthorName}, "  "))
	_, _ = fmt.Fprintln(r.out, d.Subject)
	if d.Body != "" {
		_, _ = fmt.Fprintln(r.out)
		_, _ = fmt.Fprintln(r.out, d.Body)
	}
	_, _ = fmt.Fprintln(r.out)
	r.patch(d.Patch.Files, d.Patch.Truncated)
	return nil
}

func (r repoRunner) stashes() error {
	list, err := app.RepoStashes(r.cmd.Context(), r.deps, r.ref)
	if err != nil {
		return err
	}
	if r.json {
		out := make([]stashJSON, 0, len(list))
		for _, s := range list {
			out = append(out, toStashJSON(s))
		}
		return r.emitJSON(out)
	}
	if len(list) == 0 {
		r.line(messages.CLIRepoNoStashes)
	}
	for _, s := range list {
		_, _ = fmt.Fprintln(r.out, strings.Join([]string{s.Ref, s.Date.Local().Format(humanTime), s.Branch, s.Message}, "  "))
	}
	return nil
}

func (r repoRunner) stash(index int) error {
	d, err := app.RepoStash(r.cmd.Context(), r.deps, app.RepoStashInput{RepoRefInput: r.ref, Index: index})
	if err != nil {
		return err
	}
	if r.json {
		return r.emitJSON(stashDetailJSON{Stash: toStashJSON(d.Entry), Files: filesJSON(d.Patch.Files), Truncated: d.Patch.Truncated, IncludesUntracked: d.IncludesUntracked})
	}
	_, _ = fmt.Fprintln(r.out, strings.Join([]string{d.Entry.Ref, d.Entry.Branch, d.Entry.Message}, "  "))
	_, _ = fmt.Fprintln(r.out)
	r.patch(d.Patch.Files, d.Patch.Truncated)
	if !d.IncludesUntracked {
		r.line(messages.CLIRepoUntrackedNote)
	}
	return nil
}

func (r repoRunner) diff(path string, staged bool) error {
	ctx := r.cmd.Context()
	var files []domain.FileDiff
	if path != "" {
		d, err := app.RepoDiff(ctx, r.deps, app.RepoDiffInput{RepoRefInput: r.ref, Path: path, Staged: staged})
		if err != nil {
			return err
		}
		files = []domain.FileDiff{d}
	} else {
		cs, err := app.RepoChangeSets(ctx, r.deps, r.ref)
		if err != nil {
			return err
		}
		lists := [][]domain.ChangeEntry{cs.Unstaged, cs.Untracked}
		if staged {
			lists = [][]domain.ChangeEntry{cs.Staged}
		}
		for _, list := range lists {
			for _, e := range list {
				d, err := app.RepoDiff(ctx, r.deps, app.RepoDiffInput{RepoRefInput: r.ref, Path: e.Path, Staged: staged})
				if err != nil {
					return err
				}
				files = append(files, d)
			}
		}
	}
	if r.json {
		return r.emitJSON(filesJSON(files))
	}
	r.patch(files, false)
	return nil
}

func (r repoRunner) patch(files []domain.FileDiff, truncated bool) {
	for _, f := range files {
		name := f.Path
		if f.OrigPath != "" {
			name = f.OrigPath + " -> " + f.Path
		}
		header := fmt.Sprintf("(%s, +%d -%d)", f.Status, f.Additions, f.Deletions)
		_, _ = fmt.Fprintln(r.out, "===", name, header)
		switch {
		case f.Binary:
			r.line(messages.CLIRepoBinary)
		case len(f.Hunks) == 0:
			r.line(messages.CLIRepoNoDiff)
		}
		for _, h := range f.Hunks {
			_, _ = fmt.Fprintln(r.out, h.Header)
			for _, l := range h.Lines {
				_, _ = fmt.Fprintln(r.out, l)
			}
		}
		if f.Truncated {
			r.line(messages.CLIRepoTruncated)
		}
	}
	if truncated && (len(files) == 0 || !files[len(files)-1].Truncated) {
		r.line(messages.CLIRepoTruncated)
	}
}

func (r repoRunner) fetch() error {
	res, err := app.FetchRepo(r.cmd.Context(), r.deps, r.ref)
	if err != nil {
		return err
	}
	if r.json {
		return r.emitJSON(fetchJSON{Repo: res.Alias, Remote: res.Remote, Branch: branchJSON(res.Branch)})
	}
	r.line(messages.CLIRepoFetched, res.Alias, res.Remote)
	r.branch(res.Branch)
	return nil
}

func (r repoRunner) pull() error {
	res, err := app.PullRepo(r.cmd.Context(), r.deps, r.ref)
	if err != nil {
		return err
	}
	if r.json {
		out := pullJSON{Repo: res.Alias, Upstream: res.Upstream, BeforeHead: res.BeforeHead, AfterHead: res.AfterHead, UpToDate: res.UpToDate,
			CommitsPulled: res.CommitsPulled, Ahead: res.Ahead, Behind: res.Behind, DirtyFiles: res.DirtyFiles}
		if res.Err != nil {
			out.Error = &updateErrorJSON{Code: string(domain.Code(res.Err)), Message: messages.T(messages.ForCode(domain.Code(res.Err)))}
		}
		if err := r.emitJSON(out); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintln(r.out, renderPull(res))
	}
	if res.Err != nil {
		return &cliError{text: messages.T(messages.CLIRepoPullFailed, res.Alias, messages.T(messages.ForCode(domain.Code(res.Err))))}
	}
	return nil
}

// stashAction applies, pops or drops stash@{index}, pinned to the hash the
// list shows for it right now.
func (r repoRunner) stashAction(rt *Runtime, action string, index int, yes bool) error {
	ctx := r.cmd.Context()
	list, err := app.RepoStashes(ctx, r.deps, r.ref)
	if err != nil {
		return err
	}
	in := app.RepoStashActionInput{RepoRefInput: r.ref, Index: index}
	for _, s := range list {
		if s.Index == index {
			in.Hash = s.Hash
		}
	}
	if in.Hash == "" {
		return domain.NewOpError("repo.stash", domain.CodeRefNotFound, "stash@{"+strconv.Itoa(index)+"}", "", nil)
	}
	if action == "drop" {
		d, err := app.VerifiedStash(ctx, r.deps, in)
		if err != nil {
			return err
		}
		if !yes {
			ok, err := app.ConfirmWithHelp(ctx, rt.Prompter, messages.CLIConfirmStashDrop, messages.CLIConfirmStashDropHelp, []any{d.Entry.Ref, d.Entry.Message, r.ref.Alias}, false)
			if err != nil || !ok {
				return err
			}
		}
		e, err := app.DropStash(ctx, r.deps, in)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(stashDropJSON{Repo: r.ref.Alias, Stash: toStashJSON(e)})
		}
		r.line(messages.CLIRepoStashDropped, r.ref.Alias, e.Ref, e.Message)
		return nil
	}
	run := app.ApplyStash
	if action == "pop" {
		run = app.PopStash
	}
	res, err := run(ctx, r.deps, in)
	if err != nil {
		return err
	}
	if r.json {
		out := stashApplyJSON{Repo: res.Alias, IndexRestored: res.IndexRestored, Dropped: res.Dropped, Conflicts: nonNil(res.Conflicts),
			Files: res.Files, Warnings: nonNil(res.Warnings)}
		if res.Entry.Hash != "" {
			s := toStashJSON(res.Entry)
			out.Stash = &s
		}
		if res.Err != nil {
			out.Error = &updateErrorJSON{Code: string(domain.Code(res.Err)), Message: messages.T(messages.ForCode(domain.Code(res.Err)))}
		}
		if err := r.emitJSON(out); err != nil {
			return err
		}
	} else {
		r.renderStashApply(res, action == "pop")
	}
	if res.Err != nil {
		return &cliError{text: messages.T(messages.CLIRepoStashFailed, res.Alias, "stash@{"+strconv.Itoa(index)+"}", messages.T(messages.ForCode(domain.Code(res.Err))))}
	}
	return nil
}

func (r repoRunner) renderStashApply(res app.StashApplyReport, pop bool) {
	ref := res.Entry.Ref
	switch code := domain.Code(res.Err); {
	case res.Err == nil && pop && res.Dropped:
		r.line(messages.CLIRepoStashPopped, res.Alias, ref, res.Entry.Message)
	case res.Err == nil:
		r.line(messages.CLIRepoStashApplied, res.Alias, ref, res.Entry.Message)
		if pop {
			r.line(messages.CLIRepoStashKept, ref)
		}
	case code == domain.CodeStashConflict:
		r.line(messages.CLIRepoStashConflicts, res.Alias, ref, strings.Join(res.Conflicts, ", "))
	case code == domain.CodeWorktreeDirty:
		r.line(messages.CLIRepoStashDirty, res.Alias, ref, strings.Join(res.Files, ", "))
	}
	for _, w := range res.Warnings {
		if w == app.WarningIndexNotRestored {
			r.line(messages.CLIRepoStashIndexNotRestored)
		}
	}
}

// clean permanently deletes untracked files after validating them and
// asking (unless yes).
func (r repoRunner) clean(rt *Runtime, paths []string, yes bool) error {
	ctx := r.cmd.Context()
	in := app.UntrackedPathsInput{RepoRefInput: r.ref, Paths: paths}
	t, err := app.ValidateUntracked(ctx, r.deps, in)
	if err != nil {
		return err
	}
	if !yes {
		names := make([]string, 0, len(t.Paths))
		for _, p := range t.Paths {
			names = append(names, p.Path)
		}
		ok, err := app.ConfirmWithHelp(ctx, rt.Prompter, messages.CLIConfirmClean, messages.CLIConfirmCleanHelp, []any{len(names), r.ref.Alias, strings.Join(names, ", ")}, false)
		if err != nil || !ok {
			return err
		}
	}
	res, err := app.DiscardUntracked(ctx, r.deps, in)
	if err != nil {
		return err
	}
	if r.json {
		return r.emitJSON(cleanJSON{Repo: res.Alias, Removed: res.Removed, Kept: res.Kept})
	}
	r.line(messages.CLIRepoCleaned, res.Alias, len(res.Removed))
	if len(res.Kept) > 0 {
		r.line(messages.CLIRepoCleanKept, res.Alias, strings.Join(res.Kept, ", "))
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func renderPull(r app.RepoPull) string {
	switch {
	case r.Err == nil && r.UpToDate:
		return messages.T(messages.CLIRepoPullUpToDate, r.Alias, r.Upstream)
	case r.Err == nil:
		return messages.T(messages.CLIRepoPulled, r.Alias, r.CommitsPulled, r.Upstream)
	}
	switch domain.Code(r.Err) {
	case domain.CodeWorktreeDirty:
		return messages.T(messages.CLIRepoPullDirty, r.Alias, strings.Join(r.DirtyFiles, ", "))
	case domain.CodeDiverged:
		return messages.T(messages.CLIRepoPullDiverged, r.Alias, r.Upstream, r.Ahead, r.Behind)
	}
	return messages.T(messages.CLIRepoPullFailed, r.Alias, messages.T(messages.ForCode(domain.Code(r.Err))))
}

// --json shapes (snake_case, like the other CLI --json outputs). Author
// e-mail addresses are never read, so never printed.

type upstreamJSON struct {
	Ref    string `json:"ref"`
	Remote string `json:"remote"`
	Gone   bool   `json:"gone"`
	Ahead  int    `json:"ahead"`
	Behind int    `json:"behind"`
}

type repoBranchJSON struct {
	Repo       string        `json:"repo"`
	Branch     string        `json:"branch"`
	Detached   bool          `json:"detached"`
	Head       string        `json:"head"`
	Upstream   *upstreamJSON `json:"upstream,omitempty"`
	BaseBranch string        `json:"base_branch,omitempty"`
	BaseRef    string        `json:"base_ref,omitempty"`
	BaseFound  bool          `json:"base_found"`
	BaseAhead  *int          `json:"base_ahead,omitempty"`
	BaseBehind *int          `json:"base_behind,omitempty"`
	LastFetch  *time.Time    `json:"last_fetch,omitempty"`
}

func branchJSON(b app.BranchInfo) repoBranchJSON {
	out := repoBranchJSON{Repo: b.Alias, Branch: string(b.Branch), Detached: b.Detached, Head: b.Head, BaseBranch: string(b.BaseBranch),
		BaseRef: b.BaseRef, BaseFound: b.BaseFound, BaseAhead: b.BaseAhead, BaseBehind: b.BaseBehind}
	if u := b.Upstream; u != nil {
		out.Upstream = &upstreamJSON{Ref: u.Ref, Remote: u.Remote, Gone: u.Gone, Ahead: u.Ahead, Behind: u.Behind}
	}
	if b.LastFetch != nil {
		t := b.LastFetch.UTC().Truncate(time.Second)
		out.LastFetch = &t
	}
	return out
}

type changeJSON struct {
	Path     string `json:"path"`
	OrigPath string `json:"orig_path,omitempty"`
	Status   string `json:"status"`
}

func changesJSON(list []domain.ChangeEntry) []changeJSON {
	out := make([]changeJSON, 0, len(list))
	for _, e := range list {
		out = append(out, changeJSON{Path: e.Path, OrigPath: e.OrigPath, Status: string(e.Status)})
	}
	return out
}

type repoStatusJSON struct {
	Branch     repoBranchJSON `json:"branch"`
	Staged     []changeJSON   `json:"staged"`
	Unstaged   []changeJSON   `json:"unstaged"`
	Untracked  []changeJSON   `json:"untracked"`
	Conflicted []changeJSON   `json:"conflicted"`
	Stashes    int            `json:"stashes"`
}

type commitJSON struct {
	Hash      string    `json:"hash"`
	ShortHash string    `json:"short_hash"`
	Subject   string    `json:"subject"`
	Author    string    `json:"author"`
	Date      time.Time `json:"date"`
}

func toCommitJSON(c domain.CommitInfo) commitJSON {
	return commitJSON{Hash: c.Hash, ShortHash: c.ShortHash, Subject: c.Subject, Author: c.AuthorName, Date: c.AuthorDate.UTC()}
}

type logJSON struct {
	Range    string       `json:"range"`
	Base     string       `json:"base,omitempty"`
	Upstream string       `json:"upstream,omitempty"`
	Total    int          `json:"total"`
	Offset   int          `json:"offset"`
	HasMore  bool         `json:"has_more"`
	Commits  []commitJSON `json:"commits"`
}

type hunkJSON struct {
	Header   string   `json:"header"`
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines"`
}

type fileDiffJSON struct {
	Path      string     `json:"path"`
	OrigPath  string     `json:"orig_path,omitempty"`
	Status    string     `json:"status"`
	Binary    bool       `json:"binary"`
	Truncated bool       `json:"truncated"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Hunks     []hunkJSON `json:"hunks"`
}

func filesJSON(list []domain.FileDiff) []fileDiffJSON {
	out := make([]fileDiffJSON, 0, len(list))
	for _, f := range list {
		fd := fileDiffJSON{Path: f.Path, OrigPath: f.OrigPath, Status: string(f.Status), Binary: f.Binary, Truncated: f.Truncated,
			Additions: f.Additions, Deletions: f.Deletions, Hunks: make([]hunkJSON, 0, len(f.Hunks))}
		for _, h := range f.Hunks {
			fd.Hunks = append(fd.Hunks, hunkJSON{Header: h.Header, OldStart: h.OldStart, OldLines: h.OldLines, NewStart: h.NewStart, NewLines: h.NewLines, Lines: append([]string{}, h.Lines...)})
		}
		out = append(out, fd)
	}
	return out
}

type commitDetailJSON struct {
	commitJSON
	Body      string         `json:"body"`
	Parents   []string       `json:"parents"`
	Files     []fileDiffJSON `json:"files"`
	Truncated bool           `json:"truncated"`
}

type stashJSON struct {
	Index   int       `json:"index"`
	Ref     string    `json:"ref"`
	Hash    string    `json:"hash"`
	Branch  string    `json:"branch,omitempty"`
	Message string    `json:"message"`
	Date    time.Time `json:"date"`
}

func toStashJSON(s domain.StashEntry) stashJSON {
	return stashJSON{Index: s.Index, Ref: s.Ref, Hash: s.Hash, Branch: s.Branch, Message: s.Message, Date: s.Date.UTC()}
}

type stashDetailJSON struct {
	Stash             stashJSON      `json:"stash"`
	Files             []fileDiffJSON `json:"files"`
	Truncated         bool           `json:"truncated"`
	IncludesUntracked bool           `json:"includes_untracked"`
}

type fetchJSON struct {
	Repo   string         `json:"repo"`
	Remote string         `json:"remote"`
	Branch repoBranchJSON `json:"branch"`
}

type pullJSON struct {
	Repo          string           `json:"repo"`
	Upstream      string           `json:"upstream"`
	BeforeHead    string           `json:"before_head"`
	AfterHead     string           `json:"after_head"`
	UpToDate      bool             `json:"up_to_date"`
	CommitsPulled int              `json:"commits_pulled"`
	Ahead         int              `json:"ahead"`
	Behind        int              `json:"behind"`
	DirtyFiles    []string         `json:"dirty_files,omitempty"`
	Error         *updateErrorJSON `json:"error,omitempty"`
}

type stashApplyJSON struct {
	Repo          string           `json:"repo"`
	Stash         *stashJSON       `json:"stash,omitempty"`
	IndexRestored bool             `json:"index_restored"`
	Dropped       bool             `json:"dropped"`
	Conflicts     []string         `json:"conflicts"`
	Files         []string         `json:"files,omitempty"`
	Warnings      []string         `json:"warnings"`
	Error         *updateErrorJSON `json:"error,omitempty"`
}

type stashDropJSON struct {
	Repo  string    `json:"repo"`
	Stash stashJSON `json:"stash"`
}

type cleanJSON struct {
	Repo    string   `json:"repo"`
	Removed []string `json:"removed"`
	Kept    []string `json:"kept"`
}
