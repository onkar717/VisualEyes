import axios from 'axios';
import type { Alert, Metric, PodLog, RCAResult } from '../types/metrics';

// Use environment variable for API base URL, fallback to dev default
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || (window.location.hostname === 'localhost' ? 'http://localhost:8080/api' : '/api');

interface MetricsResponse {
  timestamp: string;
  metrics: {
    cpu: Record<string, MetricData>;
    memory: Record<string, MetricData>;
    disk: Record<string, MetricData>;
    network: Record<string, MetricData>;
    load: Record<string, MetricData>;
  };
}

interface KubernetesMetricsResponse {
  timestamp: string;
  metrics: {
    nodes: {
      total: number;
      ready: number;
    };
    pods: {
      total: number;
      running: number;
    };
    resources: {
      cpu: {
        usage: number;
        total: number;
      };
      memory: {
        usage: number;
        total: number;
      };
    };
    podResources: {
      cpu: {
        usage: number;
        total: number;
      };
      memory: {
        usage: number;
        total: number;
      };
    };
  };
}

interface MetricData {
  value: number;
  unit?: string;
  tags?: Record<string, string>;
  timestamp: string;
}

const getMetrics = async (): Promise<Metric[]> => {
  const response = await axios.get<MetricsResponse>(`${API_BASE_URL}/metrics/snapshot`);
  const metrics: Metric[] = [];

  Object.entries(response.data.metrics).forEach(([category, categoryMetrics]) => {
    Object.entries(categoryMetrics).forEach(([name, data]) => {
      metrics.push({
        name: `${category}.${name}`,
        value: data.value,
        timestamp: data.timestamp,
        unit: data.unit,
        tags: data.tags
      });
    });
  });

  return metrics;
};

const getKubernetesMetrics = async (): Promise<KubernetesMetricsResponse['metrics']> => {
  const response = await axios.get<KubernetesMetricsResponse>(`${API_BASE_URL}/kubernetes/metrics`);
  return response.data.metrics;
};

// ── Alerts ────────────────────────────────────────────────────────────────────

const getAlerts = async (status: 'firing' | 'all' = 'firing'): Promise<Alert[]> => {
  const response = await axios.get<{ alerts: Alert[]; count: number }>(
    `${API_BASE_URL}/alerts?status=${status}`
  );
  return response.data.alerts ?? [];
};

const getAlertById = async (id: number): Promise<Alert> => {
  const response = await axios.get<Alert>(`${API_BASE_URL}/alerts/${id}`);
  return response.data;
};

// ── RCA ───────────────────────────────────────────────────────────────────────

const getRCA = async (alertId: number): Promise<RCAResult> => {
  const response = await axios.get<RCAResult>(`${API_BASE_URL}/rca/${alertId}`);
  return response.data;
};

const executeRCACommand = async (alertId: number, commandIndex: number): Promise<{ output: string; error?: string }> => {
  const response = await axios.post<{ output: string; error?: string }>(
    `${API_BASE_URL}/rca/${alertId}/execute`,
    { commandIndex }
  );
  return response.data;
};

// ── Logs ──────────────────────────────────────────────────────────────────────

const getPodLogs = async (params?: {
  pod?: string;
  namespace?: string;
  container?: string;
  limit?: number;
}): Promise<PodLog[]> => {
  const query = new URLSearchParams();
  if (params?.pod) query.set('pod', params.pod);
  if (params?.namespace) query.set('namespace', params.namespace);
  if (params?.container) query.set('container', params.container);
  if (params?.limit) query.set('limit', String(params.limit));

  const response = await axios.get<{ logs: PodLog[]; count: number }>(
    `${API_BASE_URL}/pod-logs?${query}`
  );
  return response.data.logs ?? [];
};

// ── Kubernetes clusters & events ──────────────────────────────────────────────

export interface ClusterHealth {
  id: number;
  name: string;
  namespace: string;
  last_seen: string;
  health_score: number;
  total_nodes: number;
  ready_nodes: number;
  total_pods: number;
  running_pods: number;
  pending_pods: number;
  failed_pods: number;
  crashloop_pods: number;
  open_incidents: number;
  cpu_usage_pct: number;
  mem_usage_pct: number;
  created_at: string;
  updated_at: string;
}

export interface K8sEvent {
  namespace: string;
  kind: string;
  object: string;
  reason: string;
  message: string;
  type: string;
  count: number;
  lastSeen: string;
  sourceNode: string;
}

const getClusters = async (): Promise<ClusterHealth[]> => {
  const response = await axios.get<ClusterHealth[]>(`${API_BASE_URL}/clusters`);
  return response.data ?? [];
};

const getK8sEvents = async (): Promise<K8sEvent[]> => {
  const response = await axios.get<{ events: K8sEvent[] }>(`${API_BASE_URL}/events`);
  return response.data.events ?? [];
};

// ── Exports ───────────────────────────────────────────────────────────────────

export const api = {
  getMetricsSnapshot: getMetrics,
  getKubernetesMetrics,
  getClusters,
  getK8sEvents,
  getAlerts,
  getAlertById,
  getRCA,
  executeRCACommand,
  getPodLogs,
};