import React, { useState } from 'react';
import {
  Box,
  Typography,
  Card,
  CardContent,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Tabs,
  Tab,
  IconButton,
  Tooltip,
  LinearProgress,
  Badge,
  Alert as MuiAlert,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
} from '@mui/material';
import {
  Refresh,
  Hub,
  CheckCircle,
  Speed,
  Memory,
  Storage,
  Schedule,
  WarningAmber,
} from '@mui/icons-material';
import { useQuery } from '@tanstack/react-query';
import { formatDistanceToNow } from 'date-fns';
import { ModernLayout } from './ModernLayout';
import { api } from '../services/api';
import { useTheme } from '../theme/ThemeContext';
import type { Alert } from '../types/metrics';
import type { ClusterHealth, K8sEvent } from '../services/api';

// ── Health Ring ────────────────────────────────────────────────────────────────

const HealthRing: React.FC<{ score: number }> = ({ score }) => {
  const r = 48;
  const circ = 2 * Math.PI * r;
  const dash = (Math.max(0, Math.min(score, 100)) / 100) * circ;
  const color = score >= 80 ? '#4caf50' : score >= 50 ? '#ff9800' : '#f44336';

  return (
    <svg viewBox="0 0 120 120" width={120} height={120}>
      <circle cx={60} cy={60} r={r} fill="none" stroke="rgba(128,128,128,0.15)" strokeWidth={10} />
      <circle
        cx={60} cy={60} r={r} fill="none" stroke={color} strokeWidth={10}
        strokeDasharray={`${dash} ${circ}`} strokeLinecap="round"
        transform="rotate(-90 60 60)"
        style={{ transition: 'stroke-dasharray 1s ease' }}
      />
      <text x={60} y={56} textAnchor="middle" fontSize={22} fontWeight={700} fill={color}>
        {Math.round(score)}
      </text>
      <text x={60} y={74} textAnchor="middle" fontSize={11} fill="rgba(150,150,150,0.9)">
        HEALTH
      </text>
    </svg>
  );
};

// ── Stat Card ──────────────────────────────────────────────────────────────────

interface StatCardProps {
  label: string;
  value: number | string;
  sub?: string;
  color: string;
  icon?: React.ReactNode;
}

const StatCard: React.FC<StatCardProps> = ({ label, value, sub, color, icon }) => {
  const { isDarkMode } = useTheme();
  return (
    <Card sx={{
      background: isDarkMode ? 'rgba(255,255,255,0.04)' : 'rgba(0,0,0,0.015)',
      border: `1px solid ${color}28`,
      borderLeft: `4px solid ${color}`,
      borderRadius: 2,
      height: '100%',
      transition: 'transform 0.15s, box-shadow 0.15s',
      '&:hover': { transform: 'translateY(-2px)', boxShadow: `0 6px 20px ${color}22` },
    }}>
      <CardContent sx={{ pb: '12px !important' }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 0.5 }}>
          <Typography variant="caption" sx={{
            color: 'text.secondary', textTransform: 'uppercase',
            letterSpacing: '0.08em', fontSize: '0.68rem', fontWeight: 600,
          }}>
            {label}
          </Typography>
          {icon && <Box sx={{ color, opacity: 0.65 }}>{icon}</Box>}
        </Box>
        <Typography variant="h4" sx={{ fontWeight: 700, color, lineHeight: 1.1, mb: 0.25 }}>
          {value}
        </Typography>
        {sub && (
          <Typography variant="caption" sx={{ color: 'text.secondary', fontSize: '0.75rem' }}>
            {sub}
          </Typography>
        )}
      </CardContent>
    </Card>
  );
};

// ── Pod Breakdown ─────────────────────────────────────────────────────────────

const PodBreakdown: React.FC<{
  running: number; pending: number; failed: number; crashloop: number;
}> = ({ running, pending, failed, crashloop }) => {
  const total = running + pending + failed + crashloop;

  const segments = [
    { label: 'Running',   count: running,   color: '#4caf50' },
    { label: 'Pending',   count: pending,   color: '#ff9800' },
    { label: 'Failed',    count: failed,    color: '#f44336' },
    { label: 'CrashLoop', count: crashloop, color: '#e91e63' },
  ];

  return (
    <Box>
      <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap', mb: 2 }}>
        {segments.map(({ label, count, color }) => (
          <Box key={label} sx={{
            display: 'flex', flexDirection: 'column', alignItems: 'center',
            px: 2.5, py: 1.25, borderRadius: 2, minWidth: 88,
            bgcolor: `${color}12`, border: `1px solid ${color}30`,
            transition: 'transform 0.15s, box-shadow 0.15s',
            '&:hover': { transform: 'translateY(-2px)', boxShadow: `0 4px 14px ${color}25` },
          }}>
            <Typography variant="h5" sx={{ fontWeight: 700, color, lineHeight: 1.1 }}>{count}</Typography>
            <Typography variant="caption" sx={{ color: 'text.secondary', fontSize: '0.68rem', mt: 0.25, letterSpacing: '0.04em' }}>
              {label}
            </Typography>
          </Box>
        ))}
      </Box>

      {total > 0 && (
        <Box sx={{ display: 'flex', height: 10, borderRadius: 5, overflow: 'hidden', gap: '2px', bgcolor: 'rgba(128,128,128,0.1)' }}>
          {segments.map(({ label, count, color }) => count > 0 && (
            <Tooltip key={label} title={`${label}: ${count}`} arrow>
              <Box sx={{
                height: '100%',
                width: `${(count / total) * 100}%`,
                bgcolor: color,
                transition: 'width 0.8s ease',
                minWidth: 3,
                cursor: 'pointer',
              }} />
            </Tooltip>
          ))}
        </Box>
      )}
    </Box>
  );
};

// ── Event reason → color ──────────────────────────────────────────────────────

const reasonColor = (reason: string): string => {
  const r = reason.toLowerCase();
  if (r.includes('oom') || r.includes('kill')) return '#f44336';
  if (r.includes('crash') || r.includes('backoff')) return '#e91e63';
  if (r.includes('fail') || r.includes('error')) return '#ff5722';
  if (r.includes('schedule') || r.includes('pull')) return '#ff9800';
  if (r.includes('evict')) return '#9c27b0';
  return '#607d8b';
};

// ── Main component ────────────────────────────────────────────────────────────

export const KubernetesDashboard: React.FC = () => {
  const { isDarkMode } = useTheme();
  const [tab, setTab] = useState(0);
  const [selectedCluster, setSelectedCluster] = useState('');

  const {
    data: clusters = [],
    isLoading: clustersLoading,
    error: clustersError,
    refetch: refetchClusters,
  } = useQuery({
    queryKey: ['clusters'],
    queryFn: api.getClusters,
    refetchInterval: 15_000,
  });

  const { data: k8sMetrics, isLoading: metricsLoading } = useQuery({
    queryKey: ['kubernetes-metrics'],
    queryFn: api.getKubernetesMetrics,
    refetchInterval: 10_000,
  });

  const { data: events = [], refetch: refetchEvents } = useQuery({
    queryKey: ['k8s-events'],
    queryFn: api.getK8sEvents,
    refetchInterval: 15_000,
  });

  const { data: alerts = [] } = useQuery({
    queryKey: ['alerts', 'firing'],
    queryFn: () => api.getAlerts('firing'),
    refetchInterval: 15_000,
  });

  const isLoading = clustersLoading && metricsLoading;
  const error = clustersError as Error | null;

  const cluster: ClusterHealth | null =
    clusters.find((c: ClusterHealth) => c.name === selectedCluster) ??
    clusters[0] ??
    null;

  const cpuPct = cluster?.cpu_usage_pct ??
    (k8sMetrics ? (k8sMetrics.resources.cpu.usage / k8sMetrics.resources.cpu.total) * 100 : 0);
  const memPct = cluster?.mem_usage_pct ??
    (k8sMetrics ? (k8sMetrics.resources.memory.usage / k8sMetrics.resources.memory.total) * 100 : 0);

  const isStale = cluster?.last_seen
    ? (Date.now() - new Date(cluster.last_seen).getTime()) > 120_000
    : false;

  const warnEvents = events.filter((e: K8sEvent) => e.type === 'Warning' || !e.type);

  const cardBg = isDarkMode
    ? 'rgba(255,255,255,0.03)'
    : 'rgba(0,0,0,0.015)';
  const cardBorder = isDarkMode
    ? '1px solid rgba(255,255,255,0.08)'
    : '1px solid rgba(0,0,0,0.08)';

  return (
    <ModernLayout title="Kubernetes" isLoading={isLoading} error={error}>

      {/* ── Header bar ─────────────────────────────────────────────────────── */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 3, flexWrap: 'wrap' }}>
        {clusters.length > 1 && (
          <FormControl size="small" sx={{ minWidth: 180 }}>
            <InputLabel>Cluster</InputLabel>
            <Select
              value={selectedCluster || cluster?.name || ''}
              label="Cluster"
              onChange={(e) => setSelectedCluster(e.target.value)}
            >
              {clusters.map((c: ClusterHealth) => (
                <MenuItem key={c.name} value={c.name}>{c.name}</MenuItem>
              ))}
            </Select>
          </FormControl>
        )}

        {cluster && (
          <>
            <Chip
              label={cluster.name}
              icon={<Hub sx={{ fontSize: 14 }} />}
              variant="outlined"
              sx={{ fontWeight: 600 }}
            />
            <Chip
              label={isStale
                ? 'Agent offline'
                : `Updated ${formatDistanceToNow(new Date(cluster.last_seen), { addSuffix: true })}`
              }
              size="small"
              color={isStale ? 'warning' : 'default'}
              icon={<Schedule sx={{ fontSize: 12 }} />}
            />
            {cluster.open_incidents > 0 && (
              <Chip
                label={`${cluster.open_incidents} open incident${cluster.open_incidents !== 1 ? 's' : ''}`}
                color="error"
                size="small"
                icon={<WarningAmber sx={{ fontSize: 14 }} />}
              />
            )}
          </>
        )}

        <Box sx={{ ml: 'auto', display: 'flex', gap: 0.5 }}>
          <Tooltip title="Refresh">
            <IconButton
              size="small"
              onClick={() => { refetchClusters(); refetchEvents(); }}
              sx={{ color: 'text.secondary' }}
            >
              <Refresh fontSize="small" />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {/* ── No agent banner ────────────────────────────────────────────────── */}
      {clusters.length === 0 && !clustersLoading && (
        <MuiAlert severity="info" sx={{ mb: 3, borderRadius: 2 }}>
          No Kubernetes cluster connected. Deploy the k8s-agent to start monitoring.
        </MuiAlert>
      )}

      {/* ── Tabs ───────────────────────────────────────────────────────────── */}
      <Tabs
        value={tab}
        onChange={(_, v) => setTab(v)}
        sx={{
          mb: 3,
          borderBottom: isDarkMode ? '1px solid rgba(255,255,255,0.1)' : '1px solid rgba(0,0,0,0.1)',
          '& .MuiTab-root': { textTransform: 'none', fontWeight: 500, fontSize: '0.88rem', minHeight: 44 },
          '& .Mui-selected': { color: '#6b4cf5 !important', fontWeight: 700 },
          '& .MuiTabs-indicator': { backgroundColor: '#6b4cf5', height: 3, borderRadius: 2 },
        }}
      >
        <Tab label="Overview" />
        <Tab label={
          <Badge badgeContent={warnEvents.length || null} color="warning" max={99}>
            <Box sx={{ pr: warnEvents.length > 0 ? 1 : 0 }}>Events</Box>
          </Badge>
        } />
        <Tab label={
          <Badge badgeContent={alerts.length || null} color="error" max={99}>
            <Box sx={{ pr: alerts.length > 0 ? 1 : 0 }}>Alerts</Box>
          </Badge>
        } />
      </Tabs>

      {/* ═══════════════════════════════════════════════════════════════════════
          Tab 0 Overview
      ════════════════════════════════════════════════════════════════════════ */}
      {tab === 0 && (
        <Box>
          {cluster ? (
            <>
              {/* Health ring + core stats ────────────────────────────────── */}
              <Box sx={{
                display: 'grid',
                gridTemplateColumns: { xs: '1fr', sm: '140px 1fr' },
                gap: 2.5,
                mb: 2.5,
              }}>
                <Card sx={{
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  p: 2, background: isDarkMode ? 'rgba(107,76,245,0.08)' : 'rgba(107,76,245,0.04)',
                  border: '1px solid rgba(107,76,245,0.22)', borderRadius: 2,
                }}>
                  <HealthRing score={cluster.health_score} />
                </Card>

                <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 2 }}>
                  <StatCard
                    label="Nodes"
                    value={`${cluster.ready_nodes}/${cluster.total_nodes}`}
                    sub={cluster.ready_nodes === cluster.total_nodes
                      ? 'All ready'
                      : `${cluster.total_nodes - cluster.ready_nodes} not ready`}
                    color={cluster.ready_nodes === cluster.total_nodes ? '#4caf50' : '#f44336'}
                    icon={<Hub sx={{ fontSize: 18 }} />}
                  />
                  <StatCard
                    label="Pods"
                    value={cluster.total_pods}
                    sub={`${cluster.running_pods} running`}
                    color="#6b4cf5"
                    icon={<Storage sx={{ fontSize: 18 }} />}
                  />
                  <StatCard
                    label="CPU Usage"
                    value={`${cpuPct.toFixed(1)}%`}
                    sub="Cluster-wide"
                    color={cpuPct > 85 ? '#f44336' : cpuPct > 70 ? '#ff9800' : '#4caf50'}
                    icon={<Speed sx={{ fontSize: 18 }} />}
                  />
                  <StatCard
                    label="Memory Usage"
                    value={`${memPct.toFixed(1)}%`}
                    sub="Cluster-wide"
                    color={memPct > 85 ? '#f44336' : memPct > 70 ? '#ff9800' : '#2196f3'}
                    icon={<Memory sx={{ fontSize: 18 }} />}
                  />
                </Box>
              </Box>

              {/* Pod breakdown ───────────────────────────────────────────── */}
              <Card sx={{ mb: 2.5, p: 2.5, background: cardBg, border: cardBorder, borderRadius: 2 }}>
                <Typography variant="caption" sx={{
                  display: 'block', mb: 2, fontWeight: 700, color: 'text.secondary',
                  textTransform: 'uppercase', letterSpacing: '0.08em', fontSize: '0.68rem',
                }}>
                  Pod Health Breakdown
                </Typography>
                <PodBreakdown
                  running={cluster.running_pods}
                  pending={cluster.pending_pods}
                  failed={cluster.failed_pods}
                  crashloop={cluster.crashloop_pods}
                />
              </Card>

              {/* Resource pressure bars ───────────────────────────────────── */}
              <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' }, gap: 2 }}>
                {[
                  { label: 'CPU Pressure', value: cpuPct, color: cpuPct > 85 ? '#f44336' : cpuPct > 70 ? '#ff9800' : '#4caf50' },
                  { label: 'Memory Pressure', value: memPct, color: memPct > 85 ? '#f44336' : memPct > 70 ? '#ff9800' : '#2196f3' },
                ].map(({ label, value, color }) => (
                  <Card key={label} sx={{ p: 2, background: cardBg, border: cardBorder, borderRadius: 2 }}>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1.25 }}>
                      <Typography variant="caption" sx={{
                        color: 'text.secondary', textTransform: 'uppercase',
                        letterSpacing: '0.06em', fontSize: '0.68rem', fontWeight: 600,
                      }}>
                        {label}
                      </Typography>
                      <Typography variant="caption" sx={{ color, fontWeight: 700, fontSize: '0.82rem' }}>
                        {value.toFixed(1)}%
                      </Typography>
                    </Box>
                    <LinearProgress
                      variant="determinate"
                      value={Math.min(value, 100)}
                      sx={{
                        height: 10, borderRadius: 5,
                        bgcolor: 'rgba(128,128,128,0.1)',
                        '& .MuiLinearProgress-bar': {
                          bgcolor: color, borderRadius: 5,
                          transition: 'transform 0.8s ease',
                        },
                      }}
                    />
                  </Card>
                ))}
              </Box>
            </>
          ) : (
            /* Fallback: k8sMetrics without a registered cluster */
            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 2 }}>
              {k8sMetrics ? (
                <>
                  <StatCard
                    label="Nodes"
                    value={k8sMetrics.nodes.ready}
                    sub={`of ${k8sMetrics.nodes.total} total`}
                    color="#6b4cf5"
                    icon={<Hub sx={{ fontSize: 18 }} />}
                  />
                  <StatCard
                    label="Pods"
                    value={k8sMetrics.pods.running}
                    sub={`of ${k8sMetrics.pods.total} total`}
                    color="#4caf50"
                    icon={<Storage sx={{ fontSize: 18 }} />}
                  />
                  <StatCard
                    label="CPU"
                    value={`${((k8sMetrics.resources.cpu.usage / k8sMetrics.resources.cpu.total) * 100).toFixed(1)}%`}
                    color="#2196f3"
                    icon={<Speed sx={{ fontSize: 18 }} />}
                  />
                  <StatCard
                    label="Memory"
                    value={`${((k8sMetrics.resources.memory.usage / k8sMetrics.resources.memory.total) * 100).toFixed(1)}%`}
                    color="#ff9800"
                    icon={<Memory sx={{ fontSize: 18 }} />}
                  />
                </>
              ) : (
                <MuiAlert severity="info" sx={{ gridColumn: '1 / -1' }}>
                  Waiting for Kubernetes metrics. Start the k8s-agent to see cluster data.
                </MuiAlert>
              )}
            </Box>
          )}
        </Box>
      )}

      {/* ═══════════════════════════════════════════════════════════════════════
          Tab 1 Warning Events
      ════════════════════════════════════════════════════════════════════════ */}
      {tab === 1 && (
        <Box>
          {warnEvents.length === 0 ? (
            <MuiAlert severity="success" icon={<CheckCircle />} sx={{ borderRadius: 2 }}>
              No warning events recorded.
            </MuiAlert>
          ) : (
            <TableContainer
              component={Paper}
              elevation={0}
              sx={{ border: cardBorder, borderRadius: 2, bgcolor: cardBg }}
            >
              <Table size="small">
                <TableHead>
                  <TableRow>
                    {['Reason', 'Object', 'Namespace', 'Message', 'Count', 'Age'].map((h) => (
                      <TableCell key={h} sx={{
                        fontWeight: 700, fontSize: '0.68rem', textTransform: 'uppercase',
                        letterSpacing: '0.08em', color: 'text.secondary',
                        borderBottom: isDarkMode ? '1px solid rgba(255,255,255,0.08)' : '1px solid rgba(0,0,0,0.08)',
                        py: 1.5,
                      }}>
                        {h}
                      </TableCell>
                    ))}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {warnEvents.map((ev: K8sEvent, i: number) => {
                    const rc = reasonColor(ev.reason);
                    return (
                      <TableRow key={i} sx={{
                        '&:hover': { bgcolor: isDarkMode ? 'rgba(255,255,255,0.03)' : 'rgba(0,0,0,0.02)' },
                        '&:last-child td': { border: 0 },
                      }}>
                        <TableCell sx={{ py: 1 }}>
                          <Chip
                            label={ev.reason}
                            size="small"
                            sx={{
                              bgcolor: `${rc}18`,
                              color: rc,
                              border: `1px solid ${rc}35`,
                              fontWeight: 700,
                              fontSize: '0.7rem',
                              height: 22,
                            }}
                          />
                        </TableCell>
                        <TableCell sx={{ fontSize: '0.8rem', fontFamily: 'monospace', py: 1 }}>
                          {ev.object || '-'}
                        </TableCell>
                        <TableCell sx={{ py: 1 }}>
                          <Chip
                            label={ev.namespace || 'default'}
                            size="small"
                            variant="outlined"
                            sx={{ fontSize: '0.68rem', height: 20 }}
                          />
                        </TableCell>
                        <TableCell sx={{ maxWidth: 320, py: 1 }}>
                          <Tooltip title={ev.message} arrow>
                            <Typography variant="caption" sx={{
                              display: 'block',
                              overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                              maxWidth: 320, color: 'text.secondary', lineHeight: 1.4,
                            }}>
                              {ev.message}
                            </Typography>
                          </Tooltip>
                        </TableCell>
                        <TableCell sx={{ py: 1 }}>
                          <Chip
                            label={ev.count}
                            size="small"
                            variant="outlined"
                            sx={{ fontSize: '0.68rem', minWidth: 36, height: 20 }}
                          />
                        </TableCell>
                        <TableCell sx={{ fontSize: '0.78rem', color: 'text.secondary', whiteSpace: 'nowrap', py: 1 }}>
                          {ev.lastSeen
                            ? formatDistanceToNow(new Date(ev.lastSeen), { addSuffix: true })
                            : '-'}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Box>
      )}

      {/* ═══════════════════════════════════════════════════════════════════════
          Tab 2 Firing Alerts
      ════════════════════════════════════════════════════════════════════════ */}
      {tab === 2 && (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          {alerts.length === 0 ? (
            <MuiAlert severity="success" icon={<CheckCircle />} sx={{ borderRadius: 2 }}>
              No firing alerts.
            </MuiAlert>
          ) : (
            alerts.map((alert: Alert) => {
              const isCrit = alert.severity === 'critical';
              const borderColor = isCrit ? '#f44336' : '#ff9800';
              return (
                <Card key={alert.id} sx={{
                  background: isDarkMode ? 'rgba(255,255,255,0.03)' : '#fff',
                  border: `1px solid ${borderColor}28`,
                  borderLeft: `4px solid ${borderColor}`,
                  borderRadius: 2,
                  transition: 'transform 0.15s, box-shadow 0.15s',
                  '&:hover': { transform: 'translateY(-1px)', boxShadow: `0 4px 16px ${borderColor}22` },
                }}>
                  <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 1 }}>
                      <Box sx={{ minWidth: 0 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 700, mb: 0.25 }}>
                          {alert.ruleName}
                        </Typography>
                        <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block' }}>
                          {alert.message}
                        </Typography>
                        <Typography variant="caption" sx={{ color: 'text.secondary', fontSize: '0.7rem' }}>
                          {alert.resourceID} · {alert.namespace}
                        </Typography>
                      </Box>
                      <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 0.5, flexShrink: 0 }}>
                        <Chip
                          label={alert.severity.toUpperCase()}
                          size="small"
                          sx={{
                            bgcolor: isCrit ? 'rgba(244,67,54,0.14)' : 'rgba(255,152,0,0.14)',
                            color: borderColor,
                            fontWeight: 700,
                            fontSize: '0.68rem',
                            height: 22,
                          }}
                        />
                        {alert.firedAt && (
                          <Typography variant="caption" sx={{ color: 'text.secondary', fontSize: '0.72rem' }}>
                            {formatDistanceToNow(new Date(alert.firedAt), { addSuffix: true })}
                          </Typography>
                        )}
                      </Box>
                    </Box>
                  </CardContent>
                </Card>
              );
            })
          )}
        </Box>
      )}
    </ModernLayout>
  );
};
