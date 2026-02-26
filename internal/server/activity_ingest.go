package server

import (
	"regexp"
	"strings"
	"time"

	"clawtivity/internal/classifier"
	"clawtivity/internal/database"
)

var projectOverridePattern = regexp.MustCompile(`(?i)\bproject\b\s*:?\s*([a-z0-9][a-z0-9._-]*)`)

type activityIngest struct {
	database.ActivityFeed
	PromptText    string   `json:"prompt_text"`
	AssistantText string   `json:"assistant_text"`
	ToolsUsed     []string `json:"tools_used"`
}

func normalizeActivity(activity *database.ActivityFeed) {
	if strings.TrimSpace(activity.ProjectTag) == "" {
		activity.ProjectTag = "unknown-project"
	}
	if strings.TrimSpace(activity.ProjectReason) == "" {
		activity.ProjectReason = "fallback:unknown"
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
	if strings.TrimSpace(activity.Thinking) == "" {
		activity.Thinking = "medium"
	}
	if strings.TrimSpace(activity.Status) == "" {
		activity.Status = "success"
	}
	if activity.CreatedAt.IsZero() {
		activity.CreatedAt = time.Now().UTC()
	}
}

func applyActivityClassification(activity *database.ActivityFeed, signals classifier.Signals) {
	if activity == nil {
		return
	}

	category := strings.ToLower(strings.TrimSpace(activity.Category))
	if category != "" && category != "general" {
		activity.Category = category
		if strings.TrimSpace(activity.CategoryReason) == "" {
			activity.CategoryReason = "provided:category"
		}
		return
	}

	derivedCategory, reason := classifier.Classify(signals)
	activity.Category = derivedCategory
	activity.CategoryReason = reason
}

func applyProjectAssociation(activity *database.ActivityFeed, promptText, assistantText string) {
	if activity == nil {
		return
	}

	candidate := extractProjectOverride(promptText)
	if candidate == "" {
		candidate = extractProjectOverride(assistantText)
	}
	if candidate == "" {
		return
	}

	activity.ProjectTag = candidate
	activity.ProjectReason = "prompt_override"
}

func extractProjectOverride(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	match := projectOverridePattern.FindStringSubmatch(trimmed)
	if len(match) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(match[1]))
}
