// Package kserve collects read-only KServe control-plane state by delegating
// to the local kubectl. It never mutates cluster resources.
package kserve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// Runner executes kubectl with the given arguments and returns stdout.
// It exists so tests can substitute a fake; production uses ExecKubectl.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// ExecKubectl runs kubectl via PATH, wired to ctx for cancellation.
func ExecKubectl(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return stdout.Bytes(), nil
}

// Options configures one collection run. Empty Namespace/Kubeconfig/Context
// are omitted so kubectl applies its normal precedence.
type Options struct {
	Name        string
	Namespace   string
	Kubeconfig  string
	KubeContext string
	Runner      Runner
}

type Collector struct {
	runner Runner
	args   []string // global kubectl arguments shared by every query
	name   string
}

func New(opts Options) (*Collector, error) {
	if strings.TrimSpace(opts.Name) == "" {
		return nil, errors.New("inference service name is required")
	}

	runner := opts.Runner
	if runner == nil {
		if _, err := exec.LookPath("kubectl"); err != nil {
			return nil, errors.New("kubectl not found in PATH; install kubectl to probe KServe")
		}
		runner = ExecKubectl
	}

	args := make([]string, 0, 6)
	if opts.Kubeconfig != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}
	if opts.KubeContext != "" {
		args = append(args, "--context", opts.KubeContext)
	}
	if opts.Namespace != "" {
		args = append(args, "--namespace", opts.Namespace)
	}

	return &Collector{runner: runner, args: args, name: opts.Name}, nil
}

// InferenceService decodes only the fields InferLens reports; unknown fields
// are tolerated so newer KServe versions stay compatible.
type InferenceService struct {
	Metadata struct {
		Name              string `json:"name"`
		Namespace         string `json:"namespace"`
		Generation        int64  `json:"generation"`
		CreationTimestamp string `json:"creationTimestamp"`
	} `json:"metadata"`
	Spec struct {
		Predictor struct {
			Model struct {
				Runtime string `json:"runtime"`
			} `json:"model"`
		} `json:"predictor"`
	} `json:"spec"`
	Status struct {
		ObservedGeneration    int64        `json:"observedGeneration"`
		URL                   string       `json:"url"`
		DeploymentMode        string       `json:"deploymentMode"`
		ClusterServingRuntime string       `json:"clusterServingRuntimeName"`
		Conditions            []Condition  `json:"conditions"`
		ModelStatus           *ModelStatus `json:"modelStatus"`
	} `json:"status"`
}

type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
	LastTransitionTime string `json:"lastTransitionTime"`
}

type ModelStatus struct {
	States struct {
		ActiveModelState string `json:"activeModelState"`
		TargetModelState string `json:"targetModelState"`
	} `json:"states"`
	TransitionStatus string `json:"transitionStatus"`
	Copies           *struct {
		FailedCopies int `json:"failedCopies"`
		TotalCopies  int `json:"totalCopies"`
	} `json:"copies"`
	LastFailureInfo *struct {
		Reason   string `json:"reason"`
		ExitCode int    `json:"exitCode"`
		Time     string `json:"time"`
		Message  string `json:"message"`
	} `json:"lastFailureInfo"`
}

// FindCondition returns the condition of the given type, or nil.
func (s *InferenceService) FindCondition(condType string) *Condition {
	for i := range s.Status.Conditions {
		if s.Status.Conditions[i].Type == condType {
			return &s.Status.Conditions[i]
		}
	}
	return nil
}

// Pod decodes predictor Pod evidence.
type Pod struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		InitContainers []ContainerSpec `json:"initContainers"`
		Containers     []ContainerSpec `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase                 string            `json:"phase"`
		Conditions            []PodCondition    `json:"conditions"`
		InitContainerStatuses []ContainerStatus `json:"initContainerStatuses"`
		ContainerStatuses     []ContainerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type ContainerSpec struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type PodCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type ContainerStatus struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int    `json:"restartCount"`
	Image        string `json:"image"`
	State        struct {
		Waiting *struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"waiting"`
		Terminated *struct {
			Reason   string `json:"reason"`
			Message  string `json:"message"`
			ExitCode int    `json:"exitCode"`
		} `json:"terminated"`
	} `json:"state"`
	LastTerminationState struct {
		Terminated *struct {
			Reason   string `json:"reason"`
			ExitCode int    `json:"exitCode"`
		} `json:"terminated"`
	} `json:"lastState"`
}

// FindCondition returns the Pod condition of the given type, or nil.
func (p *Pod) FindCondition(condType string) *PodCondition {
	for i := range p.Status.Conditions {
		if p.Status.Conditions[i].Type == condType {
			return &p.Status.Conditions[i]
		}
	}
	return nil
}

// Ready reports the Pod's Ready condition.
func (p *Pod) Ready() bool {
	cond := p.FindCondition("Ready")
	return cond != nil && cond.Status == "True"
}

// Result carries both queries' outcomes independently so one failure never
// erases the other's evidence.
type Result struct {
	InferenceService *InferenceService
	ServiceErr       error
	Pods             []Pod
	PodsErr          error
}

// Collect reads the InferenceService and its predictor Pods. Errors are
// retained per query, never merged.
func (c *Collector) Collect(ctx context.Context) Result {
	var result Result

	payload, err := c.runner(ctx, append(slices.Clone(c.args), "get",
		"inferenceservices.serving.kserve.io", c.name, "-o", "json")...)
	if err != nil {
		result.ServiceErr = fmt.Errorf("read InferenceService %q: %w", c.name, classify(err))
	} else {
		service := &InferenceService{}
		if err := json.Unmarshal(payload, service); err != nil {
			result.ServiceErr = fmt.Errorf("decode InferenceService %q: %w", c.name, err)
		} else {
			result.InferenceService = service
		}
	}

	payload, err = c.runner(ctx, append(slices.Clone(c.args), "get", "pods", "-l",
		"serving.kserve.io/inferenceservice="+c.name+",component=predictor", "-o", "json")...)
	if err != nil {
		result.PodsErr = fmt.Errorf("list predictor Pods for %q: %w", c.name, classify(err))
	} else {
		var podList struct {
			Items []Pod `json:"items"`
		}
		if err := json.Unmarshal(payload, &podList); err != nil {
			result.PodsErr = fmt.Errorf("decode predictor Pods for %q: %w", c.name, err)
		} else {
			result.Pods = podList.Items
		}
	}

	return result
}

// classify wraps a kubectl failure with the common cause when kubectl's
// stderr gives a clear signal, always preserving the original message.
func classify(err error) error {
	message := err.Error()
	for _, pattern := range []struct {
		contains string
		cause    string
	}{
		{"no configuration has been provided", "kubeconfig/context not configured (run kubectl config use-context)"},
		{"current-context is not set", "no current context (run kubectl config use-context)"},
		{"was refused", "cannot reach the Kubernetes API server (wrong context or cluster unreachable)"},
		{"i/o timeout", "cannot reach the Kubernetes API server (wrong context or cluster unreachable)"},
		{"the server has asked for the client to provide credentials", "authentication failed (kubeconfig credentials rejected)"},
		{"Unauthorized", "authentication failed (kubeconfig credentials rejected)"},
		{"Forbidden", "authorization failed (kubectl needs get/list on the resource)"},
		{"doesn't have a resource type", "InferenceService CRD not installed (is KServe on this cluster?)"},
		{"unable to recognize", "InferenceService CRD not installed (is KServe on this cluster?)"},
		{"not found", "resource not found (wrong name, namespace, or context?)"},
	} {
		if strings.Contains(message, pattern.contains) {
			return fmt.Errorf("%s: %s", pattern.cause, message)
		}
	}
	return err
}
