package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"clawtivity/internal/classifier"
	"clawtivity/internal/database"
)

var projectOverridePattern = regexp.MustCompile(`(?i)\bproject\b\s*:?\s*([a-z0-9][a-z0-9._-]*)`)
var projectPathMentionPattern = regexp.MustCompile(`(?i)/projects?/([a-z0-9][a-z0-9._-]*)`)
var projectOverrideStopwords = map[string]struct{}{
	"as":  {},
	"is":  {},
	"was": {},
	"the": {},
	"a":   {},
	"an":  {},
	"to":  {},
	"for": {},
}

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

func applyProjectAssociation(activity *database.ActivityFeed, promptText, _ string) {
	if activity == nil {
		return
	}

	candidate := extractProjectOverride(promptText)
	if candidate == "" {
		candidate = extractProjectPathMention(promptText)
		if candidate != "" {
			activity.ProjectTag = candidate
			activity.ProjectReason = "prompt_path_mention"
		}
		return
	}
	if !projectExistsUnderKnownRoots(candidate) {
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
	candidate := strings.ToLower(strings.TrimSpace(match[1]))
	candidate = strings.Trim(candidate, ".,;:!?)]}\"'")
	if candidate == "" {
		return ""
	}
	if _, stopword := projectOverrideStopwords[candidate]; stopword {
		return ""
	}
	return candidate
}

func extractProjectPathMention(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	match := projectPathMentionPattern.FindStringSubmatch(trimmed)
	if len(match) < 2 {
		return ""
	}
	candidate := strings.ToLower(strings.TrimSpace(match[1]))
	candidate = strings.Trim(candidate, ".,;:!?)]}\"'")
	if candidate == "" {
		return ""
	}
	return candidate
}

func projectExistsUnderKnownRoots(project string) bool {
	roots := discoverProjectRoots()
	if len(roots) == 0 {
		// If no roots can be discovered, keep behavior permissive.
		return true
	}

	for _, root := range roots {
		if root == "" {
			continue
		}
		target := filepath.Join(root, project)
		info, err := os.Stat(target)
		if err == nil && info.IsDir() {
			return true
		}
	}

	return false
}

func discoverProjectRoots() []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)

	add := func(dir string) {
		clean := strings.TrimSpace(filepath.Clean(dir))
		if clean == "" || seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, clean)
	}

	cwd, err := os.Getwd()
	if err != nil || strings.TrimSpace(cwd) == "" {
		return out
	}

	add(filepath.Join(cwd, "projects"))
	add(filepath.Join(cwd, "project"))

	parts := strings.Split(filepath.ToSlash(cwd), "/")
	for i, part := range parts {
		if part != "projects" && part != "project" {
			continue
		}
		root := strings.Join(parts[:i+1], "/")
		if strings.HasPrefix(filepath.ToSlash(cwd), "/") {
			root = "/" + strings.TrimPrefix(root, "/")
		}
		add(root)
	}

	return out
}
