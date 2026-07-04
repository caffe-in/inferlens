package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"inferlens/client"
	"inferlens/metrics"
)

func TestPrintPingReport(t *testing.T) {
	start := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer

	PrintPingReport(&buf, PingReport{
		Endpoint: "http://localhost:8000",
		Model:    "qwen",
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
		MetricsBefore: metrics.ParseSnapshot(`
vllm:request_success_total{model_name="qwen"} 10
vllm:prompt_tokens_total{model_name="qwen"} 100
vllm:generation_tokens_total{model_name="qwen"} 200
vllm:num_requests_waiting{model_name="qwen"} 0
vllm:num_requests_running{model_name="qwen"} 0
vllm:gpu_cache_usage_perc{model_name="qwen"} 0.12
`),
		MetricsAfter: metrics.ParseSnapshot(`
vllm:request_success_total{model_name="qwen"} 11
vllm:prompt_tokens_total{model_name="qwen"} 118
vllm:generation_tokens_total{model_name="qwen"} 264
vllm:num_requests_waiting{model_name="qwen"} 0
vllm:num_requests_running{model_name="qwen"} 0
vllm:gpu_cache_usage_perc{model_name="qwen"} 0.13
`),
	})

	got := buf.String()
	for _, want := range []string{
		"--- inferlens ping ---",
		"status: 200",
		"first token: 250ms",
		"request_success: +1",
		"prompt_tokens: +18",
		"generation_tokens: +64",
		"gpu_kv_cache: 12.0% -> 13.0%",
		"first token latency looks normal",
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
		Endpoint: "http://localhost:8000",
		Model:    "qwen",
		Result: client.StreamResult{
			StatusCode: 200,
			StartedAt:  now,
			DoneAt:     now.Add(time.Second),
		},
		MetricsErr: errors.New("connection refused"),
	})

	got := buf.String()
	if !strings.Contains(got, "unavailable: connection refused") {
		t.Fatalf("expected metrics error in output, got %q", got)
	}
	if !strings.Contains(got, "client timeline only") {
		t.Fatalf("expected degraded diagnosis, got %q", got)
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
