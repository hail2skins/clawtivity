package classifier

import "testing"

func TestClassifyExplicitOverrideWins(t *testing.T) {
	gotCategory, gotReason := Classify(Signals{
		PromptText:    "Category: research please handle this",
		AssistantText: "",
		ToolsUsed:     nil,
	})

	if gotCategory != "research" {
		t.Fatalf("expected research, got %q", gotCategory)
	}
	if gotReason == "" {
		t.Fatal("expected reason")
	}
}

func TestClassifyPromptKeywordsBeatToolSignal(t *testing.T) {
	gotCategory, gotReason := Classify(Signals{
		PromptText:    "please research and compare this",
		AssistantText: "",
		ToolsUsed:     []string{"write_file"},
	})

	if gotCategory != "research" {
		t.Fatalf("expected research from prompt intent, got %q", gotCategory)
	}
	if gotReason == "" {
		t.Fatal("expected reason")
	}
}

func TestClassifyKeywordScore(t *testing.T) {
	gotCategory, gotReason := Classify(Signals{
		PromptText:    "open a jira ticket and schedule cron automation",
		AssistantText: "I will automate this with cron",
	})

	if gotCategory != "automation" {
		t.Fatalf("expected automation, got %q", gotCategory)
	}
	if gotReason == "" {
		t.Fatal("expected reason")
	}
}

func TestClassifyToolSignalBeatsAssistantKeywordsWhenPromptIsNeutral(t *testing.T) {
	gotCategory, gotReason := Classify(Signals{
		PromptText:    "please take a look",
		AssistantText: "here is research and findings",
		ToolsUsed:     []string{"write_file"},
	})

	if gotCategory != "code" {
		t.Fatalf("expected code from tool signal, got %q", gotCategory)
	}
	if gotReason == "" {
		t.Fatal("expected reason")
	}
}

func TestClassifyFallsBackToGeneral(t *testing.T) {
	gotCategory, gotReason := Classify(Signals{
		PromptText:    "hello friend",
		AssistantText: "nice to meet you",
	})

	if gotCategory != "general" {
		t.Fatalf("expected general, got %q", gotCategory)
	}
	if gotReason == "" {
		t.Fatal("expected reason")
	}
}

func TestClassifyDoesNotTreatTestingAsCodeKeyword(t *testing.T) {
	gotCategory, gotReason := Classify(Signals{
		PromptText:    "Reply with a simple yes here. Testing.",
		AssistantText: "Yes",
	})

	if gotCategory != "general" {
		t.Fatalf("expected general, got %q", gotCategory)
	}
	if gotReason == "" {
		t.Fatal("expected reason")
	}
}

func TestClassifySingleKeywordFallsBackToGeneral(t *testing.T) {
	gotCategory, gotReason := Classify(Signals{
		PromptText:    "new test. say hi.",
		AssistantText: "hi",
	})

	if gotCategory != "general" {
		t.Fatalf("expected general, got %q", gotCategory)
	}
	if gotReason == "" {
		t.Fatal("expected reason")
	}
}

func TestClassifyOpenClawReleasePromptMapsToAdmin(t *testing.T) {
	gotCategory, gotReason := Classify(Signals{
		PromptText:    "OpenClaw update: check release docs for Discord /vc and tell me what we can do",
		AssistantText: "I reviewed the release notes and Discord feature details",
	})

	if gotCategory != "admin" {
		t.Fatalf("expected admin, got %q", gotCategory)
	}
	if gotReason == "" {
		t.Fatal("expected reason")
	}
}
