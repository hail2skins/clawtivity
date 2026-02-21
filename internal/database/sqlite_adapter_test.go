package database

import (
	"path/filepath"
	"testing"
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

	if !svc.db.Migrator().HasIndex(&ActivityFeed{}, "idx_activity_feed_session_key") {
		t.Fatal("expected activity_feed.session_key index to exist")
	}
	if !svc.db.Migrator().HasIndex(&ActivityFeed{}, "idx_activity_feed_project_tag") {
		t.Fatal("expected activity_feed.project_tag index to exist")
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

	assertColumnsPresent(t, svc, &ActivityFeed{}, []string{
		"id",
		"session_key",
		"model",
		"tokens_in",
		"tokens_out",
		"cost_estimate",
		"duration_ms",
		"project_tag",
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
		ProjectTag:   "clawtivity",
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
		ProjectTag:     "clawtivity",
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
