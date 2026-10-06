// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package portstest_test

import (
	"context"
	"testing"

	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
	"github.com/kivoradigital/wspace/internal/ports/portstest"
)

func TestFakeGit_RecordsCallSequence(t *testing.T) {
	fg := portstest.NewFakeGit()
	repo := domain.Path("/repos/api")
	ctx := context.Background()

	if _, err := fg.IsMainClone(ctx, repo); err != nil {
		t.Fatalf("IsMainClone() unexpected error: %v", err)
	}
	if err := fg.Fetch(ctx, repo, "origin"); err != nil {
		t.Fatalf("Fetch() unexpected error: %v", err)
	}
	if err := fg.WorktreeAdd(ctx, repo, ports.WorktreeSpec{Target: "/workspaces/w/api", Branch: "feature/x"}); err != nil {
		t.Fatalf("WorktreeAdd() unexpected error: %v", err)
	}

	want := []string{"IsMainClone", "Fetch", "WorktreeAdd"}
	if len(fg.Calls) != len(want) {
		t.Fatalf("len(Calls) = %d, want %d (Calls=%+v)", len(fg.Calls), len(want), fg.Calls)
	}
	for i, w := range want {
		if fg.Calls[i].Method != w {
			t.Fatalf("Calls[%d].Method = %q, want %q", i, fg.Calls[i].Method, w)
		}
		if fg.Calls[i].Repo != repo {
			t.Fatalf("Calls[%d].Repo = %q, want %q", i, fg.Calls[i].Repo, repo)
		}
	}
}

func TestFakeGit_ScriptedResponsesKeyedByMethodAndRepo(t *testing.T) {
	fg := portstest.NewFakeGit()
	repo := domain.Path("/repos/api")

	fg.IsMainCloneFunc = func(repo domain.Path) (bool, error) {
		return repo == "/repos/api", nil
	}

	got, err := fg.IsMainClone(context.Background(), repo)
	if err != nil || !got {
		t.Fatalf("IsMainClone() = (%v, %v), want (true, nil)", got, err)
	}

	got, err = fg.IsMainClone(context.Background(), "/repos/other")
	if err != nil || got {
		t.Fatalf("IsMainClone(other) = (%v, %v), want (false, nil)", got, err)
	}
}

var _ ports.GitPort = (*portstest.FakeGit)(nil)
