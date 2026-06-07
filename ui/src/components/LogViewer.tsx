import React, { useEffect, useRef, useState } from 'react';
import {
  Box,
  Chip,
  TextField,
  Typography,
  Paper,
  IconButton,
  Tooltip,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  CircularProgress,
  Stack,
} from '@mui/material';
import { Refresh, FilterAlt, Clear } from '@mui/icons-material';
import { useQuery } from '@tanstack/react-query';
import { format } from 'date-fns';
import { ModernLayout } from './ModernLayout';
import { api } from '../services/api';
import { useTheme } from '../theme/ThemeContext';
import type { PodLog } from '../types/metrics';

function LogLine({ log, isDarkMode }: { log: PodLog; isDarkMode: boolean }) {
  const isStderr = log.stream === 'stderr';
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: 1,
        py: 0.4,
        px: 1,
        borderBottom: `1px solid ${isDarkMode ? 'rgba(255,255,255,0.03)' : 'rgba(0,0,0,0.03)'}`,
        '&:hover': { background: isDarkMode ? 'rgba(255,255,255,0.03)' : 'rgba(0,0,0,0.02)' },
      }}
    >
      <Typography
        variant="caption"
        fontFamily="monospace"
        color="text.disabled"
        sx={{ whiteSpace: 'nowrap', pt: 0.1, minWidth: 130 }}
      >
        {format(new Date(log.timestamp), 'HH:mm:ss.SSS')}
      </Typography>
      <Chip
        label={log.stream}
        size="small"
        color={isStderr ? 'error' : 'default'}
        variant="outlined"
        sx={{ height: 16, fontSize: '0.6rem', minWidth: 46, flexShrink: 0 }}
      />
      <Typography
        variant="body2"
        fontFamily="monospace"
        fontSize="0.75rem"
        sx={{
          wordBreak: 'break-all',
          color: isStderr
            ? isDarkMode ? '#ef9a9a' : '#c62828'
            : isDarkMode ? '#e0e0e0' : '#222',
        }}
      >
        {log.line}
      </Typography>
    </Box>
  );
}

function deriveOptions(logs: PodLog[]) {
  const pods = [...new Set(logs.map((l) => l.pod))].sort();
  const namespaces = [...new Set(logs.map((l) => l.namespace))].sort();
  const containers = [...new Set(logs.map((l) => l.container))].sort();
  return { pods, namespaces, containers };
}

export const LogViewer: React.FC = () => {
  const { isDarkMode } = useTheme();
  const logEndRef = useRef<HTMLDivElement>(null);

  const [filters, setFilters] = useState({ pod: '', namespace: '', container: '' });
  const [activeFilters, setActiveFilters] = useState({ pod: '', namespace: '', container: '' });
  const [autoScroll, setAutoScroll] = useState(true);

  const { data: logs = [], isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ['pod-logs', activeFilters],
    queryFn: () =>
      api.getPodLogs({
        pod: activeFilters.pod || undefined,
        namespace: activeFilters.namespace || undefined,
        container: activeFilters.container || undefined,
        limit: 500,
      }),
    refetchInterval: 10_000,
  });

  const { pods, namespaces, containers } = deriveOptions(logs);

  useEffect(() => {
    if (autoScroll && logEndRef.current) {
      logEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs, autoScroll]);

  const applyFilters = () => setActiveFilters({ ...filters });
  const clearFilters = () => {
    setFilters({ pod: '', namespace: '', container: '' });
    setActiveFilters({ pod: '', namespace: '', container: '' });
  };

  const hasActiveFilter = Object.values(activeFilters).some(Boolean);

  return (
    <ModernLayout title="Pod Logs" isLoading={isLoading} error={error as Error}>
      {/* Filter bar */}
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} mb={2} alignItems="flex-end">
        <FormControl size="small" sx={{ minWidth: 160 }}>
          <InputLabel>Namespace</InputLabel>
          <Select
            value={filters.namespace}
            label="Namespace"
            onChange={(e) => setFilters((f) => ({ ...f, namespace: e.target.value }))}
          >
            <MenuItem value=""><em>All</em></MenuItem>
            {namespaces.map((ns) => <MenuItem key={ns} value={ns}>{ns}</MenuItem>)}
          </Select>
        </FormControl>

        <FormControl size="small" sx={{ minWidth: 200 }}>
          <InputLabel>Pod</InputLabel>
          <Select
            value={filters.pod}
            label="Pod"
            onChange={(e) => setFilters((f) => ({ ...f, pod: e.target.value }))}
          >
            <MenuItem value=""><em>All</em></MenuItem>
            {pods.map((p) => <MenuItem key={p} value={p}>{p}</MenuItem>)}
          </Select>
        </FormControl>

        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel>Container</InputLabel>
          <Select
            value={filters.container}
            label="Container"
            onChange={(e) => setFilters((f) => ({ ...f, container: e.target.value }))}
          >
            <MenuItem value=""><em>All</em></MenuItem>
            {containers.map((c) => <MenuItem key={c} value={c}>{c}</MenuItem>)}
          </Select>
        </FormControl>

        <TextField
          size="small"
          placeholder="Pod name (free text)"
          value={filters.pod}
          onChange={(e) => setFilters((f) => ({ ...f, pod: e.target.value }))}
          sx={{ minWidth: 180 }}
        />

        <Box sx={{ display: 'flex', gap: 0.5 }}>
          <Tooltip title="Apply filters">
            <IconButton size="small" onClick={applyFilters} color="primary">
              <FilterAlt fontSize="small" />
            </IconButton>
          </Tooltip>
          {hasActiveFilter && (
            <Tooltip title="Clear filters">
              <IconButton size="small" onClick={clearFilters}>
                <Clear fontSize="small" />
              </IconButton>
            </Tooltip>
          )}
          <Tooltip title="Refresh now">
            <span>
              <IconButton size="small" onClick={() => refetch()} disabled={isFetching}>
                <Refresh
                  fontSize="small"
                  sx={{
                    animation: isFetching ? 'spin 1s linear infinite' : 'none',
                    '@keyframes spin': { from: { transform: 'rotate(0deg)' }, to: { transform: 'rotate(360deg)' } },
                  }}
                />
              </IconButton>
            </span>
          </Tooltip>
        </Box>

        <Typography variant="caption" color="text.secondary" sx={{ ml: 'auto' }}>
          {logs.length} lines
          {isFetching && <CircularProgress size={10} sx={{ ml: 1 }} />}
        </Typography>
      </Stack>

      {/* Log pane */}
      <Paper
        elevation={0}
        sx={{
          borderRadius: 2,
          border: `1px solid ${isDarkMode ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)'}`,
          background: isDarkMode ? '#0d0d1a' : '#fafafa',
          height: 'calc(100vh - 280px)',
          minHeight: 300,
          overflowY: 'auto',
          position: 'relative',
        }}
        onScroll={(e) => {
          const el = e.currentTarget;
          const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
          setAutoScroll(atBottom);
        }}
      >
        {logs.length === 0 && !isLoading ? (
          <Box sx={{ textAlign: 'center', py: 8, color: 'text.secondary' }}>
            <Typography variant="h6">No logs found</Typography>
            <Typography variant="body2">
              {hasActiveFilter ? 'Try adjusting your filters.' : 'Waiting for the K8s log agent to ship logs.'}
            </Typography>
          </Box>
        ) : (
          logs.map((log) => <LogLine key={log.id} log={log} isDarkMode={isDarkMode} />)
        )}
        <div ref={logEndRef} />
      </Paper>

      <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
        Auto-refreshes every 10 s. {autoScroll ? 'Auto-scroll on.' : 'Scrolled up   auto-scroll paused.'}
      </Typography>
    </ModernLayout>
  );
};
