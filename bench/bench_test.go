package bench

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"inferlens/client"
)

func sseStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
	_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"there\"}}]}\n\n"))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

func TestRunReportsLatencyAndThroughput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		sseStream(w)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"--endpoint", server.URL,
		"--model", "m",
		"--prompt", "p",
		"--requests", "3",
		"--concurrency", "2",
		"--max-tokens", "8",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"requests: 3 (concurrency 2)",
		"success: 3 / failed: 0",
		"first token: p50",
		"total:       p50",
		"throughput:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in report:\n%s", want, out)
		}
	}
}

func TestRunFailsWhenAllRequestsFail(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"--endpoint", server.URL,
		"--model", "m",
		"--prompt", "p",
		"--requests", "2",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "all 2 requests failed") {
		t.Fatalf("expected all-failed error, got %v", err)
	}
	if !strings.Contains(stdout.String(), "failed: 2") {
		t.Fatalf("expected failure evidence in report:\n%s", stdout.String())
	}
}

func TestRunReportsPartialFailuresWithoutFailing(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1)%2 == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		sseStream(w)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"--endpoint", server.URL,
		"--model", "m",
		"--prompt", "p",
		"--requests", "4",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("partial failure must not fail the run: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "success: 2 / failed: 2") {
		t.Fatalf("expected mixed result:\n%s", out)
	}
	if !strings.Contains(out, "2x server returned status 500") {
		t.Fatalf("expected aggregated errors:\n%s", out)
	}
}

func TestRunValidatesArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"--model", "m", "--prompt", "p"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
	if err := Run([]string{"--endpoint", "http://x", "--model", "m", "--prompt", "p", "--requests", "0"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "requests") {
		t.Fatalf("expected requests error, got %v", err)
	}
}

func TestPercentileNearestRank(t *testing.T) {
	sorted := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	tests := []struct {
		p    float64
		want float64
	}{
		{50, 50},
		{95, 100},
		{100, 100},
	}
	for _, tt := range tests {
		if got := percentile(sorted, tt.p); got != tt.want {
			t.Fatalf("percentile(%v) = %v, want %v", tt.p, got, tt.want)
		}
	}
	if got := percentile(nil, 50); got != 0 {
		t.Fatalf("percentile(nil) = %v, want 0", got)
	}
}

func TestCollectHonoursRequestCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sseStream(w)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	samples, _ := collect(ctx, client.New(server.URL), client.ChatRequest{Model: "m", Prompt: "p", MaxTokens: 8}, 7, 3, 0)
	if len(samples) != 7 {
		t.Fatalf("expected 7 samples, got %d", len(samples))
	}
	for _, sample := range samples {
		if sample.Err != "" {
			t.Fatalf("unexpected sample error: %s", sample.Err)
		}
	}
}
