package metrics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/onkar717/visual-eyes/server/models"
)

// Collector handles collecting Kubernetes metrics
type Collector struct {
	client     *KubeletClient
	k8sClient  kubernetes.Interface // may be nil if in-cluster config fails
	namespaces map[string]struct{}  // empty = all namespaces
}

// New creates a new Kubernetes metrics collector.
// allowedNamespaces restricts pod collection; pass nil or empty slice for all namespaces.
// Out-of-cluster (dev) mode: kubelet metrics are unavailable but k8s API features still work.
func New(allowedNamespaces []string) (*Collector, error) {
	if !IsInCluster() {
		// Out-of-cluster: build k8sClient from kubeconfig but skip kubelet.
		nsSet := make(map[string]struct{}, len(allowedNamespaces))
		for _, ns := range allowedNamespaces {
			if ns != "" {
				nsSet[ns] = struct{}{}
			}
		}
		var k8sClient kubernetes.Interface
		if restCfg, err := buildK8sConfig(); err == nil {
			if cs, err := kubernetes.NewForConfig(restCfg); err == nil {
				k8sClient = cs
			}
		}
		return &Collector{
			client:     nil, // kubelet not available out-of-cluster
			k8sClient:  k8sClient,
			namespaces: nsSet,
		}, nil
	}

	kubeletClient, err := NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubelet client: %w", err)
	}

	// Build a kubernetes client: prefer in-cluster config, fall back to kubeconfig.
	var k8sClient kubernetes.Interface
	if restCfg, err := buildK8sConfig(); err == nil {
		if cs, err := kubernetes.NewForConfig(restCfg); err == nil {
			k8sClient = cs
		}
	}

	nsSet := make(map[string]struct{}, len(allowedNamespaces))
	for _, ns := range allowedNamespaces {
		if ns != "" {
			nsSet[ns] = struct{}{}
		}
	}

	return &Collector{
		client:     kubeletClient,
		k8sClient:  k8sClient,
		namespaces: nsSet,
	}, nil
}

// allowedNamespace reports whether the namespace passes the configured filter.
func (c *Collector) allowedNamespace(ns string) bool {
	if len(c.namespaces) == 0 {
		return true
	}
	_, ok := c.namespaces[ns]
	return ok
}

// Client returns the kubernetes.Interface client (may be nil outside a cluster).
func (c *Collector) Client() kubernetes.Interface { return c.k8sClient }

// Collect gathers metrics from the Kubelet API.
// Returns empty slice (not error) when running out-of-cluster where kubelet is unavailable.
func (c *Collector) Collect(ctx context.Context) ([]models.Metric, error) {
	if c.client == nil {
		return nil, nil // out-of-cluster dev mode   no kubelet access
	}
	stats, err := c.client.GetSummary()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubelet stats: %w", err)
	}

	var metrics []models.Metric
	now := time.Now().UTC()

	// Node metrics
	metrics = append(metrics, []models.Metric{
		{
			Name:      "kubernetes.node.cpu.usage",
			Value:     float64(stats.Node.CPU.UsageNanoCores) / 1e9,
			Tags:      map[string]string{"node": stats.Node.NodeName},
			Timestamp: now,
			Unit:      "cores",
		},
		{
			Name:      "kubernetes.node.memory.usage",
			Value:     float64(stats.Node.Memory.UsageBytes),
			Tags:      map[string]string{"node": stats.Node.NodeName},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "kubernetes.node.memory.available",
			Value:     float64(stats.Node.Memory.AvailableBytes),
			Tags:      map[string]string{"node": stats.Node.NodeName},
			Timestamp: now,
			Unit:      "bytes",
		},
	}...)

	// Pod and container metrics
	for _, pod := range stats.Pods {
		if !c.allowedNamespace(pod.PodRef.Namespace) {
			continue
		}
		podTags := map[string]string{
			"node":      stats.Node.NodeName,
			"pod":       pod.PodRef.Name,
			"namespace": pod.PodRef.Namespace,
		}

		// Pod-level metrics
		metrics = append(metrics, []models.Metric{
			{
				Name:      "kubernetes.pod.cpu.usage",
				Value:     float64(pod.CPU.UsageNanoCores) / 1e9,
				Tags:      podTags,
				Timestamp: now,
				Unit:      "cores",
			},
			{
				Name:      "kubernetes.pod.memory.usage",
				Value:     float64(pod.Memory.UsageBytes),
				Tags:      podTags,
				Timestamp: now,
				Unit:      "bytes",
			},
			{
				Name:      "kubernetes.pod.memory.working_set",
				Value:     float64(pod.Memory.WorkingSetBytes),
				Tags:      podTags,
				Timestamp: now,
				Unit:      "bytes",
			},
		}...)
	}

	return metrics, nil
}

// buildK8sConfig returns an in-cluster config when running inside a pod,
// or loads ~/.kube/config for out-of-cluster development use.
// The KUBECONFIG env var overrides the default kubeconfig path.
func buildK8sConfig() (*rest.Config, error) {
	// In-cluster takes precedence.
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}

	// Out-of-cluster: resolve kubeconfig path.
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home dir: %w", err)
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
