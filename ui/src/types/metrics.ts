export interface Metric {
  name: string;
  value: number;
  timestamp: string;
  tags?: Record<string, string>;
  unit?: string;
}

export interface MetricSeries {
  name: string;
  data: Array<{
    timestamp: string;
    value: number;
  }>;
}

export interface SystemMetrics {
  cpu: MetricSeries;
  memory: MetricSeries;
  disk: MetricSeries;
  network: {
    bytesReceived: MetricSeries;
    bytesSent: MetricSeries;
  };
  load: MetricSeries;
}

// ── Alerts ────────────────────────────────────────────────────────────────────
// Field names match Go JSON tags (camelCase).

export type AlertSeverity = 'critical' | 'warning' | 'info';
export type AlertStatus = 'firing' | 'resolved';
export type RCAStatus = 'pending' | 'running' | 'done' | 'failed' | '';

export interface Alert {
  id: number;
  ruleName: string;
  severity: AlertSeverity;
  status: AlertStatus;
  resourceID: string;
  namespace: string;
  value: number;
  threshold: number;
  message: string;
  firedAt: string;
  resolvedAt?: string;
  rcaStatus?: RCAStatus;
  rcaID?: number;
}

// ── RCA ───────────────────────────────────────────────────────────────────────

export type RemediationStatus = 'pending' | 'executed' | 'skipped' | 'failed';

export interface FixCommand {
  command: string;
  isAutoSafe: boolean;
  status: RemediationStatus;
  output?: string;
  error?: string;
}

export interface RCAResult {
  id: number;
  alertID: number;
  explanation: string;
  rootCause: string;
  commands: string; // JSON string of FixCommand[]   parsed on use
  status: 'pending' | 'done' | 'failed';
  model: string;
  inputTokens: number;
  createdAt: string;
  updatedAt: string;
}

// ── Logs ──────────────────────────────────────────────────────────────────────
// Field names match Go JSON tags (lowercase).

export interface PodLog {
  id: number;
  pod: string;
  namespace: string;
  container: string;
  node: string;
  stream: 'stdout' | 'stderr';
  line: string;
  timestamp: string;
}

// ── WebSocket broadcast ───────────────────────────────────────────────────────

export interface WSMetricsSnapshot {
  type: 'metrics_snapshot';
  timestamp: string;
  metrics: Record<string, Record<string, { value: number; unit?: string; tags?: Record<string, string>; timestamp: string }>>;
} 