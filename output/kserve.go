package output

import (
	"fmt"
	"io"
	"strings"

	"inferlens/client"
	"inferlens/kserve"
)

// KServeReport carries the inputs of the dedicated kserve report. Control
// plane and data plane stay separate; neither collapses into the other.
type KServeReport struct {
	Name        string
	Namespace   string
	KubeContext string
	Endpoint    string
	Model       string
	Collected   kserve.Result
	ReadyErr    error
	Probe       client.StreamResult
	ProbeErr    error
}

func PrintKServeReport(w io.Writer, report KServeReport) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "--- inferlens ping kserve ---")
	fmt.Fprintf(w, "endpoint: %s\n", report.Endpoint)
	fmt.Fprintf(w, "model: %s\n", report.Model)
	fmt.Fprintf(w, "inference service: %s\n", report.Name)
	if report.Namespace != "" {
		fmt.Fprintf(w, "namespace: %s\n", report.Namespace)
	}
	if report.KubeContext != "" {
		fmt.Fprintf(w, "context: %s\n", report.KubeContext)
	}

	printKServeControlPlane(w, report)
	printPredictorPods(w, report)
	printProbe(w, report)
	printKServeResult(w, report)
}

func printKServeControlPlane(w io.Writer, report KServeReport) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "kserve control plane:")
	if report.Collected.ServiceErr != nil {
		fmt.Fprintf(w, "  unavailable: %v\n", report.Collected.ServiceErr)
		return
	}
	service := report.Collected.InferenceService
	if service == nil {
		fmt.Fprintln(w, "  unavailable: no InferenceService collected")
		return
	}
	ready := service.FindCondition("Ready")
	switch {
	case ready == nil:
		fmt.Fprintln(w, "  ready: condition missing")
	case ready.Status == "True":
		fmt.Fprintln(w, "  ready: True")
	default:
		fmt.Fprintf(w, "  ready: %s (reason %q: %s)\n", ready.Status, ready.Reason, ready.Message)
	}

	generation := service.Metadata.Generation
	observed := service.Status.ObservedGeneration
	// ponytail: observed can exceed generation on real clusters; only a
	// strict shortfall is reported as lagging
	if observed != 0 && observed < generation {
		fmt.Fprintf(w, "  generation: %d (observed %d, lagging)\n", generation, observed)
	} else {
		fmt.Fprintf(w, "  generation: %d (observed %d)\n", generation, observed)
	}
	fmt.Fprintf(w, "  deployment mode: %s\n", valueOr(service.Status.DeploymentMode, "unknown"))
	fmt.Fprintf(w, "  url: %s\n", valueOr(service.Status.URL, "unknown"))
	fmt.Fprintf(w, "  serving runtime: %s\n", servingRuntime(service))

	model := service.Status.ModelStatus
	if model != nil {
		line := fmt.Sprintf("  model status: %s -> %s",
			valueOr(model.States.ActiveModelState, "unknown"),
			valueOr(model.States.TargetModelState, "unknown"),
		)
		if model.TransitionStatus != "" {
			line += ", " + model.TransitionStatus
		}
		if model.Copies != nil {
			line += fmt.Sprintf(", copies %d total / %d failed", model.Copies.TotalCopies, model.Copies.FailedCopies)
		}
		fmt.Fprintln(w, line)
		if failure := model.LastFailureInfo; failure != nil && failure.Reason != "" {
			fmt.Fprintf(w, "  last failure: %s (exit %d): %s\n",
				failure.Reason, failure.ExitCode, truncateLine(firstLine(failure.Message), 80))
		}
	}
}

func servingRuntime(service *kserve.InferenceService) string {
	switch {
	case service.Spec.Predictor.Model.Runtime != "" && service.Status.ClusterServingRuntime != "":
		return service.Spec.Predictor.Model.Runtime + " (cluster: " + service.Status.ClusterServingRuntime + ")"
	case service.Spec.Predictor.Model.Runtime != "":
		return service.Spec.Predictor.Model.Runtime
	default:
		return valueOr(service.Status.ClusterServingRuntime, "unknown")
	}
}

func printPredictorPods(w io.Writer, report KServeReport) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "predictor pods:")
	if report.Collected.PodsErr != nil {
		fmt.Fprintf(w, "  unavailable: %v\n", report.Collected.PodsErr)
		return
	}
	if len(report.Collected.Pods) == 0 {
		fmt.Fprintln(w, "  0 pods matched (serverless may scale to zero)")
		return
	}
	for i := range report.Collected.Pods {
		printPod(w, &report.Collected.Pods[i])
	}
}

func printPod(w io.Writer, pod *kserve.Pod) {
	if pod.Status.Phase == "Running" && pod.Ready() {
		images := make([]string, 0, len(pod.Spec.Containers))
		for _, container := range pod.Spec.Containers {
			images = append(images, container.Image)
		}
		fmt.Fprintf(w, "  %s: Running, ready (image %s)\n", pod.Metadata.Name, strings.Join(images, ", "))
		return
	}

	fmt.Fprintf(w, "  %s: %s (not ready)\n", pod.Metadata.Name, valueOr(pod.Status.Phase, "unknown"))
	if scheduled := pod.FindCondition("PodScheduled"); scheduled != nil && scheduled.Status != "True" {
		fmt.Fprintf(w, "    unschedulable: %s\n", valueOr(scheduled.Message, scheduled.Reason))
	}
	for _, status := range pod.Status.InitContainerStatuses {
		printContainerState(w, "init "+status.Name, &status)
	}
	for _, status := range pod.Status.ContainerStatuses {
		printContainerState(w, status.Name, &status)
	}
}

func printContainerState(w io.Writer, name string, status *kserve.ContainerStatus) {
	if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
		fmt.Fprintf(w, "    %s waiting: %s: %s\n", name, status.State.Waiting.Reason, status.State.Waiting.Message)
	}
	if status.State.Terminated != nil {
		fmt.Fprintf(w, "    %s terminated: %s (exit %d): %s\n",
			name, status.State.Terminated.Reason, status.State.Terminated.ExitCode,
			firstLine(status.State.Terminated.Message))
	}
	if last := status.LastTerminationState.Terminated; last != nil {
		fmt.Fprintf(w, "    %s last termination: %s (exit %d)\n", name, last.Reason, last.ExitCode)
	}
	if status.RestartCount > 0 {
		fmt.Fprintf(w, "    %s restarts: %d\n", name, status.RestartCount)
	}
}

func printProbe(w io.Writer, report KServeReport) {
	if report.Probe.StatusCode != 0 {
		fmt.Fprintf(w, "status: %d\n", report.Probe.StatusCode)
	}
	printTimeline(w, report.Probe)
}

func printKServeResult(w io.Writer, report KServeReport) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "result:")
	fmt.Fprintf(w, "  control plane: %s\n", planeOutcome(report.Collected.ServiceErr, report.ReadyErr))
	fmt.Fprintf(w, "  data plane: %s\n", planeOutcome(report.ProbeErr))
}

func planeOutcome(errs ...error) string {
	for _, err := range errs {
		if err != nil {
			return "failed: " + firstLine(err.Error())
		}
	}
	return "ok"
}

func firstLine(text string) string {
	if line, _, ok := strings.Cut(text, "\n"); ok {
		return line
	}
	return text
}

func truncateLine(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}
