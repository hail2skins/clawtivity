package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"clawtivity/internal/database"
)

var queueJSONFencePattern = regexp.MustCompile("(?s)```json\\n(.*?)\\n```")

type queuedEntry struct {
	rawJSON  string
	activity database.ActivityFeed
	valid    bool
}

func resolveQueueDir() string {
	if value := strings.TrimSpace(os.Getenv("CLAWTIVITY_QUEUE_DIR")); value != "" {
		return value
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".clawtivity/queue"
	}
	return filepath.Join(home, ".clawtivity", "queue")
}

func flushQueueOnStartup(db database.Service) {
	if db == nil {
		return
	}

	queueDir := resolveQueueDir()
	flushed, err := flushQueuedActivities(context.Background(), db, queueDir)
	if err != nil {
		fmt.Printf("[clawtivity] queue startup flush failed: %v\n", err)
		return
	}
	if flushed > 0 {
		fmt.Printf("[clawtivity] queue startup flush imported %d entries\n", flushed)
	}
}

func flushQueuedActivities(ctx context.Context, db database.Service, queueDir string) (int, error) {
	if strings.TrimSpace(queueDir) == "" {
		return 0, nil
	}

	if _, err := os.Stat(queueDir); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	files, err := filepath.Glob(filepath.Join(queueDir, "*.md"))
	if err != nil {
		return 0, err
	}

	totalFlushed := 0
	for _, filePath := range files {
		body, err := os.ReadFile(filePath)
		if err != nil {
			return totalFlushed, err
		}

		entries := parseQueueEntries(string(body))
		remaining := make([]queuedEntry, 0, len(entries))
		for _, entry := range entries {
			if !entry.valid {
				remaining = append(remaining, entry)
				continue
			}
			activity := normalizeQueuedActivity(entry.activity)
			if err := db.CreateActivity(ctx, &activity); err != nil {
				remaining = append(remaining, entry)
				continue
			}
			totalFlushed++
		}

		if err := writeQueueEntries(filePath, remaining); err != nil {
			return totalFlushed, err
		}
	}

	return totalFlushed, nil
}

func parseQueueEntries(markdown string) []queuedEntry {
	matches := queueJSONFencePattern.FindAllStringSubmatch(markdown, -1)
	entries := make([]queuedEntry, 0, len(matches))

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		raw := strings.TrimSpace(match[1])
		if raw == "" {
			continue
		}

		var activity database.ActivityFeed
		if err := json.Unmarshal([]byte(raw), &activity); err != nil {
			entries = append(entries, queuedEntry{rawJSON: raw, valid: false})
			continue
		}

		entries = append(entries, queuedEntry{rawJSON: raw, activity: activity, valid: true})
	}

	return entries
}

func normalizeQueuedActivity(activity database.ActivityFeed) database.ActivityFeed {
	if strings.TrimSpace(activity.ProjectTag) == "" {
		activity.ProjectTag = "unknown-project"
	}
	if strings.TrimSpace(activity.Channel) == "" {
		activity.Channel = "unknown-channel"
	}
	if strings.TrimSpace(activity.UserID) == "" {
		activity.UserID = "unknown-user"
	}
	if strings.TrimSpace(activity.Model) == "" {
		activity.Model = "unknown-model"
	}
	if strings.TrimSpace(activity.Category) == "" {
		activity.Category = "general"
	}
	if strings.TrimSpace(activity.Thinking) == "" {
		activity.Thinking = "medium"
	}
	if strings.TrimSpace(activity.Status) == "" {
		activity.Status = "success"
	}
	if activity.CreatedAt.IsZero() {
		activity.CreatedAt = time.Now().UTC()
	}
	return activity
}

func writeQueueEntries(filePath string, entries []queuedEntry) error {
	if len(entries) == 0 {
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	dateLabel := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# Clawtivity Fallback Queue (%s)\n\n", dateLabel))
	for _, entry := range entries {
		builder.WriteString("## queued_at: replay_pending\n")
		builder.WriteString("```json\n")
		builder.WriteString(strings.TrimSpace(entry.rawJSON))
		builder.WriteString("\n```\n\n")
	}

	return os.WriteFile(filePath, []byte(builder.String()), 0o644)
}
