package runtime

import (
	"inferlens/metrics"
)

// LlamaCPP maps llama.cpp Prometheus metrics into runtime observations.
type LlamaCPP struct{}

// Observe maps before and after Prometheus snapshots into a normalized report.
func (LlamaCPP) Observe(before, after metrics.Snapshot) Report {
	return Report{
		Common: Common{
			PromptTokens: observation(
				"prompt_tokens",
				FormatCounterDelta,
				before,
				after,
				"llamacpp:prompt_tokens_total",
			),
			GeneratedTokens: observation(
				"generated_tokens",
				FormatCounterDelta,
				before,
				after,
				"llamacpp:tokens_predicted_total",
			),
			RunningRequests: observation(
				"running_requests",
				FormatGaugeBeforeAfter,
				before,
				after,
				"llamacpp:requests_processing",
			),
			WaitingRequests: observation(
				"waiting_requests",
				FormatGaugeBeforeAfter,
				before,
				after,
				"llamacpp:requests_deferred",
			),
		},
		Native: []Observation{
			rateObservation(
				"prompt_rate",
				before,
				after,
				"llamacpp:prompt_tokens_total",
				"llamacpp:prompt_seconds_total",
			),
			rateObservation(
				"generation_rate",
				before,
				after,
				"llamacpp:tokens_predicted_total",
				"llamacpp:tokens_predicted_seconds_total",
			),
			{
				Key:    "context_high_watermark",
				Source: "llamacpp:n_tokens_max",
				Before: before.First("llamacpp:n_tokens_max"),
				After:  after.First("llamacpp:n_tokens_max"),
				Format: FormatGaugeBeforeAfter,
			},
		},
	}
}

func rateObservation(
	key string,
	before metrics.Snapshot,
	after metrics.Snapshot,
	tokensName string,
	secondsName string,
) Observation {
	// Below this window, elapsed-second measurements are too noisy to divide
	// by; a near-zero denominator would otherwise blow up into a nonsensical
	// or infinite rate.
	const minSeconds = 0.001

	value := metrics.Value{}
	tokens := metrics.Delta(before.First(tokensName), after.First(tokensName))
	seconds := metrics.Delta(before.First(secondsName), after.First(secondsName))
	if tokens.Present && seconds.Present && seconds.Value >= minSeconds {
		value = metrics.Value{
			Value:   tokens.Value / seconds.Value,
			Present: true,
		}
	}

	return Observation{
		Key:    key,
		Source: tokensName + " / " + secondsName,
		After:  value,
		Format: FormatSingleRate,
	}
}
