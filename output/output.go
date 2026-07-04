package output

import (
	"fmt"
	"io"
	"math"
	"time"

	"inferlens/client"
	"inferlens/metrics"
)

type PingReport struct {
	Mode            string
	Endpoint        string
	MetricsEndpoint string
	Auth            string
	Model           string
	Result          client.StreamResult
	MetricsBefore   metrics.Snapshot
	MetricsAfter    metrics.Snapshot
	MetricsErr      error
	ProbeErr        error
}

func PrintPingReport(w io.Writer, report PingReport) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "--- inferlens ping ---")
	fmt.Fprintf(w, "mode: %s\n", valueOr(report.Mode, "serve"))
	fmt.Fprintf(w, "endpoint: %s\n", report.Endpoint)
	fmt.Fprintf(w, "model: %s\n", report.Model)
	if report.Auth != "" {
		fmt.Fprintf(w, "auth: %s\n", report.Auth)
	}
	fmt.Fprintln(w, "streaming: required")
	printServerMetricsLine(w, report)
	if report.Result.StatusCode != 0 {
		fmt.Fprintf(w, "status: %d\n", report.Result.StatusCode)
	}
	if report.ProbeErr != nil {
		fmt.Fprintf(w, "error: %v\n", report.ProbeErr)
	}

	printTimeline(w, report.Result)
	printMetrics(w, report)
	printDiagnosis(w, report)
}

func printTimeline(w io.Writer, result client.StreamResult) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "client timeline:")
	fmt.Fprintf(w, "  total: %s\n", formatDuration(result.DoneAt.Sub(result.StartedAt)))
	fmt.Fprintf(w, "  headers: %s\n", formatSince(result.StartedAt, result.HeadersAt))
	fmt.Fprintf(w, "  first chunk: %s\n", formatSince(result.StartedAt, result.FirstChunkAt))
	fmt.Fprintf(w, "  first token: %s\n", formatSince(result.StartedAt, result.FirstTokenAt))
	fmt.Fprintf(w, "  stream: %s\n", formatBetween(result.FirstChunkAt, result.DoneAt))
	fmt.Fprintf(w, "  chunks: %d\n", result.ChunkCount)
	fmt.Fprintf(w, "  content deltas: %d\n", result.ContentDeltaCount)
	fmt.Fprintf(w, "  output rate: %s\n", formatRate(result))
}

func printMetrics(w io.Writer, report PingReport) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "vllm metrics:")
	if report.Mode == "api" {
		fmt.Fprintln(w, "  not available in api mode")
		return
	}
	if report.MetricsErr != nil {
		fmt.Fprintf(w, "  unavailable: %v\n", report.MetricsErr)
		return
	}

	before := report.MetricsBefore.Core()
	after := report.MetricsAfter.Core()
	printDelta(w, "request_success", before.RequestSuccess, after.RequestSuccess)
	printDelta(w, "prompt_tokens", before.PromptTokens, after.PromptTokens)
	printDelta(w, "generation_tokens", before.GenerationTokens, after.GenerationTokens)
	printBeforeAfter(w, "waiting", before.WaitingRequests, after.WaitingRequests)
	printBeforeAfter(w, "running", before.RunningRequests, after.RunningRequests)
	printPercentBeforeAfter(w, "gpu_kv_cache", before.GPUCacheUsage, after.GPUCacheUsage)
	printPercentBeforeAfter(w, "cpu_kv_cache", before.CPUCacheUsage, after.CPUCacheUsage)
	printDelta(w, "preemptions", before.Preemptions, after.Preemptions)
	printDelta(w, "prefix_cache_hits", before.PrefixCacheHits, after.PrefixCacheHits)
	printDelta(w, "prefix_cache_queries", before.PrefixCacheQueries, after.PrefixCacheQueries)
}

func printDiagnosis(w io.Writer, report PingReport) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "diagnosis:")
	if report.ProbeErr != nil {
		fmt.Fprintln(w, "  probe request failed before a complete stream was observed")
		return
	}

	firstToken := report.Result.FirstTokenAt.Sub(report.Result.StartedAt)
	if report.Result.FirstTokenAt.IsZero() {
		fmt.Fprintln(w, "  no content token was observed in the stream")
	} else if firstToken > 2*time.Second {
		fmt.Fprintln(w, "  first token latency is high for an interactive probe")
	} else {
		fmt.Fprintln(w, "  first token latency looks normal")
	}

	if report.Mode == "api" {
		fmt.Fprintln(w, "  server metrics are not available in api mode; diagnosis is based on the client timeline only")
		return
	}
	if report.MetricsErr != nil {
		fmt.Fprintln(w, "  server metrics were unavailable, so diagnosis is based on the client timeline only")
		return
	}

	after := report.MetricsAfter.Core()
	before := report.MetricsBefore.Core()
	if after.WaitingRequests.Present && after.WaitingRequests.Value > 0 {
		fmt.Fprintln(w, "  requests were waiting after the probe, which suggests queue pressure")
	} else if before.WaitingRequests.Present && after.WaitingRequests.Present {
		fmt.Fprintln(w, "  no queue buildup observed in before/after snapshot")
	}
	if after.GPUCacheUsage.Present && after.GPUCacheUsage.Value >= 0.9 {
		fmt.Fprintln(w, "  gpu kv cache usage is high")
	}
}

func printServerMetricsLine(w io.Writer, report PingReport) {
	if report.Mode == "api" {
		fmt.Fprintln(w, "server metrics: not available in api mode")
		return
	}
	fmt.Fprintf(w, "server metrics: %s\n", report.MetricsEndpoint)
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func printDelta(w io.Writer, label string, before, after metrics.Value) {
	delta := metrics.Delta(before, after)
	if !delta.Present {
		return
	}
	fmt.Fprintf(w, "  %s: %s\n", label, formatSignedNumber(delta.Value))
}

func printBeforeAfter(w io.Writer, label string, before, after metrics.Value) {
	if !before.Present || !after.Present {
		return
	}
	fmt.Fprintf(w, "  %s: %s -> %s\n", label, formatNumber(before.Value), formatNumber(after.Value))
}

func printPercentBeforeAfter(w io.Writer, label string, before, after metrics.Value) {
	if !before.Present || !after.Present {
		return
	}
	fmt.Fprintf(w, "  %s: %s -> %s\n", label, formatPercent(before.Value), formatPercent(after.Value))
}

func formatSince(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return "unavailable"
	}
	return formatDuration(end.Sub(start))
}

func formatBetween(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return "unavailable"
	}
	return formatDuration(end.Sub(start))
}

func formatDuration(duration time.Duration) string {
	if duration <= 0 {
		return "unavailable"
	}
	return duration.Round(time.Millisecond).String()
}

func formatRate(result client.StreamResult) string {
	if result.FirstTokenAt.IsZero() || result.DoneAt.IsZero() || result.ContentDeltaCount == 0 {
		return "unavailable"
	}
	seconds := result.DoneAt.Sub(result.FirstTokenAt).Seconds()
	if seconds <= 0 {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f deltas/s", float64(result.ContentDeltaCount)/seconds)
}

func formatNumber(value float64) string {
	if math.Abs(value-math.Round(value)) < 0.000001 {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.3f", value)
}

func formatSignedNumber(value float64) string {
	if value >= 0 {
		return "+" + formatNumber(value)
	}
	return formatNumber(value)
}

func formatPercent(value float64) string {
	if value <= 1 {
		value *= 100
	}
	return fmt.Sprintf("%.1f%%", value)
}
