// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

import "time"

// ProjectKey is the stable identity of a project record within a context.
type ProjectKey string

// Glob is a path.Match-syntax pattern.
type Glob string

// Options is the resolvable option set. Pointer fields mean "unset at this
// layer" so the Resolver (design.md §6) can distinguish unset from a zero
// value (ADR D5) — e.g. copy_env: false at the context layer must beat a
// built-in true, which zero values cannot express.
type Options struct {
	BaseBranch        *BranchName
	BranchPrefix      *string
	CopyEnv           *bool
	FetchBeforeCreate *bool
	EnvPruneDirs      []string // nil means unset; empty slice means "prune nothing"
	Remote            *string  // defaults to "origin"
}

// Context is the top-level aggregate: one named option set and its
// projects. Resolution never mixes settings across contexts.
type Context struct {
	Name           ContextName
	WorkspacesRoot Path
	ProjectsRoot   Path // optional; only used to pre-fill the wizard scan
	Defaults       Options
	IgnorePatterns []Glob // applied to discovery only, never to env files
	// IncludePatterns, when non-empty, limit discovery to repositories
	// whose folder name or root-relative path matches one of them
	// (MatchesIncludePattern). Empty means every repository is offered.
	IncludePatterns []Glob
	Projects        []Project

	// ProjectScanMaxDepth caps how many directory levels below ProjectsRoot
	// the recursive project-discovery scan (app.RunProjectWizard's own
	// scanCandidates) explores. Zero means "unset" -> resolved to
	// DefaultProjectScanMaxDepth, so an existing saved context with no
	// opinion on this gets a sensible default rather than an unbounded or
	// zero-depth walk. Raising it is only ever necessary for a projects
	// root nested deeper than that default reaches.
	ProjectScanMaxDepth int
}

// DefaultProjectScanMaxDepth is the project-discovery scan's default depth
// cap, in directory levels below ProjectsRoot (a repository directly under
// ProjectsRoot is depth 1). 5 is deep enough for the nested layouts this
// recursive scan exists to find — an org or team folder containing several
// repositories (depth 2), or one level further still, e.g.
// "<root>/<org>/<team>/<repo>" (depth 3) — with headroom to spare, while
// still bounding a scan of a root that was mistakenly pointed at something
// far broader than a projects folder (a home directory, a drive root): an
// unbounded walk of that kind of root would enumerate the entire
// filesystem before ever finishing. A context whose real layout genuinely
// nests deeper than this can raise ProjectScanMaxDepth explicitly.
const DefaultProjectScanMaxDepth = 5

// Project is an explicit managed repository record. Every optional field
// falls back through the Resolver; the common case sets only Key and
// SourceDir.
type Project struct {
	Key          ProjectKey
	SourceDir    Path           // absolute path to the main clone
	OriginBranch *BranchName    // unset => resolved BaseBranch
	DestBranch   BranchTemplate // "" => the workspace branch
	WorktreeDir  PathTemplate   // "" => "{project}" under the workspace root
	Options      Options        // per-project overrides of context defaults
}

// RepoEntry is one worktree inside a workspace.
type RepoEntry struct {
	Alias     string // directory name under Workspace.Root
	Project   ProjectKey
	SourceDir Path
	Branch    BranchName
	// BaseBranch is this repo's own comparison base, recorded only when it
	// differs from the workspace's Options.BaseBranch (e.g. the repo has no
	// such branch and its remote default was used instead). "" means
	// "unrecorded": status then resolves the base at read time.
	BaseBranch BranchName
}

// Workspace is a materialized set of worktrees under one directory.
type Workspace struct {
	Name    string
	Root    Path
	Context ContextName
	Branch  BranchName
	Created time.Time
	Options Options // the values frozen at creation time
	Repos   []RepoEntry
}

// Manifest is the persisted projection of a Workspace. SchemaVersion is the
// only field allowed to drive read-time compatibility branching.
type Manifest struct {
	SchemaVersion int
	Workspace     Workspace
	EnvCopies     []string // workspace-relative paths this tool wrote
}

// Preferences are user preferences stored in RootConfig.
type Preferences struct {
	Locale            string // "" => auto-detect
	UpdateCheck       bool
	UpdateCheckTTL    time.Duration
	TrayRefreshPeriod time.Duration
}

// RootConfig is the tiny global file: which context is active, plus prefs.
type RootConfig struct {
	SchemaVersion int
	ActiveContext ContextName
	Preferences   Preferences
}

// Overlay is a directory-scoped override. It can carry option values only —
// it can never name, define, or switch a context; that is enforced at parse
// time by the configstore adapter (phase 3), not here.
type Overlay struct {
	SchemaVersion int
	Options       Options
	Projects      map[ProjectKey]Options
}

// RepoStatus is the read-only projection used by `status`, `--json` and the
// tray.
type RepoStatus struct {
	Alias string
	// Project is the context project this repo was created from.
	Project  ProjectKey
	Branch   BranchName
	Detached bool
	Ahead    int
	Behind   int
	Dirty    bool
	Changes  ChangeSet
	// BaseBranch is the branch Ahead was counted against when the branch
	// has no upstream; "" when the upstream was used. When BaseMissing is
	// true it names the base that was expected but not found.
	BaseBranch BranchName
	// BaseMissing is true when no comparison base could be resolved in
	// this repo: Ahead/Behind are then unknown (reported as 0).
	BaseMissing bool
	// BaseBehind counts the comparison base's commits not yet in the
	// branch (what an update would integrate), against the last fetched
	// base. Nil means unknown: the branch has an upstream (no base was
	// resolved), the base is missing, or the count failed.
	BaseBehind *int
}

// RepoCoordinates identifies a GitHub repository for the update checker
// (design.md §11). Fields are build-time data (internal/buildinfo), never
// hardcoded logic.
type RepoCoordinates struct {
	Owner string
	Repo  string
}

// ReleaseInfo is the result of a ReleaseChecker.Latest call (phase 5:
// confirming/adjusting phase 1's placeholder shape, per its own handoff
// note). It deliberately does not carry an "is a newer version available"
// bool: ports.ReleaseChecker knows nothing about the caller's own running
// version (design.md §4's Latest signature takes only RepoCoordinates), so
// that comparison belongs to app.CheckForUpdate, via CompareVersion, not to
// the checker itself.
//
// Unavailable is the field phase 1's placeholder was missing: it lets a
// caller distinguish "checked, nothing wrong, Tag is meaningful" from
// "could not determine anything at all" (offline with no cache, rate-limited
// with no cache, or an unbranded build that never calls the checker) —
// without it, "no update available" and "we don't know" collapse into the
// same zero value, which is exactly the kind of state a signature must be
// able to represent to be testable at all.
type ReleaseInfo struct {
	Tag         string
	URL         string
	Stale       bool // true when Tag/URL reflect a cached result rather than a fresh 200 response
	Unavailable bool // true when no result could be determined; Tag/URL/Stale are meaningless
}
