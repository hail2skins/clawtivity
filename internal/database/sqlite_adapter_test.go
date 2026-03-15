package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNewSQLiteAdapterCreatesRequiredSchema(t *testing.T) {
	disableOpenRouterBootstrap(t)
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
	if !svc.db.Migrator().HasTable(&ModelPricing{}) {
		t.Fatal("expected model_pricing table to exist")
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
	if !svc.db.Migrator().HasIndex(&ModelPricing{}, "idx_model_pricing_lookup") {
		t.Fatal("expected model_pricing lookup index to exist")
	}
	if !svc.db.Migrator().HasIndex(&ModelPricing{}, "idx_model_pricing_source") {
		t.Fatal("expected model_pricing source index to exist")
	}
	if !svc.db.Migrator().HasIndex(&ModelPricing{}, "idx_model_pricing_stale") {
		t.Fatal("expected model_pricing stale index to exist")
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

	assertColumnsPresent(t, svc, &ModelPricing{}, []string{
		"id",
		"provider",
		"model",
		"effective_from",
		"input_cost_per_1m",
		"output_cost_per_1m",
		"reasoning_cost_per_1m",
		"currency",
		"source",
		"is_estimated",
		"is_stale",
		"last_verified_at",
		"verification_notes",
		"created_at",
		"updated_at",
	})
}

func TestSQLiteAdapterGeneratesUUIDPrimaryKeys(t *testing.T) {
	disableOpenRouterBootstrap(t)
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
	disableOpenRouterBootstrap(t)
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
	disableOpenRouterBootstrap(t)
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

func TestCreateActivityComputesReferenceCostEstimateFromLocalPricing(t *testing.T) {
	disableOpenRouterBootstrap(t)
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)
	activity := ActivityFeed{
		SessionKey:   "session-cost-1",
		Model:        "gpt-5",
		TokensIn:     120,
		TokensOut:    80,
		CostEstimate: 999.0,
		DurationMS:   1000,
		ProjectID:    mustProjectID(t, svc, "clawtivity"),
		ProjectTag:   "clawtivity",
		Category:     "general",
		Thinking:     "medium",
		Reasoning:    false,
		Channel:      "webchat",
		Status:       "success",
		UserID:       "u1",
	}

	if err := adapter.CreateActivity(t.Context(), &activity); err != nil {
		t.Fatalf("expected create activity to succeed: %v", err)
	}

	if !nearlyEqual(activity.CostEstimate, 0.0015) {
		t.Fatalf("expected computed cost_estimate 0.0015, got %.10f", activity.CostEstimate)
	}
}

func TestCreateActivityLeavesCostEstimateZeroWhenPricingUnknown(t *testing.T) {
	disableOpenRouterBootstrap(t)
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)
	activity := ActivityFeed{
		SessionKey:   "session-cost-2",
		Model:        "unknown-provider/unknown-model",
		TokensIn:     100,
		TokensOut:    50,
		CostEstimate: 9.99,
		DurationMS:   1000,
		ProjectID:    mustProjectID(t, svc, "clawtivity"),
		ProjectTag:   "clawtivity",
		Category:     "general",
		Thinking:     "medium",
		Reasoning:    false,
		Channel:      "webchat",
		Status:       "success",
		UserID:       "u1",
	}

	if err := adapter.CreateActivity(t.Context(), &activity); err != nil {
		t.Fatalf("expected create activity to succeed: %v", err)
	}

	if activity.CostEstimate != 0 {
		t.Fatalf("expected unknown pricing to yield cost_estimate 0, got %.10f", activity.CostEstimate)
	}
}

func TestCreateActivityComputesReferenceCostEstimateFromProviderQualifiedPricing(t *testing.T) {
	disableOpenRouterBootstrap(t)
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)
	lastVerifiedAt := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	pricing := ModelPricing{
		Provider:          "openrouter",
		Model:             "openai/gpt-5.4",
		EffectiveFrom:     time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		InputCostPer1M:    2.5,
		OutputCostPer1M:   15.0,
		Currency:          "USD",
		Source:            openRouterModelsAPIURL,
		LastVerifiedAt:    &lastVerifiedAt,
		VerificationNotes: "provider-qualified alias test",
	}
	if err := svc.db.Create(&pricing).Error; err != nil {
		t.Fatalf("expected provider-qualified pricing row to insert: %v", err)
	}

	activity := ActivityFeed{
		SessionKey: "session-cost-3",
		Model:      "gpt-5.4",
		TokensIn:   446319,
		TokensOut:  904,
		DurationMS: 1000,
		ProjectID:  mustProjectID(t, svc, "clawtivity"),
		ProjectTag: "clawtivity",
		Category:   "general",
		Thinking:   "medium",
		Reasoning:  false,
		Channel:    "webchat",
		Status:     "success",
		UserID:     "u1",
	}

	if err := adapter.CreateActivity(t.Context(), &activity); err != nil {
		t.Fatalf("expected create activity to succeed: %v", err)
	}

	if !nearlyEqual(activity.CostEstimate, 1.1293575) {
		t.Fatalf("expected computed cost_estimate 1.1293575, got %.10f", activity.CostEstimate)
	}
}

func TestCreateActivityComputesReferenceCostEstimateFromOpenAICodexMiniAlias(t *testing.T) {
	disableOpenRouterBootstrap(t)
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)
	activity := ActivityFeed{
		SessionKey: "session-cost-4",
		Model:      "gpt-5-codex-mini",
		TokensIn:   471355,
		TokensOut:  6527,
		DurationMS: 1000,
		ProjectID:  mustProjectID(t, svc, "clawtivity"),
		ProjectTag: "clawtivity",
		Category:   "general",
		Thinking:   "medium",
		Reasoning:  false,
		Channel:    "webchat",
		Status:     "success",
		UserID:     "u1",
	}

	if err := adapter.CreateActivity(t.Context(), &activity); err != nil {
		t.Fatalf("expected create activity to succeed: %v", err)
	}

	if !nearlyEqual(activity.CostEstimate, 0.13089275) {
		t.Fatalf("expected computed cost_estimate 0.13089275, got %.10f", activity.CostEstimate)
	}
}

func TestNewSQLiteAdapterSeedsReferenceModelPricingCatalog(t *testing.T) {
	disableOpenRouterBootstrap(t)
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)

	var prices []ModelPricing
	if err := svc.db.Order("provider asc, model asc").Find(&prices).Error; err != nil {
		t.Fatalf("expected seeded pricing rows to be queryable: %v", err)
	}
	if len(prices) == 0 {
		t.Fatal("expected seeded model pricing rows")
	}
	if len(prices) != 4 {
		t.Fatalf("expected 4 seeded pricing rows without bootstrap import, got %d", len(prices))
	}

	assertSeededModelPricing(t, prices, "openai", "gpt-5", 2.50, 15.0, false)
	assertSeededModelPricing(t, prices, "openrouter", "moonshotai/kimi-k2.5", 0.45, 2.20, false)
	assertSeededModelPricing(t, prices, "openrouter", "moonshotai/kimi-k2-thinking", 0.47, 2.0, false)
}

func TestNewSQLiteAdapterSeedsModelPricingIdempotently(t *testing.T) {
	disableOpenRouterBootstrap(t)
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	first, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected first adapter init to succeed: %v", err)
	}

	svc1 := first.(*service)
	var firstCount int64
	if err := svc1.db.Model(&ModelPricing{}).Count(&firstCount).Error; err != nil {
		t.Fatalf("expected first count query to succeed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("expected first adapter close to succeed: %v", err)
	}

	second, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected second adapter init to succeed: %v", err)
	}
	t.Cleanup(func() {
		_ = second.Close()
	})

	svc2 := second.(*service)
	var secondCount int64
	if err := svc2.db.Model(&ModelPricing{}).Count(&secondCount).Error; err != nil {
		t.Fatalf("expected second count query to succeed: %v", err)
	}

	if firstCount != secondCount {
		t.Fatalf("expected seeded model pricing count to remain stable, got %d then %d", firstCount, secondCount)
	}
}

func TestBootstrapOpenRouterModelPricingImportsCatalogWhenMissing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	originalFetch := openRouterModelsFetcher
	openRouterModelsFetcher = func(context.Context) ([]ModelPricing, error) {
		return nil, nil
	}
	t.Cleanup(func() {
		openRouterModelsFetcher = originalFetch
	})

	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)

	openRouterModelsFetcher = func(context.Context) ([]ModelPricing, error) {
		now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
		reasoning := 3.5
		return []ModelPricing{
			{
				Provider:           "openrouter",
				Model:              "moonshotai/kimi-k2.5",
				EffectiveFrom:      now,
				InputCostPer1M:     2.0,
				OutputCostPer1M:    6.0,
				ReasoningCostPer1M: &reasoning,
				Currency:           "USD",
				Source:             openRouterModelsAPIURL,
				IsEstimated:        false,
				LastVerifiedAt:     &now,
				VerificationNotes:  "Imported from OpenRouter models endpoint.",
			},
			{
				Provider:          "openrouter",
				Model:             "openrouter/hunter-alpha",
				EffectiveFrom:     now,
				InputCostPer1M:    0,
				OutputCostPer1M:   0,
				Currency:          "USD",
				Source:            openRouterModelsAPIURL,
				IsEstimated:       false,
				LastVerifiedAt:    &now,
				VerificationNotes: "Imported from OpenRouter models endpoint.",
			},
		}, nil
	}
	if err := bootstrapOpenRouterModelPricing(context.Background(), svc.db); err != nil {
		t.Fatalf("expected bootstrap import to succeed: %v", err)
	}

	var prices []ModelPricing
	if err := svc.db.Where("provider = ?", "openrouter").Order("model asc").Find(&prices).Error; err != nil {
		t.Fatalf("expected imported rows to be queryable: %v", err)
	}
	if len(prices) != 3 {
		t.Fatalf("expected 3 openrouter rows after import upsert, got %d", len(prices))
	}
	assertSeededModelPricing(t, prices, "openrouter", "moonshotai/kimi-k2.5", 2.0, 6.0, false)
	assertSeededModelPricing(t, prices, "openrouter", "openrouter/hunter-alpha", 0.0, 0.0, false)

	var imported ModelPricing
	if err := svc.db.Where("provider = ? AND model = ?", "openrouter", "moonshotai/kimi-k2.5").First(&imported).Error; err != nil {
		t.Fatalf("expected imported kimi row to exist: %v", err)
	}
	if imported.Source != openRouterModelsAPIURL {
		t.Fatalf("expected imported source %q, got %q", openRouterModelsAPIURL, imported.Source)
	}
}

func TestBootstrapOpenRouterModelPricingSkipsWhenImportedRowsAlreadyExist(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	disableOpenRouterBootstrap(t)
	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)

	called := false
	originalFetch := openRouterModelsFetcher
	openRouterModelsFetcher = func(context.Context) ([]ModelPricing, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() {
		openRouterModelsFetcher = originalFetch
	})

	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	imported := ModelPricing{
		Provider:          "openrouter",
		Model:             "x-ai/grok-4.20-beta",
		EffectiveFrom:     now,
		InputCostPer1M:    2.0,
		OutputCostPer1M:   6.0,
		Currency:          "USD",
		Source:            openRouterModelsAPIURL,
		IsEstimated:       false,
		LastVerifiedAt:    &now,
		VerificationNotes: "Imported from OpenRouter models endpoint.",
	}
	if err := svc.db.Create(&imported).Error; err != nil {
		t.Fatalf("expected imported row create to succeed: %v", err)
	}

	if err := bootstrapOpenRouterModelPricing(context.Background(), svc.db); err != nil {
		t.Fatalf("expected bootstrap skip to succeed: %v", err)
	}
	if called {
		t.Fatal("expected bootstrap import to skip fetch when imported openrouter rows already exist")
	}
}

func TestRefreshOpenRouterModelPricingIfDueRefreshesStaleCatalog(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	disableOpenRouterBootstrap(t)
	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)
	oldVerified := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	existing := ModelPricing{
		Provider:          "openrouter",
		Model:             "moonshotai/kimi-k2.5",
		EffectiveFrom:     oldVerified,
		InputCostPer1M:    0.45,
		OutputCostPer1M:   2.2,
		Currency:          "USD",
		Source:            openRouterModelsAPIURL,
		IsEstimated:       false,
		LastVerifiedAt:    &oldVerified,
		VerificationNotes: "stale import",
	}
	if err := svc.db.Create(&existing).Error; err != nil {
		t.Fatalf("expected stale imported row create to succeed: %v", err)
	}

	originalFetch := openRouterModelsFetcher
	openRouterModelsFetcher = func(context.Context) ([]ModelPricing, error) {
		now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
		return []ModelPricing{
			{
				Provider:          "openrouter",
				Model:             "moonshotai/kimi-k2.5",
				EffectiveFrom:     now,
				InputCostPer1M:    0.5,
				OutputCostPer1M:   2.4,
				Currency:          "USD",
				Source:            openRouterModelsAPIURL,
				IsEstimated:       false,
				LastVerifiedAt:    &now,
				VerificationNotes: "weekly refresh",
			},
			{
				Provider:          "openrouter",
				Model:             "openai/gpt-5.4",
				EffectiveFrom:     now,
				InputCostPer1M:    2.5,
				OutputCostPer1M:   15.0,
				Currency:          "USD",
				Source:            openRouterModelsAPIURL,
				IsEstimated:       false,
				LastVerifiedAt:    &now,
				VerificationNotes: "weekly refresh",
			},
		}, nil
	}
	t.Cleanup(func() {
		openRouterModelsFetcher = originalFetch
	})

	config := pricingRefreshConfig{
		Enabled:    true,
		Interval:   7 * 24 * time.Hour,
		StaleAfter: 7 * 24 * time.Hour,
	}
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	if err := refreshOpenRouterModelPricingIfDue(context.Background(), svc.db, now, config); err != nil {
		t.Fatalf("expected weekly refresh to succeed: %v", err)
	}

	var refreshed ModelPricing
	if err := svc.db.Where("provider = ? AND model = ?", "openrouter", "moonshotai/kimi-k2.5").First(&refreshed).Error; err != nil {
		t.Fatalf("expected refreshed kimi row to exist: %v", err)
	}
	if refreshed.InputCostPer1M != 0.5 || refreshed.OutputCostPer1M != 2.4 {
		t.Fatalf("expected refreshed kimi pricing, got %f/%f", refreshed.InputCostPer1M, refreshed.OutputCostPer1M)
	}
	if refreshed.IsStale {
		t.Fatal("expected refreshed kimi row to be marked fresh")
	}

	var added ModelPricing
	if err := svc.db.Where("provider = ? AND model = ?", "openrouter", "openai/gpt-5.4").First(&added).Error; err != nil {
		t.Fatalf("expected new model to be imported during refresh: %v", err)
	}
}

func TestRefreshOpenRouterModelPricingIfDueSkipsFreshCatalog(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	disableOpenRouterBootstrap(t)
	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	fresh := ModelPricing{
		Provider:          "openrouter",
		Model:             "moonshotai/kimi-k2.5",
		EffectiveFrom:     now,
		InputCostPer1M:    0.45,
		OutputCostPer1M:   2.2,
		Currency:          "USD",
		Source:            openRouterModelsAPIURL,
		IsEstimated:       false,
		LastVerifiedAt:    &now,
		VerificationNotes: "fresh import",
	}
	if err := svc.db.Create(&fresh).Error; err != nil {
		t.Fatalf("expected fresh imported row create to succeed: %v", err)
	}

	called := false
	originalFetch := openRouterModelsFetcher
	openRouterModelsFetcher = func(context.Context) ([]ModelPricing, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() {
		openRouterModelsFetcher = originalFetch
	})

	config := pricingRefreshConfig{
		Enabled:    true,
		Interval:   7 * 24 * time.Hour,
		StaleAfter: 7 * 24 * time.Hour,
	}
	if err := refreshOpenRouterModelPricingIfDue(context.Background(), svc.db, now, config); err != nil {
		t.Fatalf("expected fresh catalog check to succeed: %v", err)
	}
	if called {
		t.Fatal("expected refresh fetch to be skipped for fresh catalog")
	}
}

func TestRefreshOpenRouterModelPricingIfDueMarksRowsStaleOnFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "clawtivity.db")

	disableOpenRouterBootstrap(t)
	adapter, err := NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("expected adapter to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = adapter.Close()
	})

	svc := adapter.(*service)
	oldVerified := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	staleCandidate := ModelPricing{
		Provider:          "openrouter",
		Model:             "moonshotai/kimi-k2.5",
		EffectiveFrom:     oldVerified,
		InputCostPer1M:    0.45,
		OutputCostPer1M:   2.2,
		Currency:          "USD",
		Source:            openRouterModelsAPIURL,
		IsEstimated:       false,
		LastVerifiedAt:    &oldVerified,
		VerificationNotes: "stale import",
	}
	if err := svc.db.Create(&staleCandidate).Error; err != nil {
		t.Fatalf("expected stale imported row create to succeed: %v", err)
	}

	originalFetch := openRouterModelsFetcher
	openRouterModelsFetcher = func(context.Context) ([]ModelPricing, error) {
		return nil, context.DeadlineExceeded
	}
	t.Cleanup(func() {
		openRouterModelsFetcher = originalFetch
	})

	config := pricingRefreshConfig{
		Enabled:    true,
		Interval:   7 * 24 * time.Hour,
		StaleAfter: 7 * 24 * time.Hour,
	}
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	if err := refreshOpenRouterModelPricingIfDue(context.Background(), svc.db, now, config); err == nil {
		t.Fatal("expected refresh failure to be returned")
	}

	var refreshed ModelPricing
	if err := svc.db.Where("provider = ? AND model = ?", "openrouter", "moonshotai/kimi-k2.5").First(&refreshed).Error; err != nil {
		t.Fatalf("expected stale row lookup to succeed: %v", err)
	}
	if !refreshed.IsStale {
		t.Fatal("expected refresh failure to mark stale imported row as stale")
	}
}

func TestNewSQLiteAdapterBackfillsLegacyProjectTagWithoutLock(t *testing.T) {
	disableOpenRouterBootstrap(t)
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

func assertSeededModelPricing(t *testing.T, prices []ModelPricing, provider, model string, wantInput, wantOutput float64, wantEstimated bool) {
	t.Helper()

	for _, price := range prices {
		if price.Provider != provider || price.Model != model {
			continue
		}
		if !nearlyEqual(price.InputCostPer1M, wantInput) {
			t.Fatalf("expected %s/%s input cost %.2f, got %.2f", provider, model, wantInput, price.InputCostPer1M)
		}
		if !nearlyEqual(price.OutputCostPer1M, wantOutput) {
			t.Fatalf("expected %s/%s output cost %.2f, got %.2f", provider, model, wantOutput, price.OutputCostPer1M)
		}
		if price.IsEstimated != wantEstimated {
			t.Fatalf("expected %s/%s is_estimated %t, got %t", provider, model, wantEstimated, price.IsEstimated)
		}
		if price.Currency != "USD" {
			t.Fatalf("expected %s/%s currency USD, got %q", provider, model, price.Currency)
		}
		if price.Source == "" {
			t.Fatalf("expected %s/%s source to be populated", provider, model)
		}
		if price.LastVerifiedAt == nil || price.LastVerifiedAt.IsZero() {
			t.Fatalf("expected %s/%s last_verified_at to be populated", provider, model)
		}
		return
	}

	t.Fatalf("expected seeded model pricing row for %s/%s", provider, model)
}

func disableOpenRouterBootstrap(t *testing.T) {
	t.Helper()

	originalFetch := openRouterModelsFetcher
	openRouterModelsFetcher = func(context.Context) ([]ModelPricing, error) {
		return nil, nil
	}
	t.Cleanup(func() {
		openRouterModelsFetcher = originalFetch
	})
}

func nearlyEqual(a, b float64) bool {
	const epsilon = 0.000001
	if a > b {
		return a-b < epsilon
	}
	return b-a < epsilon
}
