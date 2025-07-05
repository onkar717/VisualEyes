# VisualEyes

A modern monitoring solution for system and Kubernetes metrics with real-time visualization.

## Features

- **System Metrics Monitoring**
  - CPU Usage
  - Memory Usage
  - Disk Usage
  - Network Traffic
  - System Load

- **Kubernetes Metrics Monitoring**
  - Cluster CPU Usage
  - Cluster Memory Usage
  - Pod Count
  - Node Status
  - Network Traffic
  - Disk Usage

- **Real-time Visualization**
  - Grafana Dashboards
  - In-memory Metrics Storage
  - JSON API Endpoints

## Architecture

- **Local System Agent**: Collects host metrics (CPU, memory, disk)
- **Kubernetes Agent**: Deployed as DaemonSet for cluster metrics
- **Local Server**: Stores metrics in-memory and exposes API endpoints
- **Grafana**: Visualization with custom dashboards

## Quick Start

1. Clone the repository:
   ```bash
   git clone https://github.com/onkar717/VisualEyes.git
   cd VisualEyes
   ```

2. Start the monitoring stack:
   ```bash
   docker-compose up -d
   ```

3. Access Grafana:
   - URL: http://localhost:3000
   - Username: admin
   - Password: visual_eyes

4. Deploy Kubernetes agent (if using Kubernetes):
   ```bash
   kubectl apply -f deploy/kubernetes/
   ```

## API Endpoints

- System Metrics: `http://localhost:8080/api/metrics/snapshot`
- Kubernetes Metrics: `http://localhost:8080/api/kubernetes-metrics`

## Development

Built with:
- Go for metrics collection
- React + Vite for UI
- Grafana for visualization
- Docker for containerization

## License

MIT License # VisualEyes
