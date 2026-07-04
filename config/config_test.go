package config

import (
	"testing"
	"time"
)

func TestNewServeUsesDefaultEndpoint(t *testing.T) {
	cfg, err := NewServe("", "", "qwen", "hello", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Endpoint != DefaultEndpoint {
		t.Fatalf("expected default endpoint %q, got %q", DefaultEndpoint, cfg.Endpoint)
	}
	if cfg.MetricsEndpoint != DefaultEndpoint+"/metrics" {
		t.Fatalf("expected derived metrics endpoint, got %q", cfg.MetricsEndpoint)
	}
	if cfg.MaxTokens != DefaultMaxTokens {
		t.Fatalf("expected default max tokens %d, got %d", DefaultMaxTokens, cfg.MaxTokens)
	}
	if cfg.Timeout != 0 {
		t.Fatalf("expected no timeout, got %s", cfg.Timeout)
	}
}

func TestNewServeRejectsMissingModel(t *testing.T) {
	_, err := NewServe(DefaultEndpoint, "", "", "hello", 0, 0)
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestNewServeRejectsMissingPrompt(t *testing.T) {
	_, err := NewServe(DefaultEndpoint, "", "qwen", "", 0, 0)
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestNewServeAcceptsOverrides(t *testing.T) {
	cfg, err := NewServe("http://localhost:9000/", "http://localhost:9001/metrics", "qwen", "hello", 32, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Endpoint != "http://localhost:9000/" {
		t.Fatalf("expected endpoint override, got %q", cfg.Endpoint)
	}
	if cfg.MetricsEndpoint != "http://localhost:9001/metrics" {
		t.Fatalf("expected metrics endpoint override, got %q", cfg.MetricsEndpoint)
	}
	if cfg.MaxTokens != 32 {
		t.Fatalf("expected max tokens 32, got %d", cfg.MaxTokens)
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %s", cfg.Timeout)
	}
}

func TestNewAPIRequiresEndpoint(t *testing.T) {
	_, err := NewAPI("", "qwen", "hello", 0, 0)
	if err == nil {
		t.Fatal("expected error for missing api endpoint")
	}
}

func TestNewAPIDoesNotSetMetricsEndpoint(t *testing.T) {
	cfg, err := NewAPI("https://api.example.com", "qwen", "hello", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MetricsEndpoint != "" {
		t.Fatalf("expected no metrics endpoint, got %q", cfg.MetricsEndpoint)
	}
	if cfg.Timeout != 0 {
		t.Fatalf("expected no timeout, got %s", cfg.Timeout)
	}
}

func TestNewOfflineAllowsNoTimeout(t *testing.T) {
	cfg, err := NewOffline("qwen", "hello", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timeout != 0 {
		t.Fatalf("expected no timeout, got %s", cfg.Timeout)
	}
	if cfg.MaxTokens != DefaultMaxTokens {
		t.Fatalf("expected default max tokens %d, got %d", DefaultMaxTokens, cfg.MaxTokens)
	}
}
