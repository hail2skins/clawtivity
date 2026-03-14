package server

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"testing"
)

func TestLogEventIncludesMetricsAndDetails(t *testing.T) {
	resetLogMetricsForTest()
	t.Setenv("CLAWTIVITY_LOG_LEVEL", "info")

	incActivitiesCreated()
	incQueueFlushAttempted()
	incQueueFlushSucceeded()
	incQueueFlushFailed()

	latestQueueDepth.Store(3)
	output := captureServerLogOutput(t, func() {
		logEvent("info", "api_ingest", map[string]any{"session_key": "s-1"}, 3)
	})

	entry := decodeServerLogLine(t, output)
	if entry["event"] != "api_ingest" {
		t.Fatalf("expected api_ingest event, got %v", entry["event"])
	}

	metrics, ok := entry["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("expected metrics object, got %T", entry["metrics"])
	}
	if metrics["activities_created"] != float64(1) {
		t.Fatalf("expected activities_created=1, got %v", metrics["activities_created"])
	}
	if metrics["queue_flush_attempted"] != float64(1) {
		t.Fatalf("expected queue_flush_attempted=1, got %v", metrics["queue_flush_attempted"])
	}
	if metrics["queue_flush_succeeded"] != float64(1) {
		t.Fatalf("expected queue_flush_succeeded=1, got %v", metrics["queue_flush_succeeded"])
	}
	if metrics["queue_flush_failed"] != float64(1) {
		t.Fatalf("expected queue_flush_failed=1, got %v", metrics["queue_flush_failed"])
	}
	if metrics["queue_depth"] != float64(3) {
		t.Fatalf("expected queue_depth=3, got %v", metrics["queue_depth"])
	}

	details, ok := entry["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details object, got %T", entry["details"])
	}
	if details["queue_depth"] != float64(3) {
		t.Fatalf("expected details.queue_depth=3, got %v", details["queue_depth"])
	}
}

func TestLogEventHonorsEnvLogLevel(t *testing.T) {
	resetLogMetricsForTest()
	t.Setenv("CLAWTIVITY_LOG_LEVEL", "error")

	output := captureServerLogOutput(t, func() {
		logEvent("info", "api_ingest", map[string]any{"session_key": "s-1"}, 0)
	})

	if strings.TrimSpace(output) != "" {
		t.Fatalf("expected info log to be suppressed, got %q", output)
	}
}

func TestCreateActivityHandlerLogsStructuredIngest(t *testing.T) {
	resetLogMetricsForTest()
	t.Setenv("CLAWTIVITY_LOG_LEVEL", "info")

	handler, cleanup := newTestHandler(t)
	defer cleanup()

	payload := map[string]any{
		"session_key":    "session-log-1",
		"model":          "gpt-5",
		"tokens_in":      12,
		"tokens_out":     4,
		"cost_estimate":  0.0,
		"duration_ms":    int64(50),
		"project_tag":    "clawtivity",
		"project_reason": "workspace_path",
		"channel":        "webchat",
		"status":         "success",
		"user_id":        "art",
	}

	output := captureServerLogOutput(t, func() {
		rr := performJSON(t, handler, http.MethodPost, "/api/activity", payload)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
		}
	})

	entry := decodeServerLogLine(t, output)
	if entry["event"] != "api_ingest" {
		t.Fatalf("expected api_ingest event, got %v", entry["event"])
	}
	metrics := entry["metrics"].(map[string]any)
	if metrics["activities_created"] != float64(1) {
		t.Fatalf("expected activities_created=1, got %v", metrics["activities_created"])
	}
}

func captureServerLogOutput(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	}()

	fn()
	return buf.String()
}

func decodeServerLogLine(t *testing.T, output string) map[string]any {
	t.Helper()

	line := strings.TrimSpace(output)
	if line == "" {
		t.Fatal("expected log output")
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("expected JSON log line, got %q err=%v", line, err)
	}
	return entry
}

func resetLogMetricsForTest() {
	metricsCounters.activitiesCreated.Store(0)
	metricsCounters.queueFlushAttempted.Store(0)
	metricsCounters.queueFlushSucceeded.Store(0)
	metricsCounters.queueFlushFailed.Store(0)
	latestQueueDepth.Store(0)
}
