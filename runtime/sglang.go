package runtime

import (
	"inferlens/metrics"
)

// SGLang maps SGLang Prometheus metrics into runtime observations.
type SGLang struct{}

// Observe maps before and after Prometheus snapshots into a normalized report.
func (SGLang) Observe(before, after metrics.Snapshot) Report {
	return Report{
		Common: Common{
			PromptTokens: observation(
				"prompt_tokens",
				FormatCounterDelta,
				before,
				after,
				"sglang:prompt_tokens_total",
			),
			GeneratedTokens: observation(
				"generated_tokens",
				FormatCounterDelta,
				before,
				after,
				"sglang:generation_tokens_total",
			),
			RunningRequests: observation(
				"running_requests",
				FormatGaugeBeforeAfter,
				before,
				after,
				"sglang:num_running_reqs",
			),
			WaitingRequests: observation(
				"waiting_requests",
				FormatGaugeBeforeAfter,
				before,
				after,
				"sglang:num_queue_reqs",
			),
		},
		Native: []Observation{
			observation(
				"token_usage",
				FormatRatioPercentage,
				before,
				after,
				"sglang:token_usage",
			),
			observation(
				"cache_hit_rate",
				FormatRatioPercentage,
				before,
				after,
				"sglang:cache_hit_rate",
			),
			observation(
				"gen_throughput",
				FormatSingleRate,
				before,
				after,
				"sglang:gen_throughput",
			),
			observation(
				"num_used_tokens",
				FormatGaugeBeforeAfter,
				before,
				after,
				"sglang:num_used_tokens",
			),
		},
	}
}
