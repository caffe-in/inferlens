package ping

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"inferlens/pkg/mode"
)

func TestParseArgsSelectsModeAndConfig(t *testing.T) {
	selected, args, configPath, err := parseArgs([]string{
		"--config", "custom.yaml",
		"api",
		"--model", "qwen",
		"--prompt", "api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Name() != mode.APIName {
		t.Fatalf("expected api mode, got %s", selected.Name())
	}
	if configPath != "custom.yaml" {
		t.Fatalf("expected config path, got %q", configPath)
	}
	wantArgs := []string{"--config", "custom.yaml", "--model", "qwen", "--prompt", "api"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("expected args %#v, got %#v", wantArgs, args)
	}
}

func TestParseArgsKeepsUnknownModeAsArgument(t *testing.T) {
	selected, args, _, err := parseArgs([]string{"servee", "--model", "qwen"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Name() != mode.ServeName {
		t.Fatalf("expected default serve mode, got %s", selected.Name())
	}
	if len(args) == 0 || args[0] != "servee" {
		t.Fatalf("expected unknown mode to remain as argument, got %#v", args)
	}
}

func TestParseArgsRejectsMissingConfigValue(t *testing.T) {
	_, _, _, err := parseArgs([]string{"--config"})
	if err == nil {
		t.Fatal("expected missing config value error")
	}
}

func TestRunUsesConfigEndpointAsFlagDefault(t *testing.T) {
	server := newStreamingServer(t, nil)
	defer server.Close()

	configPath := writePingConfig(t, "api:\n  endpoint: "+server.URL+"\n")
	var stdout, stderr bytes.Buffer

	err := Run([]string{
		"api",
		"--config", configPath,
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	got := stdout.String()
	if !strings.Contains(got, "endpoint: "+server.URL) {
		t.Fatalf("expected config endpoint in output, got %q", got)
	}
	if !strings.Contains(got, "mode: api") {
		t.Fatalf("expected api mode in output, got %q", got)
	}
}

func TestRunCLITimeoutZeroOverridesDefault(t *testing.T) {
	server := newStreamingServer(t, nil)
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"api",
		"--endpoint", server.URL,
		"--model", "qwen",
		"--prompt", "hello",
		"--timeout", "0",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	got := stdout.String()
	if strings.Contains(got, "total: unavailable") {
		t.Fatalf("expected --timeout 0 to disable timeout, got %q", got)
	}
}

func TestRunCLIEndpointOverridesEnvAndConfig(t *testing.T) {
	configServer := newStreamingServer(t, func(r *http.Request) {
		t.Fatalf("config endpoint should not be called: %s", r.URL.Path)
	})
	defer configServer.Close()
	envServer := newStreamingServer(t, func(r *http.Request) {
		t.Fatalf("env endpoint should not be called: %s", r.URL.Path)
	})
	defer envServer.Close()

	var cliCalls int32
	cliServer := newStreamingServer(t, func(r *http.Request) {
		atomic.AddInt32(&cliCalls, 1)
	})
	defer cliServer.Close()

	t.Setenv("OPENAI_BASE_URL", envServer.URL)
	configPath := writePingConfig(t, "api:\n  endpoint: "+configServer.URL+"\n")
	var stdout, stderr bytes.Buffer

	err := Run([]string{
		"api",
		"--config", configPath,
		"--endpoint", cliServer.URL,
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}
	if atomic.LoadInt32(&cliCalls) != 1 {
		t.Fatalf("expected cli endpoint to be called once, got %d", cliCalls)
	}
}

func TestRunServeIgnoresOpenAIAPIKey(t *testing.T) {
	var sawAuthorization atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawAuthorization.Store(true)
		}

		switch r.URL.Path {
		case "/metrics":
			_, _ = w.Write([]byte("vllm:request_success_total 1\n"))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("OPENAI_API_KEY", "secret")
	var stdout, stderr bytes.Buffer

	err := Run([]string{
		"serve",
		"--endpoint", server.URL,
		"--metrics-endpoint", server.URL + "/metrics",
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}
	if sawAuthorization.Load() {
		t.Fatal("serve mode should not send Authorization")
	}
}

func newStreamingServer(t *testing.T, onRequest func(*http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if onRequest != nil {
			onRequest(r)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
}

func writePingConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferlens.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write ping config: %v", err)
	}
	return path
}
