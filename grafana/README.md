# Grafana Configuration for VisualEyes

This directory contains Grafana configuration for visualizing VisualEyes metrics.

## Overview

Grafana is configured to read metrics from the VisualEyes server via HTTP API using the Infinity plugin. The server stores metrics in-memory and exposes them through REST endpoints.

## Configuration Files

- `docker-compose.yml` - Grafana container configuration
- `provisioning/` - Auto-provisioned data sources and dashboards
  - `datasources/` - HTTP API data source configuration
  - `dashboards/` - Pre-configured dashboards

## Data Source

The Infinity plugin is used to connect to the VisualEyes HTTP API:

- **Type**: Infinity (HTTP API)
- **URL**: `http://host.docker.internal:8080`
- **UID**: `visual-eyes-api`

## Dashboards

Two dashboards are pre-configured:

1. **System Metrics Dashboard** (`visual-eyes-dashboard.json`)
   - CPU, Memory, Disk, Network metrics
   - System load and performance indicators

2. **Kubernetes Metrics Dashboard** (`visual-eyes-kubernetes.json`)
   - Pod CPU/Memory usage
   - Node metrics
   - Network traffic
   - Pod counts

## Setup

1. Start Grafana:
   ```bash
   cd grafana
   docker-compose up -d
   ```

2. Access Grafana:
   - URL: http://localhost:3000
   - Default credentials: admin/admin

3. Verify data source connection:
   - Go to Configuration > Data Sources
   - Check that "VisualEyes API" is connected

4. View dashboards:
   - System Metrics: http://localhost:3000/d/visual-eyes-system-metrics
   - Kubernetes Metrics: http://localhost:3000/d/visual-eyes-kubernetes-metrics

## Troubleshooting

1. Check Grafana logs: `docker-compose logs grafana`
2. Verify server is running: `curl http://localhost:8080/health`
3. Test data source connection in Grafana UI
4. Check network connectivity between Grafana and server

## Architecture

```
VisualEyes Agents → HTTP POST → Local Server (in-memory) → HTTP API → Grafana (Infinity Plugin)
```

- Agents send metrics via HTTP POST to server
- Server stores metrics in-memory
- Grafana reads metrics via HTTP API using Infinity plugin
- No database required - fully in-memory architecture 