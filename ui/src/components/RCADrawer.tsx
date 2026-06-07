import React, { useState } from 'react';
import {
  Drawer,
  Box,
  Typography,
  Chip,
  Divider,
  IconButton,
  Button,
  CircularProgress,
  Tooltip,
  Paper,
  Stack,
} from '@mui/material';
import {
  Close,
  SmartToy,
  Terminal,
  CheckCircle,
  Error as ErrorIcon,
  HourglassEmpty,
  PlayArrow,
  SkipNext,
} from '@mui/icons-material';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { formatDistanceToNow } from 'date-fns';
import { api } from '../services/api';
import { useTheme } from '../theme/ThemeContext';
import type { Alert, FixCommand, RemediationStatus } from '../types/metrics';

interface RCADrawerProps {
  alert: Alert | null;
  open: boolean;
  onClose: () => void;
}

const CMD_STATUS_ICON: Record<RemediationStatus, React.ReactElement> = {
  pending:  <HourglassEmpty fontSize="small" color="disabled" />,
  executed: <CheckCircle fontSize="small" color="success" />,
  failed:   <ErrorIcon fontSize="small" color="error" />,
  skipped:  <SkipNext fontSize="small" color="disabled" />,
};

function CommandRow({
  cmd,
  index,
  alertId,
  rcaDone,
}: {
  cmd: FixCommand;
  index: number;
  alertId: number;
  rcaDone: boolean;
}) {
  const { isDarkMode } = useTheme();
  const queryClient = useQueryClient();

  const execute = useMutation({
    mutationFn: () => api.executeRCACommand(alertId, index),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rca', alertId] });
    },
  });

  const canExecute = rcaDone && cmd.status === 'pending' && !cmd.isAutoSafe;

  return (
    <Paper
      elevation={0}
      sx={{
        p: 1.5,
        mb: 1,
        border: `1px solid ${isDarkMode ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)'}`,
        borderRadius: 1.5,
        background: isDarkMode ? 'rgba(255,255,255,0.03)' : '#fafafa',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
        <Box sx={{ pt: 0.25 }}>{CMD_STATUS_ICON[cmd.status]}</Box>
        <Box sx={{ flexGrow: 1, minWidth: 0 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5 }}>
            <Terminal fontSize="small" color="action" />
            <Typography
              variant="body2"
              fontFamily="monospace"
              sx={{ wordBreak: 'break-all', flexGrow: 1 }}
            >
              {cmd.command}
            </Typography>
            {cmd.isAutoSafe && (
              <Chip label="auto-safe" color="success" size="small" variant="outlined" />
            )}
          </Box>

          {cmd.output && (
            <Box
              sx={{
                mt: 0.5,
                p: 1,
                borderRadius: 1,
                background: isDarkMode ? 'rgba(0,0,0,0.4)' : '#f0f0f0',
                fontFamily: 'monospace',
                fontSize: '0.72rem',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                color: isDarkMode ? '#a5d6a7' : '#2e7d32',
                maxHeight: 120,
                overflowY: 'auto',
              }}
            >
              {cmd.output}
            </Box>
          )}
          {cmd.error && (
            <Box
              sx={{
                mt: 0.5,
                p: 1,
                borderRadius: 1,
                background: isDarkMode ? 'rgba(244,67,54,0.1)' : '#fff3f3',
                fontFamily: 'monospace',
                fontSize: '0.72rem',
                whiteSpace: 'pre-wrap',
                color: isDarkMode ? '#ef9a9a' : '#c62828',
                maxHeight: 100,
                overflowY: 'auto',
              }}
            >
              {cmd.error}
            </Box>
          )}
        </Box>

        {canExecute && (
          <Tooltip title="Execute this command">
            <span>
              <IconButton
                size="small"
                color="primary"
                onClick={() => execute.mutate()}
                disabled={execute.isPending}
              >
                {execute.isPending ? <CircularProgress size={16} /> : <PlayArrow fontSize="small" />}
              </IconButton>
            </span>
          </Tooltip>
        )}
      </Box>
    </Paper>
  );
}

export const RCADrawer: React.FC<RCADrawerProps> = ({ alert, open, onClose }) => {
  const { isDarkMode } = useTheme();
  const [showRaw, setShowRaw] = useState(false);

  const rcaQuery = useQuery({
    queryKey: ['rca', alert?.id],
    queryFn: () => api.getRCA(alert!.id),
    enabled: open && alert !== null && (alert.rcaStatus === 'done' || alert.rcaStatus === 'running' || alert.rcaStatus === 'failed'),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === 'pending' || status === undefined ? 3000 : false;
    },
  });

  const rca = rcaQuery.data;
  const commands: FixCommand[] = (() => {
    if (!rca?.commands) return [];
    try { return JSON.parse(rca.commands); } catch { return []; }
  })();

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { xs: '100%', sm: 560 },
          background: isDarkMode
            ? 'linear-gradient(180deg, #1a1a2e 0%, #16162a 100%)'
            : 'linear-gradient(180deg, #f8f9fc 0%, #fff 100%)',
          borderLeft: `1px solid ${isDarkMode ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)'}`,
        },
      }}
    >
      {/* Header */}
      <Box sx={{ p: 2.5, pb: 1.5, display: 'flex', alignItems: 'center', gap: 1 }}>
        <SmartToy color="primary" />
        <Typography variant="h6" fontWeight={600} sx={{ flexGrow: 1 }}>
          AI Root Cause Analysis
        </Typography>
        <IconButton size="small" onClick={onClose}>
          <Close fontSize="small" />
        </IconButton>
      </Box>
      <Divider />

      <Box sx={{ overflowY: 'auto', p: 2.5, height: '100%' }}>
        {/* Alert summary */}
        {alert && (
          <Paper
            elevation={0}
            sx={{
              p: 2,
              mb: 3,
              borderRadius: 2,
              border: `1px solid ${isDarkMode ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)'}`,
              background: isDarkMode ? 'rgba(255,255,255,0.03)' : '#fff',
            }}
          >
            <Stack direction="row" spacing={1} alignItems="center" mb={1}>
              <Chip
                label={alert.severity}
                color={alert.severity === 'critical' ? 'error' : alert.severity === 'warning' ? 'warning' : 'info'}
                size="small"
                sx={{ fontWeight: 600 }}
              />
              <Typography variant="subtitle2" fontWeight={600}>{alert.ruleName}</Typography>
            </Stack>
            <Typography variant="body2" color="text.secondary">{alert.message}</Typography>
            <Typography variant="caption" color="text.secondary" display="block" mt={0.5}>
              Fired {formatDistanceToNow(new Date(alert.firedAt), { addSuffix: true })}
              {alert.namespace && ` · ${alert.namespace}`}
              {alert.resourceID && ` · ${alert.resourceID}`}
            </Typography>
          </Paper>
        )}

        {/* RCA not triggered */}
        {alert && !alert.rcaStatus && (
          <Box sx={{ textAlign: 'center', py: 6, color: 'text.secondary' }}>
            <SmartToy sx={{ fontSize: 48, opacity: 0.3, mb: 1 }} />
            <Typography>RCA has not been triggered for this alert.</Typography>
            <Typography variant="caption">
              Enable the RCA engine and set ANTHROPIC_API_KEY to enable automatic analysis.
            </Typography>
          </Box>
        )}

        {/* RCA running / loading */}
        {(alert?.rcaStatus === 'running' || alert?.rcaStatus === 'pending' || rcaQuery.isLoading) && (
          <Box sx={{ textAlign: 'center', py: 6 }}>
            <CircularProgress size={40} sx={{ mb: 2 }} />
            <Typography color="text.secondary">Claude is analysing the alert…</Typography>
          </Box>
        )}

        {/* RCA failed */}
        {rca?.status === 'failed' && (
          <Box sx={{ textAlign: 'center', py: 4, color: 'error.main' }}>
            <ErrorIcon sx={{ fontSize: 40, mb: 1 }} />
            <Typography>{rca.explanation || 'Analysis failed.'}</Typography>
          </Box>
        )}

        {/* RCA done */}
        {rca?.status === 'done' && (
          <>
            {/* Root cause */}
            <Typography variant="overline" color="text.secondary" gutterBottom>
              Root Cause
            </Typography>
            <Paper
              elevation={0}
              sx={{
                p: 2, mb: 3, borderRadius: 2,
                border: `1px solid ${isDarkMode ? 'rgba(255,165,0,0.2)' : 'rgba(255,152,0,0.2)'}`,
                background: isDarkMode ? 'rgba(255,152,0,0.06)' : 'rgba(255,243,224,0.8)',
              }}
            >
              <Typography variant="body2" lineHeight={1.7}>{rca.rootCause || ' '}</Typography>
            </Paper>

            {/* Explanation */}
            <Typography variant="overline" color="text.secondary" gutterBottom>
              Analysis
            </Typography>
            <Typography variant="body2" lineHeight={1.8} mb={3}>
              {rca.explanation}
            </Typography>

            {/* Commands */}
            {commands.length > 0 && (
              <>
                <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                  <Typography variant="overline" color="text.secondary" sx={{ flexGrow: 1 }}>
                    Remediation Commands
                  </Typography>
                  <Button
                    size="small"
                    variant="text"
                    onClick={() => setShowRaw(!showRaw)}
                    sx={{ fontSize: '0.68rem' }}
                  >
                    {showRaw ? 'card view' : 'raw JSON'}
                  </Button>
                </Box>

                {showRaw ? (
                  <Box
                    component="pre"
                    sx={{
                      p: 1.5, borderRadius: 1.5, fontSize: '0.72rem', overflowX: 'auto',
                      background: isDarkMode ? 'rgba(0,0,0,0.4)' : '#f5f5f5',
                      color: isDarkMode ? '#e0e0e0' : '#333',
                    }}
                  >
                    {JSON.stringify(commands, null, 2)}
                  </Box>
                ) : (
                  commands.map((cmd, i) => (
                    <CommandRow
                      key={i}
                      cmd={cmd}
                      index={i}
                      alertId={alert!.id}
                      rcaDone={rca.status === 'done'}
                    />
                  ))
                )}
              </>
            )}

            <Divider sx={{ my: 2 }} />
            <Typography variant="caption" color="text.secondary">
              Model: {rca.model} · {rca.inputTokens.toLocaleString()} input tokens ·{' '}
              {formatDistanceToNow(new Date(rca.updatedAt), { addSuffix: true })}
            </Typography>
          </>
        )}
      </Box>
    </Drawer>
  );
};
