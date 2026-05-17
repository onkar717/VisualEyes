import React, { useEffect, useState } from 'react';
import { CssBaseline, Box } from '@mui/material';
import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import { Dashboard } from './components/Dashboard';
import { KubernetesDashboard } from './components/KubernetesDashboard';
import { AlertsPanel } from './components/AlertsPanel';
import { LogViewer } from './components/LogViewer';
import { Navigation, type AppView } from './components/Navigation';
import { ThemeProvider } from './theme/ThemeContext';
import { useWebSocket } from './hooks/useWebSocket';
import type { WSMetricsSnapshot } from './types/metrics';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

// Derives the WebSocket URL from the current API base so it works in every env.
function wsUrl(): string {
  const host =
    window.location.hostname === 'localhost' ? 'localhost:8080' : window.location.host;
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${proto}://${host}/ws`;
}

function AppInner() {
  const [view, setView] = useState<AppView>('system');
  const qc = useQueryClient();

  // Subscribe to real-time metric snapshots from the backend broadcaster.
  const { lastMessage } = useWebSocket<WSMetricsSnapshot>(wsUrl(), {
    reconnectDelay: 4000,
  });

  // When a WS snapshot arrives, invalidate the metrics cache so the Dashboard
  // re-renders with fresh data without a full polling cycle.
  useEffect(() => {
    if (lastMessage?.type === 'metrics_snapshot') {
      qc.invalidateQueries({ queryKey: ['metrics'] });
    }
  }, [lastMessage, qc]);

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', minHeight: '100vh' }}>
      <Navigation view={view} onViewChange={setView} />
      {view === 'system'     && <Dashboard />}
      {view === 'kubernetes' && <KubernetesDashboard />}
      {view === 'alerts'     && <AlertsPanel />}
      {view === 'logs'       && <LogViewer />}
    </Box>
  );
}

export const App: React.FC = () => (
  <QueryClientProvider client={queryClient}>
    <ThemeProvider>
      <CssBaseline />
      <AppInner />
    </ThemeProvider>
  </QueryClientProvider>
);
