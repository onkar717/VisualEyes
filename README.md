# VisualEyes - Modern System & Kubernetes Monitoring Dashboard

VisualEyes is a modern, lightweight monitoring solution that provides real-time insights into both system-level metrics and Kubernetes cluster health. With its sleek UI featuring dark/light mode support and glass-morphism effects, VisualEyes offers an intuitive monitoring experience.

## Features

- Real-time system metrics monitoring
- Kubernetes cluster metrics visualization
- Modern UI with theme support (dark/light mode)
- Efficient metrics collection via agents
- Kubernetes-native deployment support
- Flexible configuration management

## Prerequisites

- Go 1.19 or later
- Node.js 16 or later
- Docker
- kubectl
- Either kind or k3d for local Kubernetes cluster

## Quick Start

### 1. Setting up Local Development Environment

```bash
# Clone the repository
git clone https://github.com/yourusername/VisualEyes.git
cd VisualEyes

# Install Go dependencies
make deps

# Build both agent and server
make build

# Start the backend server
make run-server
```

### 2. Starting the UI Development Server

```bash
# Navigate to UI directory
cd ui

# Install dependencies
npm install

# Start development server
npm run dev
```

The UI will be available at `http://localhost:5173`

### 3. Running Local System Metrics Collection

```bash
# Run agent with host metrics only
make run-agent-host
```

### 4. Setting up a Local Kubernetes Cluster

#### Using kind

```bash
# Create a kind cluster
kind create cluster --name visual-eyes

# Set context
kubectl cluster-info --context kind-visual-eyes
```

#### Using k3d

```bash
# Create a k3d cluster
k3d cluster create visual-eyes

# Set context
kubectl cluster-info --context k3d-visual-eyes
```

### 5. Deploying VisualEyes in Kubernetes

```bash
# Apply the Kubernetes configurations
kubectl apply -f deploy/kubernetes/service.yaml
kubectl apply -f deploy/kubernetes/config.yaml
kubectl apply -f deploy/kubernetes/agent.yaml

# Verify deployment
kubectl get pods -n kube-system -l app=visual-eyes-k8s-agent
kubectl get services -n kube-system -l app=visual-eyes-backend
```

Required YAML files:
- `deploy/kubernetes/service.yaml`: Backend service configuration
- `deploy/kubernetes/config.yaml`: ConfigMap for agent configuration
- `deploy/kubernetes/agent.yaml`: DaemonSet for deploying agents

## Configuration

The system uses a hierarchical configuration system:

1. Default configurations in `/configs/`
2. Kubernetes-specific overrides in `/deploy/kubernetes/config.yaml`
3. Environment variables for runtime configuration

## Accessing the Dashboard

1. Ensure the backend server is running (`make run-server`)
2. Start the UI development server (`cd ui && npm run dev`)
3. Access the dashboard at `http://localhost:5173`
4. Use the theme toggle in the top navigation bar to switch between dark and light modes

## Development Commands

```bash
# Format code
make fmt

# Run linter
make lint

# Run tests
make test

# Clean build artifacts
make clean
```

## Architecture

VisualEyes consists of three main components:

1. **Backend Server**: Handles metrics storage and API endpoints
2. **Agents**: Collect metrics from either host system or Kubernetes cluster
3. **Frontend UI**: Modern React-based dashboard for visualization

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

[Add your license information here]
