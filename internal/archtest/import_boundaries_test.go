// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Package archtest enforces the import boundaries between the hexagon's
// layers (design.md §1, rules R1-R7, plus R8-R10 for internal/engine,
// internal/rpc and internal/mcpserver) as a test rather than a convention.
package archtest

import (
	"bufio"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// requiredPackages are the boundary directories that must exist for the
// package graph to be legal. TestImportBoundaries fails loudly while any of
// them is missing.
var requiredPackages = []string{
	"internal/domain",
	"internal/ports",
	"internal/messages",
	"internal/adapters",
	"internal/app",
	"internal/cli",
	"internal/engine",
	"internal/rpc",
	"internal/mcpserver",
	"cmd/wspace",
}

// skipDirNames are directories TestImportBoundaries never descends into.
var skipDirNames = map[string]bool{
	".git":     true,
	"ws_old":   true,
	"dist":     true,
	"build":    true,
	"bin":      true,
	"testdata": true,
	"vendor":   true,
}

func TestImportBoundaries(t *testing.T) {
	root := moduleRoot(t)
	modulePath := readModulePath(t, root)

	for _, dir := range requiredPackages {
		abs := filepath.Join(root, filepath.FromSlash(dir))
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			t.Fatalf("required boundary package %q does not exist yet: %v", dir, err)
		}
		if !hasGoFiles(abs) {
			t.Fatalf("required boundary package %q has no .go files", dir)
		}
	}

	for _, pkg := range collectPackages(t, root) {
		for _, imp := range pkg.imports {
			if violation := checkEdge(pkg.path, imp, modulePath); violation != "" {
				t.Errorf("import boundary violation: package %q imports %q: %s", pkg.path, imp, violation)
			}
		}
		for _, imp := range pkg.testImports {
			if violation := checkTestEdge(pkg.path, imp, modulePath); violation != "" {
				t.Errorf("import boundary violation (test file): package %q imports %q: %s", pkg.path, imp, violation)
			}
		}
	}
}

type pkgInfo struct {
	path        string // package path relative to the module root, e.g. "internal/domain"
	imports     []string
	testImports []string // imports that only ever appear in a _test.go file in this directory
}

// moduleRoot locates the repository root by walking up from this file's own
// directory until go.mod is found.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("archtest: cannot determine caller for module root discovery")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("archtest: go.mod not found above internal/archtest")
		}
		dir = parent
	}
}

func readModulePath(t *testing.T, root string) string {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("archtest: cannot open go.mod: %v", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	t.Fatal("archtest: module directive not found in go.mod")
	return ""
}

func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

// collectPackages walks internal/ and cmd/ under root and parses the import
// list of every .go file it finds, separating _test.go imports from
// production-file imports (see checkTestEdge for why the two get different
// rules).
func collectPackages(t *testing.T, root string) []pkgInfo {
	t.Helper()
	byDir := map[string][]string{}
	testByDir := map[string][]string{}

	for _, top := range []string{"internal", "cmd"} {
		topDir := filepath.Join(root, top)
		if _, err := os.Stat(topDir); err != nil {
			continue
		}
		err := filepath.WalkDir(topDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirNames[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			dir := filepath.Dir(path)
			imports, perr := parseImports(path)
			if perr != nil {
				t.Fatalf("archtest: failed to parse %q: %v", path, perr)
			}
			rel, rerr := filepath.Rel(root, dir)
			if rerr != nil {
				t.Fatalf("archtest: failed to relativize %q: %v", dir, rerr)
			}
			pkgPath := filepath.ToSlash(rel)
			if strings.HasSuffix(path, "_test.go") {
				testByDir[pkgPath] = append(testByDir[pkgPath], imports...)
			} else {
				byDir[pkgPath] = append(byDir[pkgPath], imports...)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("archtest: walk of %q failed: %v", top, err)
		}
	}

	dirs := map[string]bool{}
	for path := range byDir {
		dirs[path] = true
	}
	for path := range testByDir {
		dirs[path] = true
	}

	var out []pkgInfo
	for path := range dirs {
		out = append(out, pkgInfo{path: path, imports: dedupe(byDir[path]), testImports: dedupe(testByDir[path])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

func parseImports(path string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var imports []string
	for _, imp := range f.Imports {
		imports = append(imports, strings.Trim(imp.Path.Value, `"`))
	}
	return imports, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// deniedDomainStdlib is the stdlib I/O subset R1/R2 forbid even though a leaf
// package otherwise may use the standard library freely.
var deniedDomainStdlib = []string{"os", "os/exec", "io/fs"}

func isDeniedDomainStdlib(imp string) bool {
	for _, d := range deniedDomainStdlib {
		if imp == d {
			return true
		}
	}
	return imp == "net" || strings.HasPrefix(imp, "net/")
}

// isStdlib reports whether imp is a standard-library import path: its first
// path segment never contains a dot for stdlib, unlike third-party module
// paths (e.g. "golang.org/x/...") or this module's own internal packages.
func isStdlib(imp string) bool {
	first := imp
	if i := strings.Index(imp, "/"); i >= 0 {
		first = imp[:i]
	}
	return !strings.Contains(first, ".")
}

func isAllowedInternal(internalImp string, allowed ...string) bool {
	for _, a := range allowed {
		if internalImp == a || strings.HasPrefix(internalImp, a+"/") {
			return true
		}
	}
	return false
}

// isAllowedThirdParty reports whether imp is one of the allowed module
// paths or a package inside one of them.
func isAllowedThirdParty(imp string, modules ...string) bool {
	for _, m := range modules {
		if imp == m || strings.HasPrefix(imp, m+"/") {
			return true
		}
	}
	return false
}

// adapterFamily returns the top-level adapter directory a package belongs
// to, e.g. "internal/adapters/git/gitfix" -> "internal/adapters/git".
func adapterFamily(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) >= 3 {
		return strings.Join(parts[:3], "/")
	}
	return p
}

// checkEdge returns a human-readable violation reason, or "" if the edge
// from pkg to imp is allowed under rules R1-R7 (design.md §1).
func checkEdge(pkg, imp, modulePath string) string {
	isInternalImp := strings.HasPrefix(imp, modulePath+"/")
	internalImp := strings.TrimPrefix(imp, modulePath+"/")
	isThirdParty := !isStdlib(imp) && !isInternalImp

	// An external test package (e.g. "package domain_test" in
	// internal/domain) legitimately imports the very package it tests. That
	// is not a layering violation under any rule.
	if isInternalImp && internalImp == pkg {
		return ""
	}

	switch {
	case pkg == "internal/archtest" || strings.HasPrefix(pkg, "internal/archtest/"):
		// The boundary test itself is not part of the hexagon.
		return ""

	case pkg == "internal/domain" || strings.HasPrefix(pkg, "internal/domain/"):
		if isInternalImp {
			return "R1: internal/domain must not import any other internal package"
		}
		if isThirdParty {
			return "R1: internal/domain must not import third-party packages"
		}
		if isDeniedDomainStdlib(imp) {
			return "R1: internal/domain must not import stdlib I/O packages"
		}
		return ""

	case pkg == "internal/messages" || strings.HasPrefix(pkg, "internal/messages/") ||
		pkg == "internal/buildinfo" || strings.HasPrefix(pkg, "internal/buildinfo/"):
		if isThirdParty {
			return "R2: leaf package must not import third-party packages"
		}
		if isInternalImp && !isAllowedInternal(internalImp, "internal/domain") {
			return "R2: leaf package may only import internal/domain"
		}
		if isDeniedDomainStdlib(imp) {
			return "R2: leaf package must not import stdlib I/O packages"
		}
		return ""

	case pkg == "internal/ports/portstest" || strings.HasPrefix(pkg, "internal/ports/portstest/"):
		if isThirdParty {
			return "internal/ports/portstest must not import third-party packages"
		}
		if isInternalImp && !isAllowedInternal(internalImp, "internal/domain", "internal/ports", "internal/messages") {
			return "internal/ports/portstest may only import domain, ports and messages"
		}
		return ""

	case pkg == "internal/ports" || strings.HasPrefix(pkg, "internal/ports/"):
		if isThirdParty {
			return "R3: internal/ports must not import third-party packages"
		}
		if isInternalImp && !isAllowedInternal(internalImp, "internal/domain", "internal/messages") {
			return "R3: internal/ports may only import domain and messages"
		}
		return ""

	case pkg == "internal/app" || strings.HasPrefix(pkg, "internal/app/"):
		if isInternalImp && !isAllowedInternal(internalImp, "internal/domain", "internal/ports", "internal/messages") {
			return "R4: internal/app must not import an adapter or cli package"
		}
		return ""

	case strings.HasPrefix(pkg, "internal/adapters/"):
		if isInternalImp {
			if isAllowedInternal(internalImp, "internal/domain", "internal/ports", "internal/messages") {
				return ""
			}
			if strings.HasPrefix(internalImp, "internal/adapters/") {
				if adapterFamily(pkg) != adapterFamily(internalImp) {
					return "R5: adapters must not import a different adapter package"
				}
				return ""
			}
			return "R5: adapters must not import app or cli"
		}
		return ""

	case pkg == "internal/cli" || strings.HasPrefix(pkg, "internal/cli/"):
		if isInternalImp && !isAllowedInternal(internalImp, "internal/app", "internal/domain", "internal/messages") {
			return "R6: internal/cli may only import app, domain and messages"
		}
		return ""

	case pkg == "internal/engine" || strings.HasPrefix(pkg, "internal/engine/"):
		// R8: the non-interactive facade sits at the application layer's
		// edge: it may drive app (and name the ports app is written
		// against, to accept them as dependencies), never an adapter or a
		// driving surface.
		if isThirdParty {
			return "R8: internal/engine must not import third-party packages"
		}
		if isInternalImp && !isAllowedInternal(internalImp, "internal/app", "internal/domain", "internal/ports", "internal/messages") {
			return "R8: internal/engine may only import app, domain, ports and messages"
		}
		return ""

	case pkg == "internal/rpc" || strings.HasPrefix(pkg, "internal/rpc/"):
		// R9: the JSON-lines server is a pure driving adapter over the
		// engine facade.
		if isThirdParty {
			return "R9: internal/rpc must not import third-party packages"
		}
		if isInternalImp && !isAllowedInternal(internalImp, "internal/engine", "internal/messages") {
			return "R9: internal/rpc may only import engine and messages"
		}
		return ""

	case pkg == "internal/mcpserver" || strings.HasPrefix(pkg, "internal/mcpserver/"):
		// R10: the MCP server is a driving adapter over the engine facade;
		// its only third-party dependency is the official MCP Go SDK.
		if isThirdParty && !isAllowedThirdParty(imp, "github.com/modelcontextprotocol/go-sdk", "github.com/google/jsonschema-go") {
			return "R10: internal/mcpserver may only import the official MCP Go SDK as a third-party package"
		}
		if isInternalImp && !isAllowedInternal(internalImp, "internal/engine", "internal/messages") {
			return "R10: internal/mcpserver may only import engine and messages"
		}
		return ""

	case strings.HasPrefix(pkg, "cmd/"):
		// R7: cmd/* is the composition root and may import anything.
		return ""

	default:
		return ""
	}
}

// checkTestEdge is checkEdge's counterpart for imports that appear only in
// a _test.go file. Production-file edges (checked by checkEdge) are R1-R7
// unchanged. _test.go files never ship in a built binary, so they get one
// narrow, additional allowance beyond checkEdge: internal/cli's own
// tests may reach internal/ports/portstest (the shared
// fakes design.md §12 requires for every layer's tests), internal/ports
// itself, and any internal/adapters/* package (design.md §12's CLI testing
// strategy exercises a real adapter — e.g. the terminal prompter — from a
// golden test, not only fakes). This mirrors phase 1's own "an external
// test package may import the package it tests" carve-out: both exist
// because a _test.go file is test infrastructure, not part of the
// production import graph R1-R7 governs.
//
// internal/ports itself is part of the same allowance: a test that drives
// a ports.Prompter-typed variable or builds ports.TextField/ConfirmField/
// ChoiceField literals cannot avoid naming the interface it satisfies.
func checkTestEdge(pkg, imp, modulePath string) string {
	violation := checkEdge(pkg, imp, modulePath)
	if violation == "" {
		return ""
	}

	internalImp := strings.TrimPrefix(imp, modulePath+"/")

	// The machine-facing surfaces' own tests build a real engine over the
	// shared portstest fakes, which needs ports, portstest and domain
	// values — test infrastructure, never shipped.
	isSurface := pkg == "internal/rpc" || strings.HasPrefix(pkg, "internal/rpc/") ||
		pkg == "internal/mcpserver" || strings.HasPrefix(pkg, "internal/mcpserver/")
	if isSurface && isAllowedInternal(internalImp, "internal/ports", "internal/domain") {
		return ""
	}

	isCli := pkg == "internal/cli" || strings.HasPrefix(pkg, "internal/cli/")
	if !isCli {
		return violation
	}

	if isAllowedInternal(internalImp, "internal/ports", "internal/adapters") {
		return ""
	}
	return violation
}
