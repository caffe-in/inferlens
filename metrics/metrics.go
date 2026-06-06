package metrics

import (
	"context"
	"time"

	"inferlens/client"
)

type RequestMetrics struct {
	StatusCode int
	Latency    time.Duration
}

type Result struct {
	Response *client.ChatResponse
	Metrics  RequestMetrics
}

func Measure(ctx context.Context, fn func(context.Context) (*client.ChatResponse, int, error)) (Result, error) {
	start := time.Now()
	response, statusCode, err := fn(ctx)
	metric := RequestMetrics{
		StatusCode: statusCode,
		Latency:    time.Since(start),
	}
	if err != nil {
		return Result{Metrics: metric}, err
	}

	return Result{
		Response: response,
		Metrics:  metric,
	}, nil
}
