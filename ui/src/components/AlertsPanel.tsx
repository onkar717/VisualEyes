import React, { useState } from 'react';
import {
  Box,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Typography,
  ToggleButtonGroup,
  ToggleButton,
  CircularProgress,
  Tooltip,
  IconButton,
} from '@mui/material';
import { Refresh, SmartToy } from '@mui/icons-material';
import { useQuery } from '@tanstack/react-query';
import { formatDistanceToNow } from 'date-fns';
import { ModernLayout } from './ModernLayout';
import { RCADrawer } from './RCADrawer';
import { api } from '../services/api';
import { useTheme } from '../theme/ThemeContext';
import type { Alert, AlertSeverity, RCAStatus } from '../types/metrics';

const SEVERITY_COLOR: Record<AlertSeverity, 'error' | 'warning' | 'info'> = {
  critical: 'error',
  warning: 'warning',
  info: 'info',
};

const RCA_STATUS_LABEL: Record<string, { label: string; color: 'default' | 'info' | 'success' | 'error' | 'warning' }> = {
  '':        { label: 'No RCA', color: 'default' },
  pending:   { label: 'Pending',  color: 'info' },
  running:   { label: 'Running',  color: 'warning' },
  done:      { label: 'Done',     color: 'success' },
  failed:    { label: 'Failed',   color: 'error' },
};

function rcaBadge(status: RCAStatus | string | undefined) {
  const s = status ?? '';
  const meta = RCA_STATUS_LABEL[s] ?? RCA_STATUS_LABEL[''];
  return (
    <Chip
      label={meta.label}
      color={meta.color}
      size="small"
      icon={s === 'running' ? <CircularProgress size={12} color="inherit" /> : undefined}
      sx={{ minWidth: 72 }}
    />
  );
}

export const AlertsPanel: React.FC = () => {
  const { isDarkMode } = useTheme();
  const [filter, setFilter] = useState<'firing' | 'all'>('firing');
  const [selectedAlert, setSelectedAlert] = useState<Alert | null>(null);

  const { data: alerts = [], isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ['alerts', filter],
    queryFn: () => api.getAlerts(filter),
    refetchInterval: 15_000,
  });

  const tableHeaderSx = {
    fontWeight: 700,
    fontSize: '0.75rem',
    textTransform: 'uppercase' as const,
    letterSpacing: '0.08em',
    color: isDarkMode ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.45)',
  };

  const rowSx = {
    cursor: 'pointer',
    '&:hover': { backgroundColor: isDarkMode ? 'rgba(255,255,255,0.04)' : 'rgba(0,0,0,0.02)' },
  };

  return (
    <ModernLayout title="Alerts" isLoading={isLoading} error={error as Error}>
      {/* Toolbar */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 3 }}>
        <ToggleButtonGroup
          value={filter}
          exclusive
          size="small"
          onChange={(_, v) => v && setFilter(v)}
        >
          <ToggleButton value="firing">Firing</ToggleButton>
          <ToggleButton value="all">All</ToggleButton>
        </ToggleButtonGroup>

        <Typography variant="body2" color="text.secondary" sx={{ flexGrow: 1 }}>
          {alerts.length} alert{alerts.length !== 1 ? 's' : ''}
        </Typography>

        <Tooltip title="Refresh">
          <span>
            <IconButton size="small" onClick={() => refetch()} disabled={isFetching}>
              <Refresh fontSize="small" sx={{ animation: isFetching ? 'spin 1s linear infinite' : 'none', '@keyframes spin': { from: { transform: 'rotate(0deg)' }, to: { transform: 'rotate(360deg)' } } }} />
            </IconButton>
          </span>
        </Tooltip>
      </Box>

      {alerts.length === 0 && !isLoading ? (
        <Box sx={{ textAlign: 'center', py: 8, color: 'text.secondary' }}>
          <Typography variant="h6">No alerts</Typography>
          <Typography variant="body2">The system is healthy.</Typography>
        </Box>
      ) : (
        <TableContainer
          component={Paper}
          elevation={0}
          sx={{
            borderRadius: 2,
            border: `1px solid ${isDarkMode ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)'}`,
            background: isDarkMode ? 'rgba(255,255,255,0.03)' : '#fff',
          }}
        >
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell sx={tableHeaderSx}>Severity</TableCell>
                <TableCell sx={tableHeaderSx}>Rule</TableCell>
                <TableCell sx={tableHeaderSx}>Resource</TableCell>
                <TableCell sx={tableHeaderSx}>Value / Threshold</TableCell>
                <TableCell sx={tableHeaderSx}>Fired</TableCell>
                <TableCell sx={tableHeaderSx}>Status</TableCell>
                <TableCell sx={tableHeaderSx}>RCA</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {alerts.map((alert) => (
                <TableRow
                  key={alert.id}
                  sx={rowSx}
                  onClick={() => setSelectedAlert(alert)}
                >
                  <TableCell>
                    <Chip
                      label={alert.severity}
                      color={SEVERITY_COLOR[alert.severity]}
                      size="small"
                      sx={{ fontWeight: 600, minWidth: 68 }}
                    />
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" fontWeight={500}>{alert.ruleName}</Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">{alert.resourceID}</Typography>
                    {alert.namespace && (
                      <Typography variant="caption" color="text.secondary">{alert.namespace}</Typography>
                    )}
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">
                      {alert.value.toFixed(2)} / {alert.threshold.toFixed(2)}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Tooltip title={alert.firedAt}>
                      <Typography variant="body2" color="text.secondary">
                        {formatDistanceToNow(new Date(alert.firedAt), { addSuffix: true })}
                      </Typography>
                    </Tooltip>
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={alert.status}
                      color={alert.status === 'firing' ? 'error' : 'default'}
                      size="small"
                      variant={alert.status === 'resolved' ? 'outlined' : 'filled'}
                    />
                  </TableCell>
                  <TableCell>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                      {rcaBadge(alert.rcaStatus)}
                      {alert.rcaStatus === 'done' && (
                        <Tooltip title="View AI analysis">
                          <SmartToy fontSize="small" color="success" />
                        </Tooltip>
                      )}
                    </Box>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <RCADrawer
        alert={selectedAlert}
        open={selectedAlert !== null}
        onClose={() => setSelectedAlert(null)}
      />
    </ModernLayout>
  );
};
