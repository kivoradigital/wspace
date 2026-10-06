// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

// Layer identifies which layer of the precedence chain (design.md §6, Chain
// B) produced a Resolved value.
type Layer int

const (
	LayerFlag Layer = iota
	LayerProject
	LayerWorkspace
	LayerOverlay
	LayerContext
	LayerBuiltin
)

func (l Layer) String() string {
	switch l {
	case LayerFlag:
		return "flag"
	case LayerProject:
		return "project"
	case LayerWorkspace:
		return "workspace"
	case LayerOverlay:
		return "overlay"
	case LayerContext:
		return "context"
	case LayerBuiltin:
		return "builtin"
	default:
		return "unknown"
	}
}

// Resolved carries a value alongside the Layer that won it. "ws info --json"
// emits every resolved key with its From layer (ADR D6): precedence becomes
// a command's output instead of an investigation across four files.
type Resolved[T any] struct {
	Value T
	From  Layer
}

// Resolver is pure: it holds only already-loaded layers and never performs
// I/O. Workspace is nil for `create` (the workspace does not exist yet);
// Overlay is nil when no .ws.yaml was found.
type Resolver struct {
	Flags     Options
	Context   *Context
	Workspace *Workspace
	Overlay   *Overlay
}

// BuiltinOptions are the last-resort defaults when no layer sets a value.
var BuiltinOptions = struct {
	BaseBranch        BranchName
	BranchPrefix      string
	CopyEnv           bool
	FetchBeforeCreate bool
	EnvPruneDirs      []string
	Remote            string
}{
	BaseBranch:        BranchName("develop"),
	BranchPrefix:      "",
	CopyEnv:           true,
	FetchBeforeCreate: true,
	EnvPruneDirs:      []string{"node_modules", "vendor", "dist", "target", ".git"},
	Remote:            "origin",
}

// BuiltinWorktreeDir is the default worktree path template when a project
// does not set one: the worktree lands directly under the workspace root,
// named after the project.
const BuiltinWorktreeDir PathTemplate = "{project}"

// layerValue is one candidate value for a resolvable key at a specific
// layer. set distinguishes "this layer had no opinion" (skip it) from "this
// layer's value is the zero value" (still wins), which is exactly the
// pointer-vs-zero-value distinction ADR D5 exists to preserve.
type layerValue[T any] struct {
	layer Layer
	value T
	set   bool
}

// resolve consolidates every Resolver method's layer walk into one generic
// helper: the first layer with set==true wins; otherwise builtin wins as
// LayerBuiltin.
func resolve[T any](layers []layerValue[T], builtin T) Resolved[T] {
	for _, l := range layers {
		if l.set {
			return Resolved[T]{Value: l.value, From: l.layer}
		}
	}
	return Resolved[T]{Value: builtin, From: LayerBuiltin}
}

// ptrLayer adapts a pointer-typed Options field ("nil means unset") into a
// layerValue.
func ptrLayer[T any](layer Layer, p *T) layerValue[T] {
	if p == nil {
		return layerValue[T]{layer: layer}
	}
	return layerValue[T]{layer: layer, value: *p, set: true}
}

// sliceLayer adapts a slice-typed Options field ("nil means unset") into a
// layerValue.
func sliceLayer(layer Layer, s []string) layerValue[[]string] {
	return layerValue[[]string]{layer: layer, value: s, set: s != nil}
}

func findProject(ctx *Context, key ProjectKey) *Project {
	if ctx == nil {
		return nil
	}
	for i := range ctx.Projects {
		if ctx.Projects[i].Key == key {
			return &ctx.Projects[i]
		}
	}
	return nil
}

// overlayPtr resolves an overlay layer for a pointer-typed field: the
// project-scoped override wins over the overlay's general option, but both
// are reported as LayerOverlay (design.md §6: "local overlay
// overlay.Projects[P][K] then overlay.Options[K]").
func overlayPtr[T any](overlay *Overlay, p ProjectKey, get func(Options) *T) layerValue[T] {
	if overlay == nil {
		return layerValue[T]{layer: LayerOverlay}
	}
	if po, ok := overlay.Projects[p]; ok {
		if v := get(po); v != nil {
			return layerValue[T]{layer: LayerOverlay, value: *v, set: true}
		}
	}
	return ptrLayer(LayerOverlay, get(overlay.Options))
}

func overlaySlice(overlay *Overlay, p ProjectKey, get func(Options) []string) layerValue[[]string] {
	if overlay == nil {
		return layerValue[[]string]{layer: LayerOverlay}
	}
	if po, ok := overlay.Projects[p]; ok {
		if v := get(po); v != nil {
			return layerValue[[]string]{layer: LayerOverlay, value: v, set: true}
		}
	}
	return sliceLayer(LayerOverlay, get(overlay.Options))
}

// BaseBranch resolves the base branch a project's worktree is created from.
func (r Resolver) BaseBranch(p ProjectKey) Resolved[BranchName] {
	layers := []layerValue[BranchName]{
		ptrLayer(LayerFlag, r.Flags.BaseBranch),
	}
	if proj := findProject(r.Context, p); proj != nil {
		layers = append(layers, ptrLayer(LayerProject, proj.Options.BaseBranch))
	} else {
		layers = append(layers, layerValue[BranchName]{layer: LayerProject})
	}
	if r.Workspace != nil {
		layers = append(layers, ptrLayer(LayerWorkspace, r.Workspace.Options.BaseBranch))
	} else {
		layers = append(layers, layerValue[BranchName]{layer: LayerWorkspace})
	}
	layers = append(layers, overlayPtr(r.Overlay, p, func(o Options) *BranchName { return o.BaseBranch }))
	if r.Context != nil {
		layers = append(layers, ptrLayer(LayerContext, r.Context.Defaults.BaseBranch))
	} else {
		layers = append(layers, layerValue[BranchName]{layer: LayerContext})
	}
	return resolve(layers, BuiltinOptions.BaseBranch)
}

// CopyEnv resolves whether env files are copied into a project's worktree.
func (r Resolver) CopyEnv(p ProjectKey) Resolved[bool] {
	layers := []layerValue[bool]{
		ptrLayer(LayerFlag, r.Flags.CopyEnv),
	}
	if proj := findProject(r.Context, p); proj != nil {
		layers = append(layers, ptrLayer(LayerProject, proj.Options.CopyEnv))
	} else {
		layers = append(layers, layerValue[bool]{layer: LayerProject})
	}
	if r.Workspace != nil {
		layers = append(layers, ptrLayer(LayerWorkspace, r.Workspace.Options.CopyEnv))
	} else {
		layers = append(layers, layerValue[bool]{layer: LayerWorkspace})
	}
	layers = append(layers, overlayPtr(r.Overlay, p, func(o Options) *bool { return o.CopyEnv }))
	if r.Context != nil {
		layers = append(layers, ptrLayer(LayerContext, r.Context.Defaults.CopyEnv))
	} else {
		layers = append(layers, layerValue[bool]{layer: LayerContext})
	}
	return resolve(layers, BuiltinOptions.CopyEnv)
}

// FetchBeforeCreate resolves whether `create` fetches before creating
// worktrees. It has no per-project override: Options carries the field, but
// no ProjectKey is threaded through, so the project layer never applies.
func (r Resolver) FetchBeforeCreate() Resolved[bool] {
	layers := []layerValue[bool]{
		ptrLayer(LayerFlag, r.Flags.FetchBeforeCreate),
		{layer: LayerProject},
	}
	if r.Workspace != nil {
		layers = append(layers, ptrLayer(LayerWorkspace, r.Workspace.Options.FetchBeforeCreate))
	} else {
		layers = append(layers, layerValue[bool]{layer: LayerWorkspace})
	}
	if r.Overlay != nil {
		layers = append(layers, ptrLayer(LayerOverlay, r.Overlay.Options.FetchBeforeCreate))
	} else {
		layers = append(layers, layerValue[bool]{layer: LayerOverlay})
	}
	if r.Context != nil {
		layers = append(layers, ptrLayer(LayerContext, r.Context.Defaults.FetchBeforeCreate))
	} else {
		layers = append(layers, layerValue[bool]{layer: LayerContext})
	}
	return resolve(layers, BuiltinOptions.FetchBeforeCreate)
}

// EnvPruneDirs resolves which directories env-file discovery prunes.
func (r Resolver) EnvPruneDirs(p ProjectKey) Resolved[[]string] {
	layers := []layerValue[[]string]{
		sliceLayer(LayerFlag, r.Flags.EnvPruneDirs),
	}
	if proj := findProject(r.Context, p); proj != nil {
		layers = append(layers, sliceLayer(LayerProject, proj.Options.EnvPruneDirs))
	} else {
		layers = append(layers, layerValue[[]string]{layer: LayerProject})
	}
	if r.Workspace != nil {
		layers = append(layers, sliceLayer(LayerWorkspace, r.Workspace.Options.EnvPruneDirs))
	} else {
		layers = append(layers, layerValue[[]string]{layer: LayerWorkspace})
	}
	layers = append(layers, overlaySlice(r.Overlay, p, func(o Options) []string { return o.EnvPruneDirs }))
	if r.Context != nil {
		layers = append(layers, sliceLayer(LayerContext, r.Context.Defaults.EnvPruneDirs))
	} else {
		layers = append(layers, layerValue[[]string]{layer: LayerContext})
	}
	return resolve(layers, BuiltinOptions.EnvPruneDirs)
}

// Remote resolves the git remote name used for fetch/base resolution.
func (r Resolver) Remote(p ProjectKey) Resolved[string] {
	layers := []layerValue[string]{
		ptrLayer(LayerFlag, r.Flags.Remote),
	}
	if proj := findProject(r.Context, p); proj != nil {
		layers = append(layers, ptrLayer(LayerProject, proj.Options.Remote))
	} else {
		layers = append(layers, layerValue[string]{layer: LayerProject})
	}
	if r.Workspace != nil {
		layers = append(layers, ptrLayer(LayerWorkspace, r.Workspace.Options.Remote))
	} else {
		layers = append(layers, layerValue[string]{layer: LayerWorkspace})
	}
	layers = append(layers, overlayPtr(r.Overlay, p, func(o Options) *string { return o.Remote }))
	if r.Context != nil {
		layers = append(layers, ptrLayer(LayerContext, r.Context.Defaults.Remote))
	} else {
		layers = append(layers, layerValue[string]{layer: LayerContext})
	}
	return resolve(layers, BuiltinOptions.Remote)
}

// BranchPrefix resolves the prefix prepended to a workspace name when
// DestBranch has to invent a branch name because neither a project's
// dest_branch template nor an explicit --branch flag supplied one
// (context-management spec: every context stores "branch_prefix";
// design.md §6 Chain B applies to it exactly like every other Options
// field). It was declared and round-tripped by the config store since the
// original design but never read anywhere until this method existed.
func (r Resolver) BranchPrefix(p ProjectKey) Resolved[string] {
	layers := []layerValue[string]{
		ptrLayer(LayerFlag, r.Flags.BranchPrefix),
	}
	if proj := findProject(r.Context, p); proj != nil {
		layers = append(layers, ptrLayer(LayerProject, proj.Options.BranchPrefix))
	} else {
		layers = append(layers, layerValue[string]{layer: LayerProject})
	}
	if r.Workspace != nil {
		layers = append(layers, ptrLayer(LayerWorkspace, r.Workspace.Options.BranchPrefix))
	} else {
		layers = append(layers, layerValue[string]{layer: LayerWorkspace})
	}
	layers = append(layers, overlayPtr(r.Overlay, p, func(o Options) *string { return o.BranchPrefix }))
	if r.Context != nil {
		layers = append(layers, ptrLayer(LayerContext, r.Context.Defaults.BranchPrefix))
	} else {
		layers = append(layers, layerValue[string]{layer: LayerContext})
	}
	return resolve(layers, BuiltinOptions.BranchPrefix)
}

// WorktreePath resolves the absolute path a project's worktree is created
// at. WorktreeDir lives on Project directly (not Options), so only the
// project and built-in layers ever apply.
func (r Resolver) WorktreePath(p ProjectKey, v Vars, root Path) (Resolved[Path], error) {
	if proj := findProject(r.Context, p); proj != nil && proj.WorktreeDir != "" {
		resolved, err := proj.WorktreeDir.Resolve(r.withPrefix(p, v), root)
		if err != nil {
			return Resolved[Path]{}, err
		}
		return Resolved[Path]{Value: resolved, From: LayerProject}, nil
	}
	resolved, err := BuiltinWorktreeDir.Resolve(v, root)
	if err != nil {
		return Resolved[Path]{}, err
	}
	return Resolved[Path]{Value: resolved, From: LayerBuiltin}, nil
}

// DestBranch resolves the branch a project's worktree is checked out onto.
// DestBranch lives on Project directly (not Options); when unset, it
// inherits "the workspace branch" — Workspace.Branch for an existing
// workspace (LayerWorkspace), or, pre-creation, the explicit --branch flag
// (LayerFlag: Chain B step 1) when one was supplied.
//
// When none of the above apply (create, no project override, no --branch),
// a branch still has to come from somewhere: the workspace name alone
// would collide across contexts that reuse names like "fix" or "main", so
// this falls back to JoinBranchPrefix(BranchPrefix(p), v.Workspace): the
// prefix is a namespace joined with exactly one "/" whether or not it was
// configured with a trailing slash (an unset prefix yields exactly the
// workspace name). This is
// what makes "ws create <name>" work without "--branch" instead of failing
// on NewBranchName(""); the composed layer is BranchPrefix's own winning
// layer, since that is the only resolved input driving the default.
func (r Resolver) DestBranch(p ProjectKey, v Vars) (Resolved[BranchName], error) {
	if proj := findProject(r.Context, p); proj != nil && proj.DestBranch != "" {
		resolved, err := proj.DestBranch.Resolve(r.withPrefix(p, v))
		if err != nil {
			return Resolved[BranchName]{}, err
		}
		return Resolved[BranchName]{Value: resolved, From: LayerProject}, nil
	}
	if r.Workspace != nil {
		return Resolved[BranchName]{Value: r.Workspace.Branch, From: LayerWorkspace}, nil
	}
	if v.Branch != "" {
		bn, err := NewBranchName(v.Branch)
		if err != nil {
			return Resolved[BranchName]{}, err
		}
		return Resolved[BranchName]{Value: bn, From: LayerFlag}, nil
	}
	prefix := r.BranchPrefix(p)
	bn, err := NewBranchName(JoinBranchPrefix(prefix.Value, v.Workspace))
	if err != nil {
		return Resolved[BranchName]{}, err
	}
	return Resolved[BranchName]{Value: bn, From: prefix.From}, nil
}

// withPrefix fills v.Prefix, when the caller left it empty, with p's
// resolved branch prefix in its normalized form ("feature" -> "feature/"),
// so a "{prefix}{workspace}" template composes exactly like the built-in
// default branch does.
func (r Resolver) withPrefix(p ProjectKey, v Vars) Vars {
	if v.Prefix == "" {
		v.Prefix = NormalizeBranchPrefix(r.BranchPrefix(p).Value)
	}
	return v
}
