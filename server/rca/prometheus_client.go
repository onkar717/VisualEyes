package rca

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
)

// PrometheusClient queries a Prometheus instance via the HTTP API.
// Used by ContextBuilder to enrich AlertContext with PromQL-sourced metrics.
type PrometheusClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPrometheusClient creates a client for the given Prometheus base URL.
func NewPrometheusClient(baseURL string) *PrometheusClient {
	return &PrometheusClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// promRangeResponse is the shape of /api/v1/query_range.
type promRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"` // [[unix-ts, "value"], ...]
		} `json:"result"`
	} `json:"data"`
}

// QueryRange executes a PromQL range query and returns the samples as Metric slices.
// lookback is the window to query (e.g. 30*time.Minute). step controls resolution.
func (c *PrometheusClient) QueryRange(query string, lookback, step time.Duration) ([]models.Metric, error) {
	if step <= 0 {
		step = 30 * time.Second
	}
	now := time.Now()
	start := now.Add(-lookback)

	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(start.Unix(), 10))
	params.Set("end", strconv.FormatInt(now.Unix(), 10))
	params.Set("step", fmt.Sprintf("%.0fs", step.Seconds()))

	reqURL := c.baseURL + "/api/v1/query_range?" + params.Encode()
	resp, err := c.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("prometheus query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("prometheus returned %d: %s", resp.StatusCode, string(body))
	}

	var pr promRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("prometheus decode: %w", err)
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus status: %s", pr.Status)
	}

	var metrics []models.Metric
	for _, result := range pr.Data.Result {
		name := result.Metric["__name__"]
		if name == "" {
			name = query // fallback to query string as metric name
		}
		tags := make(map[string]string, len(result.Metric))
		for k, v := range result.Metric {
			if k != "__name__" {
				tags[k] = v
			}
		}
		for _, val := range result.Values {
			if len(val) < 2 {
				continue
			}
			var ts time.Time
			if tsFloat, ok := val[0].(float64); ok {
				ts = time.Unix(int64(tsFloat), 0)
			}
			var value float64
			if strVal, ok := val[1].(string); ok {
				value, _ = strconv.ParseFloat(strVal, 64)
			}
			metrics = append(metrics, models.Metric{
				Name:      name,
				Value:     value,
				Tags:      tags,
				Timestamp: ts,
			})
		}
	}
	return metrics, nil
}

// promInstantResponse is the shape of /api/v1/query.
type promInstantResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"` // [unix-ts, "value"]
		} `json:"result"`
	} `json:"data"`
}

// PrometheusInstantResult holds a label set + scalar from an instant query.
type PrometheusInstantResult struct {
	Labels map[string]string
	Value  float64
}

// QueryInstant executes a PromQL instant query against /api/v1/query.
func (c *PrometheusClient) QueryInstant(query string) ([]PrometheusInstantResult, error) {
	params := url.Values{}
	params.Set("query", query)

	reqURL := c.baseURL + "/api/v1/query?" + params.Encode()
	resp, err := c.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("prometheus instant query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("prometheus returned %d: %s", resp.StatusCode, string(body))
	}

	var pr promInstantResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("prometheus decode: %w", err)
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus status: %s", pr.Status)
	}

	out := make([]PrometheusInstantResult, 0, len(pr.Data.Result))
	for _, result := range pr.Data.Result {
		var value float64
		if len(result.Value) >= 2 {
			if strVal, ok := result.Value[1].(string); ok {
				value, _ = strconv.ParseFloat(strVal, 64)
			}
		}
		out = append(out, PrometheusInstantResult{Labels: result.Metric, Value: value})
	}
	return out, nil
}

// coreCPUQuery returns the PromQL for CPU usage rate for a given pod.
func coreCPUQuery(pod, namespace string) string {
	return fmt.Sprintf(
		`sum(rate(container_cpu_usage_seconds_total{pod="%s",namespace="%s",container!=""}[5m]))`,
		pod, namespace,
	)
}

// coreMemQuery returns the PromQL for working-set memory for a given pod.
func coreMemQuery(pod, namespace string) string {
	return fmt.Sprintf(
		`sum(container_memory_working_set_bytes{pod="%s",namespace="%s",container!=""})`,
		pod, namespace,
	)
}

// errRateQuery returns PromQL for HTTP 5xx error rate percentage for a service.
// Returns a value in [0,100]. Zero when no requests are observed.
func errRateQuery(service, namespace string) string {
	return fmt.Sprintf(
		`100 * sum(rate(http_requests_total{service="%s",namespace="%s",status=~"5.."}[5m])) / `+
			`sum(rate(http_requests_total{service="%s",namespace="%s"}[5m]))`,
		service, namespace, service, namespace,
	)
}

// p99LatencyQuery returns PromQL for the P99 request latency in milliseconds for a service.
func p99LatencyQuery(service, namespace string) string {
	return fmt.Sprintf(
		`1000 * histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket{service="%s",namespace="%s"}[5m])) by (le))`,
		service, namespace,
	)
}

// oomKillQuery returns PromQL for pods terminated due to OOMKilled.
func oomKillQuery() string {
	return `kube_pod_container_status_last_terminated_reason{reason="OOMKilled"} == 1`
}

// deploymentSpecReplicasQuery returns desired replica counts per deployment.
func deploymentSpecReplicasQuery(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(`kube_deployment_spec_replicas{namespace="%s"}`, namespace)
	}
	return `kube_deployment_spec_replicas`
}

// deploymentReadyReplicasQuery returns ready replica counts per deployment.
func deploymentReadyReplicasQuery(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(`kube_deployment_status_replicas_ready{namespace="%s"}`, namespace)
	}
	return `kube_deployment_status_replicas_ready`
}

// hpaCurrentReplicasQuery returns current replica counts per HPA.
func hpaCurrentReplicasQuery(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(`kube_horizontalpodautoscaler_status_current_replicas{namespace="%s"}`, namespace)
	}
	return `kube_horizontalpodautoscaler_status_current_replicas`
}

// hpaMaxReplicasQuery returns max replica limits per HPA.
func hpaMaxReplicasQuery(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(`kube_horizontalpodautoscaler_spec_max_replicas{namespace="%s"}`, namespace)
	}
	return `kube_horizontalpodautoscaler_spec_max_replicas`
}

// pvcUnboundQuery returns PVCs not in Bound phase.
func pvcUnboundQuery(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(`kube_persistentvolumeclaim_status_phase{phase!="Bound",namespace="%s"}`, namespace)
	}
	return `kube_persistentvolumeclaim_status_phase{phase!="Bound"}`
}

// nodeCPUPressureQuery returns per-node CPU usage percentage.
func nodeCPUPressureQuery() string {
	return `100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`
}

// nodeMemPressureQuery returns per-node memory usage percentage.
func nodeMemPressureQuery() string {
	return `100 * (1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes))`
}

// resourceQuotaUsageQuery returns quota usage metrics for a namespace.
func resourceQuotaUsageQuery(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(`kube_resourcequota{type="used",namespace="%s"}`, namespace)
	}
	return `kube_resourcequota{type="used"}`
}

// podRestartRateQuery returns 30m restart increase per pod in a namespace.
func podRestartRateQuery(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf(
			`sort_desc(increase(kube_pod_container_status_restarts_total{namespace="%s"}[30m]))`,
			namespace,
		)
	}
	return `sort_desc(increase(kube_pod_container_status_restarts_total[30m]))`
}
