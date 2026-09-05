package mode

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sync"

	"inferlens/client"
	"inferlens/config"
	"inferlens/kserve"
	"inferlens/output"
)

type KServe struct {
	baseOptions
	name        string
	namespace   string
	kubeconfig  string
	kubeContext string

	// runner is nil in production (real kubectl); tests inject a fake.
	runner kserve.Runner
}

func (KServe) Name() string { return KServeName }

func (KServe) UsageNote() string {
	return "kserve mode reads cluster state with your local kubectl and probes a user-provided endpoint; start any port-forward first."
}

func (k *KServe) RegisterFlags(fs *flag.FlagSet, defaults config.ModeDefaults) {
	k.baseOptions.register(fs, defaults)
	fs.StringVar(&k.name, "name", "", "InferenceService name (required)")
	fs.StringVar(&k.namespace, "namespace", "", "Kubernetes namespace (default: kubectl's context namespace)")
	fs.StringVar(&k.kubeconfig, "kubeconfig", "", "Path to kubeconfig (default: kubectl's default)")
	fs.StringVar(&k.kubeContext, "context", "", "Kube context (default: kubectl's current context)")
}

func (k *KServe) Config() (config.Config, error) {
	cfg, err := config.NewKServe(k.name, k.endpoint, k.model, k.prompt, k.maxTokens, k.timeout)
	if err != nil {
		return config.Config{}, err
	}
	cfg.Namespace = k.namespace
	cfg.Kubeconfig = k.kubeconfig
	cfg.KubeContext = k.kubeContext
	return cfg, nil
}

func (k *KServe) Ping(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	collector, err := kserve.New(kserve.Options{
		Name:        cfg.Name,
		Namespace:   cfg.Namespace,
		Kubeconfig:  cfg.Kubeconfig,
		KubeContext: cfg.KubeContext,
		Runner:      k.runner,
	})
	if err != nil {
		return err
	}

	// Control-plane collection and the streaming probe run independently so a
	// slow or failed side never prevents the other from being attempted.
	var (
		wg       sync.WaitGroup
		collect  kserve.Result
		probe    client.StreamResult
		probeErr error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		collect = collector.Collect(ctx)
	}()
	go func() {
		defer wg.Done()
		probe, probeErr = streamChat(ctx, client.New(cfg.Endpoint), cfg, stdout)
	}()
	wg.Wait()

	readyErr := readyError(cfg.Name, collect)

	output.PrintKServeReport(stdout, output.KServeReport{
		Name:        cfg.Name,
		Namespace:   cfg.Namespace,
		KubeContext: cfg.KubeContext,
		Endpoint:    cfg.Endpoint,
		Model:       cfg.Model,
		Collected:   collect,
		ReadyErr:    readyErr,
		Probe:       probe,
		ProbeErr:    probeErr,
	})

	return errors.Join(collect.ServiceErr, readyErr, collect.PodsErr, probeErr)
}

// readyError applies the authoritative KServe verdict: only the Ready
// condition. Other False conditions (e.g. Stopped=False) are normal.
func readyError(name string, result kserve.Result) error {
	if result.ServiceErr != nil || result.InferenceService == nil {
		return nil
	}
	service := result.InferenceService
	if service.Metadata.Name != "" {
		name = service.Metadata.Name
	}

	ready := service.FindCondition("Ready")
	if ready == nil {
		return fmt.Errorf("kserve Ready condition missing on %q", name)
	}
	if ready.Status != "True" {
		return fmt.Errorf("kserve not ready: Ready=%s (reason %q: %s)", ready.Status, ready.Reason, ready.Message)
	}
	return nil
}
