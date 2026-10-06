// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package rpc

import (
	"encoding/json"

	"github.com/kivoradigital/wspace/internal/engine"
)

// ProtocolVersion is the wire schema version rpc.hello reports. Additive
// changes (new methods, new optional params, new result fields) keep it;
// any breaking change increments it.
const ProtocolVersion = 1

// request is one inbound line. ID stays raw so a non-string id can be
// rejected as invalid_request instead of being silently coerced.
type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// response is one final answer. ID is a pointer so a request whose id
// could not be read is answered with "id":null.
type response struct {
	ID     *string           `json:"id"`
	Result json.RawMessage   `json:"result,omitempty"`
	Error  *engine.ErrorBody `json:"error,omitempty"`
}

// event is one server-initiated notification tied to a request id.
type event struct {
	Event string               `json:"event"`
	ID    string               `json:"id"`
	Data  engine.ProgressEvent `json:"data"`
}

// helloResult is rpc.hello's result.
type helloResult struct {
	ProtocolVersion int      `json:"protocolVersion"`
	EngineVersion   string   `json:"engineVersion"`
	Methods         []string `json:"methods"`
}
