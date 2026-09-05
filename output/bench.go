package output

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// BenchReport carries the rendered benchmark summary. Latency values are in
// milliseconds.
type BenchReport struct {
	Endpoint     string
	Model        string
	Requests     int
	Concurrency  int
	WallSeconds  float64
	Successes    int
	Failures     int
	RPS          float64
	TokensPerSec float64
	TTFT         LatencySummary
	Total        LatencySummary
	Errors       []BenchError
}

type LatencySummary struct {
	P50 float64
	P95 float64
	Max float64
}

type BenchError struct {
	Count   int
	Message string
}

func PrintBenchReport(w io.Writer, report BenchReport) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "--- inferlens bench ---")
	fmt.Fprintf(w, "endpoint: %s\n", report.Endpoint)
	fmt.Fprintf(w, "model: %s\n", report.Model)
	fmt.Fprintf(w, "requests: %d (concurrency %d)\n", report.Requests, report.Concurrency)
	fmt.Fprintf(w, "wall time: %s\n", formatSeconds(report.WallSeconds))
	fmt.Fprintf(w, "success: %d / failed: %d\n", report.Successes, report.Failures)
	fmt.Fprintf(w, "throughput: %.1f req/s, %.1f tok/s\n", report.RPS, report.TokensPerSec)

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "latency (successful requests):")
	if report.Successes == 0 {
		fmt.Fprintln(w, "  no successful requests to measure")
	} else {
		fmt.Fprintf(w, "  first token: p50 %s, p95 %s, max %s\n",
			formatMs(report.TTFT.P50), formatMs(report.TTFT.P95), formatMs(report.TTFT.Max))
		fmt.Fprintf(w, "  total:       p50 %s, p95 %s, max %s\n",
			formatMs(report.Total.P50), formatMs(report.Total.P95), formatMs(report.Total.Max))
	}

	if len(report.Errors) > 0 {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "errors:")
		for _, benchError := range report.Errors {
			fmt.Fprintf(w, "  %dx %s\n", benchError.Count, benchError.Message)
		}
	}
}

func formatMs(value float64) string {
	return time.Duration(value * float64(time.Millisecond)).String()
}

func formatSeconds(value float64) string {
	return time.Duration(value * float64(time.Second)).Truncate(10 * time.Millisecond).String()
}

// FirstLineOf returns the first line of text, for compact error lists.
func FirstLineOf(text string) string {
	if line, _, ok := strings.Cut(text, "\n"); ok {
		return line
	}
	return text
}
