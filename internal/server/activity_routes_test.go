package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"clawtivity/internal/database"
)

func TestPostActivityCreatesEntry(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	payload := map[string]any{
		"session_key":   "session-1",
		"model":         "gpt-5",
		"tokens_in":     120,
		"tokens_out":    80,
		"cost_estimate": 0.12,
		"duration_ms":   int64(1234),
		"project_tag":   "proj-alpha",
		"external_ref":  "CLAW-4",
		"category":      "code",
		"thinking":      "high",
		"reasoning":     true,
		"channel":       "discord",
		"status":        "success",
		"user_id":       "art",
	}

	rr := performJSON(t, handler, http.MethodPost, "/api/activity", payload)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
	}

	var got database.ActivityFeed
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("expected valid json response: %v", err)
	}

	if got.ID == "" {
		t.Fatal("expected id to be generated")
	}
	if got.ProjectTag != "proj-alpha" {
		t.Fatalf("expected project_tag proj-alpha, got %q", got.ProjectTag)
	}
}

func TestGetActivitySupportsProjectModelDateFilters(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	createActivity(t, handler, map[string]any{
		"session_key":   "session-1",
		"model":         "gpt-5",
		"tokens_in":     10,
		"tokens_out":    5,
		"cost_estimate": 0.01,
		"duration_ms":   int64(100),
		"project_tag":   "proj-alpha",
		"category":      "code",
		"thinking":      "low",
		"reasoning":     true,
		"channel":       "webchat",
		"status":        "success",
		"user_id":       "u1",
		"created_at":    "2026-02-18T10:00:00Z",
	})
	createActivity(t, handler, map[string]any{
		"session_key":   "session-2",
		"model":         "gpt-4.1",
		"tokens_in":     40,
		"tokens_out":    20,
		"cost_estimate": 0.02,
		"duration_ms":   int64(200),
		"project_tag":   "proj-alpha",
		"category":      "research",
		"thinking":      "medium",
		"reasoning":     false,
		"channel":       "telegram",
		"status":        "failed",
		"user_id":       "u2",
		"created_at":    "2026-02-18T12:00:00Z",
	})
	createActivity(t, handler, map[string]any{
		"session_key":   "session-3",
		"model":         "gpt-5",
		"tokens_in":     50,
		"tokens_out":    25,
		"cost_estimate": 0.03,
		"duration_ms":   int64(300),
		"project_tag":   "proj-beta",
		"category":      "admin",
		"thinking":      "high",
		"reasoning":     true,
		"channel":       "discord",
		"status":        "pending",
		"user_id":       "u3",
		"created_at":    "2026-02-17T10:00:00Z",
	})

	req, err := http.NewRequest(http.MethodGet, "/api/activity?project=proj-alpha&model=gpt-5&date=2026-02-18", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var got []database.ActivityFeed
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("expected valid json response: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(got))
	}
	if got[0].SessionKey != "session-1" {
		t.Fatalf("expected session-1, got %q", got[0].SessionKey)
	}
}

func TestGetActivitySummaryAggregatesStats(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	createActivity(t, handler, map[string]any{
		"session_key":   "session-1",
		"model":         "gpt-5",
		"tokens_in":     100,
		"tokens_out":    50,
		"cost_estimate": 0.10,
		"duration_ms":   int64(1000),
		"project_tag":   "proj-alpha",
		"category":      "code",
		"thinking":      "high",
		"reasoning":     true,
		"channel":       "discord",
		"status":        "success",
		"user_id":       "u1",
		"created_at":    "2026-02-18T10:00:00Z",
	})
	createActivity(t, handler, map[string]any{
		"session_key":   "session-2",
		"model":         "gpt-5",
		"tokens_in":     40,
		"tokens_out":    25,
		"cost_estimate": 0.04,
		"duration_ms":   int64(600),
		"project_tag":   "proj-alpha",
		"category":      "research",
		"thinking":      "medium",
		"reasoning":     false,
		"channel":       "webchat",
		"status":        "failed",
		"user_id":       "u2",
		"created_at":    "2026-02-18T12:00:00Z",
	})
	createActivity(t, handler, map[string]any{
		"session_key":   "session-3",
		"model":         "gpt-4.1",
		"tokens_in":     500,
		"tokens_out":    500,
		"cost_estimate": 1.0,
		"duration_ms":   int64(9000),
		"project_tag":   "proj-beta",
		"category":      "admin",
		"thinking":      "low",
		"reasoning":     true,
		"channel":       "telegram",
		"status":        "success",
		"user_id":       "u3",
		"created_at":    "2026-02-18T13:00:00Z",
	})

	req, err := http.NewRequest(http.MethodGet, "/api/activity/summary?project=proj-alpha&model=gpt-5&date=2026-02-18", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var got struct {
		Count           int64          `json:"count"`
		TokensInTotal   int64          `json:"tokens_in_total"`
		TokensOutTotal  int64          `json:"tokens_out_total"`
		CostTotal       float64        `json:"cost_total"`
		DurationMSTotal int64          `json:"duration_ms_total"`
		ByStatus        map[string]int `json:"by_status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("expected valid json response: %v", err)
	}

	if got.Count != 2 {
		t.Fatalf("expected count 2, got %d", got.Count)
	}
	if got.TokensInTotal != 140 {
		t.Fatalf("expected tokens_in_total 140, got %d", got.TokensInTotal)
	}
	if got.TokensOutTotal != 75 {
		t.Fatalf("expected tokens_out_total 75, got %d", got.TokensOutTotal)
	}
	if got.CostTotal != 0.14 {
		t.Fatalf("expected cost_total 0.14, got %f", got.CostTotal)
	}
	if got.DurationMSTotal != 1600 {
		t.Fatalf("expected duration_ms_total 1600, got %d", got.DurationMSTotal)
	}
	if got.ByStatus["success"] != 1 || got.ByStatus["failed"] != 1 {
		t.Fatalf("expected by_status success=1 failed=1, got %#v", got.ByStatus)
	}
}

func TestActivityEndpointsRejectInvalidDateFilter(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	req1, err := http.NewRequest(http.MethodGet, "/api/activity?date=18-02-2026", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, rr1.Code, rr1.Body.String())
	}

	req2, err := http.NewRequest(http.MethodGet, "/api/activity/summary?date=2026/02/18", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, rr2.Code, rr2.Body.String())
	}
}

func newTestHandler(t *testing.T) (http.Handler, func()) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), fmt.Sprintf("clawtivity-%d.db", time.Now().UnixNano()))
	adapter, err := database.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite adapter: %v", err)
	}

	s := &Server{db: adapter}

	cleanup := func() {
		_ = adapter.Close()
	}

	return s.RegisterRoutes(), cleanup
}

func createActivity(t *testing.T, handler http.Handler, payload map[string]any) {
	t.Helper()

	rr := performJSON(t, handler, http.MethodPost, "/api/activity", payload)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, rr.Code, rr.Body.String())
	}
}

func performJSON(t *testing.T, handler http.Handler, method, path string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(method, path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}
