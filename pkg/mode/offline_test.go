package mode

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunOfflineHelperDecodesJSON(t *testing.T) {
	tmp := t.TempDir()
	helper := filepath.Join(tmp, "helper.sh")
	err := os.WriteFile(helper, []byte(`printf 'INFO vllm startup\n{"content":"hello","load_ms":100,"generate_ms":20,"total_ms":120,"prompt_tokens":3,"generated_tokens":4}\n'`), 0o700)
	if err != nil {
		t.Fatalf("write helper: %v", err)
	}

	result, err := runOfflineHelper(context.Background(), "/bin/sh", helper, "qwen", "hello", 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "hello" {
		t.Fatalf("expected content, got %q", result.Content)
	}
	if result.LoadMS != 100 || result.GenerateMS != 20 || result.TotalMS != 120 {
		t.Fatalf("unexpected timing result: %#v", result)
	}
	if result.PromptTokens != 3 || result.GeneratedTokens != 4 {
		t.Fatalf("unexpected token result: %#v", result)
	}
}

func TestOfflineHelperPathFindsRepositoryScript(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("make scripts dir: %v", err)
	}
	helper := filepath.Join(scriptsDir, "vllm_offline_probe.py")
	if err := os.WriteFile(helper, []byte("print('ok')"), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	got, err := offlineHelperPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Join("scripts", "vllm_offline_probe.py") {
		t.Fatalf("expected repository helper path, got %q", got)
	}
}
