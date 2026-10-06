// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

// Command ws is the pure-Go CLI composition root (design.md §2, R7). It is
// the only place internal/cli and an adapter package meet: it constructs
// every real port implementation, wires them into internal/app's
// dependency bundles, builds internal/cli.Runtime, and hands control to
// internal/cli.Execute.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kivoradigital/wspace/internal/adapters/agenthost"
	"github.com/kivoradigital/wspace/internal/adapters/configstore"
	"github.com/kivoradigital/wspace/internal/adapters/fsstore"
	"github.com/kivoradigital/wspace/internal/adapters/git"
	"github.com/kivoradigital/wspace/internal/adapters/githubrelease"
	"github.com/kivoradigital/wspace/internal/adapters/presencefs"
	"github.com/kivoradigital/wspace/internal/adapters/termprompt"
	"github.com/kivoradigital/wspace/internal/adapters/treecopy"
	"github.com/kivoradigital/wspace/internal/adapters/winenv"
	"github.com/kivoradigital/wspace/internal/app"
	"github.com/kivoradigital/wspace/internal/buildinfo"
	"github.com/kivoradigital/wspace/internal/cli"
	"github.com/kivoradigital/wspace/internal/engine"
	"github.com/kivoradigital/wspace/internal/mcpserver"
	"github.com/kivoradigital/wspace/internal/rpc"
	"github.com/kivoradigital/wspace/skills"
)

// agentCLITimeout bounds one agent CLI call (`claude mcp add`, …).
const agentCLITimeout = 30 * time.Second

func main() {
	os.Exit(run())
}

func run() int {
	rt, err := wire()
	if err != nil {
		// Never fail silently: a user with nothing printed can't act.
		fmt.Fprintf(os.Stderr, "wspace: %v\n", err)
		return 1
	}
	return cli.Execute(rt, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

// wire constructs every real adapter and assembles internal/cli.Runtime
// (R7: cmd/* is the only place adapters are constructed and injected).
func wire() (*cli.Runtime, error) {
	fs := fsstore.New()
	gitAdapter, err := git.New()
	if err != nil {
		return nil, err
	}
	store := configstore.New(fs.Paths().Config)
	prompter := termprompt.New(os.Stdin, os.Stdout)

	coordinates, _ := buildinfo.Coordinates()
	checker := githubrelease.New(fs.Paths().Cache.Join("release.json"))
	checker.UserAgent = "ws/" + buildinfo.Version

	trees := treecopy.New()
	deps := app.Deps{Store: store, Git: gitAdapter, FS: fs, Trees: trees}

	// Agent skill installation: real filesystem (symlinks), agent CLIs run
	// with a timeout, and the skill embedded in this binary.
	agentFS, agentRunner := agenthost.NewFS(), agenthost.NewRunner(agentCLITimeout)
	agentsDeps := app.AgentsDeps{FS: agentFS, Runner: agentRunner, Skill: skills.FS(), Home: fs.Paths().Home}

	eng := engine.New(engine.Deps{
		Store:       store,
		Git:         gitAdapter,
		FS:          fs,
		Checker:     checker,
		Version:     buildinfo.Version,
		Coordinates: coordinates,
		Presence:    presencefs.New(fs.Paths().Config),
		Trees:       trees,
		Agents:      &engine.AgentsDeps{FS: agentFS, Runner: agentRunner, Skill: skills.FS(), Home: fs.Paths().Home},
	})

	rt := &cli.Runtime{
		Deps:            deps,
		ExecDeps:        app.ExecDeps{Store: store},
		ProjectWizard:   app.ProjectWizardDeps{Store: store, FS: fs, Git: gitAdapter, Prompter: prompter},
		ContextWizard:   app.ContextWizardDeps{Store: store, Prompter: prompter},
		Prompter:        app.PrompterDeps{Prompter: prompter},
		InstallDeps:     app.InstallDeps{FS: fs, Prompter: prompter, UserPath: winenv.New()},
		CheckDeps:       app.CheckForUpdateDeps{Checker: checker},
		AgentsDeps:      agentsDeps,
		Version:         buildinfo.Version,
		RepoCoordinates: coordinates,
		ServeRPC: func(ctx context.Context, in io.Reader, out io.Writer) error {
			return rpc.NewServer(eng, buildinfo.Version).Serve(ctx, in, out)
		},
		ServeMCP: func(ctx context.Context, in io.Reader, out io.Writer) error {
			// An agent stops its server with a signal as often as by closing
			// stdin; cancelling ctx lets Serve remove the presence record.
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
			defer stop()
			return mcpserver.Serve(ctx, eng, buildinfo.Version, in, out)
		},
	}
	return rt, nil
}
