package output

import (
	"fmt"
	"io"
	"math"
	"time"

	"inferlens/client"
	"inferlens/metrics"
	"inferlens/runtime"
)

type PingReport struct {
	Mode            string
	Runtime         string
	Endpoint        string
	MetricsEndpoint string
	Auth            string
	Model           string
	Result          client.StreamResult
	Health          runtime.Health
	HealthErr       error
	Observations    runtime.Report
	ObservationsErr error
	ProbeErr        error
}

type OfflineReport struct {
	Python           string
	Model            string
	LoadDuration     time.Duration
	GenerateDuration time.Duration
	TotalDuration    time.Duration
	PromptTokens     int
	GeneratedTokens  int
	ProbeErr         error
}

type reportHeader struct {
	Mode          string
	Runtime       string
	Endpoint      string
	Model         string
	Python        string
	Auth          string
	Health        string
	Streaming     string
	ServerMetrics string
}

func PrintPingReport(w io.Writer, report PingReport) {
	printReportHeader(w, reportHeader{
		Mode:          valueOr(report.Mode, "serve"),
		Runtime:       report.Runtime,
		Endpoint:      report.Endpoint,
		Model:         report.Model,
		Auth:          report.Auth,
		Health:        healthText(report),
		Streaming:     "required",
		ServerMetrics: serverMetricsText(report),
	})
	if report.Result.StatusCode != 0 {
		fmt.Fprintf(w, "status: %d\n", report.Result.StatusCode)
	}

	printTimeline(w, report.Result)
	printObservations(w, report)
	printDiagnosis(w, report)
}

func PrintOfflineReport(w io.Writer, report OfflineReport) {
	printReportHeader(w, reportHeader{
		Mode:          "offline",
		Model:         report.Model,
		Python:        report.Python,
		Streaming:     "not applicable",
		ServerMetrics: "not available in offline mode",
	})

	if hasOfflineTiming(report) {
		printOfflineTimeline(w, report)
	}
	printOfflineTokens(w, report)
}

func printReportHeader(w io.Writer, header reportHeader) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "--- inferlens ping ---")
	fmt.Fprintf(w, "mode: %s\n", header.Mode)
	if header.Runtime != "" {
		fmt.Fprintf(w, "runtime: %s\n", header.Runtime)
	}
	if header.Endpoint != "" {
		fmt.Fprintf(w, "endpoint: %s\n", header.Endpoint)
	}
	fmt.Fprintf(w, "model: %s\n", header.Model)
	if header.Python != "" {
		fmt.Fprintf(w, "python: %s\n", header.Python)
	}
	if header.Auth != "" {
		fmt.Fprintf(w, "auth: %s\n", header.Auth)
	}
	if header.Health != "" {
		fmt.Fprintf(w, "health: %s\n", header.Health)
	}
	fmt.Fprintf(w, "streaming: %s\n", header.Streaming)
	fmt.Fprintf(w, "server metrics: %s\n", header.ServerMetrics)
}

func printOfflineTimeline(w io.Writer, report OfflineReport) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "offline timeline:")
	fmt.Fprintf(w, "  load: %s\n", formatDuration(report.LoadDuration))
	fmt.Fprintf(w, "  generate: %s\n", formatDuration(report.GenerateDuration))
	fmt.Fprintf(w, "  total: %s\n", formatDuration(report.TotalDuration))
}

func printOfflineTokens(w io.Writer, report OfflineReport) {
	if report.PromptTokens > 0 || report.GeneratedTokens > 0 {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "tokens:")
		if report.PromptTokens > 0 {
			fmt.Fprintf(w, "  prompt: %d\n", report.PromptTokens)
		}
		if report.GeneratedTokens > 0 {
			fmt.Fprintf(w, "  generated: %d\n", report.GeneratedTokens)
		}
	}
}

func hasOfflineTiming(report OfflineReport) bool {
	return report.LoadDuration > 0 || report.GenerateDuration > 0 || report.TotalDuration > 0
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

func printObservations(w io.Writer, report PingReport) {
	fmt.Fprintln(w, "")
	if report.Mode == "api" {
		fmt.Fprintln(w, "server observations:")
		fmt.Fprintln(w, "  not available in api mode")
		return
	}
	if report.ObservationsErr != nil {
		fmt.Fprintln(w, "server observations:")
		fmt.Fprintf(w, "  unavailable: %v\n", report.ObservationsErr)
		return
	}

	fmt.Fprintln(w, "common observations:")
	common := report.Observations.Common
	printObservation(w, common.PromptTokens)
	printObservation(w, common.GeneratedTokens)
	printObservation(w, common.RunningRequests)
	printObservation(w, common.WaitingRequests)

	if len(report.Observations.Native) > 0 {
		fmt.Fprintln(w, "")
		fmt.Fprintf(w, "%s observations:\n", valueOr(report.Runtime, "runtime"))
		for _, observation := range report.Observations.Native {
			printObservation(w, observation)
		}
	}
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
	if report.ObservationsErr != nil {
		fmt.Fprintln(w, "  server metrics were unavailable, so diagnosis is based on the client timeline only")
		return
	}

	waiting := report.Observations.Common.WaitingRequests
	if waiting.After.Present && waiting.After.Value > 0 {
		fmt.Fprintln(w, "  requests were waiting after the probe, which suggests queue pressure")
	} else if waiting.Before.Present && waiting.After.Present {
		fmt.Fprintln(w, "  no queue buildup observed in before/after snapshot")
	}

	gpuCache, ok := findObservation(report.Observations.Native, runtime.KeyGPUKVCache)
	if report.Runtime == runtime.NameVLLM && ok && gpuCache.After.Present && gpuCache.After.Value >= 0.9 {
		fmt.Fprintln(w, "  gpu kv cache usage is high")
	}
}

func serverMetricsText(report PingReport) string {
	if report.Mode == "api" {
		return "not available in api mode"
	}
	return report.MetricsEndpoint
}

func healthText(report PingReport) string {
	if report.Mode == "api" {
		return ""
	}
	if report.HealthErr != nil {
		return fmt.Sprintf("unavailable: %v", report.HealthErr)
	}

	switch {
	case report.Health.StatusCode >= 200 && report.Health.StatusCode < 300:
		return "healthy"
	case report.Health.StatusCode != 0:
		return fmt.Sprintf("unhealthy (status %d)", report.Health.StatusCode)
	default:
		// ponytail: unreachable per CheckHealth's contract; one-line fallback, no defense-in-depth
		return "unavailable"
	}
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func printObservation(w io.Writer, observation runtime.Observation) {
	value := "unavailable"
	switch observation.Format {
	case runtime.FormatCounterDelta:
		delta := metrics.Delta(observation.Before, observation.After)
		if delta.Present {
			value = formatSignedNumber(delta.Value)
		}
	case runtime.FormatGaugeBeforeAfter:
		value = formatObservationValues(observation, formatNumber)
	case runtime.FormatRatioPercentage:
		value = formatObservationValues(observation, formatPercent)
	case runtime.FormatSingleRate:
		if observation.After.Present {
			value = fmt.Sprintf("%.1f tok/s", observation.After.Value)
		}
	}

	if observation.Source == "" {
		fmt.Fprintf(w, "  %s: %s\n", observation.Key, value)
		return
	}
	fmt.Fprintf(w, "  %s: %s  [%s]\n", observation.Key, value, observation.Source)
}

func formatObservationValues(observation runtime.Observation, format func(float64) string) string {
	switch {
	case observation.Before.Present && observation.After.Present:
		return format(observation.Before.Value) + " -> " + format(observation.After.Value)
	case observation.After.Present:
		return format(observation.After.Value)
	case observation.Before.Present:
		return format(observation.Before.Value)
	default:
		return "unavailable"
	}
}

func findObservation(observations []runtime.Observation, key string) (runtime.Observation, bool) {
	for _, observation := range observations {
		if observation.Key == key {
			return observation, true
		}
	}
	return runtime.Observation{}, false
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
