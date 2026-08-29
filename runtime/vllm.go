package runtime

import (
	"inferlens/metrics"
)

// VLLM maps vLLM Prometheus metrics into runtime observations.
type VLLM struct{}

// Observation keys exposed by VLLM.Observe that other packages need to
// reference by name (e.g. output diagnosis rules).
const (
	KeyGPUKVCache = "gpu_kv_cache"
)

// Observe maps before and after Prometheus snapshots into a normalized report.
func (VLLM) Observe(before, after metrics.Snapshot) Report {
	return Report{
		Common: Common{
			PromptTokens: observation(
				"prompt_tokens",
				FormatCounterDelta,
				before,
				after,
				"vllm:prompt_tokens_total",
				"vllm:prompt_tokens",
			),
			GeneratedTokens: observation(
				"generated_tokens",
				FormatCounterDelta,
				before,
				after,
				"vllm:generation_tokens_total",
				"vllm:generation_tokens",
			),
			RunningRequests: observation(
				"running_requests",
				FormatGaugeBeforeAfter,
				before,
				after,
				"vllm:num_requests_running",
			),
			WaitingRequests: observation(
				"waiting_requests",
				FormatGaugeBeforeAfter,
				before,
				after,
				"vllm:num_requests_waiting",
			),
		},
		Native: []Observation{
			observation(
				"request_success",
				FormatCounterDelta,
				before,
				after,
				"vllm:request_success_total",
				"vllm:request_success",
			),
			observation(
				KeyGPUKVCache,
				FormatRatioPercentage,
				before,
				after,
				"vllm:gpu_cache_usage_perc",
				"vllm:kv_cache_usage_perc",
			),
			observation(
				"cpu_kv_cache",
				FormatRatioPercentage,
				before,
				after,
				"vllm:cpu_cache_usage_perc",
			),
			observation(
				"preemptions",
				FormatCounterDelta,
				before,
				after,
				"vllm:num_preemptions_total",
				"vllm:num_preemptions",
			),
			observation(
				"prefix_cache_hits",
				FormatCounterDelta,
				before,
				after,
				"vllm:prefix_cache_hits_total",
				"vllm:prefix_cache_hits",
			),
			observation(
				"prefix_cache_queries",
				FormatCounterDelta,
				before,
				after,
				"vllm:prefix_cache_queries_total",
				"vllm:prefix_cache_queries",
			),
		},
	}
}
