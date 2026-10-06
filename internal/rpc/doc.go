// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package rpc is the `wspace rpc` server: JSON-lines over stdio, one
// request per line, driving internal/engine (docs/rpc-contract.md is the
// normative contract). Requests are processed strictly
// sequentially, in arrival order; a long operation's progress events are
// written before its final response. Only protocol lines are ever written
// to the output stream.
//
// It imports internal/engine and internal/messages only — never app,
// ports or an adapter (enforced by internal/archtest).
package rpc
