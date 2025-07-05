import React from 'react';
import { Container, Typography, Box, Paper, Grid as MuiGrid } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { GaugeChart } from './GaugeChart';
import { api } from '../services/api';

const Grid = MuiGrid as any;

export const KubernetesDashboard: React.FC = () => {
  const { data: metrics, isLoading, error } = useQuery({
    queryKey: ['kubernetes-metrics'],
    queryFn: () => api.getKubernetesMetrics(),
    refetchInterval: 10000,
  });

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="calc(100vh - 64px)">
        <Typography variant="h5" color="white">Loading cluster metrics...</Typography>
      </Box>
    );
  }

  if (error) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="calc(100vh - 64px)">
        <Typography color="error" variant="h5">Error loading cluster metrics: {(error as Error).message}</Typography>
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

  return (
    <Box sx={{ backgroundColor: '#1e1e2d', minHeight: 'calc(100vh - 64px)', py: 4 }}>
      <Container maxWidth="xl">
        <Typography variant="h5" gutterBottom sx={{ color: '#ffffff', mb: 4 }}>
          Cluster Overview
        </Typography>
        
        <Grid container spacing={3}>
          {/* Node Status */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Nodes"
                value={metrics.nodes.ready}
                maxValue={metrics.nodes.total}
                unit="/1"
                color="#6b4cf5"
              />
            </Paper>
          </Grid>

          {/* Node CPU Usage */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Node CPU Use"
                value={(metrics.resources.cpu.usage / metrics.resources.cpu.total) * 100}
                maxValue={100}
                unit="%"
                color="#4caf50"
              />
            </Paper>
          </Grid>

          {/* Node Memory Usage */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Node RAM Use"
                value={(metrics.resources.memory.usage / metrics.resources.memory.total) * 100}
                maxValue={100}
                unit="%"
                color="#2196f3"
              />
            </Paper>
          </Grid>

          {/* Pod Status */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Pods"
                value={metrics.pods.running}
                maxValue={metrics.pods.total}
                unit="/25"
                color="#ff4081"
              />
            </Paper>
          </Grid>

          {/* Pod CPU Usage */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Pod CPU Use"
                value={(metrics.podResources.cpu.usage / metrics.podResources.cpu.total) * 100}
                maxValue={100}
                unit="%"
                color="#ff9800"
              />
            </Paper>
          </Grid>

          {/* Pod Memory Usage */}
          <Grid item xs={12} md={4}>
            <Paper sx={{ p: 2, backgroundColor: '#2a2a3e' }}>
              <GaugeChart
                title="Pod RAM Use"
                value={(metrics.podResources.memory.usage / metrics.podResources.memory.total) * 100}
                maxValue={100}
                unit="%"
                color="#e91e63"
              />
            </Paper>
          </Grid>
        </Grid>
      </Container>
    </Box>
  );
}; 