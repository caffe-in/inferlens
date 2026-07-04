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

	"inferlens/config"
	"inferlens/output"
)

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

func (m *Offline) RegisterFlags(fs *flag.FlagSet, defaults config.ModeDefaults) {
	fs.StringVar(&m.model, "model", "", "Model name")
	fs.StringVar(&m.prompt, "prompt", "", "Prompt text to send")
	fs.StringVar(&m.python, "python", defaults.Python, "Python interpreter with vLLM installed")
	fs.IntVar(&m.maxTokens, "max-tokens", defaults.MaxTokens, "Maximum generated tokens for the probe")
	fs.DurationVar(&m.timeout, "timeout", defaults.Timeout, "Timeout for the probe; 0 means no timeout")
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
