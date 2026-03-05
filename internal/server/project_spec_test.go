package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type promptSpecCase struct {
	Name                string `json:"name"`
	PromptText          string `json:"prompt_text"`
	ExpectedOverride    string `json:"expected_override"`
	ExpectedPathMention string `json:"expected_path_mention"`
}

func TestProjectPromptSpecCases(t *testing.T) {
	cases := loadPromptSpecCases(t)

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if got := extractProjectOverride(tc.PromptText); got != tc.ExpectedOverride {
				t.Fatalf("expected override %q, got %q", tc.ExpectedOverride, got)
			}
			if got := extractProjectPathMention(tc.PromptText); got != tc.ExpectedPathMention {
				t.Fatalf("expected path mention %q, got %q", tc.ExpectedPathMention, got)
			}
		})
	}
}

func loadPromptSpecCases(t *testing.T) []promptSpecCase {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("..", "..", "spec", "project_tag_prompt_cases.json"))
	if err != nil {
		t.Fatalf("expected prompt spec file: %v", err)
	}

	var cases []promptSpecCase
	if err := json.Unmarshal(body, &cases); err != nil {
		t.Fatalf("expected valid prompt spec json: %v", err)
	}
	return cases
}
