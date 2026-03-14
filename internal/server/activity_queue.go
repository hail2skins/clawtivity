package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"clawtivity/internal/classifier"
	"clawtivity/internal/database"
)

var queueJSONFencePattern = regexp.MustCompile("(?s)```json\\n(.*?)\\n```")

type queuedEntry struct {
	rawJSON string
	ingest  activityIngest
	valid   bool
}

func resolveQueueDir() string {
	if value := strings.TrimSpace(os.Getenv("CLAWTIVITY_QUEUE_ROOT")); value != "" {
		return value
	}

	if value := strings.TrimSpace(os.Getenv("CLAWTIVITY_QUEUE_DIR")); value != "" {
		return value
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".clawtivity/queue"
	}
	return filepath.Join(home, ".clawtivity", "queue")
}

func CountQueueDepth(queueRoot string) int {
	root := strings.TrimSpace(queueRoot)
	if root == "" {
		root = resolveQueueDir()
	}
	files, err := filepath.Glob(filepath.Join(root, "*.md"))
	if err != nil {
		storeQueueDepth(0)
		return 0
	}
	count := 0
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		count += len(parseQueueEntries(string(body)))
	}
	storeQueueDepth(count)
	return count
}

func flushQueueOnStartup(db database.Service) {
	if db == nil {
		return
	}

	queueDir := resolveQueueDir()
	_, _ = flushQueuedActivitiesWithOptions(context.Background(), db, queueDir, true)
}

func flushQueuedActivities(ctx context.Context, db database.Service, queueDir string) (int, error) {
	return flushQueuedActivitiesWithOptions(ctx, db, queueDir, false)
}

func flushQueuedActivitiesWithOptions(ctx context.Context, db database.Service, queueDir string, startup bool) (int, error) {
	if strings.TrimSpace(queueDir) == "" {
		return 0, nil
	}

	if _, err := os.Stat(queueDir); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		incQueueFlushFailed()
		logEvent("warn", "queue_flush_failed", map[string]any{
			"queue_root": queueDir,
			"startup":    startup,
			"error":      err.Error(),
		}, CountQueueDepth(queueDir))
		return 0, err
	}

	queueDepth := CountQueueDepth(queueDir)
	incQueueFlushAttempted()
	logEvent("info", "queue_flush_attempted", map[string]any{
		"queue_root": queueDir,
		"startup":    startup,
	}, queueDepth)

	files, err := filepath.Glob(filepath.Join(queueDir, "*.md"))
	if err != nil {
		incQueueFlushFailed()
		logEvent("warn", "queue_flush_failed", map[string]any{
			"queue_root": queueDir,
			"startup":    startup,
			"error":      err.Error(),
		}, queueDepth)
		return 0, err
	}

	totalFlushed := 0
	for _, filePath := range files {
		body, err := os.ReadFile(filePath)
		if err != nil {
			incQueueFlushFailed()
			logEvent("warn", "queue_flush_failed", map[string]any{
				"queue_root": queueDir,
				"file":       filePath,
				"startup":    startup,
				"error":      err.Error(),
			}, CountQueueDepth(queueDir))
			return totalFlushed, err
		}

		entries := parseQueueEntries(string(body))
		remaining := make([]queuedEntry, 0, len(entries))
		for _, entry := range entries {
			if !entry.valid {
				remaining = append(remaining, entry)
				continue
			}
			activity := entry.ingest.ActivityFeed
			normalizeActivity(&activity)
			applyProjectAssociation(&activity, entry.ingest.PromptText, entry.ingest.AssistantText)
			if err := ensureProjectRegistry(ctx, db, &activity); err != nil {
				remaining = append(remaining, entry)
				incQueueFlushFailed()
				logEvent("warn", "queue_flush_failed", map[string]any{
					"queue_root":  queueDir,
					"file":        filePath,
					"startup":     startup,
					"error":       err.Error(),
					"session_key": activity.SessionKey,
				}, CountQueueDepth(queueDir))
				logEvent("warn", "replay_failed", map[string]any{
					"queue_root":  queueDir,
					"file":        filePath,
					"startup":     startup,
					"error":       err.Error(),
					"session_key": activity.SessionKey,
				}, CountQueueDepth(queueDir))
				continue
			}
			applyActivityClassification(&activity, classifier.Signals{
				PromptText:    entry.ingest.PromptText,
				AssistantText: entry.ingest.AssistantText,
				ToolsUsed:     entry.ingest.ToolsUsed,
			})
			if err := db.CreateActivity(ctx, &activity); err != nil {
				remaining = append(remaining, entry)
				incQueueFlushFailed()
				logEvent("warn", "queue_flush_failed", map[string]any{
					"queue_root":  queueDir,
					"file":        filePath,
					"startup":     startup,
					"error":       err.Error(),
					"session_key": activity.SessionKey,
				}, CountQueueDepth(queueDir))
				logEvent("warn", "replay_failed", map[string]any{
					"queue_root":  queueDir,
					"file":        filePath,
					"startup":     startup,
					"error":       err.Error(),
					"session_key": activity.SessionKey,
				}, CountQueueDepth(queueDir))
				continue
			}
			totalFlushed++
			incQueueFlushSucceeded()
			queueDepthAfter := CountQueueDepth(queueDir)
			logEvent("info", "queue_flush_succeeded", map[string]any{
				"queue_root":  queueDir,
				"file":        filePath,
				"startup":     startup,
				"session_key": activity.SessionKey,
			}, queueDepthAfter)
			logEvent("info", "replay_succeeded", map[string]any{
				"queue_root":  queueDir,
				"file":        filePath,
				"startup":     startup,
				"session_key": activity.SessionKey,
			}, queueDepthAfter)
		}

		if err := writeQueueEntries(filePath, remaining); err != nil {
			incQueueFlushFailed()
			logEvent("warn", "queue_flush_failed", map[string]any{
				"queue_root": queueDir,
				"file":       filePath,
				"startup":    startup,
				"error":      err.Error(),
			}, CountQueueDepth(queueDir))
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

		var ingest activityIngest
		if err := json.Unmarshal([]byte(raw), &ingest); err != nil {
			entries = append(entries, queuedEntry{rawJSON: raw, valid: false})
			continue
		}

		entries = append(entries, queuedEntry{rawJSON: raw, ingest: ingest, valid: true})
	}

	return entries
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
	builder.WriteString("# Clawtivity Fallback Queue (")
	builder.WriteString(dateLabel)
	builder.WriteString(")\n\n")
	for _, entry := range entries {
		builder.WriteString("## queued_at: replay_pending\n")
		builder.WriteString("```json\n")
		builder.WriteString(strings.TrimSpace(entry.rawJSON))
		builder.WriteString("\n```\n\n")
	}

	return os.WriteFile(filePath, []byte(builder.String()), 0o644)
}
