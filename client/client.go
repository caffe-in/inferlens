package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type ChatRequest struct {
	Model  string
	Prompt string
}

type ChatResponse struct {
	Content string
}

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
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

func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, int, error) {
	body, err := json.Marshal(openAIChatRequest{
		Model: req.Model,
		Messages: []openAIChatMessage{
			{Role: "user", Content: req.Prompt},
		},
		Stream: false,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, resp.StatusCode, parseAPIError(payload, resp.StatusCode)
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, resp.StatusCode, fmt.Errorf("decode response: no choices returned")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return nil, resp.StatusCode, fmt.Errorf("decode response: empty message content")
	}

	return &ChatResponse{Content: content}, resp.StatusCode, nil
}

func parseAPIError(payload []byte, statusCode int) error {
	var apiErr apiErrorResponse
	if err := json.Unmarshal(payload, &apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
		return fmt.Errorf("vllm returned status %d: %s", statusCode, apiErr.Error.Message)
	}

	message := strings.TrimSpace(string(payload))
	if message == "" {
		return fmt.Errorf("vllm returned status %d", statusCode)
	}

	return fmt.Errorf("vllm returned status %d: %s", statusCode, message)
}
