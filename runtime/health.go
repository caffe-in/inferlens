package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const healthTimeout = 2 * time.Second

// CheckHealth probes the common runtime health endpoint.
func CheckHealth(ctx context.Context, endpoint string) (Health, error) {
	healthCtx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		healthCtx,
		http.MethodGet,
		strings.TrimRight(endpoint, "/")+"/health",
		nil,
	)
	if err != nil {
		return Health{}, fmt.Errorf("build health request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Health{}, fmt.Errorf("check health: %w", err)
	}
	defer func() {
		// Drain the body so the underlying connection can be reused for
		// keep-alive; otherwise every health check opens a new connection.
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	return Health{StatusCode: resp.StatusCode}, nil
}
