package kserve

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// readyService is a trimmed copy of a real InferenceService from a kind
// cluster (Standard deployment mode).
const readyService = `{
  "metadata": {"name": "qwen-llama", "namespace": "kserve-test", "generation": 3, "creationTimestamp": "2026-08-01T09:10:41Z"},
  "spec": {"predictor": {"model": {"runtime": ""}}},
  "status": {
    "observedGeneration": 3,
    "url": "http://qwen-llama-kserve-test.example.com",
    "deploymentMode": "Standard",
    "clusterServingRuntimeName": "llama-cpp-runtime",
    "conditions": [
      {"type": "IngressReady", "status": "True", "lastTransitionTime": "2026-08-29T09:03:54Z"},
      {"type": "PredictorReady", "status": "True", "reason": "NewReplicaSetAvailable", "message": "ReplicaSet available", "lastTransitionTime": "2026-08-29T09:03:54Z"},
      {"type": "Ready", "status": "True", "lastTransitionTime": "2026-08-29T09:03:54Z"},
      {"type": "Stopped", "status": "False", "severity": "Info", "lastTransitionTime": "2026-08-01T09:10:41Z"}
    ],
    "modelStatus": {
      "copies": {"failedCopies": 0, "totalCopies": 1},
      "states": {"activeModelState": "Loaded", "targetModelState": "Loaded"},
      "transitionStatus": "UpToDate",
      "lastFailureInfo": {"reason": "ModelLoadFailed", "exitCode": 1, "message": "stale failure\nsecond line"}
    }
  }
}`

const healthyPodList = `{"items": [{
  "metadata": {"name": "qwen-llama-predictor-6df487878f-tqvnp"},
  "spec": {"containers": [{"name": "kserve-container", "image": "llama-qwen-runtime:v1"}]},
  "status": {
    "phase": "Running",
    "conditions": [{"type": "Ready", "status": "True"}, {"type": "PodScheduled", "status": "True"}],
    "containerStatuses": [{"name": "kserve-container", "ready": true, "restartCount": 0, "image": "llama-qwen-runtime:v1", "state": {}}]
  }
}]}`

func newTestRunner(servicePayload, podPayload string, serviceErr, podErr error) Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "inferenceservices.serving.kserve.io"):
			if serviceErr != nil {
				return nil, serviceErr
			}
			return []byte(servicePayload), nil
		case strings.Contains(joined, "pods"):
			if podErr != nil {
				return nil, podErr
			}
			return []byte(podPayload), nil
		default:
			return nil, errors.New("unexpected kubectl invocation: " + joined)
		}
	}
}

func TestCollectReadyServiceAndPods(t *testing.T) {
	collector, err := New(Options{
		Name:        "qwen-llama",
		Namespace:   "kserve-test",
		Kubeconfig:  "/tmp/kubeconfig",
		KubeContext: "kind-kserve-demo",
		Runner:      newTestRunner(readyService, healthyPodList, nil, nil),
	})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	result := collector.Collect(context.Background())
	if result.ServiceErr != nil || result.PodsErr != nil {
		t.Fatalf("unexpected errors: %v / %v", result.ServiceErr, result.PodsErr)
	}

	service := result.InferenceService
	if service.Metadata.Name != "qwen-llama" || service.Metadata.Generation != 3 {
		t.Fatalf("unexpected metadata: %+v", service.Metadata)
	}
	if service.Status.DeploymentMode != "Standard" || service.Status.ClusterServingRuntime != "llama-cpp-runtime" {
		t.Fatalf("unexpected status: %+v", service.Status)
	}
	if ready := service.FindCondition("Ready"); ready == nil || ready.Status != "True" {
		t.Fatalf("expected Ready=True, got %+v", ready)
	}
	if stopped := service.FindCondition("Stopped"); stopped == nil || stopped.Status != "False" {
		t.Fatalf("expected Stopped=False to decode, got %+v", stopped)
	}
	if service.Status.ModelStatus.LastFailureInfo.Reason != "ModelLoadFailed" {
		t.Fatalf("expected last failure info, got %+v", service.Status.ModelStatus.LastFailureInfo)
	}
	if len(result.Pods) != 1 || !result.Pods[0].Ready() {
		t.Fatalf("expected one ready pod, got %+v", result.Pods)
	}
}

func TestCollectBuildsKubectlArguments(t *testing.T) {
	var recorded [][]string
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		recorded = append(recorded, args)
		return []byte(readyService), nil
	}

	collector, err := New(Options{Name: "qwen", Namespace: "ns", Kubeconfig: "/kube/config", KubeContext: "ctx", Runner: runner})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}
	_ = collector.Collect(context.Background())

	wantGlobal := []string{"--kubeconfig", "/kube/config", "--context", "ctx", "--namespace", "ns"}
	wantService := append(append([]string{}, wantGlobal...), "get", "inferenceservices.serving.kserve.io", "qwen", "-o", "json")
	wantPods := append(append([]string{}, wantGlobal...), "get", "pods", "-l", "serving.kserve.io/inferenceservice=qwen,component=predictor", "-o", "json")
	if !reflect.DeepEqual(recorded[0], wantService) {
		t.Fatalf("service args:\n got %#v\nwant %#v", recorded[0], wantService)
	}
	if !reflect.DeepEqual(recorded[1], wantPods) {
		t.Fatalf("pod args:\n got %#v\nwant %#v", recorded[1], wantPods)
	}
}

func TestCollectOmitsEmptyOverrides(t *testing.T) {
	var firstArgs []string
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		if firstArgs == nil {
			firstArgs = args
		}
		return []byte(readyService), nil
	}
	collector, err := New(Options{Name: "qwen", Runner: runner})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}
	_ = collector.Collect(context.Background())
	if strings.Contains(strings.Join(firstArgs, " "), "--namespace") {
		t.Fatalf("expected no explicit overrides, got %v", firstArgs)
	}
}

func TestCollectRetainsErrorsIndependently(t *testing.T) {
	collector, err := New(Options{
		Name:   "qwen",
		Runner: newTestRunner(readyService, "", nil, errors.New("Error from server (Forbidden): pods is forbidden")),
	})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	result := collector.Collect(context.Background())
	if result.ServiceErr != nil {
		t.Fatalf("service should succeed, got %v", result.ServiceErr)
	}
	if result.PodsErr == nil || !strings.Contains(result.PodsErr.Error(), "authorization failed") {
		t.Fatalf("expected classified pods error, got %v", result.PodsErr)
	}
	if result.InferenceService == nil {
		t.Fatal("failed pod query must not erase the InferenceService")
	}
}

func TestClassifyCommonKubectlFailures(t *testing.T) {
	tests := []struct {
		name    string
		stderr  string
		wantSub string
	}{
		{"not found", `Error from server (NotFound): inferenceservices.serving.kserve.io "qwen" not found`, "resource not found"},
		{"forbidden", "Error from server (Forbidden): inferenceservices is forbidden", "authorization failed"},
		{"no config", "error: no configuration has been provided", "kubeconfig/context not configured"},
		{"api refused", "The connection to the server localhost:8080 was refused - did you specify the right host or port?", "cannot reach the Kubernetes API server"},
		{"crd missing", `error: the server doesn't have a resource type "inferenceservices"`, "CRD not installed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := classify(errors.New(tt.stderr))
			if !strings.Contains(wrapped.Error(), tt.wantSub) || !strings.Contains(wrapped.Error(), tt.stderr) {
				t.Fatalf("expected %q to contain %q and the original", wrapped.Error(), tt.wantSub)
			}
		})
	}
}

func TestCollectMalformedJSON(t *testing.T) {
	collector, err := New(Options{Name: "qwen", Runner: newTestRunner("{not json", "{not json", nil, nil)})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}
	result := collector.Collect(context.Background())
	if result.ServiceErr == nil || !strings.Contains(result.ServiceErr.Error(), "decode InferenceService") {
		t.Fatalf("expected decode error, got %v", result.ServiceErr)
	}
	if result.PodsErr == nil || !strings.Contains(result.PodsErr.Error(), "decode predictor Pods") {
		t.Fatalf("expected decode error, got %v", result.PodsErr)
	}
}

func TestNewRejectsMissingName(t *testing.T) {
	if _, err := New(Options{Runner: func(context.Context, ...string) ([]byte, error) { return nil, nil }}); err == nil {
		t.Fatal("expected error for empty name")
	}
}
