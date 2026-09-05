package ping

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"inferlens/config"
	"inferlens/pkg/mode"
)

func Run(args []string, stdout, stderr io.Writer) error {
	selected, args, configPath, err := parseArgs(args)
	if err != nil {
		return err
	}

	defaults, err := config.LoadPingDefaults(configPath)
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&configPath, "config", configPath, "Path to ping YAML config")
	selected.RegisterFlags(fs, defaults.ForMode(selected.Name()))

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: inferlens ping [serve|api|offline] --model <model> --prompt <text> [--endpoint <url>]\n")
		if selected.UsageNote() != "" {
			fmt.Fprintln(stderr, selected.UsageNote())
		}
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unknown ping mode or argument %q", fs.Arg(0))
	}

	cfg, err := selected.Config()
	if err != nil {
		fs.Usage()
		return err
	}

	ctx := context.Background()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	return selected.Ping(ctx, cfg, stdout)
}

// ponytail: the mode keyword must be the first non-flag token (every documented
// invocation puts it there), so no per-flag "takes a value" list is needed.
// Ceiling: `ping --prompt serve` as a bare first value now needs `ping serve --prompt serve`.
func parseArgs(args []string) (mode.Mode, []string, string, error) {
	configPath, err := extractConfigPath(args)
	if err != nil {
		return nil, nil, "", err
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" {
			i++ // skip its value
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if selected, ok := mode.ByName(arg); ok {
			withoutMode := append(append([]string{}, args[:i]...), args[i+1:]...)
			return selected, withoutMode, configPath, nil
		}
		break // first non-flag token is not a mode; leave it for fs.Parse to reject
	}

	return &mode.Serve{}, args, configPath, nil
}

func extractConfigPath(args []string) (string, error) {
	for i, arg := range args {
		switch {
		case arg == "--config":
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag needs an argument: --config")
			}
			return args[i+1], nil
		case strings.HasPrefix(arg, "--config="):
			return strings.TrimPrefix(arg, "--config="), nil
		}
	}
	return "", nil
}
