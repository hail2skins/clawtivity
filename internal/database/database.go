package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var ErrInvalidDateFilter = errors.New("invalid date filter: expected YYYY-MM-DD")

// ActivityFeed is the local-first event ledger entry.
type ActivityFeed struct {
	ID             string    `gorm:"type:char(36);primaryKey" json:"id"`
	SessionKey     string    `gorm:"index:idx_activity_feed_session_key" json:"session_key"`
	Model          string    `json:"model"`
	TokensIn       int       `json:"tokens_in"`
	TokensOut      int       `json:"tokens_out"`
	CostEstimate   float64   `json:"cost_estimate"`
	DurationMS     int64     `json:"duration_ms"`
	ProjectID      string    `gorm:"type:char(36);index:idx_activity_feed_project_id" json:"project_id"`
	Project        Project   `gorm:"foreignKey:ProjectID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"-"`
	ProjectTag     string    `gorm:"-" json:"project_tag"`
	ProjectReason  string    `json:"project_reason"`
	ExternalRef    string    `json:"external_ref"`
	Category       string    `gorm:"index:idx_activity_feed_category" json:"category"`
	CategoryReason string    `json:"category_reason"`
	Thinking       string    `json:"thinking"`
	Reasoning      bool      `json:"reasoning"`
	Channel        string    `json:"channel"`
	Status         string    `gorm:"index:idx_activity_feed_status" json:"status"`
	UserID         string    `gorm:"index:idx_activity_feed_user_id" json:"user_id"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
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
	db    *gorm.DB
	sqlDB *sql.DB
}

var (
	dburl      = os.Getenv("BLUEPRINT_DB_URL")
	dbInstance *service
	dbMu       sync.Mutex
)

func New() Service {
	dbMu.Lock()
	defer dbMu.Unlock()

	if dbInstance != nil {
		return dbInstance
	}

	svc, err := newSQLiteService(dburl)
	if err != nil {
		log.Fatal(err)
	}

	dbInstance = svc
	return dbInstance
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

	if err := gormDB.AutoMigrate(&Project{}, &ActivityFeed{}, &TurnMemory{}); err != nil {
		return nil, err
	}

	if err := synchronizeActivityProjectIDs(context.Background(), gormDB); err != nil {
		return nil, err
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}

	return &service{db: gormDB, sqlDB: sqlDB}, nil
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

// Close closes the database connection.
func (s *service) Close() error {
	log.Printf("Disconnected from database: %s", dburl)
	return s.sqlDB.Close()
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

func synchronizeActivityProjectIDs(ctx context.Context, db *gorm.DB) error {
	workspace, err := upsertProjectRecord(ctx, db, "workspace", "workspace")
	if err != nil {
		return err
	}

	if db.Migrator().HasColumn(&ActivityFeed{}, "project_tag") {
		rows, err := db.WithContext(ctx).Raw(
			"SELECT DISTINCT project_tag FROM activity_feed WHERE project_id IS NULL OR TRIM(project_id) = ''",
		).Rows()
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var tag sql.NullString
			if err := rows.Scan(&tag); err != nil {
				return err
			}

			normalized := normalizeProjectSlug(tag.String)
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
