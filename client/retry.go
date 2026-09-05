package client

import (
	"context"
	"fmt"
	"io"
	"time"
)

// backoffBase is a package var so tests can shorten retry delays.
var backoffBase = 500 * time.Millisecond

// StreamChatWithRetries retries a failed streaming request with capped
// exponential backoff. A request is retried only when nothing was streamed
// to the caller yet and the failure looks transient (connection error or a
// 5xx response); 4xx responses and mid-stream failures are not retried.
// log, when non-nil, receives one line per retry attempt.
func (c *Client) StreamChatWithRetries(
	ctx context.Context,
	req ChatRequest,
	onContent func(string),
	retries int,
	log io.Writer,
) (StreamResult, error) {
	result, err := c.StreamChat(ctx, req, onContent)
	for attempt := 1; err != nil && attempt <= retries && retryable(result); attempt++ {
		if log != nil {
			fmt.Fprintf(log, "retry %d/%d: %v\n", attempt, retries, err)
		}

		delay := backoffBase << (attempt - 1)
		if delay > 4*time.Second {
			delay = 4 * time.Second
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(delay):
		}

		result, err = c.StreamChat(ctx, req, onContent)
	}
	return result, err
}

func retryable(result StreamResult) bool {
	if result.ContentDeltaCount > 0 {
		return false
	}
	return result.StatusCode == 0 || result.StatusCode >= 500
}
