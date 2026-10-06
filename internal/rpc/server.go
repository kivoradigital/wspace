// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/messages"
)

// maxLineBytes bounds one inbound request line.
const maxLineBytes = 16 << 20

// Server serves the JSON-lines protocol over one engine.
type Server struct {
	eng           *engine.Engine
	engineVersion string
	methods       map[string]handler
}

// NewServer returns a Server over eng; engineVersion is what rpc.hello
// reports as engineVersion.
func NewServer(eng *engine.Engine, engineVersion string) *Server {
	s := &Server{eng: eng, engineVersion: engineVersion}
	s.registerMethods()
	return s
}

func (s *Server) methodNames() []string {
	names := make([]string, 0, len(s.methods))
	for n := range s.methods {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Serve reads requests from in until EOF (returning nil) or until ctx is
// cancelled (returning ctx.Err()), answering each one on out before
// reading the next. Blank lines are ignored. Only a failure to read in or
// write out is returned as an error; every request-level problem becomes
// an error response.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	w := bufio.NewWriter(out)
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("rpc: read: %w", err)
			}
			return nil
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := s.handleLine(ctx, line, w); err != nil {
			return err
		}
	}
}

// handleLine answers one request line, flushing every line it writes so a
// reader sees progress as it happens.
func (s *Server) handleLine(ctx context.Context, line []byte, w *bufio.Writer) error {
	var writeErr error
	write := func(v any) {
		if writeErr != nil {
			return
		}
		b, err := json.Marshal(v)
		if err != nil {
			writeErr = fmt.Errorf("rpc: encode: %w", err)
			return
		}
		b = append(b, '\n')
		if _, err := w.Write(b); err != nil {
			writeErr = fmt.Errorf("rpc: write: %w", err)
			return
		}
		if err := w.Flush(); err != nil {
			writeErr = fmt.Errorf("rpc: write: %w", err)
		}
	}

	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		write(errorResponse(nil, invalidRequest(err.Error())))
		return writeErr
	}
	id, ok := parseID(req.ID)
	if !ok {
		write(errorResponse(nil, invalidRequest("id must be a non-empty string")))
		return writeErr
	}
	if req.Method == "" {
		write(errorResponse(&id, invalidRequest("method is required")))
		return writeErr
	}
	h, found := s.methods[req.Method]
	if !found {
		write(errorResponse(&id, &engine.Error{Code: engine.CodeMethodNotFound, Message: messages.T(messages.RPCMethodNotFound, req.Method)}))
		return writeErr
	}

	params := req.Params
	if len(params) == 0 || string(params) == "null" {
		params = json.RawMessage("{}")
	}
	emit := func(ev engine.ProgressEvent) { write(event{Event: "progress", ID: id, Data: ev}) }

	result, err := h(ctx, params, emit)
	if err != nil {
		write(errorResponse(&id, engine.AsError(err)))
		return writeErr
	}
	raw, err := encodeResult(result)
	if err != nil {
		write(errorResponse(&id, engine.AsError(err)))
		return writeErr
	}
	write(response{ID: &id, Result: raw})
	return writeErr
}

func parseID(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil || id == "" {
		return "", false
	}
	return id, true
}

func invalidRequest(reason string) *engine.Error {
	return &engine.Error{Code: engine.CodeInvalidRequest, Message: messages.T(messages.RPCInvalidRequest, reason)}
}

func errorResponse(id *string, err error) response {
	var e *engine.Error
	if !errors.As(err, &e) {
		e = engine.AsError(err)
	}
	body := e.Body()
	return response{ID: id, Error: &body}
}
