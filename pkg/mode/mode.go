package mode

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"inferlens/client"
	"inferlens/config"
	"inferlens/metrics"
	"inferlens/output"
)

const metricsFetchTimeout = 2 * time.Second

const (
	ServeName = "serve"
	APIName   = "api"
)

type Mode interface {
	Name() string
	UsageNote() string
	RegisterFlags(fs *flag.FlagSet)
	Config() (config.Config, error)
	Ping(ctx context.Context, cfg config.Config, stdout io.Writer) (output.PingReport, error)
}

func ByName(name string) (Mode, bool) {
	switch name {
	case ServeName:
		return &Serve{}, true
	case APIName:
		return &API{}, true
	default:
		return nil, false
	}
}

type baseOptions struct {
	model     string
	prompt    string
	endpoint  string
	maxTokens int
	timeout   time.Duration
}

func (o *baseOptions) register(fs *flag.FlagSet, endpoint string) {
	fs.StringVar(&o.model, "model", "", "Model name")
	fs.StringVar(&o.prompt, "prompt", "", "Prompt text to send")
	fs.StringVar(&o.endpoint, "endpoint", endpoint, "Base URL for the OpenAI-compatible server")
	fs.IntVar(&o.maxTokens, "max-tokens", config.DefaultMaxTokens, "Maximum generated tokens for the probe")
	fs.DurationVar(&o.timeout, "timeout", config.DefaultTimeout, "Timeout for the probe")
}

type Serve struct {
	baseOptions
	metricsEndpoint string
}

func (Serve) Name() string { return ServeName }

func (Serve) UsageNote() string { return "" }

func (m *Serve) RegisterFlags(fs *flag.FlagSet) {
	m.baseOptions.register(fs, config.DefaultEndpoint)
	fs.StringVar(&m.metricsEndpoint, "metrics-endpoint", "", "Metrics URL for the vLLM server")
}

func (m *Serve) Config() (config.Config, error) {
	return config.New(m.endpoint, m.metricsEndpoint, m.model, m.prompt, m.maxTokens, m.timeout)
}

func (Serve) Ping(ctx context.Context, cfg config.Config, stdout io.Writer) (output.PingReport, error) {
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

	return output.PingReport{
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
	}, probeErr
}

type API struct {
	baseOptions
}

func (API) Name() string { return APIName }

func (API) UsageNote() string {
	return "api mode requires a streaming OpenAI-compatible chat completions endpoint."
}

func (m *API) RegisterFlags(fs *flag.FlagSet) {
	m.baseOptions.register(fs, os.Getenv("OPENAI_BASE_URL"))
}

func (m *API) Config() (config.Config, error) {
	return config.NewAPI(m.endpoint, m.model, m.prompt, m.maxTokens, m.timeout)
}

func (API) Ping(ctx context.Context, cfg config.Config, stdout io.Writer) (output.PingReport, error) {
	auth := "none"
	probeClient := client.New(cfg.Endpoint)
	if token := os.Getenv("OPENAI_API_KEY"); token != "" {
		auth = "bearer"
		probeClient = client.NewWithBearerToken(cfg.Endpoint, token)
	}

	result, probeErr := streamChat(ctx, probeClient, cfg, stdout)
	return output.PingReport{
		Mode:     APIName,
		Endpoint: cfg.Endpoint,
		Auth:     auth,
		Model:    cfg.Model,
		Result:   result,
		ProbeErr: probeErr,
	}, probeErr
}

func streamChat(ctx context.Context, probeClient *client.Client, cfg config.Config, stdout io.Writer) (client.StreamResult, error) {
	return probeClient.StreamChat(ctx, client.ChatRequest{
		Model:     cfg.Model,
		Prompt:    cfg.Prompt,
		MaxTokens: cfg.MaxTokens,
	}, func(content string) {
		fmt.Fprint(stdout, content)
	})
}

func fetchMetricsSnapshot(ctx context.Context, endpoint string) (metrics.Snapshot, error) {
	metricsCtx, cancel := context.WithTimeout(ctx, metricsFetchTimeout)
	defer cancel()
	return metrics.FetchSnapshot(metricsCtx, endpoint)
}
