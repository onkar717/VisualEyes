import React from 'react';
import { Container, Typography, Box, Paper, Grid as MuiGrid } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { GaugeChart } from './GaugeChart';
import { api } from '../services/api';
import type { Metric, MetricSeries } from '../types/metrics';

const Grid = MuiGrid as any;

const processMetrics = (metrics: Metric[]): Record<string, MetricSeries> => {
  const result: Record<string, MetricSeries> = {};

  metrics.forEach((metric) => {
    if (!result[metric.name]) {
      result[metric.name] = {
        name: metric.name,
        data: [],
      };
    }
    result[metric.name].data.push({
      timestamp: metric.timestamp,
      value: metric.value,
    });
  });

  return result;
};

export const Dashboard: React.FC = () => {
  const { data: metrics, isLoading, error } = useQuery({
    queryKey: ['metrics'],
    queryFn: api.getMetricsSnapshot,
    refetchInterval: 10000,
  });

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="calc(100vh - 64px)">
        <Typography variant="h5" color="white">Loading metrics...</Typography>
      </Box>
    );
  }

  if (error) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="calc(100vh - 64px)">
        <Typography color="error" variant="h5">Error loading metrics: {(error as Error).message}</Typography>
      </Box>
    );
  }

  if (!metrics) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="calc(100vh - 64px)">
        <Typography variant="h5" color="white">No metrics available</Typography>
      </Box>
    );
  }

  const processedMetrics = processMetrics(metrics);
  const latestCpuUsage = processedMetrics['cpu.usage']?.data.slice(-1)[0]?.value || 0;
  const latestMemoryUsage = processedMetrics['memory.usage_percent']?.data.slice(-1)[0]?.value || 0;
  const latestDiskUsed = processedMetrics['disk.used']?.data.slice(-1)[0]?.value || 0;
  const latestDiskTotal = latestDiskUsed + (processedMetrics['disk.free']?.data.slice(-1)[0]?.value || 0);
  const diskUsagePercent = (latestDiskUsed / latestDiskTotal) * 100;
  const latestNetworkRecv = processedMetrics['network.bytes_recv_per_sec']?.data.slice(-1)[0]?.value || 0;
  const latestNetworkSent = processedMetrics['network.bytes_sent_per_sec']?.data.slice(-1)[0]?.value || 0;
  const latestLoad1 = processedMetrics['load.1min']?.data.slice(-1)[0]?.value || 0;
  const latestLoad5 = processedMetrics['load.5min']?.data.slice(-1)[0]?.value || 0;

  return (
    <Box sx={{ backgroundColor: '#1e1e2d', minHeight: 'calc(100vh - 64px)', py: 4 }}>
      <Container maxWidth="xl">
        <Typography variant="h5" gutterBottom sx={{ color: '#ffffff', mb: 4 }}>
          System Overview
        </Typography>
        
        <Grid container spacing={3}>
          {/* Resource Usage */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="CPU Usage"
                value={latestCpuUsage}
                maxValue={100}
                unit="%"
                color="#4caf50"
              />
            </Paper>
          </Grid>
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Memory Usage"
                value={latestMemoryUsage}
                maxValue={100}
                unit="%"
                color="#2196f3"
              />
            </Paper>
          </Grid>
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Disk Usage"
                value={diskUsagePercent}
                maxValue={100}
                unit="%"
                color="#ff9800"
              />
            </Paper>
          </Grid>

          {/* Network Traffic */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Network Received"
                value={latestNetworkRecv / 1024} // Convert to KB
                maxValue={Math.max(latestNetworkRecv / 1024, 100)} // Dynamic max
                unit="KB/s"
                color="#9c27b0"
              />
            </Paper>
          </Grid>
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Network Sent"
                value={latestNetworkSent / 1024} // Convert to KB
                maxValue={Math.max(latestNetworkSent / 1024, 100)} // Dynamic max
                unit="KB/s"
                color="#673ab7"
              />
            </Paper>
          </Grid>

          {/* System Load */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="System Load"
                value={latestLoad1}
                maxValue={Math.max(latestLoad5, latestLoad1, 1)} // Dynamic max
                unit=""
                color="#f44336"
              />
            </Paper>
          </Grid>
        </Grid>
      </Container>
    </Box>
  );
}; 