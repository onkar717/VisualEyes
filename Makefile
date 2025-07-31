.PHONY: build build-system-agent build-kube-agent build-server run-system-agent run-kube-agent run-server run-ui install-ui build-ui build-docker build-docker-tag push-docker build-and-push build-docker-server run-docker-server push-docker-server build-docker-ui run-docker-ui push-docker-ui build-docker-system-agent run-docker-system-agent push-docker-system-agent docker-up docker-down docker-logs deploy-k8s undeploy-k8s status-k8s run-all test clean fmt lint deps help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOFMT=$(GOCMD) fmt
GOLINT=golangci-lint

# Binary name
BINARY_NAME_AGENT=visual-eyes-agent
BINARY_NAME_SERVER=visual-eyes-server

# Main paths
SYSTEM_AGENT_PATH=./agents/system
KUBE_AGENT_PATH=./agents/kubernetes
SERVER_PATH=./backend

# Build all
build: build-system-agent build-kube-agent build-server

# Build system agent
build-system-agent:
	$(GOBUILD) -o bin/$(BINARY_NAME_AGENT) $(SYSTEM_AGENT_PATH)

# Build kubernetes agent
build-kube-agent:
	$(GOBUILD) -o bin/visual-eyes-kube-agent $(KUBE_AGENT_PATH)

# Build server
build-server:
	$(GOBUILD) -o bin/$(BINARY_NAME_SERVER) $(SERVER_PATH)

# Run system agent locally
run-system-agent:
	./bin/$(BINARY_NAME_AGENT)

# Run system agent with host metrics only
run-system-agent-host:
	VISUAL_EYES_AGENT_DISABLE_KUBE_METRICS=true \
	VISUAL_EYES_AGENT_DISABLE_HOST_METRICS=false \
	./bin/$(BINARY_NAME_AGENT)

# Run kubernetes agent
run-kube-agent:
	./bin/visual-eyes-kube-agent

# Run server
run-server:
	./bin/$(BINARY_NAME_SERVER)

# Run UI (requires Node.js)
run-ui:
	cd ui && npm run dev

# Install UI dependencies
install-ui:
	cd ui && npm install

# Build UI for production
build-ui:
	cd ui && npm run build

# Build Docker image for Kubernetes agent
build-docker:
	docker build -t visual-eyes-kube-agent:latest .

# Build and tag Docker image with version
build-docker-tag:
	docker build -t visual-eyes-kube-agent:latest -t visual-eyes-kube-agent:$(shell date +%Y%m%d-%H%M%S) .

# Push Docker image to Docker Hub
push-docker:
	docker tag visual-eyes-kube-agent:latest onkar4545/visual-eyes-kube-agent:latest
	docker push onkar4545/visual-eyes-kube-agent:latest

# Build and push Docker image in one command
build-and-push:
	make build-docker
	make push-docker

# Build Docker image for backend server
build-docker-server:
	docker build -f backend/Dockerfile -t visual-eyes-server:latest .

# Run backend server in Docker
run-docker-server:
	docker run --rm -p 8080:8080 --name visual-eyes-server visual-eyes-server:latest

# Push backend server to Docker Hub
push-docker-server:
	docker tag visual-eyes-server:latest onkar4545/visual-eyes-server:latest
	docker push onkar4545/visual-eyes-server:latest

# Build Docker image for UI
build-docker-ui:
	docker build -f ui/Dockerfile -t visual-eyes-ui:latest ./ui

# Run UI in Docker
run-docker-ui:
	docker run --rm -p 3000:3000 --name visual-eyes-ui visual-eyes-ui:latest

# Push UI to Docker Hub
push-docker-ui:
	docker tag visual-eyes-ui:latest onkar4545/visual-eyes-ui:latest
	docker push onkar4545/visual-eyes-ui:latest

# Build Docker image for system agent
build-docker-system-agent:
	docker build -f agents/system/Dockerfile -t visual-eyes-system-agent:latest .

# Run system agent in Docker
run-docker-system-agent:
	docker run --rm --name visual-eyes-system-agent visual-eyes-system-agent:latest

# Push system agent to Docker Hub
push-docker-system-agent:
	docker tag visual-eyes-system-agent:latest onkar4545/visual-eyes-system-agent:latest
	docker push onkar4545/visual-eyes-system-agent:latest

# Docker Compose commands
docker-up:
	docker-compose up --build -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Build all Docker images
build-all-docker: build-docker-server build-docker-ui build-docker-system-agent build-docker

# Push all Docker images
push-all-docker: push-docker-server push-docker-ui push-docker-system-agent push-docker

# Deploy to Kubernetes
deploy-k8s:
	kubectl apply -f deployments/kubernetes/

# Remove from Kubernetes
undeploy-k8s:
	kubectl delete -f deployments/kubernetes/

# Check Kubernetes deployment status
status-k8s:
	kubectl get pods -n kube-system -l app=visual-eyes-k8s-agent

# Run all components (server + system agent + UI)
run-all: run-server run-system-agent run-ui

# Run tests
test:
	$(GOTEST) -v ./...

# Clean binaries
clean:
	$(GOCLEAN)
	rm -f bin/$(BINARY_NAME_AGENT)
	rm -f bin/$(BINARY_NAME_SERVER)

# Create necessary directories
init:
	mkdir -p bin
	mkdir -p configs

fmt:
	$(GOFMT) ./...

lint:
	$(GOLINT) run

deps:
	$(GOCMD) mod download
	$(GOCMD) mod tidy

help:
	@echo "Make targets:"
	@echo "  build                - Build all components (system agent, kube agent, server)"
	@echo "  build-system-agent   - Build system agent binary"
	@echo "  build-kube-agent     - Build Kubernetes agent binary"
	@echo "  build-server         - Build backend server binary"
	@echo "  run-system-agent     - Run system agent locally"
	@echo "  run-system-agent-host - Run system agent with host metrics only"
	@echo "  run-kube-agent       - Run Kubernetes agent locally"
	@echo "  run-server           - Run backend server locally"
	@echo "  run-ui               - Run UI (requires Node.js)"
	@echo "  run-all              - Run server + system agent + UI together"
	@echo "  install-ui           - Install UI dependencies"
	@echo "  build-ui             - Build UI for production"
	@echo "  build-docker         - Build Docker image for Kubernetes agent"
	@echo "  build-docker-tag     - Build Docker image with timestamp tag"
	@echo "  push-docker          - Push Docker image to Docker Hub"
	@echo "  build-and-push       - Build and push Docker image in one command"
	@echo "  deploy-k8s           - Deploy to Kubernetes cluster"
	@echo "  undeploy-k8s         - Remove from Kubernetes cluster"
	@echo "  status-k8s           - Check Kubernetes deployment status"
	@echo "  test                 - Run tests"
	@echo "  clean                - Clean build artifacts"
	@echo "  fmt                  - Format code"
	@echo "  lint                 - Run linter"
	@echo "  deps                 - Download and tidy dependencies"
	@echo ""
	@echo "Quick Start:"
	@echo "  1. make build        - Build all components"
	@echo "  2. make install-ui   - Install UI dependencies"
	@echo "  3. make run-all      - Start everything together"
	@echo "      This Makefile is for local development only"