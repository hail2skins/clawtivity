package database

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNewSQLiteAdapterCreatesRequiredSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc, ok := adapter.(*service)
	if !ok {
		t.Fatalf("expected *service, got %T", adapter)
	}

	if !svc.db.Migrator().HasTable(&ActivityFeed{}) {
		t.Fatal("expected activity_feed table to exist")
	}
	if !svc.db.Migrator().HasTable(&TurnMemory{}) {
		t.Fatal("expected turn_memories table to exist")
	}
	if !svc.db.Migrator().HasTable(&Project{}) {
		t.Fatal("expected projects table to exist")
	}

	if !svc.db.Migrator().HasIndex(&ActivityFeed{}, "idx_activity_feed_session_key") {
		t.Fatal("expected activity_feed.session_key index to exist")
	}
	if !svc.db.Migrator().HasIndex(&ActivityFeed{}, "idx_activity_feed_project_id") {
		t.Fatal("expected activity_feed.project_id index to exist")
	}
	if !svc.db.Migrator().HasIndex(&ActivityFeed{}, "idx_activity_feed_category") {
		t.Fatal("expected activity_feed.category index to exist")
	}
	if !svc.db.Migrator().HasIndex(&ActivityFeed{}, "idx_activity_feed_status") {
		t.Fatal("expected activity_feed.status index to exist")
	}
	if !svc.db.Migrator().HasIndex(&ActivityFeed{}, "idx_activity_feed_user_id") {
		t.Fatal("expected activity_feed.user_id index to exist")
	}
	if !svc.db.Migrator().HasIndex(&TurnMemory{}, "idx_turn_memories_session_key") {
		t.Fatal("expected turn_memories.session_key index to exist")
	}
	if !svc.db.Migrator().HasIndex(&Project{}, "idx_projects_slug") {
		t.Fatal("expected projects.slug index to exist")
	}

	assertColumnsPresent(t, svc, &ActivityFeed{}, []string{
		"id",
		"session_key",
		"model",
		"tokens_in",
		"tokens_out",
		"cost_estimate",
		"duration_ms",
		"project_id",
		"project_reason",
		"external_ref",
		"category",
		"category_reason",
		"thinking",
		"reasoning",
		"channel",
		"status",
		"user_id",
		"created_at",
	})

	assertColumnsPresent(t, svc, &TurnMemory{}, []string{
		"id",
		"session_key",
		"summary",
		"tools_used",
		"files_touched",
		"key_decisions",
		"context_snippet",
		"tags",
		"created_at",
	})

	assertColumnsPresent(t, svc, &Project{}, []string{
		"id",
		"slug",
		"display_name",
		"status",
		"created_at",
		"updated_at",
	})
}

func TestSQLiteAdapterGeneratesUUIDPrimaryKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)

	feed := ActivityFeed{
		SessionKey:   "session-123",
		Model:        "gpt-5",
		TokensIn:     100,
		TokensOut:    50,
		CostEstimate: 0.01,
		DurationMS:   1200,
		ProjectID:    mustProjectID(t, svc, "clawtivity"),
		Category:     "code",
		Thinking:     "high",
		Reasoning:    true,
		Channel:      "discord",
		Status:       "success",
		UserID:       "art",
	}

	if err := svc.db.Create(&feed).Error; err != nil {
		t.Fatalf("expected activity feed insert to succeed: %v", err)
	}
	if !looksLikeUUID(feed.ID) {
		t.Fatalf("expected activity_feed.id to be a UUID, got %q", feed.ID)
	}

	memory := TurnMemory{
		SessionKey:     "session-123",
		Summary:        "implemented storage adapter",
		ToolsUsed:      "[\"go\",\"gorm\"]",
		FilesTouched:   "[\"internal/database/database.go\"]",
		KeyDecisions:   "[\"use sqlite for local-first\"]",
		ContextSnippet: "storage migration",
		Tags:           "[\"storage\",\"sqlite\"]",
	}

	if err := svc.db.Create(&memory).Error; err != nil {
		t.Fatalf("expected turn memory insert to succeed: %v", err)
	}
	if !looksLikeUUID(memory.ID) {
		t.Fatalf("expected turn_memories.id to be a UUID, got %q", memory.ID)
	}
}

func TestActivityFeedPersistsNewFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)

	created := ActivityFeed{
		SessionKey:     "session-456",
		Model:          "gpt-5",
		TokensIn:       12,
		TokensOut:      34,
		CostEstimate:   0.002,
		DurationMS:     500,
		ProjectID:      mustProjectID(t, svc, "clawtivity"),
		ProjectReason:  "workspace_path",
		Category:       "research",
		CategoryReason: "keyword_score:research=2",
		Thinking:       "medium",
		Reasoning:      true,
		Channel:        "webchat",
		Status:         "in_progress",
		UserID:         "user-1",
	}

	if err := svc.db.Create(&created).Error; err != nil {
		t.Fatalf("expected create to succeed: %v", err)
	}

	var fetched ActivityFeed
	if err := svc.db.First(&fetched, "id = ?", created.ID).Error; err != nil {
		t.Fatalf("expected read to succeed: %v", err)
	}

	if fetched.Category != created.Category {
		t.Fatalf("expected category %q, got %q", created.Category, fetched.Category)
	}
	if fetched.ProjectID == "" {
		t.Fatal("expected project_id to be populated")
	}
	if fetched.ProjectReason != created.ProjectReason {
		t.Fatalf("expected project_reason %q, got %q", created.ProjectReason, fetched.ProjectReason)
	}
	if fetched.Thinking != created.Thinking {
		t.Fatalf("expected thinking %q, got %q", created.Thinking, fetched.Thinking)
	}
	if fetched.CategoryReason != created.CategoryReason {
		t.Fatalf("expected category_reason %q, got %q", created.CategoryReason, fetched.CategoryReason)
	}
	if fetched.Reasoning != created.Reasoning {
		t.Fatalf("expected reasoning %t, got %t", created.Reasoning, fetched.Reasoning)
	}
	if fetched.Channel != created.Channel {
		t.Fatalf("expected channel %q, got %q", created.Channel, fetched.Channel)
	}
	if fetched.Status != created.Status {
		t.Fatalf("expected status %q, got %q", created.Status, fetched.Status)
	}
	if fetched.UserID != created.UserID {
		t.Fatalf("expected user_id %q, got %q", created.UserID, fetched.UserID)
	}
}

func TestProjectUpsertAndList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	project, err := adapter.UpsertProject(t.Context(), "clawtivity", "Clawtivity")
	if err != nil {
		t.Fatalf("expected upsert to succeed: %v", err)
	}
	if project.Slug != "clawtivity" {
		t.Fatalf("expected slug clawtivity, got %q", project.Slug)
	}

	// Idempotent upsert should not duplicate rows.
	_, err = adapter.UpsertProject(t.Context(), "clawtivity", "Clawtivity")
	if err != nil {
		t.Fatalf("expected idempotent upsert to succeed: %v", err)
	}

	projects, err := adapter.ListProjects(t.Context(), "active")
	if err != nil {
		t.Fatalf("expected list to succeed: %v", err)
	}
	if len(projects) < 1 {
		t.Fatalf("expected at least 1 project, got %d", len(projects))
	}
	found := false
	for _, project := range projects {
		if project.Slug == "clawtivity" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected clawtivity project to be listed")
	}
}

func TestNewSQLiteAdapterBackfillsLegacyProjectTagWithoutLock(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-clawtivity.db")

	legacyDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("expected legacy db open to succeed: %v", err)
	}

	if err := legacyDB.Exec(`
		CREATE TABLE activity_feed (
			id TEXT PRIMARY KEY,
			session_key TEXT,
			model TEXT,
			tokens_in INTEGER,
			tokens_out INTEGER,
			cost_estimate REAL,
			duration_ms INTEGER,
			project_tag TEXT,
			project_reason TEXT,
			external_ref TEXT,
			category TEXT,
			category_reason TEXT,
			thinking TEXT,
			reasoning NUMERIC,
			channel TEXT,
			status TEXT,
			user_id TEXT,
			created_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("expected legacy schema create to succeed: %v", err)
	}

	if err := legacyDB.Exec(`
		INSERT INTO activity_feed (
			id, session_key, model, tokens_in, tokens_out, cost_estimate, duration_ms,
			project_tag, project_reason, external_ref, category, category_reason,
			thinking, reasoning, channel, status, user_id, created_at
		) VALUES (
			'legacy-1', 'session-legacy', 'gpt-5', 10, 5, 0.0, 1000,
			'clawtivity', 'workspace_path', '', 'general', '',
			'medium', 0, 'webchat', 'success', 'legacy-user', datetime('now')
		)
	`).Error; err != nil {
		t.Fatalf("expected legacy row insert to succeed: %v", err)
	}

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter migration to succeed: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)

	var activity ActivityFeed
	if err := svc.db.First(&activity, "id = ?", "legacy-1").Error; err != nil {
		t.Fatalf("expected legacy activity to be readable: %v", err)
	}
	if activity.ProjectID == "" {
		t.Fatal("expected legacy activity project_id to be backfilled")
	}

	var project Project
	if err := svc.db.First(&project, "id = ?", activity.ProjectID).Error; err != nil {
		t.Fatalf("expected related project to exist: %v", err)
	}
	if project.Slug != "clawtivity" {
		t.Fatalf("expected slug clawtivity, got %q", project.Slug)
	}
}

func looksLikeUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for _, idx := range []int{8, 13, 18, 23} {
		if value[idx] != '-' {
			return false
		}
	}
	return true
}

func assertColumnsPresent(t *testing.T, svc *service, value any, required []string) {
	t.Helper()

	columnTypes, err := svc.db.Migrator().ColumnTypes(value)
	if err != nil {
		t.Fatalf("expected to inspect column types: %v", err)
	}

	seen := make(map[string]bool, len(columnTypes))
	for _, ct := range columnTypes {
		seen[ct.Name()] = true
	}

	for _, column := range required {
		if !seen[column] {
			t.Fatalf("expected column %q to exist", column)
		}
	}
}

func mustProjectID(t *testing.T, svc *service, slug string) string {
	t.Helper()

	project, err := svc.UpsertProject(t.Context(), slug, slug)
	if err != nil {
		t.Fatalf("expected project upsert to succeed: %v", err)
	}
	return project.ID
}
