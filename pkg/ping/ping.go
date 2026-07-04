package ping

import (
	"context"
	"flag"
	"fmt"
	"io"

	"inferlens/output"
	"inferlens/pkg/mode"
)

func Run(args []string, stdout, stderr io.Writer) error {
	var selected mode.Mode = &mode.Serve{}
	if len(args) > 0 {
		if next, ok := mode.ByName(args[0]); ok {
			selected = next
			args = args[1:]
		} else if args[0] == "offline" {
			return fmt.Errorf("offline mode is not implemented yet")
		}
	}

	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	fs.SetOutput(stderr)
	selected.RegisterFlags(fs)

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: inferlens ping [serve|api] --model <model> --prompt <text> [--endpoint <url>]\n")
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

	report, probeErr := selected.Ping(ctx, cfg, stdout)
	output.PrintPingReport(stdout, report)
	return probeErr
}
