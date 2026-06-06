package config

import (
	"errors"
	"fmt"
	"strings"
)

const DefaultEndpoint = "http://localhost:8000"

type Config struct {
	Endpoint string
	Model    string
	Prompt   string
}

func New(endpoint, model, prompt string) (Config, error) {
	cfg := Config{
		Endpoint: strings.TrimSpace(endpoint),
		Model:    strings.TrimSpace(model),
		Prompt:   strings.TrimSpace(prompt),
	}

	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}

	switch {
	case cfg.Model == "":
		return Config{}, errors.New("model is required")
	case cfg.Prompt == "":
		return Config{}, errors.New("prompt is required")
	case !strings.HasPrefix(cfg.Endpoint, "http://") && !strings.HasPrefix(cfg.Endpoint, "https://"):
		return Config{}, fmt.Errorf("endpoint must start with http:// or https://: %s", cfg.Endpoint)
	default:
		return cfg, nil
	}
}
