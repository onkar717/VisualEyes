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

export type AlertSeverity = 'critical' | 'warning' | 'info';
export type AlertStatus = 'firing' | 'resolved';
export type RCAStatus = 'pending' | 'running' | 'done' | 'failed' | '';

export interface Alert {
  ID: number;
  RuleName: string;
  Severity: AlertSeverity;
  Status: AlertStatus;
  ResourceID: string;
  Namespace: string;
  Value: number;
  Threshold: number;
  Message: string;
  FiredAt: string;
  ResolvedAt?: string;
  RCAStatus?: RCAStatus;
  RCAID?: number;
  CreatedAt: string;
  UpdatedAt: string;
}

// ── RCA ───────────────────────────────────────────────────────────────────────

export type RemediationStatus = 'pending' | 'executed' | 'skipped' | 'failed';

export interface FixCommand {
  command: string;
  is_auto_safe: boolean;
  status: RemediationStatus;
  output?: string;
  exec_error?: string;
}

export interface RCAResult {
  ID: number;
  AlertID: number;
  Explanation: string;
  RootCause: string;
  Commands: FixCommand[];
  Status: 'pending' | 'done' | 'failed';
  Model: string;
  InputTokens: number;
  CreatedAt: string;
  UpdatedAt: string;
}

// ── Logs ──────────────────────────────────────────────────────────────────────

export interface PodLog {
  ID: number;
  Pod: string;
  Namespace: string;
  Container: string;
  Node: string;
  Stream: 'stdout' | 'stderr';
  Line: string;
  Timestamp: string;
}

// ── WebSocket broadcast ───────────────────────────────────────────────────────

export interface WSMetricsSnapshot {
  type: 'metrics_snapshot';
  timestamp: string;
  metrics: Record<string, Record<string, { value: number; unit?: string; tags?: Record<string, string>; timestamp: string }>>;
} 