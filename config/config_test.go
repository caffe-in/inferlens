package config

import "testing"

func TestNewUsesDefaultEndpoint(t *testing.T) {
	cfg, err := New("", "qwen", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Endpoint != DefaultEndpoint {
		t.Fatalf("expected default endpoint %q, got %q", DefaultEndpoint, cfg.Endpoint)
	}
}

func TestNewRejectsMissingModel(t *testing.T) {
	_, err := New(DefaultEndpoint, "", "hello")
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestNewRejectsMissingPrompt(t *testing.T) {
	_, err := New(DefaultEndpoint, "qwen", "")
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}
