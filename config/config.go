package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"inferlens/runtime"
)

const DefaultEndpoint = "http://localhost:8000"
const DefaultMaxTokens = 128
const DefaultTimeout = 60 * time.Second

type Config struct {
	Runtime         string
	Endpoint        string
	MetricsEndpoint string
	Model           string
	Prompt          string
	MaxTokens       int
	Timeout         time.Duration
	Name            string
	Namespace       string
	Kubeconfig      string
	KubeContext     string
}

func NewServe(
	runtimeName string,
	endpoint string,
	metricsEndpoint string,
	model string,
	prompt string,
	maxTokens int,
	timeout time.Duration,
) (Config, error) {
	cfg := newOnlineConfig(endpoint, metricsEndpoint, model, prompt, maxTokens, timeout)
	cfg.Runtime = strings.TrimSpace(runtimeName)
	if cfg.Runtime == "" {
		cfg.Runtime = runtime.NameVLLM
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	if cfg.MetricsEndpoint == "" {
		cfg.MetricsEndpoint = strings.TrimRight(cfg.Endpoint, "/") + "/metrics"
	}
	if _, err := runtime.NewObserver(cfg.Runtime); err != nil {
		return Config{}, err
	}
	if err := validateOnlineConfig(cfg); err != nil {
		return Config{}, err
	}
	if !isHTTPURL(cfg.MetricsEndpoint) {
		return Config{}, fmt.Errorf("metrics endpoint must start with http:// or https://: %s", cfg.MetricsEndpoint)
	}
	return cfg, nil
}

func NewAPI(endpoint, model, prompt string, maxTokens int, timeout time.Duration) (Config, error) {
	cfg := newOnlineConfig(endpoint, "", model, prompt, maxTokens, timeout)
	if cfg.Endpoint == "" {
		return Config{}, errors.New("api endpoint is required; pass --endpoint or set OPENAI_BASE_URL")
	}
	if err := validateOnlineConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func NewKServe(name, endpoint, model, prompt string, maxTokens int, timeout time.Duration) (Config, error) {
	cfg := newOnlineConfig(endpoint, "", model, prompt, maxTokens, timeout)
	cfg.Name = strings.TrimSpace(name)
	if cfg.Name == "" {
		return Config{}, errors.New("inference service name is required; pass --name")
	}
	if cfg.Endpoint == "" {
		return Config{}, errors.New("kserve endpoint is required; establish a port-forward and pass --endpoint")
	}
	if err := validateOnlineConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func NewOffline(model, prompt string, maxTokens int, timeout time.Duration) (Config, error) {
	cfg := Config{
		Model:     strings.TrimSpace(model),
		Prompt:    strings.TrimSpace(prompt),
		MaxTokens: maxTokens,
		Timeout:   timeout,
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}

	switch {
	case cfg.Model == "":
		return Config{}, errors.New("model is required")
	case cfg.Prompt == "":
		return Config{}, errors.New("prompt is required")
	case cfg.MaxTokens < 0:
		return Config{}, fmt.Errorf("max tokens must be greater than or equal to 0: %d", cfg.MaxTokens)
	case cfg.Timeout < 0:
		return Config{}, fmt.Errorf("timeout must be greater than or equal to 0: %s", cfg.Timeout)
	default:
		return cfg, nil
	}
}

func newOnlineConfig(endpoint, metricsEndpoint, model, prompt string, maxTokens int, timeout time.Duration) Config {
	cfg := Config{
		Endpoint:        strings.TrimSpace(endpoint),
		MetricsEndpoint: strings.TrimSpace(metricsEndpoint),
		Model:           strings.TrimSpace(model),
		Prompt:          strings.TrimSpace(prompt),
		MaxTokens:       maxTokens,
		Timeout:         timeout,
	}

	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	return cfg
}

func validateOnlineConfig(cfg Config) error {
	switch {
	case cfg.Model == "":
		return errors.New("model is required")
	case cfg.Prompt == "":
		return errors.New("prompt is required")
	case cfg.Endpoint == "":
		return errors.New("endpoint is required")
	case !isHTTPURL(cfg.Endpoint):
		return fmt.Errorf("endpoint must start with http:// or https://: %s", cfg.Endpoint)
	case cfg.MaxTokens < 0:
		return fmt.Errorf("max tokens must be greater than or equal to 0: %d", cfg.MaxTokens)
	case cfg.Timeout < 0:
		return fmt.Errorf("timeout must be greater than or equal to 0: %s", cfg.Timeout)
	default:
		return nil
	}
}

func isHTTPURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
