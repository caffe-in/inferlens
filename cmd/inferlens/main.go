package main

import (
	"fmt"
	"os"
	"strings"

	"inferlens/pkg/ping"
	"inferlens/runtime"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printRootUsage()
		return fmt.Errorf("missing subcommand")
	}

	switch args[0] {
	case "ping":
		return ping.Run(args[1:], os.Stdout, os.Stderr)
	case "-h", "--help", "help":
		printRootUsage()
		return nil
	default:
		printRootUsage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func printRootUsage() {
	fmt.Fprintln(os.Stderr, "InferLens probes self-hosted and OpenAI-compatible inference servers.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  inferlens ping [serve] --runtime <%s> --model <model> --prompt <text> [--endpoint <url>]\n", strings.Join(runtime.Names(), "|"))
	fmt.Fprintln(os.Stderr, "  inferlens ping api --model <model> --prompt <text> --endpoint <url>")
	fmt.Fprintln(os.Stderr, "  inferlens ping offline --model <model> --prompt <text> [--python python3]")
}
