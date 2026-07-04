package mode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"inferlens/client"
	"inferlens/config"
	"inferlens/metrics"
	"inferlens/output"
)

const metricsFetchTimeout = 2 * time.Second

const (
	ServeName   = "serve"
	APIName     = "api"
	OfflineName = "offline"
)

type Mode interface {
	Name() string
	UsageNote() string
	RegisterFlags(fs *flag.FlagSet)
	Config() (config.Config, error)
	Ping(ctx context.Context, cfg config.Config, stdout io.Writer) error
}

func ByName(name string) (Mode, bool) {
	switch name {
	case ServeName:
		return &Serve{}, true
	case APIName:
		return &API{}, true
	case OfflineName:
		return &Offline{}, true
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

func (API) Ping(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	auth := "none"
	probeClient := client.New(cfg.Endpoint)
	if token := os.Getenv("OPENAI_API_KEY"); token != "" {
		auth = "bearer"
		probeClient = client.NewWithBearerToken(cfg.Endpoint, token)
	}

	result, probeErr := streamChat(ctx, probeClient, cfg, stdout)
	output.PrintPingReport(stdout, output.PingReport{
		Mode:     APIName,
		Endpoint: cfg.Endpoint,
		Auth:     auth,
		Model:    cfg.Model,
		Result:   result,
		ProbeErr: probeErr,
	})
	return probeErr
}

type Offline struct {
	model     string
	prompt    string
	python    string
	maxTokens int
	timeout   time.Duration
}

func (Offline) Name() string { return OfflineName }

func (Offline) UsageNote() string {
	return "offline mode runs vLLM through the local Python environment."
}

func (m *Offline) RegisterFlags(fs *flag.FlagSet) {
	python := os.Getenv("INFERLENS_PYTHON")
	if python == "" {
		python = "python3"
	}
	fs.StringVar(&m.model, "model", "", "Model name")
	fs.StringVar(&m.prompt, "prompt", "", "Prompt text to send")
	fs.StringVar(&m.python, "python", python, "Python interpreter with vLLM installed")
	fs.IntVar(&m.maxTokens, "max-tokens", config.DefaultMaxTokens, "Maximum generated tokens for the probe")
	fs.DurationVar(&m.timeout, "timeout", 0, "Timeout for the probe; 0 means no timeout")
}

func (m *Offline) Config() (config.Config, error) {
	return config.NewOffline(m.model, m.prompt, m.maxTokens, m.timeout)
}

func (m *Offline) Ping(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	helper, err := offlineHelperPath()
	if err != nil {
		output.PrintOfflineReport(stdout, output.OfflineReport{
			Python:   m.python,
			Model:    cfg.Model,
			ProbeErr: err,
		})
		return err
	}

	result, err := runOfflineHelper(ctx, m.python, helper, cfg.Model, cfg.Prompt, cfg.MaxTokens)
	if result.Content != "" {
		fmt.Fprint(stdout, result.Content)
	}

	output.PrintOfflineReport(stdout, output.OfflineReport{
		Python:           m.python,
		Model:            cfg.Model,
		LoadDuration:     time.Duration(result.LoadMS) * time.Millisecond,
		GenerateDuration: time.Duration(result.GenerateMS) * time.Millisecond,
		TotalDuration:    time.Duration(result.TotalMS) * time.Millisecond,
		PromptTokens:     result.PromptTokens,
		GeneratedTokens:  result.GeneratedTokens,
		ProbeErr:         err,
	})
	return err
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

type offlineHelperResult struct {
	Content         string `json:"content"`
	LoadMS          int64  `json:"load_ms"`
	GenerateMS      int64  `json:"generate_ms"`
	TotalMS         int64  `json:"total_ms"`
	PromptTokens    int    `json:"prompt_tokens,omitempty"`
	GeneratedTokens int    `json:"generated_tokens,omitempty"`
}

func runOfflineHelper(ctx context.Context, python, helper, model, prompt string, maxTokens int) (offlineHelperResult, error) {
	cmd := exec.CommandContext(ctx, python, helper, "--model", model, "--prompt", prompt, "--max-tokens", fmt.Sprint(maxTokens))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return offlineHelperResult{}, fmt.Errorf("run offline helper: %s", message)
	}

	var result offlineHelperResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return offlineHelperResult{}, fmt.Errorf("decode offline helper output: %w", err)
	}
	return result, nil
}

func offlineHelperPath() (string, error) {
	candidates := []string{filepath.Join("scripts", "vllm_offline_probe.py")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "scripts", "vllm_offline_probe.py"))
	}

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	return "", errors.New("offline helper not found; run from the repository root or install the scripts directory next to the inferlens binary")
}
