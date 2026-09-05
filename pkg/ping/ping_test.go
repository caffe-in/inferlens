package ping

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

func TestParseArgsTreatsRuntimeAsValueFlag(t *testing.T) {
	selected, args, _, err := parseArgs([]string{
		"--runtime", "llamacpp",
		"--model", "qwen",
		"--prompt", "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Name() != mode.ServeName {
		t.Fatalf("expected serve mode, got %s", selected.Name())
	}
	wantArgs := []string{"--runtime", "llamacpp", "--model", "qwen", "--prompt", "hello"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("expected args %#v, got %#v", wantArgs, args)
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
	for _, runtimeName := range []string{"vllm", "llamacpp"} {
		t.Run(runtimeName, func(t *testing.T) {
			var sawAuthorization atomic.Bool
			server := newRuntimeServer(t, func(_ http.ResponseWriter, r *http.Request) bool {
				if r.Header.Get("Authorization") != "" {
					sawAuthorization.Store(true)
				}
				return false
			})
			defer server.Close()

			t.Setenv("OPENAI_API_KEY", "secret")
			var stdout, stderr bytes.Buffer

			err := Run([]string{
				"serve",
				"--runtime", runtimeName,
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
		})
	}
}

func TestRunServeRuntimeConfigAndCLIOverride(t *testing.T) {
	server := newRuntimeServer(t, nil)
	defer server.Close()

	configPath := writePingConfig(t, "serve:\n  runtime: llamacpp\n  endpoint: "+server.URL+"\n")
	tests := []struct {
		name            string
		additionalArgs  []string
		expectedRuntime string
	}{
		{name: "yaml selects llama cpp", expectedRuntime: "llamacpp"},
		{name: "cli overrides yaml", additionalArgs: []string{"--runtime", "vllm"}, expectedRuntime: "vllm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{
				"serve",
				"--config", configPath,
				"--model", "qwen",
				"--prompt", "hello",
			}
			args = append(args, tt.additionalArgs...)
			var stdout, stderr bytes.Buffer

			err := Run(args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
			}
			if !strings.Contains(stdout.String(), "runtime: "+tt.expectedRuntime) {
				t.Fatalf("expected runtime %q, got %q", tt.expectedRuntime, stdout.String())
			}
		})
	}
}

func TestRunServeRejectsRuntimeBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"serve",
		"--runtime", "unknown",
		"--endpoint", server.URL,
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unsupported runtime") {
		t.Fatalf("expected unsupported runtime error, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("expected no network calls, got %d", calls.Load())
	}
}

func TestRunServeDefaultsToVLLM(t *testing.T) {
	server := newRuntimeServer(t, nil)
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"serve",
		"--endpoint", server.URL,
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "runtime: vllm") {
		t.Fatalf("expected default vllm runtime, got %q", stdout.String())
	}
}

func TestRunServeCallsHealthMetricsAndChatInOrder(t *testing.T) {
	var mu sync.Mutex
	calls := []string{}
	server := newRuntimeServer(t, func(_ http.ResponseWriter, r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, r.URL.Path)
		return false
	})
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"serve",
		"--runtime", "llamacpp",
		"--endpoint", server.URL,
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	want := []string{"/health", "/metrics", "/v1/chat/completions", "/metrics"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("expected calls %#v, got %#v", want, calls)
	}
}

func TestRunServeSGLangReportsObservations(t *testing.T) {
	server := newRuntimeServer(t, nil)
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"serve",
		"--runtime", "sglang",
		"--endpoint", server.URL,
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"runtime: sglang",
		"prompt_tokens: +10  [sglang:prompt_tokens_total]",
		"generated_tokens: +15  [sglang:generation_tokens_total]",
		"gen_throughput: 86.5 tok/s  [sglang:gen_throughput]",
		"sglang observations:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected report to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRunServeBestEffortCollection(t *testing.T) {
	tests := []struct {
		name         string
		handler      func(http.ResponseWriter, *http.Request) bool
		expectedText string
	}{
		{
			name: "health connection closes",
			handler: func(_ http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/health" {
					panic(http.ErrAbortHandler)
				}
				return false
			},
			expectedText: "health: unavailable:",
		},
		{
			name: "metrics disabled",
			handler: func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/metrics" {
					return false
				}
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":{"message":"metrics disabled"}}`))
				return true
			},
			expectedText: "server observations:\n  unavailable:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newRuntimeServer(t, tt.handler)
			defer server.Close()

			var stdout, stderr bytes.Buffer
			err := Run([]string{
				"serve",
				"--runtime", "llamacpp",
				"--endpoint", server.URL,
				"--model", "qwen",
				"--prompt", "hello",
			}, &stdout, &stderr)
			if err != nil {
				t.Fatalf("best-effort collection should not fail the probe: %v", err)
			}
			if !strings.Contains(stdout.String(), tt.expectedText) {
				t.Fatalf("expected output to contain %q, got %q", tt.expectedText, stdout.String())
			}
		})
	}
}

func TestRunServeReturnsInferenceError(t *testing.T) {
	server := newRuntimeServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/v1/chat/completions" {
			return false
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"model unavailable"}}`))
		return true
	})
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"serve",
		"--endpoint", server.URL,
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "server returned status 502: model unavailable") {
		t.Fatalf("expected original inference error, got %v", err)
	}
}

func TestRunServeMetricsAfterFailureIsBestEffort(t *testing.T) {
	var metricsCalls atomic.Int32
	server := newRuntimeServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/metrics" {
			return false
		}
		if metricsCalls.Add(1) == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("metrics failed"))
			return true
		}
		return false
	})
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"serve",
		"--endpoint", server.URL,
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("metrics after failure should not fail the probe: %v", err)
	}
	if !strings.Contains(stdout.String(), "fetch metrics after") {
		t.Fatalf("expected metrics after error in output, got %q", stdout.String())
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

func newRuntimeServer(
	t *testing.T,
	onRequest func(http.ResponseWriter, *http.Request) bool,
) *httptest.Server {
	t.Helper()
	var metricsCalls atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if onRequest != nil && onRequest(w, r) {
			return
		}

		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/metrics":
			call := metricsCalls.Add(1)
			_, _ = w.Write([]byte(runtimeMetrics(call)))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
}

func runtimeMetrics(call int32) string {
	return fmt.Sprintf(
		`vllm:request_success_total %d
vllm:prompt_tokens_total %d
vllm:generation_tokens_total %d
vllm:num_requests_running 0
vllm:num_requests_waiting 0
llamacpp:prompt_tokens_total %d
llamacpp:prompt_seconds_total %d
llamacpp:tokens_predicted_total %d
llamacpp:tokens_predicted_seconds_total %d
llamacpp:requests_processing 0
llamacpp:requests_deferred 0
llamacpp:n_tokens_max 4096
sglang:prompt_tokens_total{model_name="m"} %d
sglang:generation_tokens_total{model_name="m"} %d
sglang:num_running_reqs{model_name="m"} 0
sglang:num_queue_reqs{model_name="m"} 0
sglang:gen_throughput{model_name="m"} 86.5
`,
		call,
		call*10,
		call*20,
		call*10,
		call,
		call*20,
		call*2,
		call*10,
		call*15,
	)
}

func TestRunKServeValidatesBeforeExternalCalls(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"kserve",
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("expected missing --name error, got %v", err)
	}

	err = Run([]string{
		"kserve",
		"--name", "qwen",
		"--model", "qwen",
		"--prompt", "hello",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--endpoint") {
		t.Fatalf("expected missing --endpoint error, got %v", err)
	}
}

func writePingConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferlens.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write ping config: %v", err)
	}
	return path
}
