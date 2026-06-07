package rca

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
	"github.com/onkar717/visual-eyes/server/storage"
)

// OOMKillInfo holds info about an OOM-killed container detected via Prometheus.
type OOMKillInfo struct{ Pod, Namespace, Container string }

// DeploymentIssue holds a deployment where ready replicas < desired.
type DeploymentIssue struct {
	Namespace string
	Name      string
	Desired   float64
	Ready     float64
}

// HPAAtMaxInfo holds an HPA that is at its maximum replica count.
type HPAAtMaxInfo struct {
	Namespace string
	Name      string
	Current   float64
	Max       float64
}

// UnboundPVCInfo holds a PVC that is not in Bound phase.
type UnboundPVCInfo struct{ Namespace, Name, Phase string }

// NodePressureInfo holds per-node CPU and memory usage percentages.
type NodePressureInfo struct {
	Node   string
	CPUPct float64
	MemPct float64
}

// AlertContext bundles everything the LLM pipeline needs for high-quality RCA.
type AlertContext struct {
	Alert          models.Alert
	RecentMetrics  []models.Metric // last N samples of the triggering metric
	RelatedMetrics []models.Metric // samples of related metrics on same resource (problem pods only)
	PodLogs        []models.PodLog // current container log lines
	PrevLogs       []models.PodLog // previous container logs (pre-crash evidence for crashloop)
	SiblingAlerts  []models.Alert  // other firing alerts on the same resource
	// K8s Warning events for the alert's namespace (recent, capped at 20).
	K8sEvents []storage.K8sEvent
	// Per-namespace pod-phase summary derived from related metrics.
	NamespaceSummary map[string]namespaceStat
	// Pre-classified log patterns   populated by ClassifyLogs before LLM stages.
	LogClassification ClassifiedLogs
	// Detected metric anomalies (Z-score ≥ 2.5σ over recent samples).
	Anomalies []AnomalyResult
	// OOM-killed containers detected via Prometheus.
	OOMKills []OOMKillInfo
	// Deployments with ready replicas < desired (replica mismatches).
	DeploymentIssues []DeploymentIssue
	// HPAs currently at max replica count (can't scale out).
	HPAAtMax []HPAAtMaxInfo
	// PVCs not in Bound phase (may block pod scheduling).
	UnboundPVCs []UnboundPVCInfo
	// Per-node CPU/memory pressure.
	NodePressures []NodePressureInfo
	// NamespaceLogSummary is per-pod error category counts from parallel log scan.
	NamespaceLogSummary map[string]map[string]int
	// Observability stack availability   hints for the LLM agents.
	PrometheusURL     string
	PrometheusEnabled bool
	LokiURL           string
	LokiEnabled       bool
}

// namespaceStat holds a lightweight pod count breakdown for one namespace.
type namespaceStat struct {
	ActivePods    int
	CrashloopPods int // pods with restart_count > 5
}

// ContextBuilder assembles AlertContext from multiple stores.
type ContextBuilder struct {
	metricStore    storage.QueryableStore
	logStore       storage.LogStore // may be nil
	alertStore     storage.AlertStore
	eventBuffer    *storage.EventBuffer // may be nil
	logLines       int
	metricSamples  int
	prometheusURL     string
	prometheusEnabled bool
	prometheusClient  *PrometheusClient // non-nil when prometheusEnabled and URL set
	lokiURL        string
	lokiEnabled    bool
	lokiClient     *LokiClient // non-nil when lokiEnabled and lokiURL set
}

// NewContextBuilder creates a ContextBuilder with the given stores and limits.
func NewContextBuilder(
	ms storage.QueryableStore,
	ls storage.LogStore,
	as storage.AlertStore,
	logLines, metricSamples int,
) *ContextBuilder {
	return &ContextBuilder{
		metricStore:   ms,
		logStore:      ls,
		alertStore:    as,
		logLines:      logLines,
		metricSamples: metricSamples,
	}
}

// SetPrometheus injects Prometheus connection info into context.
// When enabled and url is non-empty, a PrometheusClient is created for live metric queries.
func (b *ContextBuilder) SetPrometheus(url string, enabled bool) {
	b.prometheusURL = url
	b.prometheusEnabled = enabled
	if enabled && url != "" {
		b.prometheusClient = NewPrometheusClient(url)
	}
}

// SetEventBuffer injects the K8s event ring buffer so recent Warning events
// can be included in RCA context.
func (b *ContextBuilder) SetEventBuffer(eb *storage.EventBuffer) {
	b.eventBuffer = eb
}

// SetLoki injects Loki connection info into context.
// When enabled and url is non-empty, a LokiClient is created for live log queries.
func (b *ContextBuilder) SetLoki(url string, enabled bool) {
	b.lokiURL = url
	b.lokiEnabled = enabled
	if enabled && url != "" {
		b.lokiClient = NewLokiClient(url)
	}
}

// Build assembles a complete AlertContext for the given alert.
func (b *ContextBuilder) Build(alert models.Alert) AlertContext {
	since := time.Now().Add(-30 * time.Minute)
	ctx := AlertContext{
		Alert:             alert,
		PrometheusURL:     b.prometheusURL,
		PrometheusEnabled: b.prometheusEnabled,
		LokiURL:           b.lokiURL,
		LokiEnabled:       b.lokiEnabled,
	}

	// Primary metric samples   use MetricName (e.g. "cpu.usage"), not RuleName.
	metricName := alert.MetricName
	if metricName == "" {
		metricName = alert.RuleName // fallback for alerts stored before MetricName was added
	}
	if samples, err := b.metricStore.QueryByName(metricName, since, b.metricSamples); err == nil {
		ctx.RecentMetrics = samples
		// Run Z-score anomaly detection on the primary metric time series.
		ctx.Anomalies = DetectAnomalies(samples)
	}

	// Related system metrics for same resource   filter to problem pods only.
	relatedNames := relatedMetrics(alert.RuleName)
	for _, name := range relatedNames {
		if samples, err := b.metricStore.QueryByName(name, since, 10); err == nil {
			ctx.RelatedMetrics = append(ctx.RelatedMetrics, samples...)
		}
	}
	// Remove healthy idle pods from related metrics to reduce LLM token usage.
	ctx.RelatedMetrics = FilterProblemPods(ctx.RelatedMetrics)

	// Supplement with Prometheus PromQL metrics when available.
	// For pod-related alerts, fetch CPU and memory via standard cAdvisor queries.
	if b.prometheusClient != nil && looksLikePod(alert.ResourceID) {
		if promSamples, err := b.prometheusClient.QueryRange(
			coreCPUQuery(alert.ResourceID, alert.Namespace),
			30*time.Minute, 30*time.Second,
		); err == nil {
			ctx.RelatedMetrics = append(ctx.RelatedMetrics, promSamples...)
		}
		if promSamples, err := b.prometheusClient.QueryRange(
			coreMemQuery(alert.ResourceID, alert.Namespace),
			30*time.Minute, 30*time.Second,
		); err == nil {
			ctx.RelatedMetrics = append(ctx.RelatedMetrics, promSamples...)
		}
	}

	// Prometheus instant queries   OOM kills, deployment mismatches, HPA, PVC, node pressure.
	if b.prometheusClient != nil {
		// OOM kills
		if oomRes, err := b.prometheusClient.QueryInstant(oomKillQuery()); err == nil {
			for _, r := range oomRes {
				if ctx.Alert.Namespace == "" || r.Labels["namespace"] == ctx.Alert.Namespace {
					ctx.OOMKills = append(ctx.OOMKills, OOMKillInfo{
						Pod:       r.Labels["pod"],
						Namespace: r.Labels["namespace"],
						Container: r.Labels["container"],
					})
				}
			}
		}

		// Deployment replica mismatches
		desiredMap := make(map[string]float64)
		if desRes, err := b.prometheusClient.QueryInstant(deploymentSpecReplicasQuery(ctx.Alert.Namespace)); err == nil {
			for _, r := range desRes {
				key := r.Labels["namespace"] + "/" + r.Labels["deployment"]
				desiredMap[key] = r.Value
			}
		}
		if rdyRes, err := b.prometheusClient.QueryInstant(deploymentReadyReplicasQuery(ctx.Alert.Namespace)); err == nil {
			for _, r := range rdyRes {
				key := r.Labels["namespace"] + "/" + r.Labels["deployment"]
				desired := desiredMap[key]
				if desired > 0 && r.Value < desired {
					ctx.DeploymentIssues = append(ctx.DeploymentIssues, DeploymentIssue{
						Namespace: r.Labels["namespace"],
						Name:      r.Labels["deployment"],
						Desired:   desired,
						Ready:     r.Value,
					})
				}
			}
		}

		// HPA at max
		hpaCurrentMap := make(map[string]float64)
		hpaNameMap := make(map[string]string)
		if hpaRes, err := b.prometheusClient.QueryInstant(hpaCurrentReplicasQuery(ctx.Alert.Namespace)); err == nil {
			for _, r := range hpaRes {
				key := r.Labels["namespace"] + "/" + r.Labels["horizontalpodautoscaler"]
				hpaCurrentMap[key] = r.Value
				hpaNameMap[key] = r.Labels["horizontalpodautoscaler"]
			}
		}
		if maxRes, err := b.prometheusClient.QueryInstant(hpaMaxReplicasQuery(ctx.Alert.Namespace)); err == nil {
			for _, r := range maxRes {
				key := r.Labels["namespace"] + "/" + r.Labels["horizontalpodautoscaler"]
				current := hpaCurrentMap[key]
				if current > 0 && current >= r.Value {
					ctx.HPAAtMax = append(ctx.HPAAtMax, HPAAtMaxInfo{
						Namespace: r.Labels["namespace"],
						Name:      r.Labels["horizontalpodautoscaler"],
						Current:   current,
						Max:       r.Value,
					})
				}
			}
		}

		// Unbound PVCs
		if pvcRes, err := b.prometheusClient.QueryInstant(pvcUnboundQuery(ctx.Alert.Namespace)); err == nil {
			for _, r := range pvcRes {
				ctx.UnboundPVCs = append(ctx.UnboundPVCs, UnboundPVCInfo{
					Namespace: r.Labels["namespace"],
					Name:      r.Labels["persistentvolumeclaim"],
					Phase:     r.Labels["phase"],
				})
			}
		}

		// Per-node resource pressure
		nodeMap := make(map[string]*NodePressureInfo)
		if cpuRes, err := b.prometheusClient.QueryInstant(nodeCPUPressureQuery()); err == nil {
			for _, r := range cpuRes {
				node := r.Labels["instance"]
				if node == "" {
					node = r.Labels["node"]
				}
				if node != "" {
					if _, ok := nodeMap[node]; !ok {
						nodeMap[node] = &NodePressureInfo{Node: node}
					}
					nodeMap[node].CPUPct = math.Round(r.Value*10) / 10
				}
			}
		}
		if memRes, err := b.prometheusClient.QueryInstant(nodeMemPressureQuery()); err == nil {
			for _, r := range memRes {
				node := r.Labels["instance"]
				if node == "" {
					node = r.Labels["node"]
				}
				if node != "" {
					if _, ok := nodeMap[node]; !ok {
						nodeMap[node] = &NodePressureInfo{Node: node}
					}
					nodeMap[node].MemPct = math.Round(r.Value*10) / 10
				}
			}
		}
		for _, np := range nodeMap {
			ctx.NodePressures = append(ctx.NodePressures, *np)
		}
	}

	// Parallel namespace log analysis   fetch top pods concurrently (G11).
	if b.logStore != nil && ctx.Alert.Namespace != "" {
		pods := uniquePodsInNamespace(ctx.RelatedMetrics, ctx.Alert.Namespace, 5)
		if len(pods) > 0 {
			type podLogResult struct {
				pod  string
				cats map[string]int
			}
			ch := make(chan podLogResult, len(pods))
			var wg sync.WaitGroup
			for _, pod := range pods {
				wg.Add(1)
				go func(p string) {
					defer wg.Done()
					logs, err := b.logStore.GetLogs(p, ctx.Alert.Namespace, 30)
					if err != nil || len(logs) == 0 {
						ch <- podLogResult{p, nil}
						return
					}
					cls := ClassifyLogs(logs, nil)
					ch <- podLogResult{p, cls.CategoryCounts}
				}(pod)
			}
			go func() { wg.Wait(); close(ch) }()
			summary := make(map[string]map[string]int)
			for r := range ch {
				if r.cats != nil && len(r.cats) > 0 {
					summary[r.pod] = r.cats
				}
			}
			if len(summary) > 0 {
				ctx.NamespaceLogSummary = summary
			}
		}
	}

	// Pod logs   prefer Loki when enabled; fall back to stored push logs.
	if looksLikePod(alert.ResourceID) {
		if b.lokiClient != nil {
			// Query Loki for live logs (last 30 min, up to logLines).
			if lokiLines, err := b.lokiClient.QueryLogs(alert.ResourceID, alert.Namespace,
				30*time.Minute, b.logLines); err == nil && len(lokiLines) > 0 {
				ctx.PodLogs = lokiLines
			}
		}
		// Always supplement with stored logs (includes prev-stream from k8s-agent).
		if b.logStore != nil {
			if logLines, err := b.logStore.GetLogs(alert.ResourceID, alert.Namespace, b.logLines); err == nil {
				for _, l := range logLines {
					if l.Stream == "prev" || l.Stream == "previous" {
						ctx.PrevLogs = append(ctx.PrevLogs, l)
					} else if b.lokiClient == nil {
						// Only use stored stdout/stderr if Loki is not providing them.
						ctx.PodLogs = append(ctx.PodLogs, l)
					}
				}
			}
		}
	}

	// Pre-classify logs deterministically before handing to LLM.
	ctx.LogClassification = ClassifyLogs(ctx.PodLogs, ctx.PrevLogs)

	// Sibling alerts on the same resource (cap at 10 to bound prompt size).
	if all, err := b.alertStore.GetActiveAlerts(); err == nil {
		for _, a := range all {
			if a.ID != alert.ID && a.ResourceID == alert.ResourceID {
				ctx.SiblingAlerts = append(ctx.SiblingAlerts, a)
				if len(ctx.SiblingAlerts) >= 10 {
					break
				}
			}
		}
	}

	// K8s Warning events   pull last 20 for the alert's namespace from event buffer.
	if b.eventBuffer != nil {
		ctx.K8sEvents = b.eventBuffer.GetRecent(alert.Namespace, 20)
	}

	// Namespace summary   count active and crashlooping pods per namespace from metrics.
	ctx.NamespaceSummary = buildNamespaceSummary(ctx.RelatedMetrics)

	// Cap related metrics to prevent oversized prompts on high-cardinality clusters.
	const maxRelatedMetrics = 50
	if len(ctx.RelatedMetrics) > maxRelatedMetrics {
		ctx.RelatedMetrics = ctx.RelatedMetrics[:maxRelatedMetrics]
	}

	return ctx
}

// buildNamespaceSummary groups related pod metrics by namespace and counts
// active pods and those with high restart counts (crashloop indicators).
func buildNamespaceSummary(metrics []models.Metric) map[string]namespaceStat {
	seen := make(map[string]bool)     // "namespace/pod" dedup
	restarts := make(map[string]float64) // pod → max restart_count
	for _, m := range metrics {
		if m.Name == "kubernetes.pod.restart_count" {
			pod := m.Tags["pod"]
			if pod != "" && m.Value > restarts[pod] {
				restarts[pod] = m.Value
			}
		}
	}
	stats := make(map[string]namespaceStat)
	for _, m := range metrics {
		ns := m.Tags["namespace"]
		pod := m.Tags["pod"]
		if ns == "" || pod == "" {
			continue
		}
		key := ns + "/" + pod
		if seen[key] {
			continue
		}
		seen[key] = true
		st := stats[ns]
		st.ActivePods++
		if restarts[pod] > 5 {
			st.CrashloopPods++
		}
		stats[ns] = st
	}
	return stats
}

// Format serialises the context into a structured prompt section for Claude.
func (c AlertContext) Format() string {
	var b strings.Builder

	// Observability stack availability   let agents know what they can reference.
	b.WriteString("=== OBSERVABILITY STACK ===\n")
	if c.PrometheusEnabled && c.PrometheusURL != "" {
		b.WriteString(fmt.Sprintf("Prometheus: AVAILABLE at %s (use PromQL for CPU/memory/error-rate queries)\n", c.PrometheusURL))
	} else {
		b.WriteString("Prometheus: NOT configured   use kubelet/agent metrics only\n")
	}
	if c.LokiEnabled && c.LokiURL != "" {
		b.WriteString(fmt.Sprintf("Loki: AVAILABLE at %s (use LogQL for log queries)\n", c.LokiURL))
	} else {
		b.WriteString("Loki: NOT configured   logs provided inline below\n")
	}
	b.WriteString("\n")

	// Statistical anomalies detected on primary metric.
	if anomSummary := AnomalySummary(c.Anomalies); anomSummary != "" {
		b.WriteString(anomSummary)
	}

	// K8s Warning events   high-value diagnostic signal from the API server.
	if len(c.K8sEvents) > 0 {
		b.WriteString("=== K8S WARNING EVENTS (recent) ===\n")
		for _, ev := range c.K8sEvents {
			b.WriteString(fmt.Sprintf("  [%s] %s/%s  reason=%s  count=%d  msg=%s\n",
				ev.LastSeen.Format("15:04:05"), ev.Kind, ev.Object,
				ev.Reason, ev.Count, ev.Message))
		}
		b.WriteString("\n")
	}

	// Namespace summary   per-namespace pod health at a glance.
	if len(c.NamespaceSummary) > 0 {
		b.WriteString("=== NAMESPACE SUMMARY ===\n")
		for ns, st := range c.NamespaceSummary {
			b.WriteString(fmt.Sprintf("  %s: active_pods=%d  crashloop_pods=%d\n",
				ns, st.ActivePods, st.CrashloopPods))
		}
		b.WriteString("\n")
	}

	// Init container failures   filter K8s events with Init: reasons (G16).
	var initEvents []storage.K8sEvent
	for _, ev := range c.K8sEvents {
		if strings.HasPrefix(ev.Reason, "Init:") || strings.Contains(ev.Message, "init container") {
			initEvents = append(initEvents, ev)
		}
	}
	if len(initEvents) > 0 {
		b.WriteString("=== INIT CONTAINER ISSUES ===\n")
		for _, ev := range initEvents {
			b.WriteString(fmt.Sprintf("  [%s] %s/%s  reason=%s  msg=%s\n",
				ev.LastSeen.Format("15:04:05"), ev.Kind, ev.Object, ev.Reason, ev.Message))
		}
		b.WriteString("\n")
	}

	// OOM kills detected via Prometheus.
	if len(c.OOMKills) > 0 {
		b.WriteString("=== OOM KILLED CONTAINERS ===\n")
		for _, o := range c.OOMKills {
			b.WriteString(fmt.Sprintf("  %s/%s  container=%s\n", o.Namespace, o.Pod, o.Container))
		}
		b.WriteString("\n")
	}

	// Deployment replica mismatches.
	if len(c.DeploymentIssues) > 0 {
		b.WriteString("=== DEPLOYMENT REPLICA MISMATCHES ===\n")
		for _, d := range c.DeploymentIssues {
			b.WriteString(fmt.Sprintf("  %s/%s  desired=%.0f  ready=%.0f\n",
				d.Namespace, d.Name, d.Desired, d.Ready))
		}
		b.WriteString("\n")
	}

	// HPAs at maximum replicas (cannot scale out).
	if len(c.HPAAtMax) > 0 {
		b.WriteString("=== HPA AT MAX REPLICAS ===\n")
		for _, h := range c.HPAAtMax {
			b.WriteString(fmt.Sprintf("  %s/%s  current=%.0f  max=%.0f  (CANNOT SCALE OUT)\n",
				h.Namespace, h.Name, h.Current, h.Max))
		}
		b.WriteString("\n")
	}

	// Unbound PVCs (may block pod scheduling).
	if len(c.UnboundPVCs) > 0 {
		b.WriteString("=== UNBOUND PVCs ===\n")
		for _, pvc := range c.UnboundPVCs {
			b.WriteString(fmt.Sprintf("  %s/%s  phase=%s\n", pvc.Namespace, pvc.Name, pvc.Phase))
		}
		b.WriteString("\n")
	}

	// Per-node resource pressure.
	if len(c.NodePressures) > 0 {
		b.WriteString("=== NODE RESOURCE PRESSURE ===\n")
		for _, np := range c.NodePressures {
			cpuFlag := ""
			if np.CPUPct > 85 {
				cpuFlag = " [CRITICAL]"
			} else if np.CPUPct > 70 {
				cpuFlag = " [HIGH]"
			}
			memFlag := ""
			if np.MemPct > 90 {
				memFlag = " [CRITICAL]"
			} else if np.MemPct > 80 {
				memFlag = " [HIGH]"
			}
			b.WriteString(fmt.Sprintf("  %s  cpu=%.1f%%%s  mem=%.1f%%%s\n",
				np.Node, np.CPUPct, cpuFlag, np.MemPct, memFlag))
		}
		b.WriteString("\n")
	}

	// Namespace-wide log error summary (parallel scan).
	if len(c.NamespaceLogSummary) > 0 {
		b.WriteString("=== NAMESPACE LOG ERROR SUMMARY ===\n")
		for pod, cats := range c.NamespaceLogSummary {
			parts := make([]string, 0, len(cats))
			for cat, cnt := range cats {
				parts = append(parts, fmt.Sprintf("%s:%d", cat, cnt))
			}
			b.WriteString(fmt.Sprintf("  %s  errors=[%s]\n", pod, strings.Join(parts, " ")))
		}
		b.WriteString("\n")
	}

	b.WriteString("=== ALERT ===\n")
	b.WriteString(fmt.Sprintf("Rule: %s | Severity: %s | Status: %s\n", c.Alert.RuleName, c.Alert.Severity, c.Alert.Status))
	b.WriteString(fmt.Sprintf("Resource: %s (namespace: %s)\n", c.Alert.ResourceID, c.Alert.Namespace))
	b.WriteString(fmt.Sprintf("Value: %.4f | Threshold: %.4f\n", c.Alert.Value, c.Alert.Threshold))
	b.WriteString(fmt.Sprintf("Fired at: %s\n", c.Alert.FiredAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Message: %s\n\n", c.Alert.Message))

	if len(c.RecentMetrics) > 0 {
		b.WriteString("=== RECENT METRIC SAMPLES (last 30m) ===\n")
		for _, m := range c.RecentMetrics {
			b.WriteString(fmt.Sprintf("  %s  %s=%.4f %s\n",
				m.Timestamp.Format("15:04:05"), m.Name, m.Value, m.Unit))
		}
		b.WriteString("\n")
	}

	if len(c.RelatedMetrics) > 0 {
		b.WriteString("=== RELATED METRICS ===\n")
		for _, m := range c.RelatedMetrics {
			b.WriteString(fmt.Sprintf("  %s  %s=%.4f %s\n",
				m.Timestamp.Format("15:04:05"), m.Name, m.Value, m.Unit))
		}
		b.WriteString("\n")
	}

	if len(c.SiblingAlerts) > 0 {
		b.WriteString("=== OTHER ACTIVE ALERTS ON SAME RESOURCE ===\n")
		for _, a := range c.SiblingAlerts {
			b.WriteString(fmt.Sprintf("  %s (%s) value=%.4f\n", a.RuleName, a.Severity, a.Value))
		}
		b.WriteString("\n")
	}

	if len(c.PodLogs) > 0 {
		b.WriteString(fmt.Sprintf("=== POD LOGS (last %d lines) ===\n", len(c.PodLogs)))
		for _, l := range c.PodLogs {
			b.WriteString(fmt.Sprintf("  [%s] %s\n", l.Timestamp.Format("15:04:05"), l.Line))
		}
		b.WriteString("\n")
	}

	if len(c.PrevLogs) > 0 {
		b.WriteString(fmt.Sprintf("=== PREVIOUS CONTAINER LOGS (pre-crash   %d lines) ===\n", len(c.PrevLogs)))
		for _, l := range c.PrevLogs {
			b.WriteString(fmt.Sprintf("  [PREV][%s] %s\n", l.Timestamp.Format("15:04:05"), l.Line))
		}
		b.WriteString("\n")
	}

	// Pre-classified log patterns   deterministic signal, placed after raw logs.
	if c.LogClassification.Summary != "" {
		b.WriteString(c.LogClassification.Summary)
	}

	return b.String()
}

// relatedMetrics returns metric names that are useful context for a given rule.
func relatedMetrics(ruleName string) []string {
	mapping := map[string][]string{
		"cpu_spike_critical":  {"memory.usage_percent", "load.load1", "load.load5"},
		"cpu_spike_warning":   {"memory.usage_percent", "load.load1"},
		"memory_spike_critical": {"cpu.usage", "disk.usage_percent"},
		"memory_spike_warning":  {"cpu.usage"},
		"disk_full_critical":    {"memory.usage_percent"},
		"k8s_pod_crash_loop":    {"kubernetes.pod.cpu.usage", "kubernetes.pod.memory.usage"},
		"k8s_node_cpu_high":     {"kubernetes.node.memory.usage"},
	}
	if related, ok := mapping[ruleName]; ok {
		return related
	}
	return nil
}

func looksLikePod(resourceID string) bool {
	// Pod names typically contain at least two "-" segments (deployment-replicaset-pod).
	return strings.Count(resourceID, "-") >= 2
}

// uniquePodsInNamespace returns up to max unique pod names in namespace from metrics.
func uniquePodsInNamespace(metrics []models.Metric, namespace string, max int) []string {
	seen := make(map[string]bool)
	var out []string
	for _, m := range metrics {
		if m.Tags["namespace"] != namespace {
			continue
		}
		pod := m.Tags["pod"]
		if pod == "" || seen[pod] {
			continue
		}
		seen[pod] = true
		out = append(out, pod)
		if len(out) >= max {
			break
		}
	}
	return out
}
