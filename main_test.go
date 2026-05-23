package main

import "testing"

func TestProcessTranscriptResolvesCommand(t *testing.T) {
	settings := DefaultSettings()

	result := processTranscript(settings, "换行")
	if result.Kind != "command" {
		t.Fatalf("expected command result, got %q", result.Kind)
	}

	if result.Command == nil {
		t.Fatal("expected command payload")
	}

	if result.Command.Type != "insert" || result.Command.Value != "\n" {
		t.Fatalf("unexpected command payload: %+v", result.Command)
	}
}

func TestProcessTranscriptQueuesInReviewMode(t *testing.T) {
	settings := DefaultSettings()
	settings.Mode = "review"

	result := processTranscript(settings, "今天整理会议纪要")
	if result.Kind != "queue" {
		t.Fatalf("expected queue result, got %q", result.Kind)
	}

	if result.Text != "今天整理会议纪要" {
		t.Fatalf("unexpected queued text: %q", result.Text)
	}
}

func TestProcessTranscriptAppliesHotwords(t *testing.T) {
	settings := DefaultSettings()
	settings.Hotwords = []Hotword{
		{Source: "flow voice", Target: "FlowVoice"},
	}

	result := processTranscript(settings, "请打开 flow voice")
	if result.Kind != "insert" {
		t.Fatalf("expected insert result, got %q", result.Kind)
	}

	if result.Text != "请打开 FlowVoice" {
		t.Fatalf("unexpected processed text: %q", result.Text)
	}
}
