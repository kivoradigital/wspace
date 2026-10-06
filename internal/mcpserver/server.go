// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/messages"
)

// ServerName is the MCP implementation name advertised on initialize.
const ServerName = "wspace"

// New returns an MCP server exposing wspace's tools over eng. Once a
// client has initialized, the server keeps a presence record for this
// process (engine.StartPresence) naming the client and its last tool, and
// removes it when the session ends.
func New(eng *engine.Engine, version string) *mcp.Server {
	s, _ := newServer(eng, version)
	return s
}

func newServer(eng *engine.Engine, version string) (*mcp.Server, *presenceHook) {
	hook := &presenceHook{eng: eng}
	s := mcp.NewServer(&mcp.Implementation{Name: ServerName, Title: "wspace", Version: version}, &mcp.ServerOptions{
		Instructions: serverInstructions,
	})
	s.AddReceivingMiddleware(hook.middleware)
	registerTools(s, eng)
	return s, hook
}

// Serve runs the server over in/out (newline-delimited JSON-RPC, the MCP
// stdio framing) until the client disconnects or ctx is cancelled. The
// presence record is removed before it returns.
func Serve(ctx context.Context, eng *engine.Engine, version string, in io.Reader, out io.Writer) error {
	s, hook := newServer(eng, version)
	defer hook.close(ctx)
	return s.Run(ctx, &mcp.IOTransport{Reader: io.NopCloser(in), Writer: nopWriteCloser{out}})
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// presenceHook starts the presence record when a client initializes,
// records every tool call, and removes the record when the session ends.
type presenceHook struct {
	eng *engine.Engine

	mu       sync.Mutex
	presence *engine.Presence
	closed   bool
}

func (h *presenceHook) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		// Protocol >= 2026-07-28 has no initialize: each request carries
		// the client's identity in _meta, so the first request starts the
		// record. Older clients are handled after initialize below.
		if method != "initialize" {
			if info := requestClientInfo(req); info != nil {
				h.start(ctx, req, info)
			}
		}
		if method == "tools/call" {
			if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && p != nil {
				h.current().ToolCalled(ctx, p.Name)
			}
		}
		res, err := next(ctx, method, req)
		if method == "initialize" && err == nil {
			if ss, ok := req.GetSession().(*mcp.ServerSession); ok {
				if p := ss.InitializeParams(); p != nil && p.ClientInfo != nil {
					h.start(ctx, req, p.ClientInfo)
				}
			}
		}
		return res, err
	}
}

// requestClientInfo is the calling client's identity as the SDK resolves
// it for a request (per-request _meta, else the session's initialize).
func requestClientInfo(req mcp.Request) *mcp.Implementation {
	if r, ok := req.(interface{ ClientInfo() *mcp.Implementation }); ok {
		return r.ClientInfo()
	}
	return nil
}

func (h *presenceHook) current() *engine.Presence {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.presence
}

func (h *presenceHook) start(ctx context.Context, req mcp.Request, info *mcp.Implementation) {
	h.mu.Lock()
	if h.presence != nil || h.closed {
		h.mu.Unlock()
		return
	}
	h.presence = h.eng.StartPresence(context.WithoutCancel(ctx), engine.ClientInfo{Name: info.Name, Version: info.Version})
	h.mu.Unlock()
	if ss, ok := req.GetSession().(*mcp.ServerSession); ok {
		go func() {
			_ = ss.Wait()
			h.close(ctx)
		}()
	}
}

func (h *presenceHook) close(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	h.presence.Close(context.WithoutCancel(ctx))
}

const serverInstructions = `wspace manages multi-repository workspaces: a workspace is a directory holding one git worktree per project, all on one branch. ` +
	`Projects (git main clones) are registered per context (a named set of roots and defaults); omit "context" to use the active one. ` +
	`Read before you write: list_contexts, list_projects, list_workspaces, workspace_status and teardown_check change nothing. ` +
	`Destructive tools (destroy_workspace, remove_project, delete_context, unregister_project, sync_env, repo_stash_drop, repo_delete_untracked, repo_discard) require confirm=true; with confirm=false they change nothing and return code "needs_confirmation" previewing every reason. ` +
	`They never force by themselves: when work would be lost they fail with "needs_confirmation" listing every reason. ` +
	`Show those reasons to the user and only retry with force=true after the user explicitly agrees to lose that work. ` +
	`update_repo and update_workspace bring repos up to date with their base branch; they refuse rather than risk uncommitted work (autostash=true opts in to stashing) and abort any conflict, leaving the repo as it was. ` +
	`To inspect one repo use repo_branch_info, repo_changes, repo_diff, repo_commits, repo_commit, repo_stashes and repo_stash (read-only); repo_fetch only updates remote-tracking refs, and repo_pull_ff only fast-forwards (never a merge commit or a rebase). ` +
	`repo_stash_apply and repo_stash_pop never discard work: on a conflict the stash entry is kept and the worktree is left with conflict markers for the user to resolve. ` +
	`repo_stage, repo_unstage, repo_discard, repo_commit_changes, repo_push and repo_stash_create change the repo: commit and push only when the user asked, with a message the user approved; never set a git identity, skip hooks or force-push. ` +
	`Use list_addable_projects before add_project to offer only projects not already in the workspace.`

// toolError is a tool-level failure whose text content is the engine's
// JSON error body, so an agent can both read it and parse code/data.
type toolError struct{ body engine.ErrorBody }

func (e *toolError) Error() string {
	b, err := json.Marshal(e.body)
	if err != nil {
		return e.body.Message
	}
	return string(b)
}

func asToolError(err error) error {
	if err == nil {
		return nil
	}
	if te, ok := err.(*toolError); ok {
		return te
	}
	return &toolError{body: engine.AsError(err).Body()}
}

// done builds a tool's success: a concise text summary for the agent and
// the typed value as structured content.
func done[Out any](out Out, err error, summary func(Out) string) (*mcp.CallToolResult, Out, error) {
	if err != nil {
		var zero Out
		return nil, zero, asToolError(err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: summary(out)}}}, out, nil
}

func fail[Out any](err error) (*mcp.CallToolResult, Out, error) {
	var zero Out
	return nil, zero, asToolError(err)
}

func boolPtr(b bool) *bool { return &b }

// readOnly annotates a tool that only reads local state.
func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: boolPtr(false)}
}

// mutating annotates a tool that changes local state.
func mutating(title string, destructive, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: boolPtr(destructive), IdempotentHint: idempotent, OpenWorldHint: boolPtr(false)}
}

// confirmRequired is a destructive tool's refusal when confirm is not
// true: needs_confirmation carrying a dry-run preview of the reasons an
// unforced run would report (possibly none) plus any extra detail.
func confirmRequired(reasons []engine.Blocker, extra map[string]any) error {
	if reasons == nil {
		reasons = []engine.Blocker{}
	}
	data := map[string]any{"reasons": reasons}
	for k, v := range extra {
		data[k] = v
	}
	return &toolError{body: engine.ErrorBody{
		Code:    engine.CodeNeedsConfirmation,
		Message: messages.T(messages.EngineConfirmRequired),
		Data:    data,
	}}
}

// progressFor returns an engine.ProgressFunc that forwards each progress
// event as an MCP progress notification when the caller sent a progress
// token, and nil (events discarded) otherwise.
func progressFor(ctx context.Context, req *mcp.CallToolRequest) engine.ProgressFunc {
	if req == nil || req.Params == nil || req.Session == nil {
		return nil
	}
	token := req.Params.GetProgressToken()
	if token == nil {
		return nil
	}
	var n float64
	return func(ev engine.ProgressEvent) {
		n++
		_ = req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: token,
			Progress:      n,
			Message:       progressMessage(ev),
		})
	}
}

func progressMessage(ev engine.ProgressEvent) string {
	if ev.Kind != engine.KindRepo {
		return ev.Message
	}
	msg := fmt.Sprintf("%s: %s %s", ev.Repo, ev.Op, ev.Phase)
	if ev.Error != "" {
		msg += " (" + ev.Error + ")"
	}
	return msg
}

// plural renders "1 repo" / "2 repos".
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
