// Package bench measures streaming latency and throughput of one
// OpenAI-compatible endpoint across N requests at C concurrency. It is
// client-side only; server observations stay the job of ping serve.
package bench

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"inferlens/client"
	"inferlens/output"
)

func Run(args []string, stdout, stderr io.Writer) error {
	var (
		endpoint    string
		model       string
		prompt      string
		requests    int
		concurrency int
		maxTokens   int
		timeout     time.Duration
		retries     int
	)

	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&endpoint, "endpoint", "", "Base URL for the OpenAI-compatible server (required)")
	fs.StringVar(&model, "model", "", "Model name (required)")
	fs.StringVar(&prompt, "prompt", "", "Prompt text to send (required)")
	fs.IntVar(&requests, "requests", 10, "Total number of requests")
	fs.IntVar(&concurrency, "concurrency", 1, "Parallel workers")
	fs.IntVar(&maxTokens, "max-tokens", 128, "Maximum generated tokens per request")
	fs.DurationVar(&timeout, "timeout", 5*time.Minute, "Overall timeout for the run")
	fs.IntVar(&retries, "retries", 0, "Retry transient failures (connection errors, 5xx) before any content is streamed")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: inferlens bench --endpoint <url> --model <model> --prompt <text> [--requests N] [--concurrency C]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unknown argument %q", fs.Arg(0))
	}
	switch {
	case endpoint == "":
		return fmt.Errorf("endpoint is required")
	case model == "":
		return fmt.Errorf("model is required")
	case prompt == "":
		return fmt.Errorf("prompt is required")
	case requests < 1:
		return fmt.Errorf("requests must be at least 1: %d", requests)
	case concurrency < 1:
		return fmt.Errorf("concurrency must be at least 1: %d", concurrency)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	samples, wall := collect(ctx, client.New(endpoint), client.ChatRequest{
		Model:     model,
		Prompt:    prompt,
		MaxTokens: maxTokens,
	}, requests, concurrency, retries)

	report := summarize(samples, requests, concurrency, wall)
	report.Endpoint = endpoint
	report.Model = model
	output.PrintBenchReport(stdout, report)

	if report.Successes == 0 {
		return fmt.Errorf("all %d requests failed", report.Requests)
	}
	return nil
}

// Sample is one request's outcome. Times are in milliseconds.
type Sample struct {
	StatusCode   int
	FirstTokenMs float64
	TotalMs      float64
	OutputDeltas int
	Err          string
}

func collect(
	ctx context.Context,
	probeClient *client.Client,
	req client.ChatRequest,
	requests, concurrency, retries int,
) ([]Sample, time.Duration) {
	var next atomic.Int64
	samples := make([]Sample, requests)

	var wg sync.WaitGroup
	start := time.Now()
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= requests {
					return
				}
				samples[i] = oneRequest(ctx, probeClient, req, retries)
			}
		}()
	}
	wg.Wait()
	return samples, time.Since(start)
}

func oneRequest(ctx context.Context, probeClient *client.Client, req client.ChatRequest, retries int) Sample {
	result, err := probeClient.StreamChatWithRetries(ctx, req, nil, retries, nil)

	sample := Sample{StatusCode: result.StatusCode, OutputDeltas: result.ContentDeltaCount}
	if !result.FirstTokenAt.IsZero() {
		sample.FirstTokenMs = ms(result.FirstTokenAt.Sub(result.StartedAt))
	}
	if !result.DoneAt.IsZero() {
		sample.TotalMs = ms(result.DoneAt.Sub(result.StartedAt))
	}
	if err != nil {
		sample.Err = err.Error()
	}
	return sample
}

func summarize(samples []Sample, requests, concurrency int, wall time.Duration) output.BenchReport {
	report := output.BenchReport{
		Requests:    requests,
		Concurrency: concurrency,
		WallSeconds: wall.Seconds(),
	}

	var firstToken, total []float64
	totalDeltas := 0
	errors := map[string]int{}
	for _, sample := range samples {
		if sample.Err != "" {
			report.Failures++
			errors[output.FirstLineOf(sample.Err)]++
			continue
		}
		report.Successes++
		if sample.FirstTokenMs > 0 {
			firstToken = append(firstToken, sample.FirstTokenMs)
		}
		if sample.TotalMs > 0 {
			total = append(total, sample.TotalMs)
		}
		totalDeltas += sample.OutputDeltas
	}

	report.TTFT = percentileSummary(firstToken)
	report.Total = percentileSummary(total)
	if wall.Seconds() > 0 {
		report.RPS = float64(report.Successes) / wall.Seconds()
		report.TokensPerSec = float64(totalDeltas) / wall.Seconds()
	}
	for message, count := range errors {
		report.Errors = append(report.Errors, output.BenchError{Count: count, Message: message})
	}
	sort.Slice(report.Errors, func(i, j int) bool { return report.Errors[i].Count > report.Errors[j].Count })

	return report
}

func percentileSummary(values []float64) output.LatencySummary {
	sort.Float64s(values)
	return output.LatencySummary{
		P50: percentile(values, 50),
		P95: percentile(values, 95),
		Max: maxValue(values),
	}
}

// percentile uses nearest-rank on a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func maxValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1] // callers pass sorted slices
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
