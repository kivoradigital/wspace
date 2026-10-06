// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package engine

import (
	"context"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/domain"
)

// VersionResult is engine.version's result.
type VersionResult struct {
	Version string `json:"version"`
}

// Version reports the engine's build version.
func (e *Engine) Version() VersionResult { return VersionResult{Version: e.deps.Version} }

// ResolvedOption is one resolved option value and the layer that won it
// ("flag", "project", "workspace", "overlay", "context", "builtin").
type ResolvedOption struct {
	Value any    `json:"value"`
	From  string `json:"from"`
}

// InfoParams selects a context for engine.info.
type InfoParams struct {
	Context string `json:"context,omitempty"`
}

// InfoResult is engine.info's result: the configuration directory and a
// context's resolved options with provenance (no directory overlay — a
// server has no meaningful working directory).
type InfoResult struct {
	ConfigDir   string                    `json:"configDir"`
	ContextName string                    `json:"contextName"`
	Options     map[string]ResolvedOption `json:"options"`
}

// Info reports resolved options for a context.
func (e *Engine) Info(ctx context.Context, p InfoParams) (InfoResult, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return InfoResult{}, err
	}
	r := domain.Resolver{Context: &c}
	base := r.BaseBranch("")
	prefix := r.BranchPrefix("")
	copyEnv := r.CopyEnv("")
	fetch := r.FetchBeforeCreate()
	prune := r.EnvPruneDirs("")
	remote := r.Remote("")
	return InfoResult{
		ConfigDir:   string(e.deps.FS.Paths().Config),
		ContextName: string(c.Name),
		Options: map[string]ResolvedOption{
			"baseBranch":        {Value: string(base.Value), From: base.From.String()},
			"branchPrefix":      {Value: prefix.Value, From: prefix.From.String()},
			"copyEnv":           {Value: copyEnv.Value, From: copyEnv.From.String()},
			"fetchBeforeCreate": {Value: fetch.Value, From: fetch.From.String()},
			"envPruneDirs":      {Value: append([]string{}, prune.Value...), From: prune.From.String()},
			"remote":            {Value: remote.Value, From: remote.From.String()},
		},
	}, nil
}

// DoctorResult is engine.doctor's result; individual findings are also
// streamed as "warn" progress events.
type DoctorResult struct {
	GitTooOld       bool `json:"gitTooOld"`
	PrunedWorktrees int  `json:"prunedWorktrees"`
}

// Doctor runs diagnostics over a context's workspaces (pruning stale
// worktree registrations, nothing else).
func (e *Engine) Doctor(ctx context.Context, p InfoParams, emit ProgressFunc) (DoctorResult, error) {
	c, err := e.resolveContext(ctx, p.Context)
	if err != nil {
		return DoctorResult{}, err
	}
	res, err := app.Doctor(ctx, e.appDeps(emit), app.DoctorInput{WorkspacesRoot: c.WorkspacesRoot})
	if err != nil {
		return DoctorResult{}, wrap(err)
	}
	return DoctorResult{GitTooOld: res.GitTooOld, PrunedWorktrees: res.PrunedWorktrees}, nil
}

// UpdateCheckResult is engine.checkUpdate's result. Unavailable means
// nothing could be determined (offline, rate-limited, unbranded build).
type UpdateCheckResult struct {
	CurrentVersion string `json:"currentVersion"`
	LatestTag      string `json:"latestTag,omitempty"`
	Available      bool   `json:"available"`
	Unavailable    bool   `json:"unavailable"`
}

// CheckForUpdate asks the release checker for the latest release. It never
// fails on a network problem; that is reported as Unavailable.
func (e *Engine) CheckForUpdate(ctx context.Context) (UpdateCheckResult, error) {
	if e.deps.Checker == nil {
		return UpdateCheckResult{CurrentVersion: e.deps.Version, Unavailable: true}, nil
	}
	res, err := app.CheckForUpdate(ctx, app.CheckForUpdateDeps{Checker: e.deps.Checker}, app.CheckForUpdateInput{
		Coordinates: e.deps.Coordinates, CurrentVersion: e.deps.Version,
	})
	if err != nil {
		return UpdateCheckResult{}, wrap(err)
	}
	return UpdateCheckResult{CurrentVersion: res.CurrentVersion, LatestTag: res.LatestTag, Available: res.Available, Unavailable: res.Unavailable}, nil
}
