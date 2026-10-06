// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/cli"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

// fixture bundles every fake port a CLI test needs, plus the Runtime built
// from them. Every path used here is a fixed literal (never a real
// t.TempDir()), so golden comparisons stay deterministic without leaning on
// normalizeGolden for the common case.
type fixture struct {
	Git   *portstest.FakeGit
	FS    *portstest.FakeFS
	Store *portstest.FakeConfigStore
	RT    *cli.Runtime
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	fs := portstest.NewFakeFS(t)
	fs.Chdir("/fixture/cwd")
	git := portstest.NewFakeGit()
	store := portstest.NewFakeConfigStore()

	store.PutContext(domain.Context{
		Name:           "work",
		WorkspacesRoot: "/fixture/workspaces",
		Projects: []domain.Project{
			{Key: "svc", SourceDir: "/fixture/src/svc"},
		},
	})
	if err := store.SaveRoot(context.Background(), domain.RootConfig{ActiveContext: "work"}); err != nil {
		t.Fatalf("seed SaveRoot: %v", err)
	}

	rt := &cli.Runtime{
		Deps:     app.Deps{Store: store, Git: git, FS: fs},
		ExecDeps: app.ExecDeps{Store: store},
		Version:  "dev",
	}
	return &fixture{Git: git, FS: fs, Store: store, RT: rt}
}

// run executes the CLI against args, feeding stdin, and returns captured
// stdout/stderr and the process exit code cli.Execute computed.
func run(rt *cli.Runtime, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	var out, errBuf bytes.Buffer
	exitCode = cli.Execute(rt, args, strings.NewReader(stdin), &out, &errBuf)
	return out.String(), errBuf.String(), exitCode
}
