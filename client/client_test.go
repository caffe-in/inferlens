package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamChatSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" from vllm\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := New(server.URL)
	var streamed strings.Builder
	resp, err := client.StreamChat(context.Background(), ChatRequest{
		Model:     "qwen",
		Prompt:    "hello",
		MaxTokens: 8,
	}, func(content string) {
		streamed.WriteString(content)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if resp.Content != "hello from vllm" {
		t.Fatalf("expected content %q, got %q", "hello from vllm", resp.Content)
	}
	if streamed.String() != resp.Content {
		t.Fatalf("expected streamed content %q, got %q", resp.Content, streamed.String())
	}
	if resp.ChunkCount != 2 {
		t.Fatalf("expected 2 chunks, got %d", resp.ChunkCount)
	}
	if resp.ContentDeltaCount != 2 {
		t.Fatalf("expected 2 content deltas, got %d", resp.ContentDeltaCount)
	}
	if resp.FirstTokenAt.IsZero() {
		t.Fatal("expected first token time")
	}
}

func TestStreamChatNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer server.Close()

	client := New(server.URL)
	resp, err := client.StreamChat(context.Background(), ChatRequest{
		Model:  "qwen",
		Prompt: "hello",
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
	if !strings.Contains(err.Error(), "server returned status 400") {
		t.Fatalf("expected runtime-neutral status error, got %v", err)
	}
}

func TestStreamChatBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("expected bearer token, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewWithBearerToken(server.URL, " secret ")
	_, err := client.StreamChat(context.Background(), ChatRequest{
		Model:  "qwen",
		Prompt: "hello",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamChatReasoningContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":null,\"reasoning_content\":\"thinking\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := New(server.URL)
	var streamed strings.Builder
	resp, err := client.StreamChat(context.Background(), ChatRequest{
		Model:  "deepseek-v4-flash",
		Prompt: "hello",
	}, func(content string) {
		streamed.WriteString(content)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "thinking" {
		t.Fatalf("expected reasoning content, got %q", resp.Content)
	}
	if streamed.String() != "thinking" {
		t.Fatalf("expected streamed reasoning content, got %q", streamed.String())
	}
	if resp.ContentDeltaCount != 1 {
		t.Fatalf("expected 1 content delta, got %d", resp.ContentDeltaCount)
	}
	if resp.FirstTokenAt.IsZero() {
		t.Fatal("expected first token time")
	}
}
