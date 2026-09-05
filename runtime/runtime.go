// Package runtime maps runtime-specific health and Prometheus data into a
// shared observation model. It does not own inference requests or output.
package runtime

import (
	"fmt"
	"strings"

	"inferlens/metrics"
)

// Runtime names accepted by ping serve.
const (
	NameVLLM     = "vllm"
	NameLlamaCPP = "llamacpp"
	NameSGLang   = "sglang"
)

// Observer maps before/after Prometheus snapshots for a specific runtime
// into a normalized Report.
type Observer interface {
	Observe(before, after metrics.Snapshot) Report
}

// Names lists the runtime names accepted by ping serve, in a stable order.
// It is the single source of truth for which runtimes are supported; both
// config validation and observer selection should derive from it.
func Names() []string {
	return []string{NameVLLM, NameLlamaCPP, NameSGLang}
}

// NewObserver returns the Observer for the given runtime name. It returns an
// error if name is not one of Names().
func NewObserver(name string) (Observer, error) {
	switch name {
	case NameVLLM:
		return VLLM{}, nil
	case NameLlamaCPP:
		return LlamaCPP{}, nil
	case NameSGLang:
		return SGLang{}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime %q; expected %s", name, strings.Join(Names(), " or "))
	}
}

// Health records the HTTP status code of a health check. A zero status code
// means no HTTP response was received.
type Health struct {
	StatusCode int
}

// Format tells output renderers how to interpret an observation's values.
type Format int

// Observation formats supported by the terminal renderer.
const (
	FormatCounterDelta Format = iota
	FormatGaugeBeforeAfter
	FormatRatioPercentage
	FormatSingleRate
)

// Observation is a normalized value backed by one or more runtime metrics.
type Observation struct {
	Key    string
	Source string
	Before metrics.Value
	After  metrics.Value
	Format Format
}

// Common contains observations shared by supported runtimes.
type Common struct {
	PromptTokens    Observation
	GeneratedTokens Observation
	RunningRequests Observation
	WaitingRequests Observation
}

// Report contains normalized common observations and runtime-native details.
type Report struct {
	Common Common
	Native []Observation
}

func observation(
	key string,
	format Format,
	before metrics.Snapshot,
	after metrics.Snapshot,
	names ...string,
) Observation {
	// Resolve both snapshots against the same metric name so a before/after
	// pair never mixes values from two differently named counters (e.g. if
	// the server was upgraded mid-measurement and renamed a metric).
	_, source := first(after, names...)
	if source == "" {
		_, source = first(before, names...)
	}
	if source == "" && len(names) > 0 {
		source = names[0]
	}

	var beforeValue, afterValue metrics.Value
	if source != "" {
		beforeValue = before.First(source)
		afterValue = after.First(source)
	}

	return Observation{
		Key:    key,
		Source: source,
		Before: beforeValue,
		After:  afterValue,
		Format: format,
	}
}

func first(snapshot metrics.Snapshot, names ...string) (metrics.Value, string) {
	for _, name := range names {
		value := snapshot.First(name)
		if value.Present {
			return value, name
		}
	}
	return metrics.Value{}, ""
}
