package classifier

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

//go:embed category_rules.json
var embeddedRules []byte

type categoryRule struct {
	Keywords []string `json:"keywords"`
	Tools    []string `json:"tools"`
}

type rules struct {
	DefaultCategory string                  `json:"default_category"`
	Categories      map[string]categoryRule `json:"categories"`
}

type Signals struct {
	PromptText    string
	AssistantText string
	ToolsUsed     []string
}

var explicitCategoryPattern = regexp.MustCompile(`(?i)category\s*:\s*([a-z_]+)`)
const minKeywordScore = 2

var loadedRules = mustLoadRules()

func mustLoadRules() rules {
	var r rules
	if err := json.Unmarshal(embeddedRules, &r); err != nil {
		panic(err)
	}
	if strings.TrimSpace(r.DefaultCategory) == "" {
		r.DefaultCategory = "general"
	}
	if r.Categories == nil {
		r.Categories = map[string]categoryRule{}
	}
	if _, ok := r.Categories[r.DefaultCategory]; !ok {
		r.Categories[r.DefaultCategory] = categoryRule{}
	}
	return r
}

func Classify(s Signals) (string, string) {
	if category, ok := detectExplicitOverride(s); ok {
		return category, fmt.Sprintf("explicit_override:category=%s", category)
	}

	if category, score, ok := detectKeywordScore(strings.ToLower(s.PromptText)); ok {
		return category, fmt.Sprintf("prompt_keyword_score:%s=%d", category, score)
	}

	if category, toolName, ok := detectToolSignal(s); ok {
		return category, fmt.Sprintf("tool_signal:%s", toolName)
	}

	if category, score, ok := detectKeywordScore(strings.ToLower(s.AssistantText)); ok {
		return category, fmt.Sprintf("assistant_keyword_score:%s=%d", category, score)
	}

	return loadedRules.DefaultCategory, "fallback:insufficient_signals"
}

func detectExplicitOverride(s Signals) (string, bool) {
	joined := strings.ToLower(strings.TrimSpace(s.PromptText + "\n" + s.AssistantText))
	if joined == "" {
		return "", false
	}

	match := explicitCategoryPattern.FindStringSubmatch(joined)
	if len(match) < 2 {
		return "", false
	}

	candidate := strings.TrimSpace(match[1])
	if _, ok := loadedRules.Categories[candidate]; !ok {
		return "", false
	}
	return candidate, true
}

func detectToolSignal(s Signals) (string, string, bool) {
	for _, rawTool := range s.ToolsUsed {
		tool := strings.ToLower(strings.TrimSpace(rawTool))
		if tool == "" {
			continue
		}
		for category, rule := range loadedRules.Categories {
			if category == loadedRules.DefaultCategory {
				continue
			}
			for _, expected := range rule.Tools {
				candidate := strings.ToLower(strings.TrimSpace(expected))
				if candidate == "" {
					continue
				}
				if strings.Contains(tool, candidate) {
					return category, tool, true
				}
			}
		}
	}

	return "", "", false
}

func detectKeywordScore(text string) (string, int, bool) {
	if strings.TrimSpace(text) == "" {
		return "", 0, false
	}

	scores := map[string]int{}
	for category, rule := range loadedRules.Categories {
		if category == loadedRules.DefaultCategory {
			continue
		}
		total := 0
		for _, keyword := range rule.Keywords {
			needle := strings.ToLower(strings.TrimSpace(keyword))
			if needle == "" {
				continue
			}
			if containsKeyword(text, needle) {
				total++
			}
		}
		if total > 0 {
			scores[category] = total
		}
	}

	if len(scores) == 0 {
		return "", 0, false
	}

	type scored struct {
		category string
		score    int
	}
	all := make([]scored, 0, len(scores))
	for category, score := range scores {
		all = append(all, scored{category: category, score: score})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score == all[j].score {
			return all[i].category < all[j].category
		}
		return all[i].score > all[j].score
	})

	if len(all) > 1 && all[0].score == all[1].score {
		return "", 0, false
	}
	if all[0].score < minKeywordScore {
		return "", 0, false
	}

	return all[0].category, all[0].score, true
}

func containsKeyword(text, keyword string) bool {
	if strings.Contains(keyword, " ") {
		return strings.Contains(text, keyword)
	}
	escaped := regexp.QuoteMeta(keyword)
	re := regexp.MustCompile(`\b` + escaped + `\b`)
	return re.MatchString(text)
}
