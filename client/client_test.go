package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello from vllm"}}]}`))
	}))
	defer server.Close()

	client := New(server.URL)
	resp, status, err := client.Chat(context.Background(), ChatRequest{
		Model:  "qwen",
		Prompt: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}
	if resp.Content != "hello from vllm" {
		t.Fatalf("expected content %q, got %q", "hello from vllm", resp.Content)
	}
}

func TestChatNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer server.Close()

	client := New(server.URL)
	_, status, err := client.Chat(context.Background(), ChatRequest{
		Model:  "qwen",
		Prompt: "hello",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, status)
	}
}
