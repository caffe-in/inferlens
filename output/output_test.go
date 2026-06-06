package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"inferlens/metrics"
)

func TestPrintChatResult(t *testing.T) {
	var buf bytes.Buffer

	PrintChatResult(&buf, "hello", metrics.RequestMetrics{
		StatusCode: 200,
		Latency:    1500 * time.Millisecond,
	})

	got := buf.String()
	if !strings.Contains(got, "hello") {
		t.Fatalf("expected response text in output, got %q", got)
	}
	if !strings.Contains(got, "status: 200") {
		t.Fatalf("expected status in output, got %q", got)
	}
	if !strings.Contains(got, "latency: 1.5s") {
		t.Fatalf("expected latency in output, got %q", got)
	}
}
