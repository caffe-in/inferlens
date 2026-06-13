package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"inferlens/client"
	"inferlens/config"
	"inferlens/metrics"
	"inferlens/output"
)

const metricsFetchTimeout = 2 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printRootUsage()
		return fmt.Errorf("missing subcommand")
	}

	switch args[0] {
	case "ping":
		return runPing(args[1:])
	case "-h", "--help", "help":
		printRootUsage()
		return nil
	default:
		printRootUsage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runPing(args []string) error {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	model := fs.String("model", "", "Model name served by vLLM")
	prompt := fs.String("prompt", "", "Prompt text to send")
	endpoint := fs.String("endpoint", config.DefaultEndpoint, "Base URL for the vLLM server")
	metricsEndpoint := fs.String("metrics-endpoint", "", "Metrics URL for the vLLM server")
	maxTokens := fs.Int("max-tokens", config.DefaultMaxTokens, "Maximum generated tokens for the probe")
	timeout := fs.Duration("timeout", config.DefaultTimeout, "Timeout for the probe")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: inferlens ping --model <model> --prompt <text> [--endpoint <url>]\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	cfg, err := config.New(*endpoint, *metricsEndpoint, *model, *prompt, *maxTokens, *timeout)
	if err != nil {
		fs.Usage()
		return err
	}

	ctx := context.Background()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	var metricsErr error
	metricsBefore, err := fetchMetricsSnapshot(ctx, cfg.MetricsEndpoint)
	if err != nil {
		metricsErr = err
	}

	vllmClient := client.New(cfg.Endpoint)
	result, probeErr := vllmClient.StreamChat(ctx, client.ChatRequest{
		Model:     cfg.Model,
		Prompt:    cfg.Prompt,
		MaxTokens: cfg.MaxTokens,
	}, func(content string) {
		fmt.Fprint(os.Stdout, content)
	})

	var metricsAfter metrics.Snapshot
	if metricsErr == nil {
		metricsAfter, err = fetchMetricsSnapshot(ctx, cfg.MetricsEndpoint)
		if err != nil {
			metricsErr = err
		}
	}

	output.PrintPingReport(os.Stdout, output.PingReport{
		Endpoint:        cfg.Endpoint,
		MetricsEndpoint: cfg.MetricsEndpoint,
		Model:           cfg.Model,
		Result:          result,
		MetricsBefore:   metricsBefore,
		MetricsAfter:    metricsAfter,
		MetricsErr:      metricsErr,
		ProbeErr:        probeErr,
	})

	return probeErr
}

func fetchMetricsSnapshot(ctx context.Context, endpoint string) (metrics.Snapshot, error) {
	metricsCtx, cancel := context.WithTimeout(ctx, metricsFetchTimeout)
	defer cancel()
	return metrics.FetchSnapshot(metricsCtx, endpoint)
}

func printRootUsage() {
	fmt.Fprintln(os.Stderr, "InferLens probes a vLLM-compatible inference server.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  inferlens ping --model <model> --prompt <text> [--endpoint <url>]")
}
