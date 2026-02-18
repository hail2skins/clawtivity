package database

import (
	"testing"
	"time"
)

func TestActivityFields(t *testing.T) {
	// Test that Activity struct has all required fields
	// This test will fail until we implement the Activity model
	
	activity := Activity{
		ID:           1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		
		// Core tracking fields
		Category:     "code",
		TokensIn:     1000,
		TokensOut:    500,
		TimeStarted:  time.Now().Add(-30 * time.Minute),
		TimeCompleted: time.Now(),
		ElapsedTime:  int64(30 * time.Minute),
		
		// Model info
		Model:        "kimi-k2.5",
		Reasoning:    true,
		Thinking:     "medium",
		
		// Work description
		Title:        "Test activity",
		Project:      "clawtivity",
		
		// Extended fields
		SessionID:    "session-123",
		Channel:      "discord",
		Status:       "success",
		ErrorMessage: "",
		ToolsUsed:    "[]string{\"exec\", \"read\"}",
		Cost:         0.005,
		ParentSession: "parent-456",
		UserID:       "art@hamcois.com",
		JiraTicket:   "CLAW-12",
		GitCommit:    "abc123",
		Tags:         "test,mvp",
		Metadata:     "{}",
	}
	
	// Verify all fields are set correctly
	if activity.Category != "code" {
		t.Errorf("Expected Category 'code', got '%s'", activity.Category)
	}
	if activity.TokensIn != 1000 {
		t.Errorf("Expected TokensIn 1000, got %d", activity.TokensIn)
	}
	if activity.Reasoning != true {
		t.Errorf("Expected Reasoning true, got %v", activity.Reasoning)
	}
	if activity.Status != "success" {
		t.Errorf("Expected Status 'success', got '%s'", activity.Status)
	}
	if activity.Project != "clawtivity" {
		t.Errorf("Expected Project 'clawtivity', got '%s'", activity.Project)
	}
}

func TestActivityCategories(t *testing.T) {
	validCategories := []string{"general", "admin", "code", "research", "other"}
	
	for _, cat := range validCategories {
		if !isValidCategory(cat) {
			t.Errorf("Expected '%s' to be a valid category", cat)
		}
	}
	
	// This should fail - tests validation
	if isValidCategory("invalid") {
		t.Error("Expected 'invalid' to not be a valid category")
	}
}

func TestActivityStatus(t *testing.T) {
	validStatuses := []string{"success", "failed", "in_progress", "pending"}
	
	for _, status := range validStatuses {
		if !isValidStatus(status) {
			t.Errorf("Expected '%s' to be a valid status", status)
		}
	}
	
	if isValidStatus("invalid") {
		t.Error("Expected 'invalid' to not be a valid status")
	}
}