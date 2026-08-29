package mode

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"inferlens/client"
	"inferlens/config"
	"inferlens/metrics"
	"inferlens/output"
	"inferlens/runtime"
)

const metricsFetchTimeout = 2 * time.Second

type Serve struct {
	baseOptions
	runtime         string
	metricsEndpoint string
}

func (Serve) Name() string { return ServeName }

func (Serve) UsageNote() string { return "" }

func (m *Serve) RegisterFlags(fs *flag.FlagSet, defaults config.ModeDefaults) {
	m.baseOptions.register(fs, defaults)
	fs.StringVar(&m.runtime, "runtime", defaults.Runtime, "Runtime observer: vllm or llamacpp")
	fs.StringVar(
		&m.metricsEndpoint,
		"metrics-endpoint",
		defaults.MetricsEndpoint,
		"Prometheus metrics URL for the selected runtime",
	)
}

func (m *Serve) Config() (config.Config, error) {
	return config.NewServe(
		m.runtime,
		m.endpoint,
		m.metricsEndpoint,
		m.model,
		m.prompt,
		m.maxTokens,
		m.timeout,
	)
}

func (Serve) Ping(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	observer, err := runtime.NewObserver(cfg.Runtime)
	if err != nil {
		return err
	}

	health, healthErr := runtime.CheckHealth(ctx, cfg.Endpoint)

	var observationsErr error
	metricsBefore, err := fetchMetricsSnapshot(ctx, cfg.MetricsEndpoint)
	if err != nil {
		observationsErr = fmt.Errorf("fetch metrics before: %w", err)
	}

	result, probeErr := streamChat(ctx, client.New(cfg.Endpoint), cfg, stdout)

	var metricsAfter metrics.Snapshot
	if observationsErr == nil {
		metricsAfter, err = fetchMetricsSnapshot(ctx, cfg.MetricsEndpoint)
		if err != nil {
			observationsErr = fmt.Errorf("fetch metrics after: %w", err)
		}
	}

	var observations runtime.Report
	if observationsErr == nil {
		observations = observer.Observe(metricsBefore, metricsAfter)
	}

	output.PrintPingReport(stdout, output.PingReport{
		Mode:            ServeName,
		Runtime:         cfg.Runtime,
		Endpoint:        cfg.Endpoint,
		MetricsEndpoint: cfg.MetricsEndpoint,
		Auth:            "none",
		Model:           cfg.Model,
		Result:          result,
		Health:          health,
		HealthErr:       healthErr,
		Observations:    observations,
		ObservationsErr: observationsErr,
		ProbeErr:        probeErr,
	})
	return probeErr
}

func fetchMetricsSnapshot(ctx context.Context, endpoint string) (metrics.Snapshot, error) {
	metricsCtx, cancel := context.WithTimeout(ctx, metricsFetchTimeout)
	defer cancel()
	return metrics.FetchSnapshot(metricsCtx, endpoint)
}
