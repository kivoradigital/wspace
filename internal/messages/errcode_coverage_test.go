// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package messages_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/messages"
)

// TestEveryErrCodeHasAMessage reads every ErrCode constant declared in the
// domain package and requires a catalog key for it: an unmapped code renders
// as "an unexpected error occurred", which tells the user nothing.
func TestEveryErrCodeHasAMessage(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "../domain/errors.go", nil, 0)
	if err != nil {
		t.Fatalf("parse domain/errors.go: %v", err)
	}
	var codes []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if ident, ok := spec.Type.(*ast.Ident); !ok || ident.Name != "ErrCode" {
			return true
		}
		for _, v := range spec.Values {
			if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				codes = append(codes, lit.Value[1:len(lit.Value)-1])
			}
		}
		return true
	})
	if len(codes) < 10 {
		t.Fatalf("found only %d ErrCode constants; the parser no longer matches errors.go", len(codes))
	}
	for _, c := range codes {
		if messages.ForCode(domain.ErrCode(c)) == messages.ErrUnknown {
			t.Errorf("ErrCode %q has no message: it would render as %q", c, messages.T(messages.ErrUnknown))
		}
	}
}
