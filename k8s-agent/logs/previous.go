package logs

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	prevLogLines    = 100 // max previous-container log lines per container
	minRestartCount = 1   // only fetch prev logs when restart count ≥ this
)

// PreviousCollector fetches terminated/previous-container logs from the k8s API.
// These are essential for CrashLoopBackOff diagnosis: the current container may
// be healthy but the previous crash's stderr tells us why it died.
type PreviousCollector struct {
	client     kubernetes.Interface
	namespaces []string // empty = all namespaces
	node       string
}

// NewPreviousCollector creates a collector targeting the given namespaces (empty = all).
func NewPreviousCollector(client kubernetes.Interface, nodeName string, namespaces []string) *PreviousCollector {
	return &PreviousCollector{
		client:     client,
		namespaces: namespaces,
		node:       nodeName,
	}
}

// Collect returns previous-container log lines for all restarting pods.
// Stream is always set to "prev" so the server routes them to PrevLogs.
func (c *PreviousCollector) Collect(ctx context.Context) ([]LogLine, error) {
	nsList := c.namespaces
	if len(nsList) == 0 {
		// Fetch from all namespaces.
		nsList = []string{metav1.NamespaceAll}
	}

	var result []LogLine
	for _, ns := range nsList {
		lines, err := c.collectNamespace(ctx, ns)
		if err != nil {
			slog.Warn("prev log collect error", "namespace", ns, "error", err)
			continue
		}
		result = append(result, lines...)
	}
	return result, nil
}

func (c *PreviousCollector) collectNamespace(ctx context.Context, ns string) ([]LogLine, error) {
	pods, err := c.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + c.node,
	})
	if err != nil {
		return nil, err
	}

	var lines []LogLine
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount < minRestartCount {
				continue
			}
			// Only fetch if container previously terminated (crashed).
			if cs.LastTerminationState.Terminated == nil {
				continue
			}
			podLines := c.fetchPrevLogs(ctx, pod.Namespace, pod.Name, cs.Name)
			lines = append(lines, podLines...)
		}
	}
	return lines, nil
}

func (c *PreviousCollector) fetchPrevLogs(ctx context.Context, ns, pod, container string) []LogLine {
	tailLines := int64(prevLogLines)
	req := c.client.CoreV1().Pods(ns).GetLogs(pod, &corev1.PodLogOptions{
		Container: container,
		Previous:  true,
		TailLines: &tailLines,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		// "previous terminated container not found" is expected for non-crashlooping pods.
		if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "no previous") {
			slog.Debug("prev log stream error", "pod", pod, "container", container, "error", err)
		}
		return nil
	}
	defer stream.Close()

	var lines []LogLine
	scanner := bufio.NewScanner(io.LimitReader(stream, 2*1024*1024)) // 2 MB cap
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, LogLine{
			Pod:       pod,
			Namespace: ns,
			Container: container,
			Node:      c.node,
			Stream:    "prev",
			Line:      line,
			Timestamp: time.Now(),
		})
	}
	return lines
}
