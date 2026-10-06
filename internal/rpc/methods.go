// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/messages"
)

// handler runs one method: params is the raw "params" value (never nil),
// emit streams progress for this request.
type handler func(ctx context.Context, params json.RawMessage, emit engine.ProgressFunc) (any, error)

// decode strictly unmarshals params into v: unknown fields and a
// non-object value are invalid_params, so a typo never silently becomes
// "parameter not given".
func decode(params json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		e := &engine.Error{Code: engine.CodeInvalidParams, Message: messages.T(messages.RPCInvalidParams, err.Error())}
		return e
	}
	return nil
}

// method adapts a typed engine call into a handler.
func method[P any, R any](call func(ctx context.Context, p P, emit engine.ProgressFunc) (R, error)) handler {
	return func(ctx context.Context, params json.RawMessage, emit engine.ProgressFunc) (any, error) {
		var p P
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return call(ctx, p, emit)
	}
}

// empty is the result of a method with nothing to return.
type empty struct{}

// noParams is the params type of a method that takes none.
type noParams struct{}

func (s *Server) registerMethods() {
	e := s.eng
	s.methods = map[string]handler{
		"rpc.hello": method(func(_ context.Context, _ noParams, _ engine.ProgressFunc) (helloResult, error) {
			return helloResult{ProtocolVersion: ProtocolVersion, EngineVersion: s.engineVersion, Methods: s.methodNames()}, nil
		}),

		"engine.version": method(func(_ context.Context, _ noParams, _ engine.ProgressFunc) (engine.VersionResult, error) {
			return e.Version(), nil
		}),
		"engine.info": method(func(ctx context.Context, p engine.InfoParams, _ engine.ProgressFunc) (engine.InfoResult, error) {
			return e.Info(ctx, p)
		}),
		"engine.doctor": method(e.Doctor),
		"engine.checkUpdate": method(func(ctx context.Context, _ noParams, _ engine.ProgressFunc) (engine.UpdateCheckResult, error) {
			return e.CheckForUpdate(ctx)
		}),

		"mcp.sessions": method(func(ctx context.Context, _ noParams, _ engine.ProgressFunc) ([]engine.MCPSession, error) {
			return e.MCPSessions(ctx)
		}),

		"agents.status": method(func(ctx context.Context, p engine.AgentsParams, _ engine.ProgressFunc) (engine.AgentsStatusResult, error) {
			return e.AgentsStatus(ctx, p)
		}),
		"agents.install": method(func(ctx context.Context, p engine.AgentsInstallParams, _ engine.ProgressFunc) (engine.AgentsChangeResult, error) {
			return e.AgentsInstall(ctx, p)
		}),
		"agents.uninstall": method(func(ctx context.Context, p engine.AgentsParams, _ engine.ProgressFunc) (engine.AgentsChangeResult, error) {
			return e.AgentsUninstall(ctx, p)
		}),
		"agents.mcpClean": method(func(ctx context.Context, p engine.AgentsMCPCleanParams, _ engine.ProgressFunc) (engine.AgentsChangeResult, error) {
			return e.AgentsMCPClean(ctx, p)
		}),

		"contexts.list": method(func(ctx context.Context, _ noParams, _ engine.ProgressFunc) ([]engine.ContextSummary, error) {
			return e.ListContexts(ctx)
		}),
		"contexts.get": method(func(ctx context.Context, p engine.ContextRef, _ engine.ProgressFunc) (engine.Context, error) {
			return e.GetContext(ctx, p)
		}),
		"contexts.create": method(func(ctx context.Context, p engine.CreateContextParams, _ engine.ProgressFunc) (engine.Context, error) {
			return e.CreateContext(ctx, p)
		}),
		"contexts.update": method(func(ctx context.Context, p engine.UpdateContextParams, _ engine.ProgressFunc) (engine.Context, error) {
			return e.UpdateContext(ctx, p)
		}),
		"contexts.delete": method(func(ctx context.Context, p engine.DeleteContextParams, _ engine.ProgressFunc) (empty, error) {
			return empty{}, e.DeleteContext(ctx, p)
		}),
		"contexts.switch": method(func(ctx context.Context, p engine.ContextRef, _ engine.ProgressFunc) (engine.ContextSummary, error) {
			return e.SwitchContext(ctx, p)
		}),
		"contexts.importLegacy": method(func(ctx context.Context, p engine.ImportLegacyParams, _ engine.ProgressFunc) (engine.ImportLegacyResult, error) {
			return e.ImportLegacyContext(ctx, p)
		}),

		"projects.scan": method(func(ctx context.Context, p engine.ScanProjectsParams, _ engine.ProgressFunc) (engine.ScanProjectsResult, error) {
			return e.ScanProjects(ctx, p)
		}),
		"projects.list": method(func(ctx context.Context, p engine.ProjectsRef, _ engine.ProgressFunc) ([]engine.Project, error) {
			return e.ListProjects(ctx, p)
		}),
		"projects.register": method(func(ctx context.Context, p engine.RegisterProjectParams, _ engine.ProgressFunc) (engine.Project, error) {
			return e.RegisterProject(ctx, p)
		}),
		"projects.registerMany": method(func(ctx context.Context, p engine.RegisterProjectsParams, _ engine.ProgressFunc) (engine.RegisterProjectsResult, error) {
			return e.RegisterProjects(ctx, p)
		}),
		"projects.update": method(func(ctx context.Context, p engine.UpdateProjectParams, _ engine.ProgressFunc) (engine.Project, error) {
			return e.UpdateProject(ctx, p)
		}),
		"projects.remove": method(func(ctx context.Context, p engine.ProjectRef, _ engine.ProgressFunc) (empty, error) {
			return empty{}, e.RemoveProject(ctx, p)
		}),

		"workspaces.list": method(func(ctx context.Context, p engine.WorkspacesRef, _ engine.ProgressFunc) ([]engine.WorkspaceStatus, error) {
			return e.ListWorkspaces(ctx, p)
		}),
		"workspaces.status": method(func(ctx context.Context, p engine.WorkspaceRef, _ engine.ProgressFunc) (engine.WorkspaceStatus, error) {
			return e.WorkspaceStatus(ctx, p)
		}),
		"workspaces.repoChanges": method(func(ctx context.Context, p engine.RepoRef, _ engine.ProgressFunc) ([]engine.FileChange, error) {
			return e.RepoChanges(ctx, p)
		}),
		"workspaces.addableProjects": method(func(ctx context.Context, p engine.WorkspaceRef, _ engine.ProgressFunc) ([]engine.Project, error) {
			return e.AddableProjects(ctx, p)
		}),
		"workspaces.teardownCheck": method(func(ctx context.Context, p engine.WorkspaceRef, _ engine.ProgressFunc) ([]engine.Blocker, error) {
			return e.TeardownBlockers(ctx, p)
		}),
		"workspaces.adoptLegacy": method(func(ctx context.Context, p engine.WorkspacesRef, _ engine.ProgressFunc) (engine.AdoptLegacyResult, error) {
			return e.AdoptLegacyWorkspaces(ctx, p)
		}),
		"workspaces.claim": method(func(ctx context.Context, p engine.ClaimWorkspacesParams, _ engine.ProgressFunc) (engine.ClaimWorkspacesResult, error) {
			return e.ClaimWorkspaces(ctx, p)
		}),
		"workspaces.create":     method(e.CreateWorkspace),
		"workspaces.destroy":    method(e.DestroyWorkspace),
		"workspaces.addRepo":    method(e.AddRepo),
		"workspaces.removeRepo": method(e.RemoveRepo),
		"workspaces.repair":     method(e.Repair),
		"workspaces.syncEnv":    method(e.SyncEnv),
		"workspaces.updateRepo": method(e.UpdateRepo),
		"workspaces.update":     method(e.UpdateWorkspace),

		"repos.inspect": method(func(ctx context.Context, p engine.RepoRef, _ engine.ProgressFunc) (engine.RepoInspection, error) {
			return e.RepoInspect(ctx, p)
		}),
		"repos.changes": method(func(ctx context.Context, p engine.RepoRef, _ engine.ProgressFunc) (engine.RepoChangeSets, error) {
			return e.RepoChangeSets(ctx, p)
		}),
		"repos.diff": method(func(ctx context.Context, p engine.RepoDiffParams, _ engine.ProgressFunc) (engine.FileDiff, error) {
			return e.RepoDiff(ctx, p)
		}),
		"repos.commits": method(func(ctx context.Context, p engine.RepoCommitsParams, _ engine.ProgressFunc) (engine.RepoCommitsResult, error) {
			return e.RepoCommits(ctx, p)
		}),
		"repos.commit": method(func(ctx context.Context, p engine.RepoCommitParams, _ engine.ProgressFunc) (engine.CommitDetail, error) {
			return e.RepoCommit(ctx, p)
		}),
		"repos.stashes": method(func(ctx context.Context, p engine.RepoRef, _ engine.ProgressFunc) ([]engine.Stash, error) {
			return e.RepoStashes(ctx, p)
		}),
		"repos.stash": method(func(ctx context.Context, p engine.RepoStashParams, _ engine.ProgressFunc) (engine.StashDetail, error) {
			return e.RepoStash(ctx, p)
		}),
		"repos.fetch": method(e.RepoFetch),
		"repos.pull":  method(e.RepoPull),
		"repos.stashApply": method(func(ctx context.Context, p engine.RepoStashActionParams, _ engine.ProgressFunc) (engine.RepoStashApplyResult, error) {
			return e.RepoStashApply(ctx, p)
		}),
		"repos.stashPop": method(func(ctx context.Context, p engine.RepoStashActionParams, _ engine.ProgressFunc) (engine.RepoStashApplyResult, error) {
			return e.RepoStashPop(ctx, p)
		}),
		"repos.stashDrop": method(func(ctx context.Context, p engine.RepoStashActionParams, _ engine.ProgressFunc) (engine.RepoStashDropResult, error) {
			return e.RepoStashDrop(ctx, p)
		}),
		"repos.validateUntracked": method(func(ctx context.Context, p engine.RepoPathsParams, _ engine.ProgressFunc) (engine.UntrackedTargetsResult, error) {
			return e.RepoValidateUntracked(ctx, p)
		}),
		"repos.discardUntracked": method(func(ctx context.Context, p engine.RepoPathsParams, _ engine.ProgressFunc) (engine.DiscardUntrackedResult, error) {
			return e.RepoDiscardUntracked(ctx, p)
		}),
		"repos.stage": method(func(ctx context.Context, p engine.RepoChangePathsParams, _ engine.ProgressFunc) (engine.RepoStageResult, error) {
			return e.RepoStage(ctx, p)
		}),
		"repos.unstage": method(func(ctx context.Context, p engine.RepoChangePathsParams, _ engine.ProgressFunc) (engine.RepoStageResult, error) {
			return e.RepoUnstage(ctx, p)
		}),
		"repos.discard": method(func(ctx context.Context, p engine.RepoChangePathsParams, _ engine.ProgressFunc) (engine.RepoDiscardResult, error) {
			return e.RepoDiscard(ctx, p)
		}),
		"repos.commitChanges": method(func(ctx context.Context, p engine.RepoCommitChangesParams, _ engine.ProgressFunc) (engine.RepoCommitChangesResult, error) {
			return e.RepoCommitChanges(ctx, p)
		}),
		"repos.push": method(e.RepoPush),
		"repos.stashCreate": method(func(ctx context.Context, p engine.RepoStashCreateParams, _ engine.ProgressFunc) (engine.RepoStashCreateResult, error) {
			return e.RepoStashCreate(ctx, p)
		}),
	}
}

// encodeResult marshals a handler's result; a nil slice becomes [] so
// every list result is an array on the wire, never null.
func encodeResult(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("rpc: encode result: %w", err)
	}
	if string(b) == "null" {
		b = []byte("[]")
	}
	return b, nil
}
