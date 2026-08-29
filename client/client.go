package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL     string
	bearerToken string
	httpClient  *http.Client
}

type ChatRequest struct {
	Model     string
	Prompt    string
	MaxTokens int
}

type StreamResult struct {
	StatusCode        int
	Content           string
	StartedAt         time.Time
	HeadersAt         time.Time
	FirstChunkAt      time.Time
	FirstTokenAt      time.Time
	DoneAt            time.Time
	ChunkCount        int
	ContentDeltaCount int
}

type openAIChatRequest struct {
	Model     string              `json:"model"`
	Messages  []openAIChatMessage `json:"messages"`
	Stream    bool                `json:"stream"`
	MaxTokens int                 `json:"max_tokens,omitempty"`
}

type openAIChatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type openAIStreamResponse struct {
	Choices []struct {
		Delta openAIChatMessage `json:"delta"`
	} `json:"choices"`
}

type apiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{},
	}
}

func NewWithBearerToken(baseURL, token string) *Client {
	c := New(baseURL)
	c.bearerToken = strings.TrimSpace(token)
	return c
}

func (c *Client) StreamChat(ctx context.Context, req ChatRequest, onContent func(string)) (StreamResult, error) {
	result := StreamResult{StartedAt: time.Now()}
	body, err := json.Marshal(openAIChatRequest{
		Model: req.Model,
		Messages: []openAIChatMessage{
			{Role: "user", Content: req.Prompt},
		},
		Stream:    true,
		MaxTokens: req.MaxTokens,
	})
	if err != nil {
		return result, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.bearerToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		result.DoneAt = time.Now()
		return result, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.HeadersAt = time.Now()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		payload, readErr := io.ReadAll(resp.Body)
		result.DoneAt = time.Now()
		if readErr != nil {
			return result, fmt.Errorf("read response: %w", readErr)
		}
		return result, parseAPIError(payload, resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			result.DoneAt = time.Now()
			return result, nil
		}

		now := time.Now()
		result.ChunkCount++
		if result.FirstChunkAt.IsZero() {
			result.FirstChunkAt = now
		}

		var parsed openAIStreamResponse
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			result.DoneAt = time.Now()
			return result, fmt.Errorf("decode stream chunk: %w", err)
		}
		for _, choice := range parsed.Choices {
			content := choice.Delta.Content
			if content == "" {
				content = choice.Delta.ReasoningContent
			}
			if content == "" {
				continue
			}
			if result.FirstTokenAt.IsZero() {
				result.FirstTokenAt = now
			}
			result.ContentDeltaCount++
			result.Content += content
			if onContent != nil {
				onContent(content)
			}
		}
	}
	result.DoneAt = time.Now()
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read stream: %w", err)
	}

	return result, nil
}

func parseAPIError(payload []byte, statusCode int) error {
	var apiErr apiErrorResponse
	if err := json.Unmarshal(payload, &apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
		return fmt.Errorf("server returned status %d: %s", statusCode, apiErr.Error.Message)
	}

	message := strings.TrimSpace(string(payload))
	if message == "" {
		return fmt.Errorf("server returned status %d", statusCode)
	}

	return fmt.Errorf("server returned status %d: %s", statusCode, message)
}
