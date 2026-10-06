// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package fsstore_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/kivoradigital/wspace/internal/adapters/fsstore"
	"github.com/kivoradigital/wspace/internal/domain"
)

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, rel := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %q: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %q: %v", rel, err)
		}
	}
}

// TestFSAdapter_FindEnvFiles_HonorsPruneDirs is a table test covering
// environment-files spec's discovery requirement: finds ".env"/".env.*",
// excludes template-like suffixes, and prunes configured directories
// (tasks.md 2.26).
func TestFSAdapter_FindEnvFiles_HonorsPruneDirs(t *testing.T) {
	tests := []struct {
		name      string
		files     []string
		pruneDirs []string
		want      []string
	}{
		{
			name:  "discovers plain and suffixed env files",
			files: []string{".env", ".env.local", "README.md"},
			want:  []string{".env", ".env.local"},
		},
		{
			name:  "excludes template-like files",
			files: []string{".env.example", ".env.template", ".env.sample", ".env.dist", ".env"},
			want:  []string{".env"},
		},
		{
			name:      "prunes configured directories",
			files:     []string{".env", "node_modules/pkg/.env", "services/worker/.env.local"},
			pruneDirs: []string{"node_modules"},
			want:      []string{".env", "services/worker/.env.local"},
		},
		{
			name:  "nested env file preserves relative path",
			files: []string{"config/.env.local"},
			want:  []string{"config/.env.local"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, tt.files...)

			fs := fsstore.New()
			got, err := fs.FindEnvFiles(domain.Path(filepath.ToSlash(root)), tt.pruneDirs)
			if err != nil {
				t.Fatalf("FindEnvFiles: %v", err)
			}
			sort.Strings(got)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)

			if len(got) != len(want) {
				t.Fatalf("expected %v, got %v", want, got)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("expected %v, got %v", want, got)
				}
			}
		})
	}
}

// gitDir marks dir (relative to root) as a main clone: a real directory
// whose ".git" entry is itself a directory.
func gitDir(t *testing.T, root, dir string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(dir), ".git")
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("mkdir %q/.git: %v", dir, err)
	}
}

// gitWorktreeFile marks dir (relative to root) as a linked worktree: a real
// directory whose ".git" entry is a plain file, exactly the shape a real
// `git worktree add` leaves behind.
func gitWorktreeFile(t *testing.T, root, dir string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(dir))
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(full, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatalf("write %q/.git: %v", dir, err)
	}
}

// TestFSAdapter_WalkGitRepos_FindsNestedRepositoriesAtAnyDepth covers this
// change's own recursive-discovery fix: a real, nested fixture tree in
// t.TempDir() with a main clone one level below root, one three levels
// below, a linked worktree that must be reported but never descended into,
// and a plain container directory in between that must itself be walked
// into (never treated as a candidate) for the recursion to ever reach what
// is nested under it.
func TestFSAdapter_WalkGitRepos_FindsNestedRepositoriesAtAnyDepth(t *testing.T) {
	root := t.TempDir()
	gitDir(t, root, "api")                  // depth 1
	gitDir(t, root, "org/team/service")     // depth 3, nested under two plain container dirs
	gitWorktreeFile(t, root, "linked-work") // depth 1, linked worktree

	fs := fsstore.New()
	got, err := fs.WalkGitRepos(domain.Path(filepath.ToSlash(root)), 5, nil)
	if err != nil {
		t.Fatalf("WalkGitRepos: %v", err)
	}
	if got.Truncated {
		t.Fatal("Truncated = true, want false (depth cap of 5 is not reached by a depth-3 fixture)")
	}

	wantMain := []string{
		filepath.ToSlash(filepath.Join(root, "api")),
		filepath.ToSlash(filepath.Join(root, "org/team/service")),
	}
	gotMain := pathsToStrings(got.MainClones)
	sort.Strings(gotMain)
	sort.Strings(wantMain)
	if !equalStrings(gotMain, wantMain) {
		t.Fatalf("MainClones = %v, want %v", gotMain, wantMain)
	}

	wantWorktrees := []string{filepath.ToSlash(filepath.Join(root, "linked-work"))}
	gotWorktrees := pathsToStrings(got.LinkedWorktrees)
	if !equalStrings(gotWorktrees, wantWorktrees) {
		t.Fatalf("LinkedWorktrees = %v, want %v", gotWorktrees, wantWorktrees)
	}
}

// TestFSAdapter_WalkGitRepos_PrunesBeforeDescending proves pruning happens
// before the recursive descent, not as a filter over the final result: a
// "node_modules"-shaped directory pruned by name never has its own contents
// inspected at all, even though a genuine (fake, for this test) main clone
// sits directly inside it — if the walk pruned only the final list, that
// nested main clone would still be found and then filtered out; if it
// prunes before descending, it is never found in the first place. Both
// would look identical from the result alone if this test only asserted
// the directory named "node_modules" itself is absent (it can never be
// present anyway, since it is a plain directory, not a repository) — so
// this test's fixture also has genuinely large synthetic content:
// asserting the nested main clone is absent from MainClones is what a
// filter-after-the-fact implementation would fail, since it would still
// have listed and stat'd its way down into it.
func TestFSAdapter_WalkGitRepos_PrunesBeforeDescending(t *testing.T) {
	root := t.TempDir()
	gitDir(t, root, "api")
	gitDir(t, root, "node_modules/some-pkg") // would be a main clone if ever visited

	fs := fsstore.New()
	prune := func(name string) bool { return name == "node_modules" }
	got, err := fs.WalkGitRepos(domain.Path(filepath.ToSlash(root)), 5, prune)
	if err != nil {
		t.Fatalf("WalkGitRepos: %v", err)
	}

	want := []string{filepath.ToSlash(filepath.Join(root, "api"))}
	gotMain := pathsToStrings(got.MainClones)
	if !equalStrings(gotMain, want) {
		t.Fatalf("MainClones = %v, want %v (the pruned node_modules subtree must never be visited)", gotMain, want)
	}
}

// TestFSAdapter_WalkGitRepos_SkipsHiddenDirectories covers the other
// always-on prune rule: a leading-dot directory is skipped regardless of
// what prune itself says (nil here, so nothing else is pruned at all).
func TestFSAdapter_WalkGitRepos_SkipsHiddenDirectories(t *testing.T) {
	root := t.TempDir()
	gitDir(t, root, "api")
	gitDir(t, root, ".hidden/nested-repo")

	fs := fsstore.New()
	got, err := fs.WalkGitRepos(domain.Path(filepath.ToSlash(root)), 5, nil)
	if err != nil {
		t.Fatalf("WalkGitRepos: %v", err)
	}

	want := []string{filepath.ToSlash(filepath.Join(root, "api"))}
	gotMain := pathsToStrings(got.MainClones)
	if !equalStrings(gotMain, want) {
		t.Fatalf("MainClones = %v, want %v (a hidden directory must never be descended into)", gotMain, want)
	}
}

// TestFSAdapter_WalkGitRepos_ReportsTruncationHonestly covers "say so
// rather than silently presenting a partial list as complete": a maxDepth
// of 1 stops before reaching a repository at depth 2, and Truncated must
// report that directories were left unexplored.
func TestFSAdapter_WalkGitRepos_ReportsTruncationHonestly(t *testing.T) {
	root := t.TempDir()
	gitDir(t, root, "org/service") // depth 2

	fs := fsstore.New()
	got, err := fs.WalkGitRepos(domain.Path(filepath.ToSlash(root)), 1, nil)
	if err != nil {
		t.Fatalf("WalkGitRepos: %v", err)
	}
	if len(got.MainClones) != 0 {
		t.Fatalf("MainClones = %v, want none (the repo is past the depth cap)", got.MainClones)
	}
	if !got.Truncated {
		t.Fatal("Truncated = false, want true (maxDepth=1 was reached with \"org\" left unexplored)")
	}
	want := []string{filepath.ToSlash(filepath.Join(root, "org"))}
	if gotDirs := pathsToStrings(got.TruncatedDirs); !equalStrings(gotDirs, want) {
		t.Fatalf("TruncatedDirs = %v, want %v (the folder whose subfolders were not searched)", gotDirs, want)
	}
}

// TestFSAdapter_WalkGitRepos_PrunedChildrenDoNotTruncate: a folder at the
// cap whose only subfolders are ignored or hidden is not a partial scan.
func TestFSAdapter_WalkGitRepos_PrunedChildrenDoNotTruncate(t *testing.T) {
	root := t.TempDir()
	gitDir(t, root, "org/node_modules/pkg")
	gitDir(t, root, "org/.cache/x")

	fs := fsstore.New()
	got, err := fs.WalkGitRepos(domain.Path(filepath.ToSlash(root)), 1, func(name string) bool { return name == "node_modules" })
	if err != nil {
		t.Fatalf("WalkGitRepos: %v", err)
	}
	if got.Truncated || len(got.TruncatedDirs) != 0 {
		t.Fatalf("Truncated = %v, TruncatedDirs = %v, want neither", got.Truncated, got.TruncatedDirs)
	}
}

// TestFSAdapter_WalkGitRepos_UnlimitedDepthNeverTruncates covers maxDepth
// <= 0 meaning "unlimited": a deeply nested repository is still found, and
// Truncated stays false.
func TestFSAdapter_WalkGitRepos_UnlimitedDepthNeverTruncates(t *testing.T) {
	root := t.TempDir()
	gitDir(t, root, "a/b/c/d/e/deep")

	fs := fsstore.New()
	got, err := fs.WalkGitRepos(domain.Path(filepath.ToSlash(root)), 0, nil)
	if err != nil {
		t.Fatalf("WalkGitRepos: %v", err)
	}
	if got.Truncated {
		t.Fatal("Truncated = true, want false (maxDepth <= 0 means unlimited)")
	}
	want := []string{filepath.ToSlash(filepath.Join(root, "a/b/c/d/e/deep"))}
	gotMain := pathsToStrings(got.MainClones)
	if !equalStrings(gotMain, want) {
		t.Fatalf("MainClones = %v, want %v", gotMain, want)
	}
}

func pathsToStrings(paths []domain.Path) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = string(p)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestFSAdapter_ListDirs_MissingRootReturnsEmptyNotError is CRITICAL-3's own
// regression test (verify-report.md): a workspaces_root that does not
// exist yet is the normal, expected state of any context with zero
// workspaces (the state every brand-new context is in before its first
// `ws create`). ListDirs must report that as an empty list, not propagate
// os.ReadDir's raw ErrNotExist — exactly like Exists and IsDir in this
// same file already treat "does not exist" as a normal zero-value answer,
// never an error, for the same underlying reason.
func TestFSAdapter_ListDirs_MissingRootReturnsEmptyNotError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "never-created")

	fs := fsstore.New()
	got, err := fs.ListDirs(domain.Path(filepath.ToSlash(root)))
	if err != nil {
		t.Fatalf("ListDirs(missing root) = %v, want nil error", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListDirs(missing root) = %v, want empty", got)
	}
}

// TestFSAdapter_ResolvesPlatformPaths asserts the WSPACE_CONFIG_HOME override,
// which is the hook every test uses instead of a real home directory
// (design.md §5, tasks.md 2.28).
func TestFSAdapter_ResolvesPlatformPaths(t *testing.T) {
	override := t.TempDir()
	t.Setenv("WSPACE_CONFIG_HOME", override)

	fs := fsstore.New()
	paths := fs.Paths()

	wantConfig := domain.Path(filepath.ToSlash(override))
	if paths.Config != wantConfig {
		t.Fatalf("expected Config=%q, got %q", wantConfig, paths.Config)
	}
}

// TestFSAdapter_ResolvesPlatformPaths_DefaultsWithoutOverride confirms the
// resolver falls back to a platform default (non-empty) when
// WSPACE_CONFIG_HOME is unset.
func TestFSAdapter_ResolvesPlatformPaths_DefaultsWithoutOverride(t *testing.T) {
	t.Setenv("WSPACE_CONFIG_HOME", "")

	fs := fsstore.New()
	paths := fs.Paths()

	if paths.Config == "" {
		t.Fatal("expected a non-empty default Config path")
	}
	if paths.Cache == "" {
		t.Fatal("expected a non-empty default Cache path")
	}
	if paths.Home == "" {
		t.Fatal("expected a non-empty Home path")
	}
}

// TestFSAdapter_LegacyFlatConfigPath_UsesXDGConfigHome asserts the legacy
// tool's own config directory ("ws", not this application's "wspace") is
// resolved from XDG_CONFIG_HOME independently of WSPACE_CONFIG_HOME (this
// change's import feature): the two must never collide, since one names
// this application's own store and the other names an older, unrelated
// tool's directory of the same generic shape.
func TestFSAdapter_LegacyFlatConfigPath_UsesXDGConfigHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CONFIG_HOME is POSIX-only; Windows resolves via %APPDATA%")
	}
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("WSPACE_CONFIG_HOME", "should-never-be-consulted")

	fs := fsstore.New()
	got := fs.LegacyFlatConfigPath()

	want := domain.Path(filepath.ToSlash(filepath.Join(xdg, "ws", "config")))
	if got != want {
		t.Fatalf("LegacyFlatConfigPath() = %q, want %q", got, want)
	}
}

// TestFSAdapter_ListFiles lists only regular files, sorted, and a missing
// directory as empty (like ListDirs).
func TestFSAdapter_ListFiles(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"b.patch", "a.patch"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	fs := fsstore.New()
	got, err := fs.ListFiles(domain.Path(filepath.ToSlash(root)))
	if err != nil || len(got) != 2 || got[0] != "a.patch" || got[1] != "b.patch" {
		t.Fatalf("ListFiles = %v, %v", got, err)
	}
	if got, err := fs.ListFiles(domain.Path(filepath.ToSlash(filepath.Join(root, "missing")))); err != nil || len(got) != 0 {
		t.Fatalf("ListFiles(missing) = %v, %v", got, err)
	}
}
