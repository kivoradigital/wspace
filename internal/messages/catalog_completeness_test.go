// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package messages_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/kivoradigital/wspace/internal/messages"
)

// declaredKeys parses keys.go with go/ast and returns every declared Key
// constant's string value (design.md §10: "TestCatalogCompleteness iterates
// every declared Key constant (discovered by parsing keys.go with go/ast)
// against every registered locale").
func declaredKeys(t *testing.T) []messages.Key {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller for keys.go discovery")
	}
	keysPath := filepath.Join(filepath.Dir(thisFile), "keys.go")

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, keysPath, nil, 0)
	if err != nil {
		t.Fatalf("parse keys.go: %v", err)
	}

	var keys []messages.Key
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, val := range vs.Values {
				lit, ok := val.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				unquoted, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote %q: %v", lit.Value, err)
				}
				keys = append(keys, messages.Key(unquoted))
			}
		}
	}
	if len(keys) == 0 {
		t.Fatal("declaredKeys: found zero Key constants in keys.go — parser or path is wrong")
	}
	return keys
}

// TestCatalogCompleteness covers tasks.md 4b.1: every declared Key must
// have an "en" catalog entry, and "en" must carry no orphan entry beyond
// the declared set.
func TestCatalogCompleteness(t *testing.T) {
	if err := messages.Use("en"); err != nil {
		t.Fatalf("Use(en): %v", err)
	}

	declared := declaredKeys(t)
	declaredSet := make(map[messages.Key]bool, len(declared))
	for _, k := range declared {
		declaredSet[k] = true
	}

	entries := messages.EnglishEntriesForTest()

	for _, k := range declared {
		if _, ok := entries[k]; !ok {
			t.Errorf("declared key %q has no \"en\" catalog entry", k)
		}
	}
	for k := range entries {
		if !declaredSet[k] {
			t.Errorf("\"en\" catalog entry %q is not a declared Key constant (orphan)", k)
		}
	}
}
