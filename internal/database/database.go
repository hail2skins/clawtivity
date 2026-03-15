package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var ErrInvalidDateFilter = errors.New("invalid date filter: expected YYYY-MM-DD")

// ActivityFeed is the local-first event ledger entry.
type ActivityFeed struct {
	ID               string    `gorm:"type:char(36);primaryKey" json:"id"`
	SessionKey       string    `gorm:"index:idx_activity_feed_session_key" json:"session_key"`
	Model            string    `json:"model"`
	TokensIn         int       `json:"tokens_in"`
	TokensOut        int       `json:"tokens_out"`
	CostEstimate     float64   `json:"cost_estimate"`
	DurationMS       int64     `json:"duration_ms"`
	ProjectID        string    `gorm:"type:char(36);index:idx_activity_feed_project_id" json:"project_id"`
	Project          Project   `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"-"`
	LegacyProjectTag string    `gorm:"column:project_tag" json:"-"`
	ProjectTag       string    `gorm:"-" json:"project_tag"`
	ProjectReason    string    `json:"project_reason"`
	ExternalRef      string    `json:"external_ref"`
	Category         string    `gorm:"index:idx_activity_feed_category" json:"category"`
	CategoryReason   string    `json:"category_reason"`
	Thinking         string    `json:"thinking"`
	Reasoning        bool      `json:"reasoning"`
	Channel          string    `json:"channel"`
	Status           string    `gorm:"index:idx_activity_feed_status" json:"status"`
	UserID           string    `gorm:"index:idx_activity_feed_user_id" json:"user_id"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (ActivityFeed) TableName() string {
	return "activity_feed"
}

func (a *ActivityFeed) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = generateUUIDv4()
	}
	return nil
}

// TurnMemory stores compact summaries of a session turn.
type TurnMemory struct {
	ID             string    `gorm:"type:char(36);primaryKey" json:"id"`
	SessionKey     string    `gorm:"index:idx_turn_memories_session_key" json:"session_key"`
	Summary        string    `json:"summary"`
	ToolsUsed      string    `gorm:"type:json" json:"tools_used"`
	FilesTouched   string    `gorm:"type:json" json:"files_touched"`
	KeyDecisions   string    `gorm:"type:json" json:"key_decisions"`
	ContextSnippet string    `json:"context_snippet"`
	Tags           string    `gorm:"type:json" json:"tags"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (TurnMemory) TableName() string {
	return "turn_memories"
}

func (m *TurnMemory) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = generateUUIDv4()
	}
	return nil
}

// Project stores known project tags for deterministic project assignment.
type Project struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	Slug        string    `gorm:"uniqueIndex:idx_projects_slug" json:"slug"`
	DisplayName string    `json:"display_name"`
	Status      string    `gorm:"index:idx_projects_status" json:"status"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Project) TableName() string {
	return "projects"
}

func (p *Project) BeforeCreate(_ *gorm.DB) error {
	if p.ID == "" {
		p.ID = generateUUIDv4()
	}
	return nil
}

// ModelPricing stores local reference pricing for provider/model pairs.
type ModelPricing struct {
	ID                  string     `gorm:"type:char(36);primaryKey" json:"id"`
	Provider            string     `gorm:"uniqueIndex:idx_model_pricing_lookup,priority:1;index:idx_model_pricing_source" json:"provider"`
	Model               string     `gorm:"uniqueIndex:idx_model_pricing_lookup,priority:2" json:"model"`
	EffectiveFrom       time.Time  `gorm:"uniqueIndex:idx_model_pricing_lookup,priority:3" json:"effective_from"`
	InputCostPer1M      float64    `gorm:"column:input_cost_per_1m" json:"input_cost_per_1m"`
	OutputCostPer1M     float64    `gorm:"column:output_cost_per_1m" json:"output_cost_per_1m"`
	ReasoningCostPer1M  *float64   `gorm:"column:reasoning_cost_per_1m" json:"reasoning_cost_per_1m,omitempty"`
	Currency            string     `json:"currency"`
	Source              string     `gorm:"index:idx_model_pricing_source" json:"source"`
	IsEstimated         bool       `json:"is_estimated"`
	IsStale             bool       `gorm:"index:idx_model_pricing_stale" json:"is_stale"`
	LastVerifiedAt      *time.Time `gorm:"column:last_verified_at" json:"last_verified_at,omitempty"`
	VerificationNotes   string     `gorm:"column:verification_notes" json:"verification_notes"`
	CreatedAt           time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (ModelPricing) TableName() string {
	return "model_pricing"
}

func (m *ModelPricing) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = generateUUIDv4()
	}
	return nil
}

// Service represents a service that interacts with a database.
type Service interface {
	// Health returns a map of health status information.
	Health() map[string]string

	CreateActivity(ctx context.Context, activity *ActivityFeed) error
	ListActivities(ctx context.Context, filters ActivityFilters) ([]ActivityFeed, error)
	SummarizeActivities(ctx context.Context, filters ActivityFilters) (ActivitySummary, error)
	UpsertProject(ctx context.Context, slug, displayName string) (Project, error)
	ListProjects(ctx context.Context, status string) ([]Project, error)
	ListProjectsWithStats(ctx context.Context, status string) ([]ProjectSummary, error)
	ListModelPricing(ctx context.Context, provider string) ([]ModelPricing, error)
	ResolveReferenceCost(ctx context.Context, model string, tokensIn, tokensOut int) (float64, bool, error)

	// Close terminates the database connection.
	Close() error
}

type ActivityFilters struct {
	ProjectTag string
	Model      string
	Date       string
}

type ActivitySummary struct {
	Count           int64          `gorm:"column:count" json:"count"`
	TokensInTotal   int64          `gorm:"column:tokens_in_total" json:"tokens_in_total"`
	TokensOutTotal  int64          `gorm:"column:tokens_out_total" json:"tokens_out_total"`
	CostTotal       float64        `gorm:"column:cost_total" json:"cost_total"`
	DurationMSTotal int64          `gorm:"column:duration_ms_total" json:"duration_ms_total"`
	ByStatus        map[string]int `json:"by_status"`
}

type ProjectSummary struct {
	ID             string    `json:"id"`
	Slug           string    `json:"slug"`
	DisplayName    string    `json:"display_name"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ActivityCount  int64     `json:"activity_count"`
	TokensInTotal  int64     `json:"tokens_in_total"`
	TokensOutTotal int64     `json:"tokens_out_total"`
	CostTotal      float64   `json:"cost_total"`
}

type service struct {
	db                   *gorm.DB
	sqlDB                *sql.DB
	dsn                  string
	pricingRefreshCancel context.CancelFunc
}

type seededModelPricing struct {
	Provider           string   `json:"provider"`
	Model              string   `json:"model"`
	EffectiveFrom      string   `json:"effective_from"`
	InputCostPer1M     float64  `json:"input_cost_per_1m"`
	OutputCostPer1M    float64  `json:"output_cost_per_1m"`
	ReasoningCostPer1M *float64 `json:"reasoning_cost_per_1m"`
	Currency           string   `json:"currency"`
	Source             string   `json:"source"`
	IsEstimated        bool     `json:"is_estimated"`
	LastVerifiedAt     string   `json:"last_verified_at"`
	VerificationNotes  string   `json:"verification_notes"`
}

//go:embed model_pricing_seed.json
var modelPricingSeedJSON []byte

const openRouterModelsAPIURL = "https://openrouter.ai/api/v1/models"

var openRouterHTTPClient = &http.Client{Timeout: 15 * time.Second}
var openRouterModelsFetcher = fetchOpenRouterModelPricing

type pricingRefreshConfig struct {
	Enabled    bool
	Interval   time.Duration
	StaleAfter time.Duration
}

func New() (Service, error) {
	return newSQLiteService(os.Getenv("BLUEPRINT_DB_URL"))
}

func NewSQLiteAdapter(dsn string) (Service, error) {
	return newSQLiteService(dsn)
}

func newSQLiteService(dsn string) (*service, error) {
	if dsn == "" {
		dsn = "./test.db"
	}

	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	legacyProjectTags, err := loadLegacyProjectTags(context.Background(), gormDB)
	if err != nil {
		return nil, err
	}

	if err := gormDB.AutoMigrate(&Project{}, &ActivityFeed{}, &TurnMemory{}, &ModelPricing{}); err != nil {
		return nil, err
	}

	if err := seedModelPricingCatalog(context.Background(), gormDB); err != nil {
		return nil, err
	}
	if err := bootstrapOpenRouterModelPricing(context.Background(), gormDB); err != nil {
		log.Printf("openrouter pricing bootstrap skipped: %v", err)
	}
	refreshConfig := resolvePricingRefreshConfig()
	if err := refreshOpenRouterModelPricingIfDue(context.Background(), gormDB, time.Now().UTC(), refreshConfig); err != nil {
		log.Printf("openrouter pricing refresh skipped: %v", err)
	}

	if err := synchronizeActivityProjectIDs(context.Background(), gormDB, legacyProjectTags); err != nil {
		return nil, err
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}

	svc := &service{db: gormDB, sqlDB: sqlDB, dsn: dsn}
	svc.startPricingRefreshWorker(refreshConfig)

	return svc, nil
}

// Health checks the health of the database connection by pinging the database.
func (s *service) Health() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	stats := make(map[string]string)

	err := s.sqlDB.PingContext(ctx)
	if err != nil {
		stats["status"] = "down"
		stats["error"] = fmt.Sprintf("db down: %v", err)
		return stats
	}

	stats["status"] = "up"
	stats["message"] = "It's healthy"

	dbStats := s.sqlDB.Stats()
	stats["open_connections"] = strconv.Itoa(dbStats.OpenConnections)
	stats["in_use"] = strconv.Itoa(dbStats.InUse)
	stats["idle"] = strconv.Itoa(dbStats.Idle)
	stats["wait_count"] = strconv.FormatInt(dbStats.WaitCount, 10)
	stats["wait_duration"] = dbStats.WaitDuration.String()
	stats["max_idle_closed"] = strconv.FormatInt(dbStats.MaxIdleClosed, 10)
	stats["max_lifetime_closed"] = strconv.FormatInt(dbStats.MaxLifetimeClosed, 10)

	if dbStats.OpenConnections > 40 {
		stats["message"] = "The database is experiencing heavy load."
	}
	if dbStats.WaitCount > 1000 {
		stats["message"] = "The database has a high number of wait events, indicating potential bottlenecks."
	}
	if dbStats.MaxIdleClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many idle connections are being closed, consider revising the connection pool settings."
	}
	if dbStats.MaxLifetimeClosed > int64(dbStats.OpenConnections)/2 {
		stats["message"] = "Many connections are being closed due to max lifetime, consider increasing max lifetime or revising the connection usage pattern."
	}

	return stats
}

func (s *service) CreateActivity(ctx context.Context, activity *ActivityFeed) error {
	if strings.TrimSpace(activity.ProjectID) == "" {
		return errors.New("project_id is required")
	}
	costEstimate, matched, err := s.ResolveReferenceCost(ctx, activity.Model, activity.TokensIn, activity.TokensOut)
	if err != nil {
		return err
	}
	if matched {
		activity.CostEstimate = costEstimate
	} else {
		activity.CostEstimate = 0
	}
	activity.LegacyProjectTag = strings.TrimSpace(strings.ToLower(activity.ProjectTag))
	return s.db.WithContext(ctx).Create(activity).Error
}

func (s *service) ListActivities(ctx context.Context, filters ActivityFilters) ([]ActivityFeed, error) {
	tx, err := applyActivityFilters(s.db.WithContext(ctx).Model(&ActivityFeed{}), filters)
	if err != nil {
		return nil, err
	}

	var activities []ActivityFeed
	if err := tx.Preload("Project").Order("activity_feed.created_at desc").Find(&activities).Error; err != nil {
		return nil, err
	}
	populateProjectTags(activities)
	return activities, nil
}

func (s *service) SummarizeActivities(ctx context.Context, filters ActivityFilters) (ActivitySummary, error) {
	tx, err := applyActivityFilters(s.db.WithContext(ctx).Model(&ActivityFeed{}), filters)
	if err != nil {
		return ActivitySummary{}, err
	}

	// Use a temp struct to avoid GORM trying to map to ByStatus map field
	var result struct {
		Count           int64   `gorm:"column:count" json:"count"`
		TokensInTotal   int64   `gorm:"column:tokens_in_total" json:"tokens_in_total"`
		TokensOutTotal  int64   `gorm:"column:tokens_out_total" json:"tokens_out_total"`
		CostTotal       float64 `gorm:"column:cost_total" json:"cost_total"`
		DurationMSTotal int64   `gorm:"column:duration_ms_total" json:"duration_ms_total"`
	}
	if err := tx.Select(
		"COUNT(*) AS count, " +
			"COALESCE(SUM(activity_feed.tokens_in), 0) AS tokens_in_total, " +
			"COALESCE(SUM(activity_feed.tokens_out), 0) AS tokens_out_total, " +
			"COALESCE(SUM(activity_feed.cost_estimate), 0) AS cost_total, " +
			"COALESCE(SUM(activity_feed.duration_ms), 0) AS duration_ms_total",
	).Scan(&result).Error; err != nil {
		return ActivitySummary{}, err
	}

	summary := ActivitySummary{
		Count:           result.Count,
		TokensInTotal:   result.TokensInTotal,
		TokensOutTotal:  result.TokensOutTotal,
		CostTotal:       result.CostTotal,
		DurationMSTotal: result.DurationMSTotal,
	}

	statusTx, err := applyActivityFilters(s.db.WithContext(ctx).Model(&ActivityFeed{}), filters)
	if err != nil {
		return ActivitySummary{}, err
	}

	var grouped []struct {
		Status string
		Count  int64
	}
	if err := statusTx.Select("activity_feed.status AS status, COUNT(*) AS count").Group("activity_feed.status").Scan(&grouped).Error; err != nil {
		return ActivitySummary{}, err
	}

	summary.ByStatus = make(map[string]int, len(grouped))
	for _, row := range grouped {
		if row.Status == "" {
			continue
		}
		summary.ByStatus[row.Status] = int(row.Count)
	}

	return summary, nil
}

func (s *service) UpsertProject(ctx context.Context, slug, displayName string) (Project, error) {
	normalizedSlug := normalizeProjectSlug(slug)
	if normalizedSlug == "" {
		return Project{}, errors.New("project slug cannot be empty")
	}

	project := Project{
		Slug:        normalizedSlug,
		DisplayName: strings.TrimSpace(displayName),
		Status:      "active",
	}
	if project.DisplayName == "" {
		project.DisplayName = normalizedSlug
	}

	if err := s.db.WithContext(ctx).Where("slug = ?", normalizedSlug).FirstOrCreate(&project).Error; err != nil {
		return Project{}, err
	}

	if strings.TrimSpace(displayName) != "" && project.DisplayName != strings.TrimSpace(displayName) {
		project.DisplayName = strings.TrimSpace(displayName)
		if err := s.db.WithContext(ctx).Save(&project).Error; err != nil {
			return Project{}, err
		}
	}

	return project, nil
}

func (s *service) ListProjects(ctx context.Context, status string) ([]Project, error) {
	tx := s.db.WithContext(ctx).Model(&Project{})
	if trimmed := strings.TrimSpace(status); trimmed != "" {
		tx = tx.Where("status = ?", trimmed)
	}

	var projects []Project
	if err := tx.Order("slug asc").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (s *service) ListProjectsWithStats(ctx context.Context, status string) ([]ProjectSummary, error) {
	tx := s.db.WithContext(ctx).
		Table("projects AS p").
		Select(
			"p.id, p.slug, p.display_name, p.status, p.created_at, p.updated_at, " +
				"COUNT(a.id) AS activity_count, " +
				"COALESCE(SUM(a.tokens_in), 0) AS tokens_in_total, " +
				"COALESCE(SUM(a.tokens_out), 0) AS tokens_out_total, " +
				"COALESCE(SUM(a.cost_estimate), 0) AS cost_total",
		).
		Joins("LEFT JOIN activity_feed AS a ON a.project_id = p.id").
		Group("p.id, p.slug, p.display_name, p.status, p.created_at, p.updated_at").
		Order("p.slug asc")

	if trimmed := strings.TrimSpace(status); trimmed != "" {
		tx = tx.Where("p.status = ?", trimmed)
	}

	var rows []ProjectSummary
	if err := tx.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *service) ListModelPricing(ctx context.Context, provider string) ([]ModelPricing, error) {
	tx := s.db.WithContext(ctx).Model(&ModelPricing{})
	if trimmed := strings.TrimSpace(provider); trimmed != "" {
		tx = tx.Where("provider = ?", strings.ToLower(trimmed))
	}

	var rows []ModelPricing
	if err := tx.Order("provider asc, model asc, effective_from desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *service) ResolveReferenceCost(ctx context.Context, model string, tokensIn, tokensOut int) (float64, bool, error) {
	pricing, matched, err := s.lookupReferencePricing(ctx, model)
	if err != nil || !matched {
		return 0, false, err
	}

	inputCost := (float64(tokensIn) / 1_000_000) * pricing.InputCostPer1M
	outputCost := (float64(tokensOut) / 1_000_000) * pricing.OutputCostPer1M

	return inputCost + outputCost, true, nil
}

// Close closes the database connection.
func (s *service) Close() error {
	if s.pricingRefreshCancel != nil {
		s.pricingRefreshCancel()
	}
	log.Printf("Disconnected from database: %s", s.dsn)
	return s.sqlDB.Close()
}

func (s *service) startPricingRefreshWorker(config pricingRefreshConfig) {
	if !config.Enabled || config.Interval <= 0 {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.pricingRefreshCancel = cancel

	ticker := time.NewTicker(config.Interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := refreshOpenRouterModelPricingIfDue(ctx, s.db, time.Now().UTC(), config); err != nil {
					log.Printf("openrouter pricing refresh failed: %v", err)
				}
			}
		}
	}()
}

func resolvePricingRefreshConfig() pricingRefreshConfig {
	interval := resolveDurationEnv("CLAWTIVITY_PRICING_REFRESH_INTERVAL", 7*24*time.Hour)
	staleAfter := resolveDurationEnv("CLAWTIVITY_PRICING_STALE_AFTER", interval)
	if staleAfter < interval {
		staleAfter = interval
	}

	enabled := true
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAWTIVITY_PRICING_REFRESH_ENABLED"))) {
	case "0", "false", "no", "off":
		enabled = false
	}

	return pricingRefreshConfig{
		Enabled:    enabled,
		Interval:   interval,
		StaleAfter: staleAfter,
	}
}

func resolveDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func applyActivityFilters(tx *gorm.DB, filters ActivityFilters) (*gorm.DB, error) {
	if filters.ProjectTag != "" {
		tx = tx.Joins("JOIN projects ON projects.id = activity_feed.project_id")
		tx = tx.Where("projects.slug = ?", normalizeProjectSlug(filters.ProjectTag))
	}
	if filters.Model != "" {
		tx = tx.Where("activity_feed.model = ?", filters.Model)
	}
	if filters.Date != "" {
		start, err := time.Parse("2006-01-02", filters.Date)
		if err != nil {
			return nil, ErrInvalidDateFilter
		}
		end := start.Add(24 * time.Hour)
		tx = tx.Where("activity_feed.created_at >= ? AND activity_feed.created_at < ?", start, end)
	}
	return tx, nil
}

func generateUUIDv4() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	)
}

func normalizeProjectSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func populateProjectTags(activities []ActivityFeed) {
	for i := range activities {
		if activities[i].Project.Slug != "" {
			activities[i].ProjectTag = activities[i].Project.Slug
		}
	}
}

func synchronizeActivityProjectIDs(ctx context.Context, db *gorm.DB, legacyProjectTags map[string]string) error {
	workspace, err := upsertProjectRecord(ctx, db, "workspace", "workspace")
	if err != nil {
		return err
	}

	for activityID, rawTag := range legacyProjectTags {
		normalized := normalizeProjectSlug(rawTag)
		if normalized == "" {
			normalized = "workspace"
		}
		project, err := upsertProjectRecord(ctx, db, normalized, normalized)
		if err != nil {
			return err
		}
		if err := db.WithContext(ctx).Exec(
			`UPDATE activity_feed
			 SET project_id = ?
			 WHERE id = ?
			   AND (project_id IS NULL OR TRIM(project_id) = '')`,
			project.ID,
			activityID,
		).Error; err != nil {
			return err
		}
	}

	if tableHasColumn(ctx, db, "activity_feed", "project_tag") {
		var tagRows []struct {
			Tag string `gorm:"column:tag"`
		}
		if err := db.WithContext(ctx).Raw(
			`SELECT DISTINCT lower(trim(coalesce(project_tag, ''))) AS tag
			 FROM activity_feed
			 WHERE project_id IS NULL OR TRIM(project_id) = ''`,
		).Scan(&tagRows).Error; err != nil {
			return err
		}

		for _, row := range tagRows {
			normalized := normalizeProjectSlug(row.Tag)
			if normalized == "" {
				normalized = "workspace"
			}

			project, err := upsertProjectRecord(ctx, db, normalized, normalized)
			if err != nil {
				return err
			}

			if err := db.WithContext(ctx).Exec(
				`UPDATE activity_feed
				 SET project_id = ?
				 WHERE (project_id IS NULL OR TRIM(project_id) = '')
				   AND lower(trim(coalesce(project_tag, ''))) = ?`,
				project.ID,
				normalized,
			).Error; err != nil {
				return err
			}
		}
	}

	if err := db.WithContext(ctx).Exec(
		`UPDATE activity_feed
		 SET project_id = ?
		 WHERE project_id IS NULL OR TRIM(project_id) = ''`,
		workspace.ID,
	).Error; err != nil {
		return err
	}

	return nil
}

func upsertProjectRecord(ctx context.Context, db *gorm.DB, slug, displayName string) (Project, error) {
	normalizedSlug := normalizeProjectSlug(slug)
	if normalizedSlug == "" {
		return Project{}, errors.New("project slug cannot be empty")
	}

	project := Project{
		Slug:        normalizedSlug,
		DisplayName: strings.TrimSpace(displayName),
		Status:      "active",
	}
	if project.DisplayName == "" {
		project.DisplayName = normalizedSlug
	}

	if err := db.WithContext(ctx).Where("slug = ?", normalizedSlug).FirstOrCreate(&project).Error; err != nil {
		return Project{}, err
	}
	return project, nil
}

func tableHasColumn(ctx context.Context, db *gorm.DB, tableName, columnName string) bool {
	if strings.TrimSpace(tableName) == "" || strings.TrimSpace(columnName) == "" {
		return false
	}

	var count int64
	if err := db.WithContext(ctx).Raw(
		fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info(%q) WHERE name = ?", tableName),
		columnName,
	).Scan(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func tableExists(ctx context.Context, db *gorm.DB, tableName string) bool {
	if strings.TrimSpace(tableName) == "" {
		return false
	}

	var count int64
	if err := db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		tableName,
	).Scan(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func loadLegacyProjectTags(ctx context.Context, db *gorm.DB) (map[string]string, error) {
	result := map[string]string{}
	if !tableExists(ctx, db, "activity_feed") {
		return result, nil
	}
	if !tableHasColumn(ctx, db, "activity_feed", "id") || !tableHasColumn(ctx, db, "activity_feed", "project_tag") {
		return result, nil
	}

	var rows []struct {
		ID  string `gorm:"column:id"`
		Tag string `gorm:"column:tag"`
	}
	if err := db.WithContext(ctx).Raw(
		`SELECT id, lower(trim(coalesce(project_tag, ''))) AS tag
		 FROM activity_feed
		 WHERE trim(coalesce(project_tag, '')) <> ''`,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		id := strings.TrimSpace(row.ID)
		if id == "" {
			continue
		}
		result[id] = normalizeProjectSlug(row.Tag)
	}

	return result, nil
}

func seedModelPricingCatalog(ctx context.Context, db *gorm.DB) error {
	rows, err := loadSeededModelPricing()
	if err != nil {
		return err
	}

	for _, row := range rows {
		var existing ModelPricing
		err := db.WithContext(ctx).
			Where("provider = ? AND model = ?", row.Provider, row.Model).
			Order("effective_from desc").
			First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	}

	return nil
}

func loadSeededModelPricing() ([]ModelPricing, error) {
	var seeds []seededModelPricing
	if err := json.Unmarshal(modelPricingSeedJSON, &seeds); err != nil {
		return nil, err
	}

	rows := make([]ModelPricing, 0, len(seeds))
	for _, seed := range seeds {
		effectiveFrom, err := time.Parse(time.RFC3339, strings.TrimSpace(seed.EffectiveFrom))
		if err != nil {
			return nil, fmt.Errorf("parse model pricing effective_from for %s/%s: %w", seed.Provider, seed.Model, err)
		}

		var lastVerifiedAt *time.Time
		if trimmed := strings.TrimSpace(seed.LastVerifiedAt); trimmed != "" {
			parsed, err := time.Parse(time.RFC3339, trimmed)
			if err != nil {
				return nil, fmt.Errorf("parse model pricing last_verified_at for %s/%s: %w", seed.Provider, seed.Model, err)
			}
			lastVerifiedAt = &parsed
		}

		rows = append(rows, ModelPricing{
			Provider:           strings.ToLower(strings.TrimSpace(seed.Provider)),
			Model:              strings.TrimSpace(seed.Model),
			EffectiveFrom:      effectiveFrom,
			InputCostPer1M:     seed.InputCostPer1M,
			OutputCostPer1M:    seed.OutputCostPer1M,
			ReasoningCostPer1M: seed.ReasoningCostPer1M,
			Currency:           strings.ToUpper(strings.TrimSpace(seed.Currency)),
			Source:             strings.TrimSpace(seed.Source),
			IsEstimated:        seed.IsEstimated,
			LastVerifiedAt:     lastVerifiedAt,
			VerificationNotes:  strings.TrimSpace(seed.VerificationNotes),
		})
	}

	return rows, nil
}

func (s *service) lookupReferencePricing(ctx context.Context, model string) (ModelPricing, bool, error) {
	normalizedModel := strings.TrimSpace(model)
	if normalizedModel == "" || normalizedModel == "unknown-model" {
		return ModelPricing{}, false, nil
	}

	var pricing ModelPricing
	err := s.db.WithContext(ctx).
		Where("model = ?", normalizedModel).
		Order("CASE WHEN provider = 'openrouter' THEN 0 ELSE 1 END").
		Order("effective_from desc").
		First(&pricing).Error
	if err == nil {
		return pricing, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		aliasCandidates := providerQualifiedModelCandidates(normalizedModel)
		if len(aliasCandidates) == 0 {
			return ModelPricing{}, false, nil
		}

		err = s.db.WithContext(ctx).
			Where("model IN ?", aliasCandidates).
			Order("CASE WHEN provider = 'openrouter' THEN 0 ELSE 1 END").
			Order("effective_from desc").
			First(&pricing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ModelPricing{}, false, nil
		}
		if err != nil {
			return ModelPricing{}, false, err
		}

		return pricing, true, nil
	}
	if err != nil {
		return ModelPricing{}, false, err
	}

	return pricing, true, nil
}

func providerQualifiedModelCandidates(model string) []string {
	candidates := append([]string{}, explicitModelPricingAliases(model)...)
	if strings.Contains(model, "/") {
		return candidates
	}

	candidates = append(candidates,
		"openai/"+model,
		"anthropic/"+model,
		"google/"+model,
		"moonshotai/"+model,
		"meta-llama/"+model,
		"x-ai/"+model,
	)

	return uniqueStrings(candidates)
}

func explicitModelPricingAliases(model string) []string {
	switch strings.TrimSpace(model) {
	case "gpt-5-codex-mini", "openai/gpt-5-codex-mini":
		return []string{
			"gpt-5-mini",
			"openai/gpt-5-mini",
		}
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}

	return result
}

func bootstrapOpenRouterModelPricing(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&ModelPricing{}).Where("provider = ? AND source = ?", "openrouter", openRouterModelsAPIURL).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	rows, err := openRouterModelsFetcher(ctx)
	if err != nil {
		return err
	}
	return upsertOpenRouterModelPricing(ctx, db, rows)
}

func refreshOpenRouterModelPricingIfDue(ctx context.Context, db *gorm.DB, now time.Time, config pricingRefreshConfig) error {
	if !config.Enabled {
		return nil
	}

	if err := markOpenRouterPricingStale(ctx, db, now, config.StaleAfter); err != nil {
		return err
	}

	due, err := openRouterPricingRefreshDue(ctx, db, now, config.Interval)
	if err != nil || !due {
		return err
	}

	rows, err := openRouterModelsFetcher(ctx)
	if err != nil {
		if staleErr := markOpenRouterPricingStale(ctx, db, now, config.StaleAfter); staleErr != nil {
			return staleErr
		}
		return err
	}

	return upsertOpenRouterModelPricing(ctx, db, rows)
}

func openRouterPricingRefreshDue(ctx context.Context, db *gorm.DB, now time.Time, interval time.Duration) (bool, error) {
	if interval <= 0 {
		return false, nil
	}

	var latest ModelPricing
	err := db.WithContext(ctx).
		Model(&ModelPricing{}).
		Where("provider = ? AND source = ?", "openrouter", openRouterModelsAPIURL).
		Where("last_verified_at IS NOT NULL").
		Order("last_verified_at desc").
		First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if latest.LastVerifiedAt == nil {
		return true, nil
	}

	return now.Sub(latest.LastVerifiedAt.UTC()) >= interval, nil
}

func markOpenRouterPricingStale(ctx context.Context, db *gorm.DB, now time.Time, staleAfter time.Duration) error {
	if staleAfter <= 0 {
		staleAfter = 7 * 24 * time.Hour
	}

	cutoff := now.Add(-staleAfter)
	base := db.WithContext(ctx).Model(&ModelPricing{}).Where("provider = ? AND source = ?", "openrouter", openRouterModelsAPIURL)

	if err := base.Where("last_verified_at IS NULL OR last_verified_at < ?", cutoff).Update("is_stale", true).Error; err != nil {
		return err
	}
	if err := base.Where("last_verified_at IS NOT NULL AND last_verified_at >= ?", cutoff).Update("is_stale", false).Error; err != nil {
		return err
	}

	return nil
}

func upsertOpenRouterModelPricing(ctx context.Context, db *gorm.DB, rows []ModelPricing) error {
	if len(rows) == 0 {
		return nil
	}

	for _, row := range rows {
		row.IsStale = false

		var existing ModelPricing
		err := db.WithContext(ctx).
			Where("provider = ? AND model = ?", row.Provider, row.Model).
			Order("effective_from desc").
			First(&existing).Error
		if err == nil {
			if err := db.WithContext(ctx).Model(&existing).Updates(map[string]any{
				"effective_from":        row.EffectiveFrom,
				"input_cost_per_1m":     row.InputCostPer1M,
				"output_cost_per_1m":    row.OutputCostPer1M,
				"reasoning_cost_per_1m": row.ReasoningCostPer1M,
				"currency":              row.Currency,
				"source":                row.Source,
				"is_estimated":          row.IsEstimated,
				"is_stale":              false,
				"last_verified_at":      row.LastVerifiedAt,
				"verification_notes":    row.VerificationNotes,
			}).Error; err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	}

	return nil
}

func fetchOpenRouterModelPricing(ctx context.Context) ([]ModelPricing, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterModelsAPIURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := openRouterHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter models request failed: %s", resp.Status)
	}

	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt            string `json:"prompt"`
				Completion        string `json:"completion"`
				InternalReasoning string `json:"internal_reasoning"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	rows := make([]ModelPricing, 0, len(payload.Data))
	for _, item := range payload.Data {
		modelID := strings.TrimSpace(item.ID)
		if modelID == "" {
			continue
		}

		inputCost, err := parsePerTokenPriceToPerMillion(item.Pricing.Prompt)
		if err != nil {
			return nil, fmt.Errorf("parse openrouter prompt price for %s: %w", modelID, err)
		}
		outputCost, err := parsePerTokenPriceToPerMillion(item.Pricing.Completion)
		if err != nil {
			return nil, fmt.Errorf("parse openrouter completion price for %s: %w", modelID, err)
		}

		var reasoningCost *float64
		if strings.TrimSpace(item.Pricing.InternalReasoning) != "" {
			value, err := parsePerTokenPriceToPerMillion(item.Pricing.InternalReasoning)
			if err != nil {
				return nil, fmt.Errorf("parse openrouter reasoning price for %s: %w", modelID, err)
			}
			reasoningCost = &value
		}

		verifiedAt := now
		rows = append(rows, ModelPricing{
			Provider:           "openrouter",
			Model:              modelID,
			EffectiveFrom:      now,
			InputCostPer1M:     inputCost,
			OutputCostPer1M:    outputCost,
			ReasoningCostPer1M: reasoningCost,
			Currency:           "USD",
			Source:             openRouterModelsAPIURL,
			IsEstimated:        false,
			LastVerifiedAt:     &verifiedAt,
			VerificationNotes:  "Imported from OpenRouter models endpoint.",
		})
	}

	return rows, nil
}

func parsePerTokenPriceToPerMillion(value string) (float64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}

	pricePerToken, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, err
	}

	return pricePerToken * 1_000_000, nil
}
