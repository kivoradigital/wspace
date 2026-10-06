// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package git

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/domain"
)

// Captured from real git 2.54 output (core.quotepath=false, a/ b/
// prefixes): a modification, an untracked add whose name holds a space
// (git appends a TAB to such ---/+++ names), a C-quoted name, a binary
// file, a rename with an edit and a pure mode change.
const capturedPatch = "diff --git a/f.txt b/f.txt\n" +
	"index 422c2b7..55dce13 100644\n" +
	"--- a/f.txt\n" +
	"+++ b/f.txt\n" +
	"@@ -1,2 +1,2 @@\n" +
	" a\n" +
	"-b\n" +
	"+B\n" +
	"diff --git a/z q.txt b/z q.txt\n" +
	"new file mode 100644\n" +
	"index 0000000..587be6b\n" +
	"--- /dev/null\n" +
	"+++ b/z q.txt\t\n" +
	"@@ -0,0 +1 @@\n" +
	"+x\n" +
	"\\ No newline at end of file\n" +
	"diff --git \"a/we\\\"ird\\nnl.txt\" \"b/we\\\"ird\\nnl.txt\"\n" +
	"deleted file mode 100644\n" +
	"index bca70f3..0000000\n" +
	"--- \"a/we\\\"ird\\nnl.txt\"\n" +
	"+++ /dev/null\n" +
	"@@ -1 +0,0 @@\n" +
	"-q\n" +
	"diff --git a/b.bin b/b.bin\n" +
	"new file mode 100644\n" +
	"index 0000000..88768ef\n" +
	"Binary files /dev/null and b/b.bin differ\n" +
	"diff --git a/old name.go b/new name.go\n" +
	"similarity index 90%\n" +
	"rename from old name.go\n" +
	"rename to new name.go\n" +
	"index 1111111..2222222 100644\n" +
	"--- a/old name.go\t\n" +
	"+++ b/new name.go\t\n" +
	"@@ -3,4 +3,5 @@ func main() {\n" +
	" x\n" +
	"+y\n" +
	" z\n" +
	"diff --git a/run.sh b/run.sh\n" +
	"old mode 100644\n" +
	"new mode 100755\n"

func TestParsePatch_FilesStatusesAndHunks(t *testing.T) {
	p := parsePatch(capturedPatch, false, domain.DefaultDiffLimits)

	if p.Truncated || len(p.Files) != 6 {
		t.Fatalf("patch = %+v", p)
	}
	f := p.Files[0]
	if f.Path != "f.txt" || f.Status != domain.FileModified || f.Additions != 1 || f.Deletions != 1 || len(f.Hunks) != 1 {
		t.Fatalf("modified file = %+v", f)
	}
	h := f.Hunks[0]
	if h.Header != "@@ -1,2 +1,2 @@" || h.OldStart != 1 || h.OldLines != 2 || h.NewStart != 1 || h.NewLines != 2 ||
		!reflect.DeepEqual(h.Lines, []string{" a", "-b", "+B"}) {
		t.Fatalf("hunk = %+v", h)
	}

	added := p.Files[1]
	if added.Path != "z q.txt" || added.Status != domain.FileAdded || added.Additions != 1 ||
		added.Hunks[0].NewStart != 1 || added.Hunks[0].NewLines != 1 || added.Hunks[0].OldLines != 0 ||
		!reflect.DeepEqual(added.Hunks[0].Lines, []string{"+x", "\\ No newline at end of file"}) {
		t.Fatalf("added file = %+v", added)
	}

	if d := p.Files[2]; d.Path != "we\"ird\nnl.txt" || d.Status != domain.FileDeleted || d.Deletions != 1 {
		t.Fatalf("quoted deleted file = %+v", d)
	}
	if b := p.Files[3]; b.Path != "b.bin" || !b.Binary || b.Status != domain.FileAdded || len(b.Hunks) != 0 {
		t.Fatalf("binary file = %+v", b)
	}
	r := p.Files[4]
	if r.Path != "new name.go" || r.OrigPath != "old name.go" || r.Status != domain.FileRenamed || r.Hunks[0].Header != "@@ -3,4 +3,5 @@ func main() {" {
		t.Fatalf("renamed file = %+v", r)
	}
	if m := p.Files[5]; m.Path != "run.sh" || m.Status != domain.FileModified || len(m.Hunks) != 0 {
		t.Fatalf("mode change = %+v", m)
	}
}

func TestParsePatch_LineCapTruncatesTheFileAndThePatch(t *testing.T) {
	p := parsePatch(capturedPatch, false, domain.DiffLimits{MaxBytes: 1 << 20, MaxLines: 2})

	if !p.Truncated || len(p.Files) != 1 || !p.Files[0].Truncated {
		t.Fatalf("patch = %+v", p)
	}
	if got := p.Files[0].Hunks[0].Lines; !reflect.DeepEqual(got, []string{" a", "-b"}) {
		t.Fatalf("lines = %q", got)
	}
}

func TestParsePatch_ByteTruncatedInputDropsThePartialLastLine(t *testing.T) {
	raw := capturedPatch[:strings.Index(capturedPatch, "+B")+1] // ends mid-line: "+"
	p := parsePatch(raw, true, domain.DefaultDiffLimits)

	if !p.Truncated || len(p.Files) != 1 || !p.Files[0].Truncated {
		t.Fatalf("patch = %+v", p)
	}
	if got := p.Files[0].Hunks[0].Lines; !reflect.DeepEqual(got, []string{" a", "-b"}) {
		t.Fatalf("lines = %q", got)
	}
}

func TestParsePatch_EmptyIsNoFiles(t *testing.T) {
	if p := parsePatch("", false, domain.DefaultDiffLimits); len(p.Files) != 0 || p.Files == nil || p.Truncated {
		t.Fatalf("patch = %+v", p)
	}
}

func TestUnquoteC(t *testing.T) {
	tests := map[string]string{
		`"a/we\"ird\nnl.txt"`: "a/we\"ird\nnl.txt",
		`"tab\there"`:         "tab\there",
		`"back\\slash"`:       `back\slash`,
		`"\303\251t\303\251"`: "été",
		"plain":               "plain",
	}
	for in, want := range tests {
		if got := unquoteC(in); got != want {
			t.Errorf("unquoteC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseCommitLog_NULRecordsUnitSeparatedFields(t *testing.T) {
	raw := "1111111111111111111111111111111111111111\x1f1111111\x1fAda Acme\x1f2026-10-03T10:00:00+02:00\x1ffeat: one\x00" +
		"2222222222222222222222222222222222222222\x1f2222222\x1fBo Acme\x1f2026-10-02T09:30:00Z\x1fsubject with \x1f? no\x00"

	got, err := parseCommitLog(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("commits = %+v", got)
	}
	want0 := domain.CommitInfo{Hash: strings.Repeat("1", 40), ShortHash: "1111111", AuthorName: "Ada Acme", Subject: "feat: one",
		AuthorDate: time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)}
	if got[0].Hash != want0.Hash || got[0].ShortHash != want0.ShortHash || got[0].AuthorName != want0.AuthorName ||
		got[0].Subject != want0.Subject || !got[0].AuthorDate.Equal(want0.AuthorDate) {
		t.Fatalf("commit 0 = %+v", got[0])
	}
	if got[1].Subject != "subject with \x1f? no" {
		t.Fatalf("subject = %q", got[1].Subject)
	}
}

func TestParseStashList(t *testing.T) {
	raw := "stash@{0}\x1fc860f81de27981ab9bc0fdc11f95d7d95dd7ee73\x1fOn main: wip msg\x1f2026-10-04T00:45:08+02:00\x00" +
		"stash@{1}\x1fd860f81de27981ab9bc0fdc11f95d7d95dd7ee73\x1fWIP on feat: 13fa57b fix\x1f2026-10-03T00:45:08+02:00\x00"

	got, err := parseStashList(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Index != 0 || got[0].Ref != "stash@{0}" || got[0].Branch != "main" || got[0].Message != "wip msg" ||
		got[1].Index != 1 || got[1].Branch != "feat" || got[1].Message != "13fa57b fix" || got[1].Hash[:4] != "d860" {
		t.Fatalf("stashes = %+v", got)
	}
}

func TestParseUpstream(t *testing.T) {
	if _, ok := parseUpstream("\x1f\x1f\x1f\n"); ok {
		t.Fatal("no upstream must be not ok")
	}
	u, ok := parseUpstream("refs/remotes/origin/feat\x1forigin/feat\x1forigin\x1f\n")
	if !ok || u != (domain.UpstreamInfo{Ref: "origin/feat", Remote: "origin"}) {
		t.Fatalf("upstream = %+v %v", u, ok)
	}
	u, ok = parseUpstream("refs/remotes/origin/gone\x1forigin/gone\x1forigin\x1fgone\n")
	if !ok || !u.Gone {
		t.Fatalf("gone upstream = %+v %v", u, ok)
	}
	u, ok = parseUpstream("refs/remotes/origin/x\x1forigin/x\x1forigin\x1f\x1frefs/heads/y\n")
	if !ok || u != (domain.UpstreamInfo{Ref: "origin/x", Remote: "origin", RemoteRef: "refs/heads/y"}) {
		t.Fatalf("upstream with remote ref = %+v %v", u, ok)
	}
}

func TestParseOverwrittenFiles(t *testing.T) {
	stderr := "error: Your local changes to the following files would be overwritten by merge:\n\tf.txt\n\tdir/g.txt\n" +
		"Please commit your changes or stash them before you merge.\nAborting\n"
	if got := parseOverwrittenFiles(stderr); !reflect.DeepEqual(got, []string{"f.txt", "dir/g.txt"}) {
		t.Fatalf("files = %q", got)
	}
	untracked := "error: The following untracked working tree files would be overwritten by merge:\n\tn2.txt\nPlease move or remove them before you merge.\nAborting\n"
	if got := parseOverwrittenFiles(untracked); !reflect.DeepEqual(got, []string{"n2.txt"}) {
		t.Fatalf("files = %q", got)
	}
	if got := parseOverwrittenFiles("fatal: Not possible to fast-forward, aborting.\n"); got != nil {
		t.Fatalf("files = %q", got)
	}
}
