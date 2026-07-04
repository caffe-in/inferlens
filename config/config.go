package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const DefaultEndpoint = "http://localhost:8000"
const DefaultMaxTokens = 128
const DefaultTimeout = 60 * time.Second

type Config struct {
	Endpoint        string
	MetricsEndpoint string
	Model           string
	Prompt          string
	MaxTokens       int
	Timeout         time.Duration
}

func New(endpoint, metricsEndpoint, model, prompt string, maxTokens int, timeout time.Duration) (Config, error) {
	return newConfig(endpoint, metricsEndpoint, model, prompt, maxTokens, timeout, true)
}

func NewAPI(endpoint, model, prompt string, maxTokens int, timeout time.Duration) (Config, error) {
	if strings.TrimSpace(endpoint) == "" {
		return Config{}, errors.New("api endpoint is required; pass --endpoint or set OPENAI_BASE_URL")
	}
	cfg, err := newConfig(endpoint, "", model, prompt, maxTokens, timeout, false)
	if err != nil {
		return Config{}, err
	}
	cfg.MetricsEndpoint = ""
	return cfg, nil
}

func newConfig(endpoint, metricsEndpoint, model, prompt string, maxTokens int, timeout time.Duration, defaultEndpoint bool) (Config, error) {
	cfg := Config{
		Endpoint:        strings.TrimSpace(endpoint),
		MetricsEndpoint: strings.TrimSpace(metricsEndpoint),
		Model:           strings.TrimSpace(model),
		Prompt:          strings.TrimSpace(prompt),
		MaxTokens:       maxTokens,
		Timeout:         timeout,
	}

	if cfg.Endpoint == "" && defaultEndpoint {
		cfg.Endpoint = DefaultEndpoint
	}
	if cfg.MetricsEndpoint == "" && cfg.Endpoint != "" {
		cfg.MetricsEndpoint = strings.TrimRight(cfg.Endpoint, "/") + "/metrics"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	switch {
	case cfg.Model == "":
		return Config{}, errors.New("model is required")
	case cfg.Prompt == "":
		return Config{}, errors.New("prompt is required")
	case cfg.Endpoint == "":
		return Config{}, errors.New("endpoint is required")
	case !strings.HasPrefix(cfg.Endpoint, "http://") && !strings.HasPrefix(cfg.Endpoint, "https://"):
		return Config{}, fmt.Errorf("endpoint must start with http:// or https://: %s", cfg.Endpoint)
	case !strings.HasPrefix(cfg.MetricsEndpoint, "http://") && !strings.HasPrefix(cfg.MetricsEndpoint, "https://"):
		return Config{}, fmt.Errorf("metrics endpoint must start with http:// or https://: %s", cfg.MetricsEndpoint)
	case cfg.MaxTokens < 0:
		return Config{}, fmt.Errorf("max tokens must be greater than or equal to 0: %d", cfg.MaxTokens)
	case cfg.Timeout < 0:
		return Config{}, fmt.Errorf("timeout must be greater than or equal to 0: %s", cfg.Timeout)
	default:
		return cfg, nil
	}
}
