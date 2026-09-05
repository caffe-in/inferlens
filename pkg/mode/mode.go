package mode

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"inferlens/client"
	"inferlens/config"
)

const (
	ServeName   = "serve"
	APIName     = "api"
	OfflineName = "offline"
	KServeName  = "kserve"
)

// Names lists ping mode names in a stable order; usage strings derive from it.
func Names() []string {
	return []string{ServeName, APIName, OfflineName, KServeName}
}

type Mode interface {
	Name() string
	UsageNote() string
	RegisterFlags(fs *flag.FlagSet, defaults config.ModeDefaults)
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
	case KServeName:
		return &KServe{}, true
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

func (o *baseOptions) register(fs *flag.FlagSet, defaults config.ModeDefaults) {
	fs.StringVar(&o.model, "model", "", "Model name")
	fs.StringVar(&o.prompt, "prompt", "", "Prompt text to send")
	fs.StringVar(&o.endpoint, "endpoint", defaults.Endpoint, "Base URL for the OpenAI-compatible server")
	fs.IntVar(&o.maxTokens, "max-tokens", defaults.MaxTokens, "Maximum generated tokens for the probe")
	fs.DurationVar(&o.timeout, "timeout", defaults.Timeout, "Timeout for the probe")
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
