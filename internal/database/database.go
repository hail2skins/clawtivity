package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ActivityFeed is the local-first event ledger entry.
type ActivityFeed struct {
	ID           string    `gorm:"type:char(36);primaryKey" json:"id"`
	SessionKey   string    `gorm:"index:idx_activity_feed_session_key" json:"session_key"`
	Model        string    `json:"model"`
	TokensIn     int       `json:"tokens_in"`
	TokensOut    int       `json:"tokens_out"`
	CostEstimate float64   `json:"cost_estimate"`
	DurationMS   int64     `json:"duration_ms"`
	ProjectTag   string    `gorm:"index:idx_activity_feed_project_tag" json:"project_tag"`
	ExternalRef  string    `json:"external_ref"`
	Category     string    `gorm:"index:idx_activity_feed_category" json:"category"`
	Thinking     string    `json:"thinking"`
	Reasoning    bool      `json:"reasoning"`
	Channel      string    `json:"channel"`
	Status       string    `gorm:"index:idx_activity_feed_status" json:"status"`
	UserID       string    `gorm:"index:idx_activity_feed_user_id" json:"user_id"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
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

// Service represents a service that interacts with a database.
type Service interface {
	// Health returns a map of health status information.
	Health() map[string]string

	// Close terminates the database connection.
	Close() error
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

	if err := gormDB.AutoMigrate(&ActivityFeed{}, &TurnMemory{}); err != nil {
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

// Close closes the database connection.
func (s *service) Close() error {
	log.Printf("Disconnected from database: %s", dburl)
	return s.sqlDB.Close()
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
