package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"inferlens/kserve"
)

func printKServeFixture(t *testing.T, report KServeReport) string {
	t.Helper()
	var stdout bytes.Buffer
	PrintKServeReport(&stdout, report)
	return stdout.String()
}

func crashLoopPod() kserve.Pod {
	var pod kserve.Pod
	pod.Metadata.Name = "qwen-predictor-1"
	pod.Status.Phase = "Running"
	pod.Spec.Containers = []kserve.ContainerSpec{{Name: "kserve-container", Image: "runtime:v1"}}
	pod.Status.Conditions = []kserve.PodCondition{{Type: "Ready", Status: "False"}}
	return pod
}

func TestPrintKServeReportHealthyCompact(t *testing.T) {
	service := &kserve.InferenceService{}
	service.Metadata.Name = "qwen"
	service.Metadata.Generation = 3
	service.Status.ObservedGeneration = 3
	service.Status.DeploymentMode = "Standard"
	service.Status.URL = "http://qwen.example.com"
	service.Status.ClusterServingRuntime = "runtime"
	service.Status.Conditions = []kserve.Condition{{Type: "Ready", Status: "True"}}

	pod := kserve.Pod{}
	pod.Metadata.Name = "qwen-predictor-1"
	pod.Spec.Containers = []kserve.ContainerSpec{{Name: "kserve-container", Image: "runtime:v1"}}
	pod.Status.Phase = "Running"
	pod.Status.Conditions = []kserve.PodCondition{{Type: "Ready", Status: "True"}}

	out := printKServeFixture(t, KServeReport{
		Name:      "qwen",
		Endpoint:  "http://localhost:8080",
		Model:     "qwen",
		Collected: kserve.Result{InferenceService: service, Pods: []kserve.Pod{pod}},
	})

	for _, want := range []string{
		"  ready: True\n",
		"  qwen-predictor-1: Running, ready (image runtime:v1)\n",
		"  control plane: ok\n",
		"  data plane: ok\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in report:\n%s", want, out)
		}
	}
}

func TestPrintKServeReportExpandsNonReadyPod(t *testing.T) {
	pod := crashLoopPod()
	pod.Status.ContainerStatuses = []kserve.ContainerStatus{{
		Name:         "kserve-container",
		Ready:        false,
		RestartCount: 7,
	}}
	pod.Status.ContainerStatuses[0].State.Waiting = &struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}{Reason: "CrashLoopBackOff", Message: "back-off 5m0s restarting failed container"}

	out := printKServeFixture(t, KServeReport{
		Name:      "qwen",
		Collected: kserve.Result{Pods: []kserve.Pod{pod}, PodsErr: errors.New("ignored when set")},
	})

	// PodsErr set → pods section unavailable, pod list not rendered.
	if !strings.Contains(out, "predictor pods:\n  unavailable:") {
		t.Fatalf("expected pods unavailable, got:\n%s", out)
	}
}

func TestPrintKServeReportPodEvidence(t *testing.T) {
	pod := crashLoopPod()
	status := kserve.ContainerStatus{Name: "kserve-container", Ready: false, RestartCount: 3}
	status.State.Waiting = &struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}{Reason: "CrashLoopBackOff", Message: "back-off restarting"}
	pod.Status.ContainerStatuses = []kserve.ContainerStatus{status}

	initStatus := kserve.ContainerStatus{Name: "storage-initializer", Ready: false}
	initStatus.State.Terminated = &struct {
		Reason   string `json:"reason"`
		Message  string `json:"message"`
		ExitCode int    `json:"exitCode"`
	}{Reason: "OOMKilled", ExitCode: 137, Message: "out of memory\nlong log"}
	pod.Status.InitContainerStatuses = []kserve.ContainerStatus{initStatus}

	scheduled := kserve.PodCondition{Type: "PodScheduled", Status: "False", Reason: "Unschedulable", Message: "0/2 nodes are available"}
	pod.Status.Conditions = append(pod.Status.Conditions, scheduled)

	out := printKServeFixture(t, KServeReport{
		Name:      "qwen",
		Collected: kserve.Result{Pods: []kserve.Pod{pod}},
	})

	for _, want := range []string{
		"qwen-predictor-1: Running (not ready)",
		"unschedulable: 0/2 nodes are available",
		"init storage-initializer terminated: OOMKilled (exit 137): out of memory",
		"kserve-container waiting: CrashLoopBackOff: back-off restarting",
		"kserve-container restarts: 3",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in report:\n%s", want, out)
		}
	}
}

func TestPrintKServeReportResultSection(t *testing.T) {
	out := printKServeFixture(t, KServeReport{
		Name:      "qwen",
		Collected: kserve.Result{ServiceErr: errors.New("read InferenceService \"qwen\": boom")},
		ReadyErr:  nil,
		ProbeErr:  nil,
	})
	if !strings.Contains(out, "control plane: failed: read InferenceService \"qwen\": boom") {
		t.Fatalf("expected control-plane failure line:\n%s", out)
	}
	if !strings.Contains(out, "data plane: ok") {
		t.Fatalf("expected data-plane ok line:\n%s", out)
	}
}
