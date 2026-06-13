package metrics

import "testing"

func TestParseSnapshotCoreMetrics(t *testing.T) {
	snapshot := ParseSnapshot(`
# HELP vllm:num_requests_running Running requests.
vllm:request_success_total{model_name="qwen"} 3
vllm:request_success_total{model_name="llama"} 2
vllm:prompt_tokens_total{model_name="qwen"} 42
vllm:generation_tokens_total{model_name="qwen"} 64
vllm:num_requests_running{model_name="qwen"} 1
vllm:num_requests_waiting{model_name="qwen"} 0
vllm:gpu_cache_usage_perc{model_name="qwen"} 0.25
vllm:time_to_first_token_seconds_bucket{le="1"} 12
`)

	core := snapshot.Core()
	if !core.RequestSuccess.Present || core.RequestSuccess.Value != 5 {
		t.Fatalf("expected summed request success 5, got %+v", core.RequestSuccess)
	}
	if !core.PromptTokens.Present || core.PromptTokens.Value != 42 {
		t.Fatalf("expected prompt tokens 42, got %+v", core.PromptTokens)
	}
	if !core.GenerationTokens.Present || core.GenerationTokens.Value != 64 {
		t.Fatalf("expected generation tokens 64, got %+v", core.GenerationTokens)
	}
	if !core.RunningRequests.Present || core.RunningRequests.Value != 1 {
		t.Fatalf("expected running requests 1, got %+v", core.RunningRequests)
	}
	if _, ok := snapshot.Values["vllm:time_to_first_token_seconds_bucket"]; ok {
		t.Fatal("expected histogram buckets to be ignored")
	}
}

func TestDeltaRequiresBothValues(t *testing.T) {
	got := Delta(Value{Value: 1, Present: true}, Value{})
	if got.Present {
		t.Fatalf("expected missing delta, got %+v", got)
	}

	got = Delta(Value{Value: 1, Present: true}, Value{Value: 4, Present: true})
	if !got.Present || got.Value != 3 {
		t.Fatalf("expected delta 3, got %+v", got)
	}
}
