package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const DefaultConfigPath = "cfg/default.yaml"

type ModeDefaults struct {
	Endpoint        string
	MetricsEndpoint string
	Python          string
	MaxTokens       int
	Timeout         time.Duration
}

type PingDefaults struct {
	Serve   ModeDefaults
	API     ModeDefaults
	Offline ModeDefaults
}

func BuiltinPingDefaults() PingDefaults {
	return PingDefaults{
		Serve: ModeDefaults{
			Endpoint:  DefaultEndpoint,
			MaxTokens: DefaultMaxTokens,
			Timeout:   DefaultTimeout,
		},
		API: ModeDefaults{
			MaxTokens: DefaultMaxTokens,
			Timeout:   DefaultTimeout,
		},
		Offline: ModeDefaults{
			Python:    "python3",
			MaxTokens: DefaultMaxTokens,
			Timeout:   0,
		},
	}
}

func LoadPingDefaults(configPath string) (PingDefaults, error) {
	return loadPingDefaults(DefaultConfigPath, configPath)
}

func (d PingDefaults) ForMode(name string) ModeDefaults {
	switch name {
	case "api":
		return d.API
	case "offline":
		return d.Offline
	default:
		return d.Serve
	}
}

func loadPingDefaults(defaultPath, configPath string) (PingDefaults, error) {
	defaults := BuiltinPingDefaults()
	if err := mergePingConfigFile(&defaults, defaultPath, false); err != nil {
		return PingDefaults{}, err
	}
	if configPath != "" {
		if err := mergePingConfigFile(&defaults, configPath, true); err != nil {
			return PingDefaults{}, err
		}
	}
	applyPingEnv(&defaults)
	return defaults, nil
}

func applyPingEnv(defaults *PingDefaults) {
	if endpoint := os.Getenv("OPENAI_BASE_URL"); endpoint != "" {
		defaults.API.Endpoint = endpoint
	}
	if python := os.Getenv("INFERLENS_PYTHON"); python != "" {
		defaults.Offline.Python = python
	}
}

func mergePingConfigFile(defaults *PingDefaults, path string, required bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if !required && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var file pingConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode config %s: %w", path, err)
	}

	mergeModeDefaults(&defaults.Serve, file.Serve)
	mergeModeDefaults(&defaults.API, file.API)
	mergeModeDefaults(&defaults.Offline, file.Offline)
	return nil
}

func mergeModeDefaults(defaults *ModeDefaults, cfg modeConfig) {
	if cfg.Endpoint != nil {
		defaults.Endpoint = strings.TrimSpace(*cfg.Endpoint)
	}
	if cfg.MetricsEndpoint != nil {
		defaults.MetricsEndpoint = strings.TrimSpace(*cfg.MetricsEndpoint)
	}
	if cfg.Python != nil {
		defaults.Python = strings.TrimSpace(*cfg.Python)
	}
	if cfg.MaxTokens != nil {
		defaults.MaxTokens = *cfg.MaxTokens
	}
	if cfg.Timeout.Set {
		defaults.Timeout = cfg.Timeout.Value
	}
}

type pingConfig struct {
	Serve   modeConfig `yaml:"serve"`
	API     modeConfig `yaml:"api"`
	Offline modeConfig `yaml:"offline"`
}

type modeConfig struct {
	Endpoint        *string       `yaml:"endpoint"`
	MetricsEndpoint *string       `yaml:"metrics_endpoint"`
	Python          *string       `yaml:"python"`
	MaxTokens       *int          `yaml:"max_tokens"`
	Timeout         durationValue `yaml:"timeout"`
}

type durationValue struct {
	Value time.Duration
	Set   bool
}

func (d *durationValue) UnmarshalYAML(value *yaml.Node) error {
	d.Set = true
	if value.Value == "0" {
		d.Value = 0
		return nil
	}

	duration, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", value.Value, err)
	}
	d.Value = duration
	return nil
}
