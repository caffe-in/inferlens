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

// First returns the first present metric from names.
func (s Snapshot) First(names ...string) Value {
	for _, name := range names {
		value, ok := s.Values[name]
		if ok {
			return Value{Value: value, Present: true}
		}
	}
	return Value{}
}
