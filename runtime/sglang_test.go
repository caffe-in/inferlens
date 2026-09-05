package runtime

import (
	"math"
	"testing"

	"inferlens/metrics"
)

// Fixture lines use SGLang's {model_name="..."} labels and include histogram
// sub-series to prove the parser strips labels and skips histograms.
const sglangBefore = `sglang:prompt_tokens_total{model_name="llama"} 100
sglang:generation_tokens_total{model_name="llama"} 90
sglang:num_running_reqs{model_name="llama"} 2
sglang:num_queue_reqs{model_name="llama"} 3
sglang:token_usage{model_name="llama"} 0.2
sglang:cache_hit_rate{model_name="llama"} 0.1
sglang:gen_throughput{model_name="llama"} 80
sglang:num_used_tokens{model_name="llama"} 1000
sglang:time_to_first_token_seconds_sum{model_name="llama"} 2.35
sglang:time_to_first_token_seconds_count{model_name="llama"} 11008
`

const sglangAfter = `sglang:prompt_tokens_total{model_name="llama"} 136
sglang:generation_tokens_total{model_name="llama"} 105
sglang:num_running_reqs{model_name="llama"} 0
sglang:num_queue_reqs{model_name="llama"} 0
sglang:token_usage{model_name="llama"} 0.28
sglang:cache_hit_rate{model_name="llama"} 0.0075
sglang:gen_throughput{model_name="llama"} 86.5
sglang:num_used_tokens{model_name="llama"} 123859
`

func TestSGLangObserveMapsLabeledMetrics(t *testing.T) {
	before := metrics.ParseSnapshot(sglangBefore)
	after := metrics.ParseSnapshot(sglangAfter)

	report := (SGLang{}).Observe(before, after)

	assertDelta(t, report.Common.PromptTokens, 36)
	assertDelta(t, report.Common.GeneratedTokens, 15)
	assertValues(t, report.Common.RunningRequests, 2, 0)
	assertValues(t, report.Common.WaitingRequests, 3, 0)

	if report.Common.PromptTokens.Source != "sglang:prompt_tokens_total" {
		t.Fatalf("expected source name, got %q", report.Common.PromptTokens.Source)
	}

	tokenUsage := findNative(t, report, "token_usage")
	assertValues(t, tokenUsage, 0.2, 0.28)
	cacheHitRate := findNative(t, report, "cache_hit_rate")
	assertValues(t, cacheHitRate, 0.1, 0.0075)
	genThroughput := findNative(t, report, "gen_throughput")
	if !genThroughput.After.Present || math.Abs(genThroughput.After.Value-86.5) > 1e-9 {
		t.Fatalf("expected gen throughput 86.5, got %+v", genThroughput.After)
	}
	usedTokens := findNative(t, report, "num_used_tokens")
	assertValues(t, usedTokens, 1000, 123859)
}

func TestSGLangObserveMetricsDisabled(t *testing.T) {
	report := (SGLang{}).Observe(metrics.Snapshot{}, metrics.Snapshot{})

	if report.Common.PromptTokens.Source == "" {
		t.Fatal("expected source name to fall back to the documented metric name")
	}
	if report.Common.PromptTokens.After.Present {
		t.Fatal("expected prompt tokens to be unavailable")
	}
}
