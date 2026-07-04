package ping

import (
	"reflect"
	"testing"

	"inferlens/pkg/mode"
)

func TestParseArgsSelectsModeAndConfig(t *testing.T) {
	selected, args, configPath, err := parseArgs([]string{
		"--config", "custom.yaml",
		"api",
		"--model", "qwen",
		"--prompt", "api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Name() != mode.APIName {
		t.Fatalf("expected api mode, got %s", selected.Name())
	}
	if configPath != "custom.yaml" {
		t.Fatalf("expected config path, got %q", configPath)
	}
	wantArgs := []string{"--config", "custom.yaml", "--model", "qwen", "--prompt", "api"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("expected args %#v, got %#v", wantArgs, args)
	}
}

func TestParseArgsKeepsUnknownModeAsArgument(t *testing.T) {
	selected, args, _, err := parseArgs([]string{"servee", "--model", "qwen"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Name() != mode.ServeName {
		t.Fatalf("expected default serve mode, got %s", selected.Name())
	}
	if len(args) == 0 || args[0] != "servee" {
		t.Fatalf("expected unknown mode to remain as argument, got %#v", args)
	}
}

func TestParseArgsRejectsMissingConfigValue(t *testing.T) {
	_, _, _, err := parseArgs([]string{"--config"})
	if err == nil {
		t.Fatal("expected missing config value error")
	}
}
