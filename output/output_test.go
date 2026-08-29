package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"inferlens/client"
	"inferlens/metrics"
	"inferlens/runtime"
)

func TestPrintPingReport(t *testing.T) {
	start := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer

	observations := (runtime.VLLM{}).Observe(
		metrics.ParseSnapshot(`
vllm:request_success_total{model_name="qwen"} 10
vllm:prompt_tokens_total{model_name="qwen"} 100
vllm:generation_tokens_total{model_name="qwen"} 200
vllm:num_requests_waiting{model_name="qwen"} 0
vllm:num_requests_running{model_name="qwen"} 0
vllm:gpu_cache_usage_perc{model_name="qwen"} 0.12
`),
		metrics.ParseSnapshot(`
vllm:request_success_total{model_name="qwen"} 11
vllm:prompt_tokens_total{model_name="qwen"} 118
vllm:generation_tokens_total{model_name="qwen"} 264
vllm:num_requests_waiting{model_name="qwen"} 0
vllm:num_requests_running{model_name="qwen"} 0
vllm:gpu_cache_usage_perc{model_name="qwen"} 0.95
`),
	)

	PrintPingReport(&buf, PingReport{
		Mode:     "serve",
		Runtime:  runtime.NameVLLM,
		Endpoint: "http://localhost:8000",
		Model:    "qwen",
		Health:   runtime.Health{StatusCode: 200},
		Result: client.StreamResult{
			StatusCode:        200,
			StartedAt:         start,
			HeadersAt:         start.Add(100 * time.Millisecond),
			FirstChunkAt:      start.Add(200 * time.Millisecond),
			FirstTokenAt:      start.Add(250 * time.Millisecond),
			DoneAt:            start.Add(2 * time.Second),
			ChunkCount:        3,
			ContentDeltaCount: 2,
		},
		Observations: observations,
	})

	got := buf.String()
	for _, want := range []string{
		"--- inferlens ping ---",
		"runtime: vllm",
		"health: healthy",
		"status: 200",
		"first token: 250ms",
		"common observations:",
		"prompt_tokens: +18  [vllm:prompt_tokens_total]",
		"generated_tokens: +64  [vllm:generation_tokens_total]",
		"vllm observations:",
		"request_success: +1",
		"gpu_kv_cache: 12.0% -> 95.0%",
		"first token latency looks normal",
		"gpu kv cache usage is high",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestPrintPingReportWithMetricsError(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()

	PrintPingReport(&buf, PingReport{
		Mode:      "serve",
		Runtime:   runtime.NameLlamaCPP,
		Endpoint:  "http://localhost:8080",
		Model:     "qwen",
		HealthErr: errors.New("health connection refused"),
		Result: client.StreamResult{
			StatusCode: 200,
			StartedAt:  now,
			DoneAt:     now.Add(time.Second),
		},
		ObservationsErr: errors.New("connection refused"),
	})

	got := buf.String()
	if !strings.Contains(got, "unavailable: connection refused") {
		t.Fatalf("expected metrics error in output, got %q", got)
	}
	if !strings.Contains(got, "client timeline only") {
		t.Fatalf("expected degraded diagnosis, got %q", got)
	}
	if !strings.Contains(got, "health: unavailable: health connection refused") {
		t.Fatalf("expected health error in output, got %q", got)
	}
}

func TestPrintPingReportLlamaCPPObservationsHaveNoRuntimeDiagnosis(t *testing.T) {
	var buf bytes.Buffer
	start := time.Now()
	observations := (runtime.LlamaCPP{}).Observe(
		metrics.ParseSnapshot(`
llamacpp:prompt_tokens_total 10
llamacpp:prompt_seconds_total 1
llamacpp:tokens_predicted_total 20
llamacpp:tokens_predicted_seconds_total 2
llamacpp:requests_processing 0
llamacpp:requests_deferred 0
llamacpp:n_tokens_max 2048
`),
		metrics.ParseSnapshot(`
llamacpp:prompt_tokens_total 30
llamacpp:prompt_seconds_total 1.5
llamacpp:tokens_predicted_total 50
llamacpp:tokens_predicted_seconds_total 3.5
llamacpp:requests_processing 0
llamacpp:requests_deferred 0
llamacpp:n_tokens_max 4096
`),
	)

	PrintPingReport(&buf, PingReport{
		Mode:         "serve",
		Runtime:      runtime.NameLlamaCPP,
		Endpoint:     "http://localhost:8080",
		Model:        "qwen",
		Health:       runtime.Health{StatusCode: 200},
		Observations: observations,
		Result: client.StreamResult{
			StartedAt:    start,
			FirstTokenAt: start.Add(100 * time.Millisecond),
			DoneAt:       start.Add(time.Second),
		},
	})

	got := buf.String()
	for _, want := range []string{
		"runtime: llamacpp",
		"prompt_tokens: +20  [llamacpp:prompt_tokens_total]",
		"prompt_rate: 40.0 tok/s",
		"generation_rate: 20.0 tok/s",
		"context_high_watermark: 2048 -> 4096",
		"no queue buildup observed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
	if strings.Contains(got, "gpu kv cache") {
		t.Fatalf("llama.cpp should not produce vllm diagnosis, got %q", got)
	}
}

func TestPrintPingReportAPIMode(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now()

	PrintPingReport(&buf, PingReport{
		Mode:     "api",
		Endpoint: "https://api.example.com",
		Auth:     "bearer",
		Model:    "qwen",
		Result: client.StreamResult{
			StatusCode:   200,
			StartedAt:    now,
			HeadersAt:    now.Add(100 * time.Millisecond),
			FirstChunkAt: now.Add(200 * time.Millisecond),
			FirstTokenAt: now.Add(250 * time.Millisecond),
			DoneAt:       now.Add(time.Second),
		},
	})

	got := buf.String()
	for _, want := range []string{
		"mode: api",
		"auth: bearer",
		"streaming: required",
		"server metrics: not available in api mode",
		"server observations:",
		"server metrics are not available in api mode",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestPrintOfflineReport(t *testing.T) {
	var buf bytes.Buffer

	PrintOfflineReport(&buf, OfflineReport{
		Python:           "python3",
		Model:            "qwen",
		LoadDuration:     2 * time.Second,
		GenerateDuration: 300 * time.Millisecond,
		TotalDuration:    2300 * time.Millisecond,
		PromptTokens:     4,
		GeneratedTokens:  8,
	})

	got := buf.String()
	for _, want := range []string{
		"mode: offline",
		"python: python3",
		"streaming: not applicable",
		"server metrics: not available in offline mode",
		"load: 2s",
		"generated: 8",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestPrintOfflineReportWithErrorOmitsEmptyTimeline(t *testing.T) {
	var buf bytes.Buffer

	PrintOfflineReport(&buf, OfflineReport{
		Python:   "python3",
		Model:    "qwen",
		ProbeErr: errors.New("vllm missing"),
	})

	got := buf.String()
	for _, unwanted := range []string{
		"error:",
		"offline timeline:",
		"unavailable",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("expected output to omit %q, got %q", unwanted, got)
		}
	}
}
