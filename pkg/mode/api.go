package mode

import (
	"context"
	"flag"
	"io"
	"os"

	"inferlens/client"
	"inferlens/config"
	"inferlens/output"
)

type API struct {
	baseOptions
}

func (API) Name() string { return APIName }

func (API) UsageNote() string {
	return "api mode requires a streaming OpenAI-compatible chat completions endpoint."
}

func (m *API) RegisterFlags(fs *flag.FlagSet, defaults config.ModeDefaults) {
	m.baseOptions.register(fs, defaults)
}

func (m *API) Config() (config.Config, error) {
	cfg, err := config.NewAPI(m.endpoint, m.model, m.prompt, m.maxTokens, m.timeout)
	cfg.Retries = m.retries
	return cfg, err
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
