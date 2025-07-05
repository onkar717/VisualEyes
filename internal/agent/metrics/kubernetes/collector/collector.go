package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/onkar717/visual-eyes/internal/agent/metrics/kubernetes/client"
	"github.com/onkar717/visual-eyes/internal/models"
)

// Collector handles collecting Kubernetes metrics
type Collector struct {
	client *client.KubeletClient
}

// New creates a new Kubernetes metrics collector
func New() (*Collector, error) {
	// Check if we're running in a Kubernetes cluster
	if !client.IsInCluster() {
		return nil, fmt.Errorf("not running in a Kubernetes cluster")
	}

	kubeletClient, err := client.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubelet client: %w", err)
	}

	return &Collector{
		client: kubeletClient,
	}, nil
}

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

		// Container metrics
		for _, container := range pod.Containers {
			containerTags := map[string]string{
				"node":      stats.Node.NodeName,
				"pod":       pod.PodRef.Name,
				"namespace": pod.PodRef.Namespace,
				"container": container.Name,
			}

			metrics = append(metrics, []models.Metric{
				{
					Name:      "kubernetes.container.cpu.usage",
					Value:     float64(container.CPU.UsageNanoCores) / 1e9,
					Tags:      containerTags,
					Timestamp: now,
					Unit:      "cores",
				},
				{
					Name:      "kubernetes.container.memory.usage",
					Value:     float64(container.Memory.UsageBytes),
					Tags:      containerTags,
					Timestamp: now,
					Unit:      "bytes",
				},
				{
					Name:      "kubernetes.container.memory.working_set",
					Value:     float64(container.Memory.WorkingSetBytes),
					Tags:      containerTags,
					Timestamp: now,
					Unit:      "bytes",
				},
			}...)
		}
	}

	return metrics, nil
}
