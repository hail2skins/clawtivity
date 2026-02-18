package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebRouteServesActivityDashboard(t *testing.T) {
	s := &Server{}
	r := s.RegisterRoutes()

	req, err := http.NewRequest(http.MethodGet, "/web", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	body := rr.Body.String()
	checks := []string{
		"Activity Dashboard",
		"id=\"project-filter\"",
		"id=\"model-filter\"",
		"id=\"date-from\"",
		"id=\"date-to\"",
		"id=\"token-chart\"",
		"id=\"cost-by-project\"",
		"id=\"activity-timeline\"",
	}

	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Fatalf("expected dashboard html to contain %q", check)
		}
	}
}
