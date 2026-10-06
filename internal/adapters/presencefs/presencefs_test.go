// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Kivora Digital S.L.

package presencefs_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kivoradigital/wspace/internal/adapters/presencefs"
	"github.com/kivoradigital/wspace/internal/domain"
	"github.com/kivoradigital/wspace/internal/ports"
)

var _ ports.PresenceStore = (*presencefs.Store)(nil)

func session(pid int, started time.Time) ports.AgentSession {
	return ports.AgentSession{
		PID: pid, ClientName: "agent", ClientVersion: "1.0",
		StartedAt: started, LastActivityAt: started,
	}
}

func TestDir_IsRunMCPUnderConfig(t *testing.T) {
	got := presencefs.Dir(domain.Path("/cfg/wspace"))
	if got != "/cfg/wspace/run/mcp" {
		t.Fatalf("Dir = %q, want /cfg/wspace/run/mcp", got)
	}
}

func TestPut_WritesOneJSONFilePerPIDAtomically(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run", "mcp")
	s := presencefs.NewAt(dir, func(int) bool { return true })
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	rec := session(4242, started)
	rec.LastTool = "list_workspaces"
	rec.LastToolAt = started.Add(time.Minute)
	if err := s.Put(context.Background(), rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "4242.json" {
		t.Fatalf("dir entries = %v, want exactly 4242.json (no temp file left)", entries)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "4242.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("record is not JSON: %v", err)
	}
	for _, key := range []string{"pid", "clientName", "clientVersion", "startedAt", "lastActivityAt", "lastTool", "lastToolAt"} {
		if _, ok := doc[key]; !ok {
			t.Fatalf("record %s lacks %q", raw, key)
		}
	}

	rec.LastTool = "workspace_status"
	if err := s.Put(context.Background(), rec); err != nil {
		t.Fatalf("second Put: %v", err)
	}
	live, err := s.Live(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 || live[0].LastTool != "workspace_status" || !live[0].StartedAt.Equal(started) {
		t.Fatalf("Live = %+v, want the replaced record", live)
	}
}

func TestLive_IgnoresAndDeletesDeadProcessesAndJunk(t *testing.T) {
	dir := t.TempDir()
	alive := map[int]bool{10: true, 30: true}
	s := presencefs.NewAt(dir, func(pid int) bool { return alive[pid] })
	base := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	for _, rec := range []ports.AgentSession{session(30, base.Add(time.Hour)), session(10, base), session(20, base)} {
		if err := s.Put(context.Background(), rec); err != nil {
			t.Fatal(err)
		}
	}
	// Junk: an unparsable record of a live pid, a stray file, a dead
	// pid's leftover temp file.
	_ = os.WriteFile(filepath.Join(dir, "30.json.bad"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, ".20-123.tmp"), []byte("{"), 0o600)

	live, err := s.Live(context.Background())
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	if len(live) != 2 || live[0].PID != 10 || live[1].PID != 30 {
		t.Fatalf("Live = %+v, want pids 10 then 30 (oldest first)", live)
	}
	if _, err := os.Stat(filepath.Join(dir, "20.json")); !os.IsNotExist(err) {
		t.Fatalf("dead pid's record still on disk (stat err %v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".20-123.tmp")); !os.IsNotExist(err) {
		t.Fatalf("dead pid's temp file still on disk (stat err %v)", err)
	}
}

func TestLive_CorruptRecordOfLiveProcessIsSkipped(t *testing.T) {
	dir := t.TempDir()
	s := presencefs.NewAt(dir, func(int) bool { return true })
	_ = os.WriteFile(filepath.Join(dir, "77.json"), []byte("{not json"), 0o600)
	live, err := s.Live(context.Background())
	if err != nil || len(live) != 0 {
		t.Fatalf("Live = %+v, %v; want no sessions and no error", live, err)
	}
}

func TestLive_RecordWhosePIDDisagreesWithItsNameIsSkipped(t *testing.T) {
	dir := t.TempDir()
	s := presencefs.NewAt(dir, func(int) bool { return true })
	raw, _ := json.Marshal(map[string]any{"pid": 5, "clientName": "x", "startedAt": time.Now().UTC(), "lastActivityAt": time.Now().UTC()})
	_ = os.WriteFile(filepath.Join(dir, "6.json"), raw, 0o600)
	if live, _ := s.Live(context.Background()); len(live) != 0 {
		t.Fatalf("Live = %+v, want the mismatched record skipped", live)
	}
}

func TestLive_MissingDirectoryIsEmpty(t *testing.T) {
	s := presencefs.NewAt(filepath.Join(t.TempDir(), "absent"), func(int) bool { return true })
	live, err := s.Live(context.Background())
	if err != nil || len(live) != 0 {
		t.Fatalf("Live = %+v, %v; want empty, nil", live, err)
	}
}

func TestRemove_DeletesAndToleratesMissing(t *testing.T) {
	dir := t.TempDir()
	s := presencefs.NewAt(dir, func(int) bool { return true })
	if err := s.Put(context.Background(), session(9, time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(context.Background(), 9); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := s.Remove(context.Background(), 9); err != nil {
		t.Fatalf("Remove of a missing record: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("dir still holds %v", entries)
	}
}

func TestProcessAlive_SelfIsAliveAndAnUnusedPIDIsNot(t *testing.T) {
	if !presencefs.ProcessAlive(os.Getpid()) {
		t.Fatal("own pid reported dead")
	}
	if presencefs.ProcessAlive(0) || presencefs.ProcessAlive(-1) {
		t.Fatal("non-positive pid reported alive")
	}
	// A pid far above any default pid_max on darwin/linux.
	if presencefs.ProcessAlive(1 << 30) {
		t.Fatal("unused pid reported alive")
	}
}

func TestNew_UsesConfigDir(t *testing.T) {
	cfg := t.TempDir()
	s := presencefs.New(domain.Path(filepath.ToSlash(cfg)))
	if err := s.Put(context.Background(), session(os.Getpid(), time.Now())); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "run", "mcp", strings.TrimSpace(itoa(os.Getpid()))+".json")); err != nil {
		t.Fatalf("record not under <config>/run/mcp: %v", err)
	}
	live, _ := s.Live(context.Background())
	if len(live) != 1 {
		t.Fatalf("Live with the real liveness check = %+v, want this process", live)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
