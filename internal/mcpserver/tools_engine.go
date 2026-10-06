// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
)

// DoctorOutput is run_doctor's result: engine.doctor plus the warnings it
// streamed.
type DoctorOutput struct {
	GitTooOld       bool     `json:"gitTooOld"`
	PrunedWorktrees int      `json:"prunedWorktrees"`
	Warnings        []string `json:"warnings"`
}

func registerEngineTools(s *mcp.Server, eng *engine.Engine) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "engine_version",
		Description: "Report the wspace engine's build version. Use it to tell the user which wspace they run or when a tool behaves unexpectedly.",
		Annotations: readOnly("Engine version"),
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ NoInput) (*mcp.CallToolResult, engine.VersionResult, error) {
		return done(eng.Version(), nil, func(v engine.VersionResult) string { return "wspace engine " + v.Version })
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "engine_info",
		Description: "Show the configuration directory and a context's resolved options (baseBranch, branchPrefix, copyEnv, fetchBeforeCreate, envPruneDirs, remote), each with the layer that set it. " +
			"Use it before create_workspace to know which base branch and branch prefix will apply.",
		Annotations: readOnly("Engine info"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ContextInput) (*mcp.CallToolResult, engine.InfoResult, error) {
		res, err := eng.Info(ctx, engine.InfoParams{Context: in.Context})
		return done(res, err, func(r engine.InfoResult) string {
			return fmt.Sprintf("Context %s, config in %s, base branch %v.", r.ContextName, r.ConfigDir, r.Options["baseBranch"].Value)
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "check_update",
		Description: "Ask GitHub whether a newer wspace release exists. It contacts the network but never fails on a network problem: " +
			"it then reports unavailable=true. When bundledBy is set, wspace is bundled with that app and updates only with it: " +
			"no network call is made, and never suggest updating wspace separately. Use only when the user asks about updates.",
		Annotations: &mcp.ToolAnnotations{Title: "Check for update", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: boolPtr(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ NoInput) (*mcp.CallToolResult, engine.UpdateCheckResult, error) {
		res, err := eng.CheckForUpdate(ctx)
		return done(res, err, func(r engine.UpdateCheckResult) string {
			switch {
			case r.BundledBy != "":
				return fmt.Sprintf("wspace %s is bundled with %s and updates with it.", r.CurrentVersion, r.BundledBy)
			case r.Unavailable:
				return "Could not determine whether an update is available."
			case r.Available:
				return fmt.Sprintf("Update available: %s (running %s).", r.LatestTag, r.CurrentVersion)
			default:
				return fmt.Sprintf("wspace %s is up to date.", r.CurrentVersion)
			}
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "run_doctor",
		Description: "Diagnose a context: check that git is recent enough and prune stale git worktree registrations of its workspaces (registrations whose folder is gone). " +
			"It never touches files or branches. Use it when worktree operations fail with stale-registration errors.",
		Annotations: mutating("Run doctor", false, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ContextInput) (*mcp.CallToolResult, DoctorOutput, error) {
		warnings := []string{}
		forward := progressFor(ctx, req)
		res, err := eng.Doctor(ctx, engine.InfoParams{Context: in.Context}, func(ev engine.ProgressEvent) {
			if ev.Kind == engine.KindWarn {
				warnings = append(warnings, ev.Message)
			}
			if forward != nil {
				forward(ev)
			}
		})
		out := DoctorOutput{GitTooOld: res.GitTooOld, PrunedWorktrees: res.PrunedWorktrees, Warnings: warnings}
		return done(out, err, func(o DoctorOutput) string {
			git := "git is supported"
			if o.GitTooOld {
				git = "git is too old"
			}
			return fmt.Sprintf("Doctor: %s, %s pruned, %s.", git, plural(o.PrunedWorktrees, "stale worktree registration", "stale worktree registrations"), plural(len(o.Warnings), "warning", "warnings"))
		})
	})
}
