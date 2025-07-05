package kubernetes

import (
	"fmt"
	"time"

	"github.com/onkar717/visual-eyes/internal/agent/metrics/kubernetes/client"
	"github.com/onkar717/visual-eyes/internal/models"
)

// Collect gathers metrics from the Kubernetes node and containers
func Collect() ([]models.Metric, error) {
	// Create a new Kubelet client
	kubeletClient, err := client.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubelet client: %w", err)
	}

	// Get metrics from Kubelet Summary API
	stats, err := kubeletClient.GetSummary()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubelet stats: %w", err)
	}

	now := time.Now().UTC()
	var metrics []models.Metric

	// Node metrics
	nodeTags := map[string]string{
		"node": stats.Node.NodeName,
		"type": "node",
	}

	metrics = append(metrics, []models.Metric{
		{
			Name:      "kubernetes.node.cpu.usage",
			Value:     float64(stats.Node.CPU.UsageNanoCores) / 1e9,
			Tags:      nodeTags,
			Timestamp: now,
			Unit:      "cores",
		},
		{
			Name:      "kubernetes.node.memory.usage",
			Value:     float64(stats.Node.Memory.UsageBytes),
			Tags:      nodeTags,
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "kubernetes.node.memory.available",
			Value:     float64(stats.Node.Memory.AvailableBytes),
			Tags:      nodeTags,
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
			"type":      "pod",
		}

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
				"type":      "container",
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
