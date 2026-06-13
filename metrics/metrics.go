package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type Value struct {
	Value   float64
	Present bool
}

type Snapshot struct {
	Values map[string]float64
}

type CoreSnapshot struct {
	RequestSuccess     Value
	PromptTokens       Value
	GenerationTokens   Value
	RunningRequests    Value
	WaitingRequests    Value
	GPUCacheUsage      Value
	CPUCacheUsage      Value
	Preemptions        Value
	PrefixCacheHits    Value
	PrefixCacheQueries Value
}

func FetchSnapshot(ctx context.Context, endpoint string) (Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build metrics request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read metrics: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return Snapshot{}, fmt.Errorf("metrics endpoint returned status %d: %s", resp.StatusCode, message)
	}

	return ParseSnapshot(string(body)), nil
}

func ParseSnapshot(payload string) Snapshot {
	values := map[string]float64{}
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name, value, ok := parseSample(line)
		if !ok {
			continue
		}
		if strings.HasSuffix(name, "_bucket") || strings.HasSuffix(name, "_sum") || strings.HasSuffix(name, "_count") {
			continue
		}
		values[name] += value
	}

	return Snapshot{Values: values}
}

func (s Snapshot) Core() CoreSnapshot {
	return CoreSnapshot{
		RequestSuccess:     s.first("vllm:request_success_total", "vllm:request_success"),
		PromptTokens:       s.first("vllm:prompt_tokens_total", "vllm:prompt_tokens"),
		GenerationTokens:   s.first("vllm:generation_tokens_total", "vllm:generation_tokens"),
		RunningRequests:    s.first("vllm:num_requests_running"),
		WaitingRequests:    s.first("vllm:num_requests_waiting"),
		GPUCacheUsage:      s.first("vllm:gpu_cache_usage_perc", "vllm:kv_cache_usage_perc"),
		CPUCacheUsage:      s.first("vllm:cpu_cache_usage_perc"),
		Preemptions:        s.first("vllm:num_preemptions_total", "vllm:num_preemptions"),
		PrefixCacheHits:    s.first("vllm:prefix_cache_hits_total", "vllm:prefix_cache_hits"),
		PrefixCacheQueries: s.first("vllm:prefix_cache_queries_total", "vllm:prefix_cache_queries"),
	}
}

func Delta(before, after Value) Value {
	if !before.Present || !after.Present {
		return Value{}
	}
	return Value{Value: after.Value - before.Value, Present: true}
}

func parseSample(line string) (string, float64, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", 0, false
	}

	name := fields[0]
	if idx := strings.IndexByte(name, '{'); idx >= 0 {
		name = name[:idx]
	}
	value, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return "", 0, false
	}
	return name, value, true
}

func (s Snapshot) first(names ...string) Value {
	for _, name := range names {
		value, ok := s.Values[name]
		if ok {
			return Value{Value: value, Present: true}
		}
	}
	return Value{}
}
