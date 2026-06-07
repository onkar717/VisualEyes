package rca

import (
	"fmt"
	"math"
	"strings"

	"github.com/onkar717/visual-eyes/server/models"
)

const zScoreThreshold = 2.5

// AnomalyResult tags a metric sample as anomalous.
type AnomalyResult struct {
	MetricName string
	Value      float64
	ZScore     float64
	Mean       float64
	Stddev     float64
}

// DetectAnomalies runs Z-score analysis over a slice of same-name metric samples.
// Returns results where |z| >= zScoreThreshold (default 2.5σ).
// Returns nil when fewer than 3 samples (insufficient baseline).
func DetectAnomalies(samples []models.Metric) []AnomalyResult {
	if len(samples) < 3 {
		return nil
	}

	// Compute mean.
	sum := 0.0
	for _, s := range samples {
		sum += s.Value
	}
	mean := sum / float64(len(samples))

	// Compute population stddev.
	variance := 0.0
	for _, s := range samples {
		d := s.Value - mean
		variance += d * d
	}
	stddev := math.Sqrt(variance / float64(len(samples)))
	if stddev < 1e-9 {
		return nil // constant series   no anomaly possible
	}

	var out []AnomalyResult
	for _, s := range samples {
		z := math.Abs(s.Value-mean) / stddev
		if z >= zScoreThreshold {
			out = append(out, AnomalyResult{
				MetricName: s.Name,
				Value:      s.Value,
				ZScore:     z,
				Mean:       mean,
				Stddev:     stddev,
			})
		}
	}
	return out
}

// AnomalySummary formats detected anomalies as a text block for LLM context.
func AnomalySummary(anomalies []AnomalyResult) string {
	if len(anomalies) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("=== STATISTICAL ANOMALIES (Z-score ≥ 2.5σ) ===\n")
	for _, a := range anomalies {
		sb.WriteString(fmt.Sprintf(
			"  %s: value=%.4f  mean=%.4f  σ=%.4f  z=%.2f\n",
			a.MetricName, a.Value, a.Mean, a.Stddev, a.ZScore,
		))
	}
	sb.WriteString("\n")
	return sb.String()
}

// maxProblemPods caps the number of distinct problem pods fed to the LLM
// to prevent 413 / token-overflow on large clusters.
const maxProblemPods = 50

// FilterProblemPods returns only pod-level metrics where the pod shows signs of trouble:
// restart_count > 2, or cpu/memory usage is non-zero (active workload with potential issue).
// Healthy idle pods are excluded to reduce LLM token usage on large clusters.
// The result is further capped at maxProblemPods distinct pods.
func FilterProblemPods(metrics []models.Metric) []models.Metric {
	// Build per-pod restart map.
	restarts := make(map[string]float64)
	for _, m := range metrics {
		if m.Name == "kubernetes.pod.restart_count" {
			pod := m.Tags["pod"]
			if pod != "" && m.Value > restarts[pod] {
				restarts[pod] = m.Value
			}
		}
	}

	var out []models.Metric
	seen := make(map[string]bool)
	podCount := 0
	for _, m := range metrics {
		pod := m.Tags["pod"]
		if pod == "" {
			out = append(out, m) // node-level or untagged   always include
			continue
		}
		key := pod + "/" + m.Tags["namespace"]
		if seen[key] {
			out = append(out, m)
			continue
		}
		// Include pod if restarts > 2, or if metric value is non-trivially non-zero.
		if restarts[pod] > 2 || m.Value > 0.001 {
			if podCount >= maxProblemPods {
				continue // hard cap reached   skip remaining problem pods
			}
			seen[key] = true
			podCount++
			out = append(out, m)
		}
	}
	return out
}
