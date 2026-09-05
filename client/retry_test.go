package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func shortBackoff(t *testing.T) {
	t.Helper()
	previous := backoffBase
	backoffBase = time.Millisecond
	t.Cleanup(func() { backoffBase = previous })
}

const sseHello = "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"

func TestStreamChatWithRetriesRecoversAfterServerError(t *testing.T) {
	shortBackoff(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseHello))
	}))
	defer server.Close()

	var log strings.Builder
	result, err := New(server.URL).StreamChatWithRetries(
		context.Background(), ChatRequest{Model: "m", Prompt: "p", MaxTokens: 8},
		nil, 2, &log,
	)
	if err != nil {
		t.Fatalf("expected recovery, got %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts.Load())
	}
	if result.ContentDeltaCount != 1 || !strings.Contains(log.String(), "retry 1/2") {
		t.Fatalf("unexpected result/log:\n%+v\n%s", result, log.String())
	}
}

func TestStreamChatWithRetriesDoesNotRetryClientErrors(t *testing.T) {
	shortBackoff(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
	}))
	defer server.Close()

	_, err := New(server.URL).StreamChatWithRetries(
		context.Background(), ChatRequest{Model: "m", Prompt: "p"}, nil, 3, nil,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts.Load() != 1 {
		t.Fatalf("4xx must not be retried, attempts: %d", attempts.Load())
	}
}

func TestStreamChatWithRetriesDoesNotRetryAfterContentStreamed(t *testing.T) {
	shortBackoff(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		// one content delta, then the connection breaks without [DONE]
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"par\"}}]}\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		// give the client time to consume the flushed delta, then abort so
		// the failure happens mid-stream with content already delivered
		time.Sleep(100 * time.Millisecond)
		panic(http.ErrAbortHandler)
	}))
	defer server.Close()

	var streamed strings.Builder
	_, err := New(server.URL).StreamChatWithRetries(
		context.Background(), ChatRequest{Model: "m", Prompt: "p"},
		func(content string) { streamed.WriteString(content) }, 3, nil,
	)
	if err == nil {
		t.Fatal("expected mid-stream error")
	}
	if attempts.Load() != 1 {
		t.Fatalf("partial output must not be retried, attempts: %d", attempts.Load())
	}
	if streamed.String() != "par" {
		t.Fatalf("partial content should have streamed once, got %q", streamed.String())
	}
}

func TestStreamChatWithRetriesExhaustsAttempts(t *testing.T) {
	shortBackoff(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := New(server.URL).StreamChatWithRetries(
		context.Background(), ChatRequest{Model: "m", Prompt: "p"}, nil, 2, nil,
	)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts.Load() != 3 { // initial attempt + 2 retries
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}
