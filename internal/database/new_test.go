package database

import (
	"path/filepath"
	"testing"
)

func TestNewReturnsServiceAndNoSingletonReuse(t *testing.T) {
	firstPath := filepath.Join(t.TempDir(), "first.db")
	secondPath := filepath.Join(t.TempDir(), "second.db")

	t.Setenv("BLUEPRINT_DB_URL", firstPath)
	first, err := New()
	if err != nil {
		t.Fatalf("expected first New() to succeed: %v", err)
	}
	t.Cleanup(func() {
		_ = first.Close()
	})

	t.Setenv("BLUEPRINT_DB_URL", secondPath)
	second, err := New()
	if err != nil {
		t.Fatalf("expected second New() to succeed: %v", err)
	}
	t.Cleanup(func() {
		_ = second.Close()
	})

	firstSvc, ok := first.(*service)
	if !ok {
		t.Fatalf("expected first to be *service, got %T", first)
	}
	secondSvc, ok := second.(*service)
	if !ok {
		t.Fatalf("expected second to be *service, got %T", second)
	}

	if firstSvc == secondSvc {
		t.Fatal("expected distinct service instances, got singleton reuse")
	}
}

func TestNewReturnsErrorForInvalidPath(t *testing.T) {
	t.Setenv("BLUEPRINT_DB_URL", filepath.Join(t.TempDir(), "missing", "db.sqlite"))

	_, err := New()
	if err == nil {
		t.Fatal("expected New() to return an error for invalid DB path")
	}
}
