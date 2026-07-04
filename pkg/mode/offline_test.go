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
	err := os.WriteFile(helper, []byte(`printf '{"content":"hello","load_ms":100,"generate_ms":20,"total_ms":120,"prompt_tokens":3,"generated_tokens":4}'`), 0o700)
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
