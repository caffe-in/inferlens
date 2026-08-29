package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPingDefaultsMergesFilesAndEnv(t *testing.T) {
	tmp := t.TempDir()
	defaultPath := writeTestConfig(t, tmp, "default.yaml", `
serve:
  runtime: llamacpp
  endpoint: http://localhost:9000
  metrics_endpoint: http://localhost:9000/metrics
api:
  timeout: 10s
offline:
  python: python-default
`)
	userPath := writeTestConfig(t, tmp, "user.yaml", `
api:
  endpoint: https://file.example.com
  timeout: 5s
offline:
  python: python-file
  timeout: 0
`)
	t.Setenv("OPENAI_BASE_URL", "https://env.example.com")
	t.Setenv("INFERLENS_PYTHON", "python-env")

	defaults, err := loadPingDefaults(defaultPath, userPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if defaults.Serve.Endpoint != "http://localhost:9000" {
		t.Fatalf("expected serve endpoint from default config, got %q", defaults.Serve.Endpoint)
	}
	if defaults.Serve.Runtime != "llamacpp" {
		t.Fatalf("expected serve runtime from default config, got %q", defaults.Serve.Runtime)
	}
	if defaults.Serve.MetricsEndpoint != "http://localhost:9000/metrics" {
		t.Fatalf("expected serve metrics endpoint from default config, got %q", defaults.Serve.MetricsEndpoint)
	}
	if defaults.API.Endpoint != "https://env.example.com" {
		t.Fatalf("expected api endpoint from env, got %q", defaults.API.Endpoint)
	}
	if defaults.API.Timeout != 5*time.Second {
		t.Fatalf("expected api timeout from user config, got %s", defaults.API.Timeout)
	}
	if defaults.Offline.Python != "python-env" {
		t.Fatalf("expected offline python from env, got %q", defaults.Offline.Python)
	}
	if defaults.Offline.Timeout != 0 {
		t.Fatalf("expected offline timeout 0, got %s", defaults.Offline.Timeout)
	}
}

func TestLoadPingDefaultsRejectsUnknownFields(t *testing.T) {
	tmp := t.TempDir()
	userPath := writeTestConfig(t, tmp, "bad.yaml", `
serve:
  unknown: true
`)

	_, err := loadPingDefaults(filepath.Join(tmp, "missing.yaml"), userPath)
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected known fields error, got %v", err)
	}
}

func writeTestConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}
