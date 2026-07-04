package mode

import (
	"context"
	"flag"
	"io"
	"time"

	"inferlens/client"
	"inferlens/config"
	"inferlens/metrics"
	"inferlens/output"
)

const metricsFetchTimeout = 2 * time.Second

type Serve struct {
	baseOptions
	metricsEndpoint string
}

func (Serve) Name() string { return ServeName }

func (Serve) UsageNote() string { return "" }

func (m *Serve) RegisterFlags(fs *flag.FlagSet, defaults config.ModeDefaults) {
	m.baseOptions.register(fs, defaults)
	fs.StringVar(&m.metricsEndpoint, "metrics-endpoint", defaults.MetricsEndpoint, "Metrics URL for the vLLM server")
}

func (m *Serve) Config() (config.Config, error) {
	return config.NewServe(m.endpoint, m.metricsEndpoint, m.model, m.prompt, m.maxTokens, m.timeout)
}

func (Serve) Ping(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	var metricsErr error
	metricsBefore, err := fetchMetricsSnapshot(ctx, cfg.MetricsEndpoint)
	if err != nil {
		metricsErr = err
	}

	result, probeErr := streamChat(ctx, client.New(cfg.Endpoint), cfg, stdout)

	var metricsAfter metrics.Snapshot
	if metricsErr == nil {
		metricsAfter, err = fetchMetricsSnapshot(ctx, cfg.MetricsEndpoint)
		if err != nil {
			metricsErr = err
		}
	}

	output.PrintPingReport(stdout, output.PingReport{
		Mode:            ServeName,
		Endpoint:        cfg.Endpoint,
		MetricsEndpoint: cfg.MetricsEndpoint,
		Auth:            "none",
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
