package metrics

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/onkar717/visual-eyes/backend/models"
)

// Collector handles collecting Kubernetes metrics
type Collector struct {
	client    *KubeletClient
	k8sClient kubernetes.Interface // may be nil if in-cluster config fails
}

// New creates a new Kubernetes metrics collector
func New() (*Collector, error) {
	if !IsInCluster() {
		return nil, fmt.Errorf("not running in a Kubernetes cluster")
	}

	kubeletClient, err := NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubelet client: %w", err)
	}

	// Build a standard in-cluster kubernetes client for Events collection.
	var k8sClient kubernetes.Interface
	if restCfg, err := rest.InClusterConfig(); err == nil {
		if cs, err := kubernetes.NewForConfig(restCfg); err == nil {
			k8sClient = cs
		}
	}

	return &Collector{
		client:    kubeletClient,
		k8sClient: k8sClient,
	}, nil
}

// Client returns the kubernetes.Interface client (may be nil outside a cluster).
func (c *Collector) Client() kubernetes.Interface { return c.k8sClient }

// Collect gathers metrics from the Kubelet API
func (c *Collector) Collect(ctx context.Context) ([]models.Metric, error) {
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
