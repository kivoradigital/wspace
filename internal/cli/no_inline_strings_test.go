// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// printLikeFuncs are the fmt functions this check watches: any of these
// reaching a raw English string literal (as opposed to a messages.T(...)
// call result or a pure punctuation/format-verb template) is exactly the
// R8 defect design.md §10 describes.
var printLikeFuncs = map[string]bool{
	"Fprintln": true, "Fprint": true, "Fprintf": true,
	"Println": true, "Print": true, "Printf": true,
	"Sprintf": true, "Sprintln": true, "Sprint": true,
}

// verbPattern strips fmt verbs (%s, %d, %[1]s, %v, %%, ...) so what
// remains of a literal can be checked for leftover prose. A literal made
// of nothing but verbs and punctuation (e.g. "%s: %s", "%s (%s)") is
// template plumbing, not a user-facing string of its own — the actual
// words always come from a messages.T(...) argument feeding that verb.
var verbPattern = regexp.MustCompile(`%\[\d+\][a-zA-Z%]|%[-+ 0#]*\d*\.?\d*[a-zA-Z%]`)

// prosePattern matches any letter, which is enough to detect an English
// word once verbs have been stripped.
var prosePattern = regexp.MustCompile(`[A-Za-z]`)

func isProseLiteral(s string) bool {
	return prosePattern.MatchString(verbPattern.ReplaceAllString(s, ""))
}

// TestNoInlineUserStrings covers tasks.md 4b.3 (R8): every internal/cli
// production file must route user-facing prose through messages.T; a
// literal string reaching a print-like call is a defect. Test files and
// pure punctuation/format templates are exempt (design.md §10: "Test
// files and struct tags are exempt").
func TestNoInlineUserStrings(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !printLikeFuncs[sel.Sel.Name] {
				return true
			}

			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				unquoted := strings.Trim(lit.Value, "`\"")
				if isProseLiteral(unquoted) {
					pos := fset.Position(lit.Pos())
					t.Errorf("%s:%d: inline literal %q reaches %s(...) — route it through messages.T instead",
						pos.Filename, pos.Line, unquoted, sel.Sel.Name)
				}
			}
			return true
		})
	}
}
