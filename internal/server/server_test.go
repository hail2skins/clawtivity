package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePortDefaultsTo18730(t *testing.T) {
	t.Setenv("PORT", "")

	if got := resolvePort(); got != 18730 {
		t.Fatalf("expected default port 18730, got %d", got)
	}
}

func TestResolvePortHonorsEnvironment(t *testing.T) {
	t.Setenv("PORT", "19000")

	if got := resolvePort(); got != 19000 {
		t.Fatalf("expected env port 19000, got %d", got)
	}
}

func TestResolvePortFallsBackOnInvalidValue(t *testing.T) {
	t.Setenv("PORT", "not-a-number")

	if got := resolvePort(); got != 18730 {
		t.Fatalf("expected fallback port 18730, got %d", got)
	}
}

func TestResolvePortFromRealEnvDoesNotPanic(t *testing.T) {
	_ = os.Getenv("PORT")
	_ = resolvePort()
}

func TestNewServerReturnsErrorWhenDBInitFails(t *testing.T) {
	t.Setenv("BLUEPRINT_DB_URL", filepath.Join(t.TempDir(), "missing", "db.sqlite"))

	srv, err := NewServer()
	if err == nil {
		t.Fatal("expected NewServer to return DB init error")
	}
	if srv != nil {
		t.Fatal("expected nil server when DB init fails")
	}
}
