package runtime

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"inferlens/metrics"
)

func TestVLLMObserveSupportsMetricAliases(t *testing.T) {
	tests := []struct {
		name              string
		requestMetric     string
		promptMetric      string
		generationMetric  string
		gpuCacheMetric    string
		preemptionMetric  string
		prefixHitsMetric  string
		prefixQueryMetric string
	}{
		{
			name:              "current names",
			requestMetric:     "vllm:request_success_total",
			promptMetric:      "vllm:prompt_tokens_total",
			generationMetric:  "vllm:generation_tokens_total",
			gpuCacheMetric:    "vllm:gpu_cache_usage_perc",
			preemptionMetric:  "vllm:num_preemptions_total",
			prefixHitsMetric:  "vllm:prefix_cache_hits_total",
			prefixQueryMetric: "vllm:prefix_cache_queries_total",
		},
		{
			name:              "legacy names",
			requestMetric:     "vllm:request_success",
			promptMetric:      "vllm:prompt_tokens",
			generationMetric:  "vllm:generation_tokens",
			gpuCacheMetric:    "vllm:kv_cache_usage_perc",
			preemptionMetric:  "vllm:num_preemptions",
			prefixHitsMetric:  "vllm:prefix_cache_hits",
			prefixQueryMetric: "vllm:prefix_cache_queries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := metrics.ParseSnapshot(
				tt.requestMetric + " 10\n" +
					tt.promptMetric + " 100\n" +
					tt.generationMetric + " 200\n" +
					"vllm:num_requests_running 1\n" +
					"vllm:num_requests_waiting 0\n" +
					tt.gpuCacheMetric + " 0.25\n" +
					"vllm:cpu_cache_usage_perc 0.10\n" +
					tt.preemptionMetric + " 2\n" +
					tt.prefixHitsMetric + " 3\n" +
					tt.prefixQueryMetric + " 4\n",
			)
			after := metrics.ParseSnapshot(
				tt.requestMetric + " 11\n" +
					tt.promptMetric + " 118\n" +
					tt.generationMetric + " 232\n" +
					"vllm:num_requests_running 0\n" +
					"vllm:num_requests_waiting 1\n" +
					tt.gpuCacheMetric + " 0.30\n" +
					"vllm:cpu_cache_usage_perc 0.12\n" +
					tt.preemptionMetric + " 3\n" +
					tt.prefixHitsMetric + " 5\n" +
					tt.prefixQueryMetric + " 7\n",
			)

			report := (VLLM{}).Observe(before, after)
			assertDelta(t, report.Common.PromptTokens, 18)
			assertDelta(t, report.Common.GeneratedTokens, 32)
			assertValues(t, report.Common.RunningRequests, 1, 0)
			assertValues(t, report.Common.WaitingRequests, 0, 1)

			assertDelta(t, findNative(t, report, "request_success"), 1)
			assertValues(t, findNative(t, report, "gpu_kv_cache"), 0.25, 0.30)
			assertDelta(t, findNative(t, report, "preemptions"), 1)
			assertDelta(t, findNative(t, report, "prefix_cache_hits"), 2)
			assertDelta(t, findNative(t, report, "prefix_cache_queries"), 3)
		})
	}
}

func TestLlamaCPPObserveDerivesRates(t *testing.T) {
	before := metrics.ParseSnapshot(`
llamacpp:prompt_tokens_total 100
llamacpp:prompt_seconds_total 2
llamacpp:tokens_predicted_total 200
llamacpp:tokens_predicted_seconds_total 10
llamacpp:requests_processing 1
llamacpp:requests_deferred 0
llamacpp:n_tokens_max 2048
`)
	after := metrics.ParseSnapshot(`
llamacpp:prompt_tokens_total 118
llamacpp:prompt_seconds_total 2.4
llamacpp:tokens_predicted_total 232
llamacpp:tokens_predicted_seconds_total 11.6
llamacpp:requests_processing 0
llamacpp:requests_deferred 0
llamacpp:n_tokens_max 4096
`)

	report := (LlamaCPP{}).Observe(before, after)
	assertDelta(t, report.Common.PromptTokens, 18)
	assertDelta(t, report.Common.GeneratedTokens, 32)
	assertValues(t, report.Common.RunningRequests, 1, 0)

	promptRate := findNative(t, report, "prompt_rate")
	if !promptRate.After.Present || math.Abs(promptRate.After.Value-45) > 0.000001 {
		t.Fatalf("expected prompt rate 45, got %+v", promptRate.After)
	}
	generationRate := findNative(t, report, "generation_rate")
	if !generationRate.After.Present || math.Abs(generationRate.After.Value-20) > 0.000001 {
		t.Fatalf("expected generation rate 20, got %+v", generationRate.After)
	}
	contextHighWatermark := findNative(t, report, "context_high_watermark")
	if !contextHighWatermark.After.Present || contextHighWatermark.After.Value != 4096 {
		t.Fatalf("expected context high watermark 4096, got %+v", contextHighWatermark.After)
	}
}

func TestLlamaCPPObserveOmitsRateWithoutPositiveSecondsDelta(t *testing.T) {
	tests := []struct {
		name          string
		beforeSeconds string
		afterSeconds  string
	}{
		{name: "missing seconds", beforeSeconds: "", afterSeconds: ""},
		{name: "zero seconds delta", beforeSeconds: "1", afterSeconds: "1"},
		{name: "negative seconds delta", beforeSeconds: "2", afterSeconds: "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := metrics.ParseSnapshot("llamacpp:prompt_tokens_total 10\n" + metricLine("llamacpp:prompt_seconds_total", tt.beforeSeconds))
			after := metrics.ParseSnapshot("llamacpp:prompt_tokens_total 20\n" + metricLine("llamacpp:prompt_seconds_total", tt.afterSeconds))

			report := (LlamaCPP{}).Observe(before, after)
			promptRate := findNative(t, report, "prompt_rate")
			if promptRate.After.Present {
				t.Fatalf("expected unavailable prompt rate, got %+v", promptRate.After)
			}
		})
	}
}

func TestRuntimeHealth(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "healthy", statusCode: http.StatusOK},
		{name: "loading", statusCode: http.StatusServiceUnavailable},
		{name: "missing", statusCode: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/health" {
					t.Fatalf("unexpected health path %q", r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			health, err := CheckHealth(context.Background(), server.URL)
			if err != nil {
				t.Fatalf("unexpected health error: %v", err)
			}
			if health.StatusCode != tt.statusCode {
				t.Fatalf("unexpected health result: %+v", health)
			}
		})
	}
}

func TestRuntimeHealthConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()

	health, err := CheckHealth(context.Background(), endpoint)
	if err == nil {
		t.Fatal("expected connection error")
	}
	if health.StatusCode != 0 {
		t.Fatalf("expected unknown health, got %+v", health)
	}
}

func findNative(t *testing.T, report Report, key string) Observation {
	t.Helper()
	for _, observation := range report.Native {
		if observation.Key == key {
			return observation
		}
	}
	t.Fatalf("native observation %q not found", key)
	return Observation{}
}

func assertDelta(t *testing.T, observation Observation, expected float64) {
	t.Helper()
	delta := metrics.Delta(observation.Before, observation.After)
	if !delta.Present || delta.Value != expected {
		t.Fatalf("expected delta %.3f for %s, got %+v", expected, observation.Key, delta)
	}
}

func assertValues(t *testing.T, observation Observation, expectedBefore, expectedAfter float64) {
	t.Helper()
	if !observation.Before.Present || observation.Before.Value != expectedBefore {
		t.Fatalf("expected before %.3f for %s, got %+v", expectedBefore, observation.Key, observation.Before)
	}
	if !observation.After.Present || observation.After.Value != expectedAfter {
		t.Fatalf("expected after %.3f for %s, got %+v", expectedAfter, observation.Key, observation.After)
	}
}

func metricLine(name, value string) string {
	if value == "" {
		return ""
	}
	return name + " " + value + "\n"
}
