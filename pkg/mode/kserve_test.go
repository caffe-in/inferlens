package mode

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inferlens/config"
	"inferlens/kserve"
)

const notReadyService = `{
  "metadata": {"name": "qwen", "namespace": "ns", "generation": 4},
  "status": {
    "observedGeneration": 3,
    "conditions": [
      {"type": "PredictorReady", "status": "False", "reason": "PredictorConfigurationReady", "message": "waiting for predictor"},
      {"type": "Ready", "status": "False", "reason": "PredictorReady", "message": "waiting for predictor"}
    ]
  }
}`

func newStreamTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)
	return server
}

func runKServePing(t *testing.T, runner kserve.Runner, endpoint string) (string, error) {
	t.Helper()
	mode := &KServe{runner: runner}
	cfg := config.Config{
		Name:      "qwen",
		Endpoint:  endpoint,
		Model:     "qwen",
		Prompt:    "hello",
		MaxTokens: 16,
	}
	var stdout bytes.Buffer
	err := mode.Ping(context.Background(), cfg, &stdout)
	return stdout.String(), err
}

func TestKServePingAllSuccess(t *testing.T) {
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "pods") {
			return []byte(`{"items": [{
				"metadata": {"name": "qwen-predictor-1"},
				"spec": {"containers": [{"name": "kserve-container", "image": "runtime:v1"}]},
				"status": {"phase": "Running", "conditions": [{"type": "Ready", "status": "True"}]}
			}]}`), nil
		}
		return []byte(readyServiceJSON), nil
	}

	out, err := runKServePing(t, runner, newStreamTestServer(t).URL)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	for _, want := range []string{
		"inference service: qwen",
		"ready: True",
		"generation: 3 (observed 3)",
		"deployment mode: Standard",
		"qwen-predictor-1: Running, ready",
		"control plane: ok",
		"data plane: ok",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in report:\n%s", want, out)
		}
	}
}

func TestKServePingZeroPodsStillSucceeds(t *testing.T) {
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "pods") {
			return []byte(`{"items": []}`), nil
		}
		return []byte(readyServiceJSON), nil
	}

	out, err := runKServePing(t, runner, newStreamTestServer(t).URL)
	if err != nil {
		t.Fatalf("scale-to-zero must not fail the probe: %v", err)
	}
	if !strings.Contains(out, "0 pods matched") {
		t.Fatalf("expected zero-pod evidence:\n%s", out)
	}
}

func TestKServePingNotReadyFails(t *testing.T) {
	runner := func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte(notReadyService), nil
	}

	out, err := runKServePing(t, runner, newStreamTestServer(t).URL)
	if err == nil || !strings.Contains(err.Error(), "kserve not ready: Ready=False") {
		t.Fatalf("expected not-ready error, got %v", err)
	}
	if !strings.Contains(out, "generation: 4 (observed 3, lagging)") {
		t.Fatalf("expected generation lag evidence:\n%s", out)
	}
	if !strings.Contains(out, "data plane: ok") {
		t.Fatalf("inference evidence must survive control-plane failure:\n%s", out)
	}
}

func TestKServePingControlPlaneFailsInferenceSucceeds(t *testing.T) {
	runner := func(_ context.Context, _ ...string) ([]byte, error) {
		return nil, errors.New(`Error from server (NotFound): inferenceservices.serving.kserve.io "qwen" not found`)
	}

	out, err := runKServePing(t, runner, newStreamTestServer(t).URL)
	if err == nil || !strings.Contains(err.Error(), "resource not found") {
		t.Fatalf("expected classified not-found error, got %v", err)
	}
	for _, want := range []string{"unavailable: read InferenceService", "control plane: failed", "data plane: ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in report:\n%s", want, out)
		}
	}
}

func TestKServePingInferenceFailsControlPlaneSucceeds(t *testing.T) {
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "pods") {
			return []byte(`{"items": []}`), nil
		}
		return []byte(readyServiceJSON), nil
	}

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(dead.Close)

	out, err := runKServePing(t, runner, dead.URL)
	if err == nil {
		t.Fatal("expected probe error")
	}
	for _, want := range []string{"ready: True", "control plane: ok", "data plane: failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in report:\n%s", want, out)
		}
	}
}

func TestKServeConfigRequiresNameAndEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		setName bool
		setEP   bool
		wantErr string
	}{
		{name: "missing name", setEP: true, wantErr: "--name"},
		{name: "missing endpoint", setName: true, wantErr: "--endpoint"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode := &KServe{}
			if tt.setName {
				mode.name = "qwen"
			}
			if tt.setEP {
				mode.endpoint = "http://localhost:8080"
			}
			_, err := mode.Config()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q error, got %v", tt.wantErr, err)
			}
		})
	}
}

const readyServiceJSON = `{
  "metadata": {"name": "qwen", "namespace": "ns", "generation": 3},
  "status": {
    "observedGeneration": 3,
    "url": "http://qwen.example.com",
    "deploymentMode": "Standard",
    "clusterServingRuntimeName": "llama-cpp-runtime",
    "conditions": [{"type": "Ready", "status": "True"}],
    "modelStatus": {"states": {"activeModelState": "Loaded", "targetModelState": "Loaded"}, "transitionStatus": "UpToDate"}
  }
}`
