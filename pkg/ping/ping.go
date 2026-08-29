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

func parseArgs(args []string) (mode.Mode, []string, string, error) {
	var selected mode.Mode = &mode.Serve{}
	modeSelected := false
	configPath := ""
	parsed := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config":
			if i+1 >= len(args) {
				return nil, nil, "", fmt.Errorf("flag needs an argument: --config")
			}
			configPath = args[i+1]
			parsed = append(parsed, arg, args[i+1])
			i++
		case strings.HasPrefix(arg, "--config="):
			configPath = strings.TrimPrefix(arg, "--config=")
			parsed = append(parsed, arg)
		case !modeSelected && !strings.HasPrefix(arg, "-"):
			next, ok := mode.ByName(arg)
			if ok {
				selected = next
				modeSelected = true
				continue
			}
			parsed = append(parsed, arg)
		default:
			parsed = append(parsed, arg)
			if flagTakesValue(arg) && i+1 < len(args) {
				i++
				parsed = append(parsed, args[i])
			}
		}
	}

	return selected, parsed, configPath, nil
}

func flagTakesValue(arg string) bool {
	if !strings.HasPrefix(arg, "--") || strings.Contains(arg, "=") {
		return false
	}
	switch arg {
	case "--model", "--prompt", "--endpoint", "--metrics-endpoint", "--runtime", "--max-tokens", "--timeout", "--python":
		return true
	default:
		return false
	}
}
