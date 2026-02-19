package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clawtivity/internal/database"
)

func TestFlushQueuedActivitiesImportsAndDeletesProcessedFile(t *testing.T) {
	adapter, cleanup := newQueueTestAdapter(t)
	defer cleanup()

	queueRoot := t.TempDir()
	filePath := filepath.Join(queueRoot, "2026-02-19.md")
	body := strings.Join([]string{
		"# Clawtivity Fallback Queue (2026-02-19)",
		"",
		"## queued_at: 2026-02-19T00:00:00Z",
		"```json",
		`{"session_key":"q-1","model":"gpt-5","tokens_in":10,"tokens_out":5,"cost_estimate":0,"duration_ms":100,"project_tag":"clawtivity","external_ref":"","category":"general","thinking":"medium","reasoning":false,"channel":"webchat","status":"success","user_id":"u1","created_at":"2026-02-19T00:00:00Z"}`,
		"```",
		"",
	}, "\n")
	if err := os.WriteFile(filePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	flushed, err := flushQueuedActivities(context.Background(), adapter, queueRoot)
	if err != nil {
		t.Fatalf("expected no error flushing queue: %v", err)
	}
	if flushed != 1 {
		t.Fatalf("expected 1 flushed row, got %d", flushed)
	}

	activities, err := adapter.ListActivities(context.Background(), database.ActivityFilters{ProjectTag: "clawtivity"})
	if err != nil {
		t.Fatalf("expected list to work: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("expected 1 inserted activity, got %d", len(activities))
	}
	if activities[0].SessionKey != "q-1" {
		t.Fatalf("expected session q-1, got %q", activities[0].SessionKey)
	}

	files, err := filepath.Glob(filepath.Join(queueRoot, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected queue files to be deleted, got %v", files)
	}
}

func TestFlushQueuedActivitiesKeepsMalformedQueueEntries(t *testing.T) {
	adapter, cleanup := newQueueTestAdapter(t)
	defer cleanup()

	queueRoot := t.TempDir()
	filePath := filepath.Join(queueRoot, "2026-02-19.md")
	body := strings.Join([]string{
		"# Clawtivity Fallback Queue (2026-02-19)",
		"",
		"## queued_at: 2026-02-19T00:00:00Z",
		"```json",
		"not-json",
		"```",
		"",
	}, "\n")
	if err := os.WriteFile(filePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	flushed, err := flushQueuedActivities(context.Background(), adapter, queueRoot)
	if err != nil {
		t.Fatalf("expected no error flushing malformed queue: %v", err)
	}
	if flushed != 0 {
		t.Fatalf("expected 0 flushed rows, got %d", flushed)
	}

	kept, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected queue file to remain: %v", err)
	}
	if !strings.Contains(string(kept), "not-json") {
		t.Fatalf("expected malformed payload to remain in queue file")
	}
}

func TestFlushQueueOnStartupUsesConfiguredQueueDir(t *testing.T) {
	adapter, cleanup := newQueueTestAdapter(t)
	defer cleanup()

	queueRoot := t.TempDir()
	t.Setenv("CLAWTIVITY_QUEUE_DIR", queueRoot)

	filePath := filepath.Join(queueRoot, "2026-02-19.md")
	body := strings.Join([]string{
		"# Clawtivity Fallback Queue (2026-02-19)",
		"",
		"## queued_at: 2026-02-19T00:00:00Z",
		"```json",
		`{"session_key":"startup-1","model":"gpt-5","tokens_in":1,"tokens_out":1,"cost_estimate":0,"duration_ms":10,"project_tag":"clawtivity","external_ref":"","category":"general","thinking":"medium","reasoning":false,"channel":"webchat","status":"success","user_id":"u1","created_at":"2026-02-19T00:00:00Z"}`,
		"```",
		"",
	}, "\n")
	if err := os.WriteFile(filePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	flushQueueOnStartup(adapter)

	activities, err := adapter.ListActivities(context.Background(), database.ActivityFilters{ProjectTag: "clawtivity"})
	if err != nil {
		t.Fatalf("expected list to work: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("expected startup queue flush to insert 1 row, got %d", len(activities))
	}
}

func newQueueTestAdapter(t *testing.T) (database.Service, func()) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), fmt.Sprintf("queue-%d.db", time.Now().UnixNano()))
	adapter, err := database.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite adapter: %v", err)
	}

	cleanup := func() {
		_ = adapter.Close()
	}

	return adapter, cleanup
}
