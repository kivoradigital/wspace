// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package domain

// overlayForbiddenKeys are the top-level keys an overlay document must
// never contain (ADR D7: "Overlay can never select a context"). A local
// .ws.yaml may override option values only; it may never name, define or
// switch a context.
var overlayForbiddenKeys = map[string]bool{
	"context":  true,
	"contexts": true,
}

// ValidateOverlayScope rejects a raw overlay document (identified by its
// top-level key names) that declares a context name or context-scoped
// fields, per ADR D7. It is a pure function so the configstore adapter can
// run it before ever attempting a strict struct decode, giving a specific
// CodeOverlayScope error instead of a generic "unknown field" decode
// failure that would look identical to an ordinary typo.
func ValidateOverlayScope(keys []string) error {
	for _, k := range keys {
		if overlayForbiddenKeys[k] {
			return NewOpError("config.load_overlay", CodeOverlayScope, k, "", nil)
		}
	}
	return nil
}
